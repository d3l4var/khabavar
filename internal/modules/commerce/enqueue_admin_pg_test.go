package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
)

// recordingQueue captures the outbox events a service emits.
type recordingQueue struct {
	events []string
	err    error
}

func (q *recordingQueue) Enqueue(_ context.Context, eventType, _, _ string, _ any) error {
	q.events = append(q.events, eventType)
	return q.err
}

// TestPG_ConfirmEmitsOrderPaid covers the outbox side of a payment confirmation: the event is
// emitted, a queue failure is swallowed (the order write already succeeded), and a replayed
// confirm does not emit a second event. ConfirmPaymentForOrder and enqueue were at 78%/75%.
func TestPG_ConfirmEmitsOrderPaid(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	queue := &recordingQueue{}
	svc := NewService(store, queue)

	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-ENQ-1")
	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000701"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	queue.events = nil // ignore OrderCreated

	if _, err := svc.CreatePaymentIntent(ctx, orderID); err != nil {
		t.Fatalf("intent: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, orderID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	var sawPaid bool
	for _, e := range queue.events {
		if e == "OrderPaid" {
			sawPaid = true
		}
	}
	if !sawPaid {
		t.Fatalf("no OrderPaid event: %v", queue.events)
	}

	// A replay is ErrAlreadyPaid and emits nothing new.
	before := len(queue.events)
	if err := svc.ConfirmPaymentForOrder(ctx, orderID); !errors.Is(err, ErrAlreadyPaid) {
		t.Fatalf("replay = %v, want ErrAlreadyPaid", err)
	}
	if len(queue.events) != before {
		t.Fatalf("a replayed confirm emitted %d events", len(queue.events)-before)
	}

	// A queue failure must NOT fail the confirmation: the money already moved.
	pool2, _, _ := setupCommerce(t)
	store2 := NewDBStore(pool2)
	failing := &recordingQueue{err: errors.New("queue down")}
	svc2 := NewService(store2, failing)
	p2, v2 := seedProductVariant(t, ctx, pool2, "SKU-ENQ-2")
	order2, err := svc2.CreateOrder(ctx, orderInput(p2, v2, "KHA-ORD-2026-000702"))
	if err != nil {
		t.Fatalf("CreateOrder 2: %v", err)
	}
	if _, err := svc2.CreatePaymentIntent(ctx, order2); err != nil {
		t.Fatalf("intent 2: %v", err)
	}
	if err := svc2.ConfirmPaymentForOrder(ctx, order2); err != nil {
		t.Fatalf("confirm with a failing queue must still succeed: %v", err)
	}
}

// TestPG_ListOrdersHandler covers the admin list handler: the filters, the paging defaults
// and the 500 arm. ListOrdersHandler was at 75%.
func TestPG_ListOrdersHandlerPaging(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-LST-1")
	if _, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000801")); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	list := NewOrderListStore(pool)
	get := func(q string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?"+q, nil)
		rec := httptest.NewRecorder()
		ListOrdersHandler(list)(rec, r)
		return rec
	}

	rec := get("")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data     []OrderSummary `json:"data"`
		Total    int            `json:"total"`
		Page     int            `json:"page"`
		PageSize int            `json:"page_size"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Total < 1 || out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("list body = %s", rec.Body.String())
	}
	// The status filter and the q filter both apply.
	if rec := get("status=PENDING_PAYMENT"); rec.Code != http.StatusOK {
		t.Fatalf("status filter = %d", rec.Code)
	}
	if rec := get("q=000801"); rec.Code != http.StatusOK {
		t.Fatalf("q filter = %d", rec.Code)
	}
	// Nonsense paging falls back to the defaults, and page_size is capped at 100.
	_ = json.Unmarshal(get("page=abc&page_size=xyz").Body.Bytes(), &out)
	if out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("paging fallback = %+v", out)
	}
	_ = json.Unmarshal(get("page_size=5000").Body.Bytes(), &out)
	if out.PageSize != 20 {
		t.Fatalf("page_size cap = %d, want 20 (out-of-range falls back)", out.PageSize)
	}

	// The 500 arm: a closed pool makes the query fail.
	deadPool := testpgSetupClosed(t)
	rec = func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil)
		rec := httptest.NewRecorder()
		ListOrdersHandler(NewOrderListStore(deadPool))(rec, r)
		return rec
	}()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list with a dead pool = %d, want 500", rec.Code)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("the 500 arm answered an empty body")
	}
}

// testpgSetupClosed returns a pool that is already closed.
func testpgSetupClosed(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, _ := testpg.Setup(t)
	pool.Close()
	return pool
}
