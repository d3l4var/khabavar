// Package worker implements the transactional-outbox dispatcher (ADR-ARCH-007,
// SAD §۱۳.۲). It drains domain_events rows whose published_at IS NULL, dispatches
// each to the handler registered for its event_type, and marks the row published (or
// bumps retry_count on failure, dead-lettering after maxRetries).
//
// The worker unit (KH_ROLE=worker) runs Dispatcher.Run on a ticker; the identity and
// api units do not. Domain modules enqueue events by INSERTing into domain_events in
// the same transaction as their write (§۱۴.۳) — this package only consumes.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoHandler signals that no handler is registered for an event_type and no
// fallback is set. Drain treats this as a failure subject to the retry/dead-letter
// policy, so misconfigured event_types surface in retry_count instead of being lost.
var ErrNoHandler = errors.New("worker: no handler for event_type")

// Event mirrors a domain_events row claimed for dispatch.
type Event struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       json.RawMessage
	OccurredAt    time.Time
	RetryCount    int
}

// Handler processes a single event. A nil error marks the event published; any error
// triggers a retry (or dead-letter once retry_count reaches maxRetries).
type Handler func(ctx context.Context, e Event) error

// OutboxStore is the persistence boundary for the outbox. The DB implementation uses
// SELECT ... FOR UPDATE SKIP LOCKED so multiple worker replicas do not duplicate
// work (ADR-ARCH-007); unit tests use an in-memory implementation (outbox_test.go).
type OutboxStore interface {
	Claim(ctx context.Context, limit int) ([]Event, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string) error
}

// Dispatcher drains the outbox and dispatches events to registered handlers.
type Dispatcher struct {
	store      OutboxStore
	handlers   map[string]Handler
	fallback   Handler
	logger     *slog.Logger
	batchSize  int
	maxRetries int
	now        func() time.Time
	mu         sync.RWMutex
}

func NewDispatcher(store OutboxStore, logger *slog.Logger, batchSize, maxRetries int) *Dispatcher {
	if batchSize <= 0 {
		batchSize = 100
	}
	if maxRetries < 0 {
		maxRetries = 5
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		store:      store,
		handlers:   map[string]Handler{},
		logger:     logger,
		batchSize:  batchSize,
		maxRetries: maxRetries,
		now:        time.Now,
	}
}

func (d *Dispatcher) Register(eventType string, h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[eventType] = h
}

func (d *Dispatcher) SetFallback(h Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.fallback = h
}

func (d *Dispatcher) Drain(ctx context.Context) (int, error) {
	events, err := d.store.Claim(ctx, d.batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim: %w", err)
	}
	for _, e := range events {
		if ctx.Err() != nil {
			return len(events), ctx.Err()
		}
		d.dispatch(ctx, e)
	}
	return len(events), nil
}

func (d *Dispatcher) dispatch(ctx context.Context, e Event) {
	d.mu.RLock()
	h, ok := d.handlers[e.EventType]
	fb := d.fallback
	d.mu.RUnlock()

	if !ok {
		h = fb
	}
	if h == nil {
		d.logger.Warn("worker: no handler", "event_type", e.EventType, "id", e.ID)
		d.applyFailure(ctx, e, ErrNoHandler)
		return
	}

	if err := h(ctx, e); err != nil {
		d.logger.Error("worker: dispatch failed", "event_type", e.EventType, "id", e.ID, "err", err)
		d.applyFailure(ctx, e, err)
		return
	}
	if err := d.store.MarkPublished(ctx, e.ID); err != nil {
		d.logger.Error("worker: mark published", "id", e.ID, "err", err)
	}
}

func (d *Dispatcher) applyFailure(ctx context.Context, e Event, cause error) {
	if e.RetryCount >= d.maxRetries {
		d.logger.Error("worker: dead-letter after max retries",
			"event_type", e.EventType, "id", e.ID, "retries", e.RetryCount, "cause", cause)
		if err := d.store.MarkPublished(ctx, e.ID); err != nil {
			d.logger.Error("worker: dead-letter mark", "id", e.ID, "err", err)
		}
		return
	}
	if err := d.store.MarkFailed(ctx, e.ID); err != nil {
		d.logger.Error("worker: mark failed", "id", e.ID, "err", err)
	}
}

func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	d.drainOnce(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.drainOnce(ctx)
		}
	}
}

func (d *Dispatcher) drainOnce(ctx context.Context) {
	n, err := d.Drain(ctx)
	if err != nil {
		d.logger.Error("worker: drain", "err", err)
		return
	}
	if n > 0 {
		d.logger.Info("worker: drained", "events", n)
	}
}

// ── MemOutbox ─────────────────────────────────────────────────────────────────

