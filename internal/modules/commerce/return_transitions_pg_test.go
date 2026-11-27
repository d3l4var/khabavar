package commerce

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_ReturnTransitions walks the claim lifecycle. Before this existed a customer could
// file a return and nothing could move it out of REQUESTED.
func TestPG_ReturnTransitions(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	orderID, _ := seedPaidOrder(t, ctx, pool, 100000)
	customerID := seedCustomerRow(t, ctx, pool)
	// The claim is scoped by customer, so the order must belong to them.
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 'DELIVERED', customer_id = $2 WHERE id = $1`, orderID, customerID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	claim, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "خرابی")
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}

	// A rejection without a note is refused: the customer is owed a reason.
	if _, err := svc.DecideReturn(ctx, claim.ID, false, ""); !errors.Is(err, ErrReturnDecisionNote) {
		t.Fatalf("reject without note = %v, want ErrReturnDecisionNote", err)
	}
	// Receiving before approval is not a legal transition.
	if _, err := svc.ReceiveReturn(ctx, claim.ID, ""); !errors.Is(err, ErrReturnTransition) {
		t.Fatalf("receive before approve = %v, want ErrReturnTransition", err)
	}

	approved, err := svc.DecideReturn(ctx, claim.ID, true, "تأیید شد")
	if err != nil || approved.Status != ReturnApproved {
		t.Fatalf("approve = %+v, %v", approved, err)
	}
	// Approving twice loses the race rather than overwriting.
	if _, err := svc.DecideReturn(ctx, claim.ID, true, ""); !errors.Is(err, ErrReturnTransition) {
		t.Fatalf("second approve = %v, want ErrReturnTransition", err)
	}
	received, err := svc.ReceiveReturn(ctx, claim.ID, "رسید")
	if err != nil || received.Status != ReturnReceived {
		t.Fatalf("receive = %+v, %v", received, err)
	}
	// RECEIVED → REFUNDED is legal; RECEIVED → APPROVED is not.
	if !CanTransitionReturn(ReturnReceived, ReturnRefunded) {
		t.Fatal("RECEIVED → REFUNDED must be legal")
	}
	if CanTransitionReturn(ReturnReceived, ReturnApproved) {
		t.Fatal("RECEIVED → APPROVED must not be legal")
	}
	// An unknown claim is a 404, not a 500.
	if _, err := store.GetReturnRequest(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrReturnNotFound) {
		t.Fatalf("unknown claim = %v, want ErrReturnNotFound", err)
	}
}

// TestPG_ReturnHTTPHandlers pins the status codes the panel sees.
func TestPG_ReturnHTTPHandlers(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	orderID, _ := seedPaidOrder(t, ctx, pool, 100000)
	customerID := seedCustomerRow(t, ctx, pool)
	// The claim is scoped by customer, so the order must belong to them.
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 'DELIVERED', customer_id = $2 WHERE id = $1`, orderID, customerID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	claim, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "خرابی")
	if err != nil {
		t.Fatalf("create claim: %v", err)
	}

	adminID := seedAdmin(t, ctx, pool)
	call := func(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(http.MethodPost, "/x", nil)
		} else {
			r = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		}
		r.SetPathValue("id", claim.ID)
		r = r.WithContext(shared.WithActorID(r.Context(), adminID))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Rejection without a note is a 400 with a code the panel can branch on.
	rec := call(RejectReturnHandler(svc), `{}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision_note_required") {
		t.Fatalf("reject without note = %d: %s", rec.Code, rec.Body.String())
	}
	// Receiving before approval is a 409.
	rec = call(ReceiveReturnHandler(svc), "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "invalid_return_transition") {
		t.Fatalf("receive before approve = %d: %s", rec.Code, rec.Body.String())
	}
	// Approve, then receive.
	rec = call(ApproveReturnHandler(svc), `{"note":"ok"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "APPROVED") {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(ReceiveReturnHandler(svc), `{"note":"رسید"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "RECEIVED") {
		t.Fatalf("receive = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown claim is a 404.
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec = httptest.NewRecorder()
	ApproveReturnHandler(svc)(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown claim = %d, want 404", rec.Code)
	}
}

// TestPG_ListReturns covers the panel queue: staff had no way to find a claim, only to act on
// an id they already knew.
func TestPG_ListReturns(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	customerID := seedCustomerRow(t, ctx, pool)
	orderA, _ := seedPaidOrder(t, ctx, pool, 100000)
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 'DELIVERED', customer_id = $2 WHERE id = $1`, orderA, customerID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	first, err := svc.CreateReturnRequest(ctx, orderA, customerID, 1, "خرابی")
	if err != nil {
		t.Fatalf("claim A: %v", err)
	}
	// seedPaidOrder reuses one product slug, so a second call collides. The second claim only
	// needs a row that belongs to another order, which a direct insert gives us.
	var orderB string
	if err := pool.QueryRow(ctx, `INSERT INTO orders (order_number, status, currency, subtotal, grand_total,
			shipping_address_snapshot, customer_id)
		VALUES ('KHA-ORD-2026-900001', 'DELIVERED', 'IRR', 50000, 50000, '{}'::jsonb, $1) RETURNING id`, customerID).Scan(&orderB); err != nil {
		t.Fatalf("order B: %v", err)
	}
	if _, err := svc.CreateReturnRequest(ctx, orderB, customerID, 2, "اشتباه"); err != nil {
		t.Fatalf("claim B: %v", err)
	}
	if _, err := svc.DecideReturn(ctx, first.ID, true, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Unfiltered: both claims.
	all, total, err := svc.ListReturns(ctx, "", 1, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total < 2 || len(all) < 2 {
		t.Fatalf("list = %d items, total %d, want both", len(all), total)
	}
	// Filtered: only the approved one.
	approved, atotal, err := svc.ListReturns(ctx, ReturnApproved, 1, 20)
	if err != nil {
		t.Fatalf("list approved: %v", err)
	}
	if atotal != 1 || len(approved) != 1 || approved[0].ID != first.ID {
		t.Fatalf("filtered = %d items, total %d, want just %s", len(approved), atotal, first.ID)
	}
	// An unknown status matches nothing rather than everything.
	if _, none, err := svc.ListReturns(ctx, ReturnRefunded, 1, 20); err != nil || none != 0 {
		t.Fatalf("refunded = %d, %v; want 0", none, err)
	}
}

// TestPG_RefundReturn closes the lifecycle: RECEIVED → REFUNDED moves money through the same
// audited refund path, with the amount chosen by the operator (the claim has no line item).
func TestPG_RefundReturn(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	orderID, _ := seedPaidOrder(t, ctx, pool, 100000)
	customerID := seedCustomerRow(t, ctx, pool)
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 'DELIVERED', customer_id = $2 WHERE id = $1`, orderID, customerID); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	claim, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "خرابی")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Refunding before the goods came back is not a legal transition.
	if _, err := svc.RefundReturn(ctx, claim.ID, 40000, "زود"); !errors.Is(err, ErrReturnTransition) {
		t.Fatalf("refund before receive = %v, want ErrReturnTransition", err)
	}
	if _, err := svc.DecideReturn(ctx, claim.ID, true, "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.ReceiveReturn(ctx, claim.ID, "رسید"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	// A zero amount is refused: a refund of nothing is not a refund.
	if _, err := svc.RefundReturn(ctx, claim.ID, 0, "صفر"); !errors.Is(err, ErrRefundAmountInvalid) {
		t.Fatalf("zero amount = %v, want ErrRefundAmountInvalid", err)
	}
	// More than was paid is refused by the refund path itself.
	if _, err := svc.RefundReturn(ctx, claim.ID, 999999, "زیاد"); !errors.Is(err, ErrRefundExceedsPaid) {
		t.Fatalf("over-refund = %v, want ErrRefundExceedsPaid", err)
	}

	out, err := svc.RefundReturn(ctx, claim.ID, 40000, "بازپرداخت مرجوعی")
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if out.Status != ReturnRefunded {
		t.Fatalf("status = %s, want REFUNDED", out.Status)
	}
	// The decision note carries the refund id, so the claim and the money are linked.
	if out.DecisionNote == "" {
		t.Fatal("decision note must carry the refund id")
	}
	// The money actually moved.
	var refunded int64
	if err := pool.QueryRow(ctx, `SELECT refunded_total FROM orders WHERE id = $1`, orderID).Scan(&refunded); err != nil {
		t.Fatalf("read refunded: %v", err)
	}
	if refunded != 40000 {
		t.Fatalf("refunded_total = %d, want 40000", refunded)
	}
	// Refunding twice is refused: the claim is no longer RECEIVED.
	if _, err := svc.RefundReturn(ctx, claim.ID, 40000, "دوباره"); !errors.Is(err, ErrReturnTransition) {
		t.Fatalf("second refund = %v, want ErrReturnTransition", err)
	}
}
