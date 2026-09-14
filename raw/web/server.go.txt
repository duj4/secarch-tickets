package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	"secarch-tickets/internal/api"
	"secarch-tickets/internal/appconfig"
	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/db"
	"secarch-tickets/internal/httpclient"
	"secarch-tickets/internal/logger"
	"secarch-tickets/internal/middleware"
	"secarch-tickets/internal/oidcauth"
	"secarch-tickets/internal/secarch"

	"github.com/gin-gonic/gin"
)

//go:embed templates/*.html static
var webFiles embed.FS

// Run initializes and starts the SecArch Tickets web service.
//
// It loads configuration, prepares shared clients, registers embedded
// templates and static assets, and starts the HTTPS server.
func Run(configPath string) error {
	cfg, err := appconfig.Load(configPath)
	if err != nil {
		return err
	}
	env := cfg.Server.Environment
	tlsPaths := cfg.TLS
	if err := validateTLSPaths(tlsPaths); err != nil {
		return err
	}
	logger.Info("loading configuration", "env", env, "config_file", configPath)

	ctx := context.Background()
	proxy, err := cfg.Proxy.Resolver()
	if err != nil {
		return err
	}
	var oidcClient *http.Client
	if cfg.OIDC.Enabled {
		oidcClient = httpclient.NewClient(time.Duration(cfg.OIDC.HTTPTimeoutSeconds)*time.Second, proxy)
		defer oidcClient.CloseIdleConnections()
	}
	authManager, err := oidcauth.New(ctx, cfg.OIDC, oidcClient)
	if err != nil {
		return fmt.Errorf("failed to initialize OIDC: %w", err)
	}
	logger.Info("OIDC authentication configured", "enabled", authManager.Enabled())

	pool, err := db.NewPool(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("failed to create db pool: %w", err)
	}
	defer pool.Close()
	if err := db.EnsureSchema(ctx, pool); err != nil {
		return fmt.Errorf("failed to initialize/check DB schema: %w", err)
	}

	var cmdbProxy httpclient.ProxyFunc
	if cfg.Proxy.CMDBEnabled {
		cmdbProxy = proxy
	}
	cmdbConfig := cfg.CMDB
	cmdbClient, err := cmdb.NewClient(cmdbConfig, cmdbProxy)
	if err != nil {
		return fmt.Errorf("failed to create CMDB client: %w", err)
	}

	service := secarch.NewTicketService(pool, cmdbClient, secarch.RefreshPolicy{
		Timeout:                  cmdbConfig.HTTPTimeout(),
		SuccessCooldown:          cmdbConfig.RefreshSuccessCooldown(),
		FailureBackoff:           cmdbConfig.RefreshFailureBackoff(),
		FailureBackoffMultiplier: cmdbConfig.RefreshFailureBackoffMultiplier,
		CircuitBreakerThreshold:  cmdbConfig.RefreshCircuitBreakerThreshold,
		CircuitOpenDuration:      cmdbConfig.RefreshCircuitOpenDuration(),
		TicketKeyBatchSize:       cmdbConfig.ObjectBatchSize,
	})
	if err := service.Bootstrap(ctx); err != nil {
		return err
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
	registerRoutes(r, service, authManager, cmdbConfig.TicketBrowseURL)

	// Start the HTTPS server.
	return runTLSServer(r, env, cfg.Server.ListenAddr, tlsPaths.ServerCert, tlsPaths.ServerKey)

}

// registerRoutes registers all page and API routes for the web service.
//
// The API handlers share the database pool and CMDB client created during startup.
func registerRoutes(r *gin.Engine, service *secarch.TicketService, authManager *oidcauth.Manager, ticketBrowseURL string) {
	// Health check.
	r.GET("/healthz", api.HealthHandler)
	authManager.RegisterRoutes(r)

	protected := r.Group("/")
	protected.Use(authManager.RequireAuth())

	// SecArch tickets page.
	protected.GET("/", func(c *gin.Context) {
		principal, _ := oidcauth.PrincipalFromContext(c)
		c.HTML(http.StatusOK, "secarch_tickets.html", gin.H{
			"TicketBrowseURL": ticketBrowseURL,
			"OIDCEnabled":     authManager.Enabled(),
			"Username":        principal.Username,
			"IsAdmin":         principal.IsAdmin,
		})
	})

	protected.GET("/api/tickets", api.ListTicketsHandler(service))
	protected.POST("/api/tickets/refresh", api.RefreshTicketsHandler(service))
	protected.PUT("/api/tickets/:ticket_number/expected-date", api.UpdateExpectedDateHandler(service))
	protected.GET("/api/tickets/:ticket_number/updates", api.ListTicketUpdatesHandler(service))
	protected.POST("/api/tickets/:ticket_number/updates", api.CreateTicketUpdateHandler(service))
	protected.GET("/api/statistics/closed", api.ClosedStatisticsHandler(service))
}

// runTLSServer starts the Gin HTTPS server.
func runTLSServer(r *gin.Engine, env, listenAddr, certFilePath, keyFilePath string) error {

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

// validateTLSPaths verifies that every required TLS path is set and accessible.
func validateTLSPaths(paths appconfig.TLSConfig) error {
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