type MemOutbox struct {
	mu       sync.Mutex
	events   map[string]*memEvent
	order    []string
	pubOrder []string
	seq      atomic.Int64
}

type memEvent struct {
	ev        Event
	published bool
}

func NewMemOutbox() *MemOutbox {
	return &MemOutbox{events: map[string]*memEvent{}}
}

func (m *MemOutbox) Seed(e Event) string {
	if e.ID == "" {
		e.ID = fmt.Sprintf("evt-%s-%d", e.EventType, m.seq.Add(1))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events[e.ID] = &memEvent{ev: e}
	m.order = append(m.order, e.ID)
	return e.ID
}

func (m *MemOutbox) Claim(_ context.Context, limit int) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Event
	for _, id := range m.order {
		me := m.events[id]
		if me.published {
			continue
		}
		out = append(out, me.ev)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemOutbox) MarkPublished(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	me, ok := m.events[id]
	if !ok {
		return fmt.Errorf("mark published: %s not found", id)
	}
	if !me.published {
		me.published = true
		m.pubOrder = append(m.pubOrder, id)
	}
	return nil
}

func (m *MemOutbox) MarkFailed(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	me, ok := m.events[id]
	if !ok {
		return fmt.Errorf("mark failed: %s not found", id)
	}
	me.ev.RetryCount++
	return nil
}

func (m *MemOutbox) PublishedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.pubOrder))
	copy(cp, m.pubOrder)
	return cp
}

func (m *MemOutbox) RetryCount(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	me, ok := m.events[id]
	if !ok {
		return -1
	}
	return me.ev.RetryCount
}

// ── DBOutbox ──────────────────────────────────────────────────────────────────

// DefaultClaimLease is how long a claimed event stays invisible to other workers
// before it is considered abandoned and becomes claimable again. It must exceed the
// slowest handler: a handler that outlives the lease can be dispatched twice.
const DefaultClaimLease = 60 * time.Second

type DBOutbox struct {
	pool     *pgxpool.Pool
	lease    time.Duration
	workerID string
}

func NewDBOutbox(pool *pgxpool.Pool) *DBOutbox {
	return &DBOutbox{
		pool:     pool,
		lease:    DefaultClaimLease,
		workerID: "worker-" + uuid.NewString()[:8],
	}
}

// SetLease overrides the claim lease (tests use a short lease to exercise expiry).
func (s *DBOutbox) SetLease(d time.Duration) {
	if d > 0 {
		s.lease = d
	}
}

// claimSQL claims a batch atomically. FOR UPDATE SKIP LOCKED alone is not enough:
// in autocommit the row locks vanish when the statement ends, so two replicas claim
// the same rows and dispatch them twice. The UPDATE records the claim (claimed_at /
// claimed_by) inside the same statement, and the WHERE clause skips rows whose lease
// is still live. A crashed worker's rows become claimable again once the lease
// expires, so no event is lost.
const claimSQL = `
WITH picked AS (
    SELECT id
      FROM domain_events
     WHERE published_at IS NULL
       AND (claimed_at IS NULL OR claimed_at < now() - ($2::double precision * interval '1 second'))
     ORDER BY occurred_at
     LIMIT $1
     FOR UPDATE SKIP LOCKED
)
UPDATE domain_events d
   SET claimed_at = now(), claimed_by = $3
  FROM picked
 WHERE d.id = picked.id
RETURNING d.id, d.aggregate_type, d.aggregate_id, d.event_type, d.payload,
          d.occurred_at, d.retry_count`

func (s *DBOutbox) Claim(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, claimSQL, limit, s.lease.Seconds(), s.workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var id, aggID string
		if err := rows.Scan(&id, &e.AggregateType, &aggID, &e.EventType, &e.Payload, &e.OccurredAt, &e.RetryCount); err != nil {
			return nil, err
		}
		e.ID = id
		e.AggregateID = aggID
		out = append(out, e)
	}
	return out, rows.Err()
}

// MarkPublished clears the claim and stamps published_at, so the row is never
// claimed again.
func (s *DBOutbox) MarkPublished(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE domain_events
		    SET published_at = now(), claimed_at = NULL, claimed_by = NULL
		  WHERE id = $1`, id)
	return err
}

// MarkFailed bumps retry_count and releases the lease so the retry is eligible
// immediately instead of waiting for the lease to expire.
func (s *DBOutbox) MarkFailed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE domain_events
		    SET retry_count = retry_count + 1, claimed_at = NULL, claimed_by = NULL
		  WHERE id = $1`, id)
	return err
}

var (
	_ OutboxStore = (*DBOutbox)(nil)
	_ OutboxStore = (*MemOutbox)(nil)
)

var _ = pgx.ErrNoRows
