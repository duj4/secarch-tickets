package db

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLFiles contains embedded SQL files used by the database package.
//
//go:embed sql/*.sql
var SQLFiles embed.FS

// LoadConfig reads PostgreSQL settings from a JSON file, applies defaults, and validates the result.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}

	// Apply defaults.
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 10
	}

	if cfg.ConnectTimeoutSeconds <= 0 {
		cfg.ConnectTimeoutSeconds = 5
	}

	if strings.TrimSpace(cfg.TargetSessionAttrs) == "" {
		cfg.TargetSessionAttrs = "read-write"
	}

	for i := range cfg.Servers {
		cfg.Servers[i] = strings.TrimSpace(cfg.Servers[i])
	}

	// Validate the completed configuration.
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validate checks required fields and supported values in Config.
func (cfg *Config) validate() error {
	if len(cfg.Servers) == 0 {
		return fmt.Errorf("db_servers must contain at least one server")
	}

	for i, server := range cfg.Servers {
		if err := requireString(fmt.Sprintf("db_servers[%d]", i), server); err != nil {
			return err
		}

		if strings.Contains(server, ",") {
			return fmt.Errorf("db_servers[%d] must contain exactly one server", i)
		}
	}

	if cfg.Port != 5051 {
		return fmt.Errorf("db_port must be 5051")
	}

	if err := requireString("db_name", cfg.DBName); err != nil {
		return err
	}

	if err := requireString("db_user", cfg.User); err != nil {
		return err
	}

	if err := requireOneOfString("ssl_mode", cfg.SSLMode, "verify-full"); err != nil {
		return err
	}

	if err := requireOneOfString("target_session_attrs", cfg.TargetSessionAttrs, "read-write"); err != nil {
		return err
	}

	return nil
}

// requireString ensures a string field is not empty.
func requireString(name, val string) error {
	if val == "" {
		return fmt.Errorf("%s is required", name)
	}

	return nil
}

// requireOneOfString ensures a value matches the required value.
func requireOneOfString(name, val, target string) error {
	if val == "" {
		return fmt.Errorf("%s is required", name)
	}

	if val != target {
		return fmt.Errorf("%s must be %s", name, target)
	}

	return nil
}

// ConnectTimeout converts timeout seconds to time.Duration.
func (cfg *Config) ConnectTimeout() time.Duration {
	return time.Duration(cfg.ConnectTimeoutSeconds) * time.Second
}

// StartupTimeout allows one connection timeout per configured server plus
// a small allowance for DNS resolution and PostgreSQL session validation.
func (cfg *Config) StartupTimeout() time.Duration {
	return cfg.ConnectTimeout()*time.Duration(len(cfg.Servers)) + time.Second
}

// buildPoolConfig parses the PostgreSQL multi-host settings into a pgx pool configuration.
func buildPoolConfig(cfg Config) (*pgxpool.Config, error) {
	params := []string{
		"host=" + quoteConnStringValue(strings.Join(cfg.Servers, ",")),
		fmt.Sprintf("port=%d", cfg.Port),
		"user=" + quoteConnStringValue(cfg.User),
		"dbname=" + quoteConnStringValue(cfg.DBName),
		"sslmode=" + quoteConnStringValue(cfg.SSLMode),
		"target_session_attrs=" + quoteConnStringValue(cfg.TargetSessionAttrs),
		fmt.Sprintf("connect_timeout=%d", cfg.ConnectTimeoutSeconds),
	}

	if cfg.SSLCert != "" {
		params = append(params, "sslcert="+quoteConnStringValue(cfg.SSLCert))
	}
	if cfg.SSLKey != "" {
		params = append(params, "sslkey="+quoteConnStringValue(cfg.SSLKey))
	}
	if cfg.SSLRootCert != "" {
		params = append(params, "sslrootcert="+quoteConnStringValue(cfg.SSLRootCert))
	}

	poolCfg, err := pgxpool.ParseConfig(strings.Join(params, " "))
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	return poolCfg, nil
}

// quoteConnStringValue escapes a value for PostgreSQL's keyword/value connection string format.
func quoteConnStringValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}

// NewPool creates and verifies a PostgreSQL connection pool.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := buildPoolConfig(cfg)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	// Verify connectivity before returning the pool.
	ctxPing, cancel := context.WithTimeout(ctx, cfg.StartupTimeout())
	defer cancel()

	if err := pool.Ping(ctxPing); err != nil {
		pool.Close()
		return nil, fmt.Errorf(
			"db: ping servers=%s port=%d db=%s: %w",
			strings.Join(cfg.Servers, ","),
			cfg.Port,
			cfg.DBName,
			err,
		)
	}

	return pool, nil
}
