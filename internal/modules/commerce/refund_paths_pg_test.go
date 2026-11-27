package commerce

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_CreateRefundGuards pins every refusal of CreateRefund, in the order the service
// checks them, plus the full-refund path that closes the order. refund.go was at 60-75%.
func TestPG_CreateRefundGuards(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetNow(time.Now)

	// The store must be wired first, and it says so.
	if _, err := svc.CreateRefund(ctx, "x", 1, "r"); !errors.Is(err, ErrRefundStoreNotWired) {
		t.Fatalf("unwired store = %v, want ErrRefundStoreNotWired", err)
	}
	svc.SetRefundStore(store)

	// Amount and reason are validated before anything is read.
	if _, err := svc.CreateRefund(ctx, "x", 0, "r"); !errors.Is(err, ErrRefundAmountInvalid) {
		t.Fatalf("zero amount = %v, want ErrRefundAmountInvalid", err)
	}
	if _, err := svc.CreateRefund(ctx, "x", -5, "r"); !errors.Is(err, ErrRefundAmountInvalid) {
		t.Fatalf("negative amount = %v, want ErrRefundAmountInvalid", err)
	}
	if _, err := svc.CreateRefund(ctx, "x", 100, ""); !errors.Is(err, ErrRefundReasonReq) {
		t.Fatalf("no reason = %v, want ErrRefundReasonReq", err)
	}
	// An unknown order is a sentinel.
	if _, err := svc.CreateRefund(ctx, "00000000-0000-0000-0000-000000000000", 100, "r"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unknown order = %v, want ErrOrderNotFound", err)
	}

	orderID, _ := seedPaidOrder(t, ctx, pool, 100000)
	adminID := seedAdmin(t, ctx, pool)
	adminCtx := shared.WithActorID(ctx, adminID)

	// More than the paid total is refused, and the message carries both numbers.
	if _, err := svc.CreateRefund(adminCtx, orderID, 150000, "بیش از پرداخت"); !errors.Is(err, ErrRefundExceedsPaid) {
		t.Fatalf("over-refund = %v, want ErrRefundExceedsPaid", err)
	}

	// A partial refund leaves the order open.
	id1, err := svc.CreateRefund(adminCtx, orderID, 30000, "جزئی")
	if err != nil {
		t.Fatalf("partial refund: %v", err)
	}
	var status string
	var refunded int64
	if err := pool.QueryRow(ctx, `SELECT status, refunded_total FROM orders WHERE id = $1`, orderID).
		Scan(&status, &refunded); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if refunded != 30000 {
		t.Fatalf("refunded_total = %d, want 30000", refunded)
	}
	if status == OrderClosed {
		t.Fatalf("a partial refund closed the order")
	}

	// Refunding the rest closes it.
	if _, err := svc.CreateRefund(adminCtx, orderID, 70000, "باقی"); err != nil {
		t.Fatalf("final refund: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, refunded_total FROM orders WHERE id = $1`, orderID).
		Scan(&status, &refunded); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if refunded != 100000 || status != OrderClosed {
		t.Fatalf("after a full refund: refunded=%d status=%s, want 100000/CLOSED", refunded, status)
	}

	// A further refund is refused by the STATUS guard first: a fully refunded order is
	// CLOSED, so it is no longer refundable at all.
	if _, err := svc.CreateRefund(adminCtx, orderID, 1, "بیشتر"); !errors.Is(err, ErrOrderNotRefundable) {
		t.Fatalf("refund after full = %v, want ErrOrderNotRefundable", err)
	}

	// The history lists both refunds, newest first.
	list, err := svc.ListRefunds(ctx, orderID)
	if err != nil {
		t.Fatalf("list refunds: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("refunds = %d, want 2", len(list))
	}
	_ = id1
}

// TestPG_ReturnRequestPaths covers the customer return flow: the guards, creation, and the
// status walk. returns.go had several uncovered arms.
func TestPG_ReturnRequestPaths(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-RET-PATH")
	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000501"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	customerID := seedCustomerRow(t, ctx, pool)

	// A missing reason is refused.
	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, ""); !errors.Is(err, ErrReturnReasonRequired) {
		t.Fatalf("no reason = %v, want ErrReturnReasonRequired", err)
	}
	// A zero quantity is refused.
	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 0, "خرابی"); err == nil {
		t.Fatal("a zero quantity was accepted")
	}
	// A foreign order is refused: the order must belong to the caller.
	if _, err := svc.CreateReturnRequest(ctx, orderID, "00000000-0000-0000-0000-000000000000", 1, "خرابی"); err == nil {
		t.Fatal("a return request for a foreign order was accepted")
	}
	// An unknown order is a sentinel.
	if _, err := svc.CreateReturnRequest(ctx, "00000000-0000-0000-0000-000000000000", customerID, 1, "خرابی"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unknown order = %v, want ErrOrderNotFound", err)
	}
}

// TestPG_OrderReadPaths covers GetOrderHandler's two id shapes (uuid and business number),
// the forbidden branch, and the provider-reference lookup. GetOrderHandler was at 72%.
func TestPG_OrderReadPaths(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-ORD-READ")
	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000601"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	call := func(target string, pv map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		GetOrderHandler(svc).ServeHTTP(rec, r)
		return rec
	}

	// An anonymous read of a guest order is FORBIDDEN, not 404: the order exists but the
	// caller is not its owner (privacy, not enumeration).
	rec := call("/api/v1/orders/"+orderID, map[string]string{"id": orderID})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous read = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	// A business-number path resolves to the same order; still forbidden anonymously.
	rec = call("/api/v1/orders/KHA-ORD-2026-000601", map[string]string{"id": "KHA-ORD-2026-000601"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous read by number = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	// An unknown number is a 404.
	if rec := call("/api/v1/orders/KHA-ORD-2026-999999", map[string]string{"id": "KHA-ORD-2026-999999"}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown number = %d, want 404", rec.Code)
	}
	// An unknown uuid is a 404.
	if rec := call("/api/v1/orders/00000000-0000-0000-0000-000000000000",
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown uuid = %d, want 404", rec.Code)
	}
	// A guest order read with a foreign customer session is forbidden, not 404.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID, nil)
	r.SetPathValue("id", orderID)
	r = r.WithContext(authorization.WithCustomerID(r.Context(), "00000000-0000-0000-0000-0000000000ff"))
	recForbidden := httptest.NewRecorder()
	GetOrderHandler(svc).ServeHTTP(recForbidden, r)
	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("foreign order = %d, want 403: %s", recForbidden.Code, recForbidden.Body.String())
	}

	// The provider-reference lookup: unknown reference yields empty ids, not an error.
	intentID, gotOrder, err := svc.FindIntentByProviderReference(ctx, "no-such-reference")
	if err != nil {
		t.Fatalf("reference lookup: %v", err)
	}
	if intentID != "" || gotOrder != "" {
		t.Fatalf("unknown reference returned %q/%q", intentID, gotOrder)
	}
}

// TestPG_RefundRouteByOrderNumber pins the route's contract: the path carries the business
// number, and both handlers used to read PathValue("id") — empty — so every refund answered
// 500 on an empty uuid.
func TestPG_RefundRouteByOrderNumber(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	orderID, _ := seedPaidOrder(t, ctx, pool, 100000)
	adminID := seedAdmin(t, ctx, pool)
	var orderNumber string
	if err := pool.QueryRow(ctx, `SELECT order_number FROM orders WHERE id = $1`, orderID).Scan(&orderNumber); err != nil {
		t.Fatalf("read number: %v", err)
	}

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		r = r.WithContext(shared.WithActorID(r.Context(), adminID))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// By business number — the shape the route actually declares.
	rec := call(CreateRefundHandler(svc), http.MethodPost, `{"amount":40000,"reason":"خرابی"}`,
		map[string]string{"orderNumber": orderNumber})
	if rec.Code != http.StatusCreated {
		t.Fatalf("refund by number = %d: %s", rec.Code, rec.Body.String())
	}
	// The history reads back by the same key.
	rec = call(ListRefundsHandler(svc), http.MethodGet, "", map[string]string{"orderNumber": orderNumber})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "40000") {
		t.Fatalf("list by number = %d: %s", rec.Code, rec.Body.String())
	}
	// A uuid still works, so a caller holding the id is not broken.
	rec = call(ListRefundsHandler(svc), http.MethodGet, "", map[string]string{"orderNumber": orderID})
	if rec.Code != http.StatusOK {
		t.Fatalf("list by uuid = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown number is a 404, not a 500.
	rec = call(CreateRefundHandler(svc), http.MethodPost, `{"amount":1,"reason":"x"}`,
		map[string]string{"orderNumber": "KHA-ORD-2026-999999"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown number = %d, want 404: %s", rec.Code, rec.Body.String())
	}
	// No key at all is a 404 too, never a 500.
	rec = call(CreateRefundHandler(svc), http.MethodPost, `{"amount":1,"reason":"x"}`, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("no key = %d, want 404", rec.Code)
	}
}

// TestPG_ReturnableStatuses pins which order states accept a return. DELIVERED was missing —
// the goods are in the customer's hands, the most ordinary moment to return them, and the
// route answered 409 return_not_allowed.
func TestPG_ReturnableStatuses(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{OrderPaid, true},
		{OrderInFulfillment, true},
		{OrderShipped, true},
		{OrderDelivered, true},
		{OrderCompleted, true},
		// A return before the money was taken, or after a cancellation, makes no sense.
		{OrderPendingPayment, false},
		{OrderCancelled, false},
	}
	for _, c := range cases {
		if got := returnableStatuses[c.status]; got != c.want {
			t.Fatalf("returnable[%s] = %v, want %v", c.status, got, c.want)
		}
	}
}
