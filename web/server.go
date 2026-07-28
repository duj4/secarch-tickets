package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"secarch-tickets/internal/api"
	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/db"
	"secarch-tickets/internal/logger"
	"secarch-tickets/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*.html static
var webFiles embed.FS

const (
	defaultListenAddr = ":8443"
	defaultTLSDir     = "/d/d1/ai-info/tls"
	defaultCACertFile = "/etc/pki/ca-trust/source/anchors/katello-server-ca.pem"
	defaultConfigDir  = "/d/d1/ai-info/config"
)

// TLSPaths groups the server and client certificate paths used by the web service.
type TLSPaths struct {
	ServerCert string
	ServerKey  string
	ClientCert string
	ClientKey  string
	CACert     string
}

// Run initializes and starts the SecArch Tickets web service.
//
// It loads configuration, prepares shared clients, registers embedded
// templates and static assets, and starts the HTTPS server.
func Run() error {
	// Resolve the runtime environment.
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if env == "" {
		env = "qa"
	}

	if env != "qa" && env != "prod" {
		return fmt.Errorf("unsupported APP_ENV %q, expected qa or prod", env)
	}

	// Resolve TLS assets for the server and outbound client requests.
	tlsPaths, err := resolveTLSPaths(env)
	if err != nil {
		return err
	}

	if err := validateTLSPaths(tlsPaths); err != nil {
		return err
	}

	// Resolve the environment-specific configuration directory.
	configDir, err := resolveConfigDir(env)
	if err != nil {
		return err
	}

	// Build paths to the configuration files used by this process.
	dbConfigPath := filepath.Join(configDir, "db.json")
	cmdbConfigPath := filepath.Join(configDir, "cmdb.json")

	logger.Info(
		"loading configuration",
		"env", env,
		"config_dir", configDir,
		"db_config", dbConfigPath,
		"cmdb_config", cmdbConfigPath,
	)

	// Load and validate database configuration.
	dbConfig, err := db.LoadConfig(dbConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load db config: %w", err)
	}

	// Apply the shared client mTLS paths to the database configuration.
	dbConfig.SSLRootCert = tlsPaths.CACert
	dbConfig.SSLCert = tlsPaths.ClientCert
	dbConfig.SSLKey = tlsPaths.ClientKey

	// Use one root context for startup-time initialization.
	ctx := context.Background()

	// Create the database pool and keep it alive for the server lifetime.
	pool, err := db.NewPool(ctx, dbConfig)
	if err != nil {
		return fmt.Errorf("failed to create db pool: %w", err)
	}
	defer pool.Close()

	// Ensure the application schema exists before serving traffic.
	if err := db.EnsureSchema(ctx, pool); err != nil {
		return fmt.Errorf("failed to initialize/check DB schema: %w", err)
	}

	// Load and validate CMDB configuration.
	cmdbConfig, err := cmdb.LoadConfig(cmdbConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load cmdb config: %w", err)
	}

	// Apply the shared client mTLS paths to the CMDB configuration.
	cmdbConfig.CACertPath = tlsPaths.CACert
	cmdbConfig.ClientCertPath = tlsPaths.ClientCert
	cmdbConfig.ClientKeyPath = tlsPaths.ClientKey

	// Create the shared CMDB client.
	cmdbClient, err := cmdb.NewClient(cmdbConfig)
	if err != nil {
		return fmt.Errorf("failed to create CMDB client: %w", err)
	}

	if cmdbClient == nil {
		return fmt.Errorf("failed to create CMDB client: client is nil")
	}

	// Set Gin mode before creating the engine.
	if env == "prod" {
		gin.SetMode(gin.ReleaseMode)
		logger.Info("gin running in release mode (Prod)")
	} else {
		logger.Info("gin running in debug mode (QA)")
	}

	// Initialize the Gin engine and middleware stack.
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.GinLogger())

	if env == "prod" {
		if err := r.SetTrustedProxies(nil); err != nil {
			return fmt.Errorf("failed to set trusted proxies: %w", err)
		}
	}

	// Load embedded HTML templates.
	tmpl, err := template.ParseFS(webFiles, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse embedded templates: %w", err)
	}
	r.SetHTMLTemplate(tmpl)

	// Load embedded static assets.
	staticFS, err := fs.Sub(webFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to load embedded static files: %w", err)
	}

	// Expose static assets only under /static.
	r.StaticFS("/static", http.FS(staticFS))

	// Register pages and API routes.
	registerRoutes(r, pool, cmdbClient)

	// Start the HTTPS server.
	return runTLSServer(r, env, tlsPaths.ServerCert, tlsPaths.ServerKey)

}

