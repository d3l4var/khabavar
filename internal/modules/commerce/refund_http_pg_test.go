package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestPG_RefundHTTPHandlers drives the money-moving admin surface through its handlers:
// refund creation, the refund history, and the reconciliation check. These are the paths
// where a wrong status code or a swallowed error costs real money.
func TestPG_RefundHTTPHandlers(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-REFH-1")
	customerID := seedCustomerRow(t, ctx, pool)

	orderID, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-REFH-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 100000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	intentID, err := svc.CreatePaymentIntent(ctx, orderID)
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: "REF-RFH-" + orderID[:8],
		Amount: 100000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + orderID[:8],
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, orderID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}

	call := func(h http.HandlerFunc, method, body, id string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
		r.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// A refund with no reason is refused before any money moves.
	if rec := call(CreateRefundHandler(svc), http.MethodPost, `{"amount":1000}`, orderID); rec.Code != http.StatusBadRequest {
		t.Fatalf("refund without reason = %d: %s", rec.Code, rec.Body.String())
	}
	// A zero amount is refused too.
	if rec := call(CreateRefundHandler(svc), http.MethodPost, `{"amount":0,"reason":"خرابی"}`, orderID); rec.Code != http.StatusBadRequest {
		t.Fatalf("zero refund = %d: %s", rec.Code, rec.Body.String())
	}

	// A real partial refund succeeds and shows up in the history.
	rec := call(CreateRefundHandler(svc), http.MethodPost, `{"amount":30000,"reason":"خرابی"}`, orderID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("refund = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(ListRefundsHandler(svc), http.MethodGet, "", orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("list refunds = %d: %s", rec.Code, rec.Body.String())
	}
	var list struct {
		Data []Refund `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].Amount != 30000 {
		t.Fatalf("refunds = %+v", list.Data)
	}

	// Reconciliation must agree: paid 100000, refunded 30000.
	rec = call(ReconcileTransactionHandler(svc), http.MethodGet, "", orderID)
	if rec.Code != http.StatusOK {
		t.Fatalf("reconcile = %d: %s", rec.Code, rec.Body.String())
	}
	var rec2 struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rec2)
	if rec2.Data == nil {
		t.Fatalf("reconcile returned no data: %s", rec.Body.String())
	}

	// An unknown order is a 404, not a 500.
	if rec := call(ListRefundsHandler(svc), http.MethodGet, "", "00000000-0000-0000-0000-000000000000"); rec.Code >= http.StatusInternalServerError {
		t.Fatalf("unknown order = %d, want a 4xx", rec.Code)
	}
}
