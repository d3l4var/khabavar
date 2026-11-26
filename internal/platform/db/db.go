// Package db opens a pgx connection pool bound to the app_runtime role
// (Migrations 000010 — least privilege). No client ever connects with superuser (DEP §۷).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool wraps *pgxpool.Pool. The embedded pool's Ping satisfies platform/health.Checker.
type Pool struct {
	*pgxpool.Pool
}

func Open(ctx context.Context, url string, maxConns int) (*Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return &Pool{p}, nil
}

func (p *Pool) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}
