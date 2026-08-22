// Package postgres holds the read-only repository the demo paginates with, and
// the SQL that makes the keyset walk cheap.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/clevertechware/concevoir-une-api-paginee-golang/internal/config"
	"github.com/clevertechware/concevoir-une-api-paginee-golang/pkg/logger"
)

// NewPool opens a connection pool and verifies it is reachable. The caller owns
// the pool and must Close it.
func NewPool(ctx context.Context, cfg config.Postgres, log logger.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parsing database DSN: %w", err)
	}
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConns = cfg.MaxConns

	if log.Enabled(logger.LevelDebug) {
		poolConfig.ConnConfig.Tracer = newQueryTracer(log)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return pool, nil
}
