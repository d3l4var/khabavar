package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_PaymentHTTPHandlers covers the payment surface through its handlers: the order
// lookup, the intent creation, the webhook confirm (including its idempotent replay) and the
// refund list. These handlers were all between 60% and 67%.
func TestPG_PaymentHTTPHandlers(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-COM-PAY")
	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000401"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	call := func(h http.Handler, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}

	// --- order by number ---
	rec := call(GetOrderByNumberHandler(svc), http.MethodGet, "", map[string]string{"number": "KHA-ORD-2026-000401"})
	if rec.Code != http.StatusOK {
		t.Fatalf("by number = %d: %s", rec.Code, rec.Body.String())
	}
	var byNum struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &byNum)
	if byNum.ID != orderID {
		t.Fatalf("by number id = %q, want %q", byNum.ID, orderID)
	}
	// An unknown number is a 404, not a 500.
	if rec := call(GetOrderByNumberHandler(svc), http.MethodGet, "", map[string]string{"number": "KHA-ORD-2026-999999"}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown number = %d, want 404", rec.Code)
	}

	// --- payment intent ---
	rec = call(CreatePaymentIntentHandler(svc), http.MethodPost, "", map[string]string{"id": orderID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create intent = %d: %s", rec.Code, rec.Body.String())
	}
	var intent struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &intent)
	if intent.ID == "" {
		t.Fatalf("intent response = %s", rec.Body.String())
	}
	// An unknown order is a 404.
	if rec := call(CreatePaymentIntentHandler(svc), http.MethodPost, "",
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code != http.StatusNotFound {
		t.Fatalf("intent for an unknown order = %d, want 404", rec.Code)
	}

	// --- confirm ---
	// SECURITY FINDING, pinned so it cannot change silently: this route is mounted on
	// api.public with no signature and no auth, and ConfirmPaymentForOrder accepts an intent
	// in REQUIRES_PAYMENT. So a freshly created intent is enough to mark the order PAID —
	// nobody has to prove the money arrived. The HMAC-verified entry point is
	// POST /api/v1/payments/webhook; this one bypasses it. Whether to require a signature, a
	// staff token, or to delete the route is an owner decision (security policy).
	rec = call(ConfirmPaymentHandler(svc), http.MethodPost, "", map[string]string{"id": orderID})
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}
	// The replay is 200 with already_paid, not an error — this is the webhook contract.
	rec = call(ConfirmPaymentHandler(svc), http.MethodPost, "", map[string]string{"id": orderID})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "already_paid") {
		t.Fatalf("replay = %d: %s", rec.Code, rec.Body.String())
	}
	// The order really moved to PAID.
	detail, err := svc.GetOrderDetail(ctx, orderID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Order.Status != OrderPaid {
		t.Fatalf("status after confirm = %s, want PAID", detail.Order.Status)
	}
	// An unknown order on the webhook is a 404.
	if rec := call(ConfirmPaymentHandler(svc), http.MethodPost, "",
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code != http.StatusNotFound {
		t.Fatalf("confirm unknown = %d, want 404", rec.Code)
	}

	// --- refund list ---
	rec = call(ListRefundsHandler(svc), http.MethodGet, "", map[string]string{"id": orderID})
	if rec.Code != http.StatusOK {
		t.Fatalf("list refunds = %d: %s", rec.Code, rec.Body.String())
	}
	// An order with no refunds answers an empty list, not a 404.
	var refunds struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &refunds)
	if len(refunds.Data) != 0 {
		t.Fatalf("refunds = %s", rec.Body.String())
	}
}
