package appconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"secarch-tickets/internal/cmdb"
	"secarch-tickets/internal/db"
	"secarch-tickets/internal/httpclient"
	"secarch-tickets/internal/oidcauth"
)

const DefaultPath = "/d/d1/secarch-tickets/config/app.json"

type ServerConfig struct {
	Environment string `json:"environment"`
	ListenAddr  string `json:"listen_addr"`
}

type TLSConfig struct {
	ServerCert string `json:"server_cert"`
	ServerKey  string `json:"server_key"`
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
	CACert     string `json:"ca_cert"`
}

type ProxyConfig struct {
	httpclient.ProxyConfig
	CMDBEnabled bool `json:"cmdb_enabled"`
}

// Config is the complete runtime configuration, loaded from one app.json file.
type Config struct {
	Server ServerConfig
	TLS    TLSConfig
	DB     db.Config
	CMDB   cmdb.Config
	OIDC   oidcauth.Config
	Proxy  ProxyConfig
}

// Load reads a single file without consulting application or proxy environment
// variables. Relative certificate paths are resolved against the file directory.
func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open app configuration: %w", err)
	}
	defer file.Close()
	var input struct {
		Server ServerConfig    `json:"server"`
		TLS    TLSConfig       `json:"tls"`
		DB     db.Config       `json:"db"`
		CMDB   cmdb.Config     `json:"cmdb"`
		OIDC   json.RawMessage `json:"oidc"`
		Proxy  ProxyConfig     `json:"proxy"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return Config{}, fmt.Errorf("decode app configuration: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, fmt.Errorf("app configuration must contain exactly one JSON object")
	}
	cfg := Config{Server: input.Server, TLS: input.TLS, DB: input.DB, CMDB: input.CMDB, Proxy: input.Proxy}
	cfg.Server.Environment = strings.ToLower(strings.TrimSpace(cfg.Server.Environment))
	if cfg.Server.Environment == "" {
		cfg.Server.Environment = "qa"
	}
	if cfg.Server.Environment != "qa" && cfg.Server.Environment != "prod" {
		return Config{}, fmt.Errorf("server.environment must be qa or prod")
	}
	cfg.Server.ListenAddr = strings.TrimSpace(cfg.Server.ListenAddr)
	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = ":8443"
	}
	if _, _, err := net.SplitHostPort(cfg.Server.ListenAddr); err != nil {
		return Config{}, fmt.Errorf("server.listen_addr must contain a host and port, for example :8443")
	}

	baseDir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return Config{}, fmt.Errorf("resolve app configuration directory: %w", err)
	}
	clientBase := "/d/d1/secarch-tickets/tls/itsm_jsm_" + cfg.Server.Environment
	for _, item := range []struct {
		path     *string
		fallback string
	}{
		{&cfg.TLS.ServerCert, "/d/d1/secarch-tickets/tls/tls.pem"},
		{&cfg.TLS.ServerKey, "/d/d1/secarch-tickets/tls/tls.key"},
		{&cfg.TLS.ClientCert, clientBase + ".pem"},
		{&cfg.TLS.ClientKey, clientBase + ".key"},
		{&cfg.TLS.CACert, "/etc/pki/ca-trust/source/anchors/katello-server-ca.pem"},
	} {
		*item.path = strings.TrimSpace(*item.path)
		if *item.path == "" {
			*item.path = item.fallback
		} else if !filepath.IsAbs(*item.path) {
			*item.path = filepath.Join(baseDir, *item.path)
		}
	}
	if cfg.DB, err = db.PrepareConfig(cfg.DB); err != nil {
		return Config{}, fmt.Errorf("db configuration: %w", err)
	}
	if cfg.CMDB, err = cmdb.PrepareConfig(cfg.CMDB); err != nil {
		return Config{}, fmt.Errorf("cmdb configuration: %w", err)
	}
	cfg.DB.SSLRootCert, cfg.DB.SSLCert, cfg.DB.SSLKey = cfg.TLS.CACert, cfg.TLS.ClientCert, cfg.TLS.ClientKey
	cfg.CMDB.CACertPath, cfg.CMDB.ClientCertPath, cfg.CMDB.ClientKeyPath = cfg.TLS.CACert, cfg.TLS.ClientCert, cfg.TLS.ClientKey
	if cfg.OIDC, err = oidcauth.ParseConfig(input.OIDC); err != nil {
		return Config{}, err
	}
	if _, err := cfg.Proxy.Resolver(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
