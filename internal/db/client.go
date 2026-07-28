package db

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
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

	// Validate the completed configuration.
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validate checks required fields and supported values in Config.
func (cfg *Config) validate() error {
	if err := requireString("db_server", cfg.Host); err != nil {
		return err
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

// NewPool creates and verifies a PostgreSQL connection pool.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s dbname=%s sslmode=%s sslcert=%s sslkey=%s sslrootcert=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.DBName,
		cfg.SSLMode,
		cfg.SSLCert,
		cfg.SSLKey,
		cfg.SSLRootCert,
	)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}

	// Verify connectivity before returning the pool.
	ctxPing, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout())
	defer cancel()

	if err := pool.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("db: ping %s:%d/%s: %w", cfg.Host, cfg.Port, cfg.DBName, err)
	}

	return pool, nil
}
