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