// registerRoutes registers all page and API routes for the web service.
//
// The API handlers share the database pool and CMDB client created during startup.
func registerRoutes(r *gin.Engine, pool *pgxpool.Pool, cmdbClient *cmdb.Client) {
	// Health check.
	r.GET("/healthz", api.HealthHandler)

	// SecArch tickets.
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/secarch/tickets")
	})

	r.POST("/api/secarch/tickets", api.CreateTicketHandler(pool, cmdbClient))
	r.GET("/api/secarch/tickets", api.ListTicketsHandler(pool, cmdbClient))
	r.PUT("/api/secarch/tickets/:ticket_number", api.UpdateTicketHandler(pool, cmdbClient))
	r.DELETE("/api/secarch/tickets/:ticket_number", api.DeleteTicketHandler(pool, cmdbClient))

	r.GET("/secarch/tickets", func(c *gin.Context) {
		c.HTML(http.StatusOK, "secarch_tickets.html", nil)
	})

}

// runTLSServer starts the Gin HTTPS server.
func runTLSServer(r *gin.Engine, env, certFilePath, keyFilePath string) error {
	listenAddr := strings.TrimSpace(os.Getenv("APP_LISTEN_ADDR"))
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}

	logger.Info(
		"starting service",
		"listen", listenAddr,
		"env", env,
		"cert", certFilePath,
		"key", keyFilePath,
	)

	if err := r.RunTLS(listenAddr, certFilePath, keyFilePath); err != nil {
		return fmt.Errorf("failed to start HTTPS server: %w", err)
	}

	return nil
}

// resolveConfigDir resolves the effective environment-specific config directory.
func resolveConfigDir(env string) (string, error) {
	baseConfigDir := strings.TrimSpace(os.Getenv("APP_CONFIG_DIR"))
	if baseConfigDir == "" {
		baseConfigDir = defaultConfigDir
		logger.Info("APP_CONFIG_DIR not set, using default", "path", baseConfigDir)
	}

	return baseConfigDir, nil
}

// resolveTLSPaths returns the certificate paths used for server TLS and client mTLS.
func resolveTLSPaths(env string) (TLSPaths, error) {
	tlsDir := strings.TrimSpace(os.Getenv("APP_TLS_DIR"))
	if tlsDir == "" {
		tlsDir = defaultTLSDir
	}

	var clientName string
	switch env {
	case "qa":
		clientName = "itsm_jsm_qa"
	case "prod":
		clientName = "itsm_jsm_prod"
	default:
		return TLSPaths{}, fmt.Errorf("unsupported APP_ENV %q, expected qa or prod", env)
	}

	return TLSPaths{
		ServerCert: filepath.Join(tlsDir, "tls.pem"),
		ServerKey:  filepath.Join(tlsDir, "tls.key"),
		ClientCert: filepath.Join(tlsDir, clientName+".pem"),
		ClientKey:  filepath.Join(tlsDir, clientName+".key"),
		CACert:     defaultCACertFile,
	}, nil
}

// validateTLSPaths verifies that every required TLS path is set and accessible.
func validateTLSPaths(paths TLSPaths) error {
	checks := map[string]string{
		"server cert": paths.ServerCert,
		"server key":  paths.ServerKey,
		"client cert": paths.ClientCert,
		"client key":  paths.ClientKey,
		"CA cert":     paths.CACert,
	}

	for name, path := range checks {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s path is empty", name)
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%s %q is not accessible: %w", name, path, err)
		}
	}

	return nil
}
