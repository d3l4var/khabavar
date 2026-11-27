package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// actor_id and aggregate_id are uuid columns, so the fixtures use real uuids.
const (
	admin1 = "11111111-1111-1111-1111-111111111111"
	admin2 = "22222222-2222-2222-2222-222222222222"
	ord1   = "33333333-3333-3333-3333-333333333333"
	ord2   = "44444444-4444-4444-4444-444444444444"
	// entity_id is a uuid column too: a non-uuid value is a driver error, not a 500.
	sbID     = "55555555-5555-5555-5555-555555555555"
	entityID = "66666666-6666-6666-6666-666666666666"
)

func setupPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	if err := migrations.ApplyAll(context.Background(), pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// TestPG_RecorderWritesTheTrail covers the audit recorder, which was at 0%. IAM §۷ makes
// the trail mandatory for money operations, so a recorder that silently writes nothing is
// the worst failure mode in the system.
func TestPG_RecorderWritesTheTrail(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	rec := NewRecorder(pool)

	// A full record: actor, action, entity, before/after and the ip.
	if err := rec.Record(ctx, "ADMIN", admin1, "inventory.adjust", "stock_balance", sbID,
		map[string]any{"on_hand": 10}, map[string]any{"on_hand": 12}, "10.0.0.1"); err != nil {
		t.Fatalf("record: %v", err)
	}
	var (
		actorType, action, entityType string
		actorID, entityID, ip         *string
		oldRaw, newRaw                []byte
	)
	if err := pool.QueryRow(ctx,
		`SELECT actor_type, action, entity_type, actor_id, entity_id, ip_address::text, old_values, new_values
		   FROM audit_logs WHERE action = 'inventory.adjust'`).
		Scan(&actorType, &action, &entityType, &actorID, &entityID, &ip, &oldRaw, &newRaw); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if actorType != "ADMIN" || entityType != "stock_balance" {
		t.Fatalf("row = %s/%s", actorType, entityType)
	}
	if actorID == nil || *actorID != admin1 || entityID == nil || *entityID != sbID {
		t.Fatalf("ids = %v/%v", actorID, entityID)
	}
	// inet normalises a bare address to a /32 network, so compare the host part.
	if ip == nil || !strings.HasPrefix(*ip, "10.0.0.1") {
		t.Fatalf("ip = %v", ip)
	}
	var oldMap, newMap map[string]any
	_ = json.Unmarshal(oldRaw, &oldMap)
	_ = json.Unmarshal(newRaw, &newMap)
	if oldMap["on_hand"] != float64(10) || newMap["on_hand"] != float64(12) {
		t.Fatalf("values = %s / %s", oldRaw, newRaw)
	}

	// Empty actor/entity/ip become SQL NULL, not the empty string: a trail row must not
	// claim an actor that does not exist.
	if err := rec.Record(ctx, "SYSTEM", "", "worker.tick", "job", "", nil, nil, ""); err != nil {
		t.Fatalf("record anonymous: %v", err)
	}
	var (
		aID, eID, ipAddr *string
		old2, new2       []byte
	)
	if err := pool.QueryRow(ctx,
		`SELECT actor_id, entity_id, ip_address::text, old_values, new_values
		   FROM audit_logs WHERE action = 'worker.tick'`).Scan(&aID, &eID, &ipAddr, &old2, &new2); err != nil {
		t.Fatalf("read anonymous: %v", err)
	}
	if aID != nil || eID != nil || ipAddr != nil {
		t.Fatalf("empty strings were stored instead of NULL: %v %v %v", aID, eID, ipAddr)
	}
	if old2 != nil || new2 != nil {
		t.Fatalf("nil values were stored instead of NULL: %s %s", old2, new2)
	}

	// A value that cannot be marshalled is an error, and nothing is written.
	bad := make(chan int)
	if err := rec.Record(ctx, "ADMIN", admin1, "bad.payload", "x", "y", bad, nil, ""); err == nil {
		t.Fatal("unmarshallable old_values was accepted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action = 'bad.payload'`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("a failed record still wrote %d rows", count)
	}
}

// TestPG_OutboxPublish covers the outbox writer (ADR-ARCH-007): the row lands unpublished
// and the payload round-trips.
func TestPG_OutboxPublish(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	pub := NewOutboxPublisher()

	if err := pub.Publish(ctx, pool, "order", ord1, "OrderConfirmed",
		map[string]any{"order_id": ord1, "total": 150000}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	var (
		aggType, aggID, eventType string
		payload                   []byte
		publishedAt               *string
	)
	if err := pool.QueryRow(ctx,
		`SELECT aggregate_type, aggregate_id, event_type, payload, published_at::text
		   FROM domain_events WHERE event_type = 'OrderConfirmed'`).
		Scan(&aggType, &aggID, &eventType, &payload, &publishedAt); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if aggType != "order" || aggID != ord1 {
		t.Fatalf("row = %s/%s", aggType, aggID)
	}
	// Unpublished, so the worker's index picks it up.
	if publishedAt != nil {
		t.Fatalf("published_at = %v, want NULL", *publishedAt)
	}
	var m map[string]any
	_ = json.Unmarshal(payload, &m)
	if m["total"] != float64(150000) {
		t.Fatalf("payload = %s", payload)
	}

	// A nil payload is a NULL column, not the string "null".
	if err := pub.Publish(ctx, pool, "order", ord2, "OrderCancelled", nil); err != nil {
		t.Fatalf("publish nil: %v", err)
	}
	var nilPayload []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM domain_events WHERE event_type = 'OrderCancelled'`).Scan(&nilPayload); err != nil {
		t.Fatalf("read nil payload: %v", err)
	}
	if nilPayload != nil {
		t.Fatalf("nil payload stored as %s", nilPayload)
	}
}

// TestPG_AuditList covers the read side and its handler: the action/actor filters, the
// from/to window and the paging metadata.
func TestPG_AuditList(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	rec := NewRecorder(pool)
	for _, a := range []struct{ action, actor string }{
		{"price.approved", admin1},
		{"price.approved", admin2},
		{"refund.issued", admin1},
	} {
		if err := rec.Record(ctx, "ADMIN", a.actor, a.action, "x", entityID, nil, nil, ""); err != nil {
			t.Fatalf("seed %s: %v", a.action, err)
		}
	}
	store := NewListStore(pool)
	get := func(q string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs?"+q, nil)
		rec := httptest.NewRecorder()
		ListHandler(store)(rec, r)
		return rec
	}

	rec1 := get("action=price.approved")
	if rec1.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec1.Code, rec1.Body.String())
	}
	var out struct {
		Data     []Entry `json:"data"`
		Total    int     `json:"total"`
		Page     int     `json:"page"`
		PageSize int     `json:"page_size"`
	}
	_ = json.Unmarshal(rec1.Body.Bytes(), &out)
	if out.Total != 2 || len(out.Data) != 2 {
		t.Fatalf("action filter = %s", rec1.Body.String())
	}
	for _, e := range out.Data {
		if e.Action != "price.approved" {
			t.Fatalf("filter leaked %s", e.Action)
		}
	}

	// The actor filter narrows to one.
	var byActor struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(get("actor_id="+admin1).Body.Bytes(), &byActor)
	if byActor.Total != 2 {
		t.Fatalf("actor filter total = %d, want 2", byActor.Total)
	}

	// A window in the future matches nothing; a window ending now matches everything.
	var future struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(get("from=2030-01-01").Body.Bytes(), &future)
	if future.Total != 0 {
		t.Fatalf("future window total = %d, want 0", future.Total)
	}
	var past struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(get("to=2030-01-01").Body.Bytes(), &past)
	if past.Total < 3 {
		t.Fatalf("past window total = %d, want >= 3", past.Total)
	}

	// Paging metadata and the cap.
	_ = json.Unmarshal(get("page_size=1&page=2").Body.Bytes(), &out)
	if out.PageSize != 1 || out.Page != 2 || len(out.Data) != 1 {
		t.Fatalf("paging = %+v", out)
	}
}
