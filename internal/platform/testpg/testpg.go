// Package testpg provides a shared Postgres test harness for PG integration
// tests across modules. Tests skip cleanly when KH_TEST_PG_DSN is unset so the
// default test suite runs without a database.
//
// Usage:
//
//	pool, cleanup := testpg.Setup(t)
//	defer cleanup()
//	migrations.ApplyAll(ctx, pool, nil)
//
// The harness drops the public schema of the target database on cleanup so each
// test starts from a clean slate. NEVER point KH_TEST_PG_DSN at a production
// database.
package testpg

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DSNEnvVar is the env var that opts a test run into PG integration tests.
const DSNEnvVar = "KH_TEST_PG_DSN"

// SkipIfNoDSN skips the calling test when KH_TEST_PG_DSN is unset or empty.
// Use it at the top of every PG-backed test.
func SkipIfNoDSN(t *testing.T) {
	t.Helper()
	if os.Getenv(DSNEnvVar) == "" {
		t.Skipf("set %s to run PG integration tests", DSNEnvVar)
	}
}

// Setup opens a pool against KH_TEST_PG_DSN and returns it along with a cleanup
// function that drops the public schema and closes the pool. The schema drop is
// what gives each test a fresh slate.
func Setup(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	SkipIfNoDSN(t)
	dsn := os.Getenv(DSNEnvVar)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}

	cleanup := func() {
		dCtx, dCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dCancel()
		if _, err := pool.Exec(dCtx, `DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public;`); err != nil {
			t.Logf("cleanup: drop schema: %v", err)
		}
		pool.Close()
	}
	return pool, cleanup
}

// SeedOIDCClient inserts an oauth_clients row for use by OIDC tests.
func SeedOIDCClient(ctx context.Context, pool *pgxpool.Pool, clientID, serviceCode, name string, redirectURIs, scopes []string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO oauth_clients (client_id, service_code, name, redirect_uris, scopes, is_active)
		 VALUES ($1,$2,$3,$4,$5,true)
		 ON CONFLICT (client_id) DO NOTHING`,
		clientID, serviceCode, name, redirectURIs, scopes)
	if err != nil {
		return fmt.Errorf("seed oauth_clients: %w", err)
	}
	return nil
}

// SeedCustomer upserts a customer by phone and returns its UUID.
func SeedCustomer(ctx context.Context, pool *pgxpool.Pool, phone string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO customers (phone, phone_verified_at)
		 VALUES ($1, now())
		 ON CONFLICT (phone) DO UPDATE SET updated_at = now()
		 RETURNING id`, phone).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("seed customer: %w", err)
	}
	return id, nil
}
