package migrations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplyAll runs every up migration in order against the pool. It is idempotent
// for IF EXISTS / IF NOT EXISTS migrations (all KHABAVAR migrations are written
// this way) so it is safe to call on a partially-migrated database. It does NOT
// track applied state in a schema_migrations table — that is the responsibility
// of the deploy pipeline (golang-migrate). ApplyAll is intended for integration
// tests and local development.
func ApplyAll(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	ms, err := UpMigrations()
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	for _, m := range ms {
		if _, err := pool.Exec(ctx, m.SQL); err != nil {
			return fmt.Errorf("apply %s_%s: %w", m.Seq, m.Name, err)
		}
		log.Info("migrations: applied", "seq", m.Seq, "name", m.Name)
	}
	return nil
}
