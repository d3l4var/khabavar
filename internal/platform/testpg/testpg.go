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
// Isolation: every test gets its own schema (test_<random>) and the pool's
// search_path points at it, so packages can run in parallel against one database
// without dropping each other's tables. Cleanup drops only that schema.
//
// NEVER point KH_TEST_PG_DSN at a production database.
package testpg

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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

// Setup opens a pool isolated to a fresh schema and returns it with a cleanup
// function that drops that schema and closes the pool.
func Setup(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	SkipIfNoDSN(t)
	dsn := os.Getenv(DSNEnvVar)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = 5
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	// search_path may name a schema that does not exist yet: Postgres skips it
	// until it is created, so CREATE SCHEMA below lands in the right place.
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping db: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		pool.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}

	cleanup := func() {
		dCtx, dCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer dCancel()
		if _, err := pool.Exec(dCtx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Logf("cleanup: drop schema %s: %v", schema, err)
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

// SeedCustomer upserts a customer by phone and returns its UUID. The unique index on
// phone is partial (WHERE phone IS NOT NULL, §۱۵), so the conflict target must repeat
// that predicate or Postgres cannot infer the index.
func SeedCustomer(ctx context.Context, pool *pgxpool.Pool, phone string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO customers (phone, phone_verified_at)
		 VALUES ($1, now())
		 ON CONFLICT (phone) WHERE phone IS NOT NULL DO UPDATE SET updated_at = now()
		 RETURNING id`, phone).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("seed customer: %w", err)
	}
	return id, nil
}
