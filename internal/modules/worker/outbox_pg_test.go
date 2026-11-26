// outbox_pg_test.go exercises DBOutbox (the pgx implementation of OutboxStore)
// against a real Postgres. Skipped unless KH_TEST_PG_DSN is set.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupPG(t *testing.T) (*pgxpool.Pool, *DBOutbox) {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool, NewDBOutbox(pool)
}

func seedEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType, aggID string, payload any) string {
	t.Helper()
	// domain_events.aggregate_id is a UUID column; call sites may pass a readable
	// label ("agg") so normalise it to a real UUID here.
	if _, err := uuid.Parse(aggID); err != nil {
		aggID = uuid.NewString()
	}
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		raw = b
	}
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO domain_events (aggregate_type, aggregate_id, event_type, payload)
		 VALUES ('test', $1, $2, $3) RETURNING id`,
		aggID, eventType, raw).Scan(&id)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return id
}

func TestPG_DBOutbox_ClaimReturnsUnpublishedOnly(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	idA := seedEvent(t, ctx, pool, "OrderConfirmed", "ord-1", nil)
	idB := seedEvent(t, ctx, pool, "OrderConfirmed", "ord-2", nil)

	events, err := store.Claim(ctx, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("claim returned %d events, want 2", len(events))
	}
	got := map[string]bool{}
	for _, e := range events {
		got[e.ID] = true
		if e.EventType != "OrderConfirmed" {
			t.Fatalf("event_type=%s", e.EventType)
		}
	}
	if !got[idA] || !got[idB] {
		t.Fatalf("missing ids: got %v want %s,%s", got, idA, idB)
	}

	// A second claim must return nothing: both rows carry a live lease. This is the
	// property that stops two worker replicas from dispatching the same event twice.
	if again, err := store.Claim(ctx, 10); err != nil {
		t.Fatalf("claim 2: %v", err)
	} else if len(again) != 0 {
		t.Fatalf("claim 2 = %+v, want none (lease still live)", again)
	}

	if err := store.MarkPublished(ctx, idA); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	// Simulate a worker that died mid-dispatch: its lease expires and the event
	// becomes claimable again, so nothing is lost.
	if _, err := pool.Exec(ctx,
		`UPDATE domain_events SET claimed_at = now() - interval '10 minutes' WHERE id = $1`, idB); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	events3, err := store.Claim(ctx, 10)
	if err != nil {
		t.Fatalf("claim 3: %v", err)
	}
	if len(events3) != 1 || events3[0].ID != idB {
		t.Fatalf("claim 3 = %+v, want only %s", events3, idB)
	}
}

func TestPG_DBOutbox_ClaimRespectsLimit(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	for i := 0; i < 5; i++ {
		seedEvent(t, ctx, pool, "Bulk", "agg", nil)
	}
	events, err := store.Claim(ctx, 3)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("claim returned %d, want 3 (limit)", len(events))
	}
}

func TestPG_DBOutbox_MarkFailedIncrementsRetryCount(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	id := seedEvent(t, ctx, pool, "RetryMe", "agg", nil)

	if err := store.MarkFailed(ctx, id); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if err := store.MarkFailed(ctx, id); err != nil {
		t.Fatalf("mark failed 2: %v", err)
	}

	events, err := store.Claim(ctx, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, e := range events {
		if e.ID == id {
			if e.RetryCount != 2 {
				t.Fatalf("retry_count=%d want 2", e.RetryCount)
			}
			return
		}
	}
	t.Fatalf("event %s not found in claim", id)
}

func TestPG_DBOutbox_ConcurrentClaimDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	for i := 0; i < 50; i++ {
		seedEvent(t, ctx, pool, "Concurrent", "agg", nil)
	}

	const workers = 2
	var (
		mu     sync.Mutex
		allIDs = map[string]int{}
		dups   atomic.Int32
		wg     sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				events, err := store.Claim(ctx, 10)
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				if len(events) == 0 {
					return
				}
				mu.Lock()
				for _, e := range events {
					allIDs[e.ID]++
					if allIDs[e.ID] > 1 {
						dups.Add(1)
					}
				}
				mu.Unlock()
				for _, e := range events {
					if err := store.MarkPublished(ctx, e.ID); err != nil {
						t.Errorf("mark published: %v", err)
					}
				}
			}
		}()
	}
	wg.Wait()

	if dups.Load() > 0 {
		t.Fatalf("FOR UPDATE SKIP LOCKED failed: %d duplicate claims", dups.Load())
	}
	if len(allIDs) != 50 {
		t.Fatalf("claimed %d unique ids, want 50", len(allIDs))
	}
}

func TestPG_Dispatcher_DrainsDBOutboxAndMarksPublished(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	idA := seedEvent(t, ctx, pool, "OrderConfirmed", "ord-1", map[string]any{"total": "100"})
	idB := seedEvent(t, ctx, pool, "OrderConfirmed", "ord-2", map[string]any{"total": "200"})

	var handled atomic.Int32
	d := NewDispatcher(store, nil, 10, 5)
	d.Register("OrderConfirmed", func(_ context.Context, _ Event) error {
		handled.Add(1)
		return nil
	})

	n, err := d.Drain(ctx)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 2 {
		t.Fatalf("drained %d, want 2", n)
	}
	if handled.Load() != 2 {
		t.Fatalf("handler invoked %d times, want 2", handled.Load())
	}

	var publishedCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM domain_events WHERE id = ANY($1) AND published_at IS NOT NULL`,
		[]string{idA, idB}).Scan(&publishedCount); err != nil {
		t.Fatalf("count published: %v", err)
	}
	if publishedCount != 2 {
		t.Fatalf("published_count=%d want 2", publishedCount)
	}

	n2, _ := d.Drain(ctx)
	if n2 != 0 {
		t.Fatalf("second drain = %d, want 0", n2)
	}
}

func TestPG_Dispatcher_RetriesOnDBFailureThenDeadLetters(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	id := seedEvent(t, ctx, pool, "Flaky", "agg", nil)

	var attempts atomic.Int32
	d := NewDispatcher(store, nil, 10, 2)
	d.Register("Flaky", func(_ context.Context, _ Event) error {
		if attempts.Add(1) <= 2 {
			return errors.New("transient")
		}
		return nil
	})

	d.Drain(ctx)
	if rc := dbRetryCount(t, ctx, pool, id); rc != 1 {
		t.Fatalf("after 1st drain retry_count=%d want 1", rc)
	}
	d.Drain(ctx)
	if rc := dbRetryCount(t, ctx, pool, id); rc != 2 {
		t.Fatalf("after 2nd drain retry_count=%d want 2", rc)
	}
	d.Drain(ctx)
	var published bool
	err := pool.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM domain_events WHERE id=$1`, id).Scan(&published)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if !published {
		t.Fatal("poison event must be dead-lettered (published_at set)")
	}
}

func dbRetryCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string) int {
	t.Helper()
	var rc int
	if err := pool.QueryRow(ctx, `SELECT retry_count FROM domain_events WHERE id=$1`, id).Scan(&rc); err != nil {
		t.Fatalf("query retry_count: %v", err)
	}
	return rc
}

var _ = time.Second
