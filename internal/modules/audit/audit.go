// Package audit implements the audit module (SAD §۴.۱): Audit Log + Domain Events
// + Transactional Outbox. Entities: audit_logs, domain_events. All modules write only.
//
// Seven sensitive operation categories are recorded in append-only audit_logs and are
// tamper-proof (§۱۳.۱, ADR-ARCH-010): sale price, purchase cost, supplier approval,
// inventory change, refund, order status change, shipping terms. Reading audit_logs
// requires the audit.read permission (only SUPER_ADMIN and FINANCE — IAM §۷).
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// ActorType values (audit_logs.actor_type CHECK constraint).
const (
	ActorCustomer = "CUSTOMER"
	ActorAdmin    = "ADMIN"
	ActorSystem   = "SYSTEM"
)

// DBExec is satisfied by *pgxpool.Pool and pgx.Tx, so the Outbox can publish within
// a caller's transaction (ADR-ARCH-007: payment confirmation + event in one tx).
type DBExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Recorder writes append-only audit_logs rows (§۱۳.۱, ADR-ARCH-010).
type Recorder struct {
	db DBExec
}

// NewRecorder builds a Recorder bound to db (pool or tx).
func NewRecorder(db DBExec) *Recorder { return &Recorder{db: db} }

// Record inserts an audit row. oldValues/newValues may be nil (stored as NULL).
func (r *Recorder) Record(ctx context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error {
	oldb, err := marshalJSON(oldValues)
	if err != nil {
		return err
	}
	newb, err := marshalJSON(newValues)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO audit_logs (id, actor_type, actor_id, action, entity_type, entity_id, old_values, new_values, ip_address)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		uuid.NewString(), actorType, nullableStr(actorID), action, entityType, nullableStr(entityID), oldb, newb, nullableStr(ip))
	return err
}

// OutboxPublisher publishes domain_events rows (ADR-ARCH-007). Pass the caller's
// pgx.Tx as db so the event commits atomically with the state change (e.g. payment
// confirmation + OrderConfirmed in one tx — Commerce §۱۰, ADR-ARCH-00۷).
type OutboxPublisher struct{}

// NewOutboxPublisher builds an OutboxPublisher (stateless; db passed per call).
func NewOutboxPublisher() *OutboxPublisher { return &OutboxPublisher{} }

// Publish inserts a domain event row with published_at NULL; the worker marks it
// published after successful dispatch (ix_domain_events_unpublished selects these).
func (p *OutboxPublisher) Publish(ctx context.Context, db DBExec, aggregateType, aggregateID, eventType string, payload any) error {
	pb, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx,
		`INSERT INTO domain_events (id, aggregate_type, aggregate_id, event_type, payload, occurred_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		uuid.NewString(), aggregateType, aggregateID, eventType, pb, time.Now().UTC())
	return err
}

// marshalJSON returns nil for nil input (→ SQL NULL) or a JSON string for jsonb
// columns. A string is used so pgx sends text that PostgreSQL casts to jsonb.
func marshalJSON(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
