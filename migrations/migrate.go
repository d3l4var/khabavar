package migrations

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// applyLockID serialises concurrent ApplyAll calls. Integration tests in different
// packages share one database and each applies the full migration set; without the
// lock two of them can race on the global objects (roles in 000010) and fail with
// "role already exists".
const applyLockID int64 = 0x6b68616261766172 // "khabavar"

// ApplyAll runs every up migration in order against the pool. It is idempotent
// for IF EXISTS / IF NOT EXISTS migrations (all KHABAVAR migrations are written
// this way) so it is safe to call on a partially-migrated database. It does NOT
// track applied state in a schema_migrations table — that is the responsibility
// of the deploy pipeline (golang-migrate). ApplyAll is intended for integration
// tests and local development.
//
// Migrations run on a single connection under a session advisory lock, so the whole
// set executes in one session (search_path and temp state stay consistent) and
// concurrent test packages queue instead of colliding.
func ApplyAll(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	ms, err := UpMigrations()
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", applyLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", applyLockID)
	}()

	for _, m := range ms {
		if _, err := conn.Exec(ctx, m.SQL); err != nil {
			return fmt.Errorf("apply %s_%s: %w", m.Seq, m.Name, err)
		}
		log.Info("migrations: applied", "seq", m.Seq, "name", m.Name)
	}
	return nil
}
