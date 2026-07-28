package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureSchema applies the embedded database schema required by the application.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	data, err := SQLFiles.ReadFile("sql/schema_secarch_ticket.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}

	if _, err := pool.Exec(ctx, string(data)); err != nil {
		return fmt.Errorf("execute schema: %w", err)
	}

	return nil
}
