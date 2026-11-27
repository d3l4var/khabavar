package commerce

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

	// --- confirm: the route is public, so it requires the provider signature ---
	signed := func(id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
		r.SetPathValue("id", id)
		mac := hmac.New(sha256.New, []byte(testWebhookSecret))
		mac.Write([]byte(""))
		r.Header.Set("X-Provider-Signature", hex.EncodeToString(mac.Sum(nil)))
		rec := httptest.NewRecorder()
		ConfirmPaymentHandler(svc, testWebhookSecret).ServeHTTP(rec, r)
		return rec
	}
	// An unsigned call is refused: the order id alone must not mark an order paid.
	if rec := call(ConfirmPaymentHandler(svc, testWebhookSecret), http.MethodPost, "", map[string]string{"id": orderID}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned confirm = %d, want 401: %s", rec.Code, rec.Body.String())
	}
	// A wrong signature is refused too.
	bad := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
	bad.SetPathValue("id", orderID)
	bad.Header.Set("X-Provider-Signature", "deadbeef")
	recBad := httptest.NewRecorder()
	ConfirmPaymentHandler(svc, testWebhookSecret).ServeHTTP(recBad, bad)
	if recBad.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", recBad.Code)
	}
	// The order is untouched by the refused calls.
	if d, _ := svc.GetOrderDetail(ctx, orderID); d.Order.Status != OrderPendingPayment {
		t.Fatalf("a refused confirm changed the status to %s", d.Order.Status)
	}
	rec = signed(orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d: %s", rec.Code, rec.Body.String())
	}
	// The replay is 200 with already_paid, not an error — this is the webhook contract.
	rec = signed(orderID)
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
	if rec := signed("00000000-0000-0000-0000-000000000000"); rec.Code != http.StatusNotFound {
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
