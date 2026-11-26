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

// TestCheckoutErrorMessages pins the two error shapes the HTTP layer switches on. A changed
// message here silently turns a 422 into a 500, so it is worth asserting.
func TestCheckoutErrorMessages(t *testing.T) {
	u := &ErrCheckoutUnavailable{Lines: []UnavailableLine{{VariantID: "v1", Reason: "price_unavailable"}}}
	if u.Error() == "" || !strings.Contains(u.Error(), "1") {
		t.Fatalf("ErrCheckoutUnavailable.Error() = %q, want the line count", u.Error())
	}
	// MarshalPayload is used by handlers that hand an event payload to the outbox.
	b, err := MarshalPayload(map[string]any{"order_id": "o1"})
	if err != nil || !strings.Contains(string(b), "o1") {
		t.Fatalf("MarshalPayload = %s, %v", b, err)
	}
	// An unmarshalable value must be an error, not a silent nil.
	if _, err := MarshalPayload(make(chan int)); err == nil {
		t.Fatal("MarshalPayload must fail on an unmarshalable value")
	}
}

// TestPG_CheckoutErrorBranches drives the checkout handler's refusal paths: no address, an
// empty cart, and a line that is not sellable.
func TestPG_CheckoutErrorBranches(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-CEB-1")
	customerID := seedCustomerRow(t, ctx, pool)
	svc.SetCartStore(store)

	call := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/checkout", strings.NewReader(body))
		r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
		rec := httptest.NewRecorder()
		CheckoutHandler(svc)(rec, r)
		return rec
	}

	// No shipping address.
	if rec := call(`{"payment_provider":"mock"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no address = %d: %s", rec.Code, rec.Body.String())
	}

	// Empty cart: a conflict, not a 500.
	if rec := call(`{"shipping_address":{"city":"تهران"},"payment_provider":"mock"}`); rec.Code != http.StatusConflict {
		t.Fatalf("empty cart = %d: %s", rec.Code, rec.Body.String())
	}

	// A line with no approved price is 422 with the offending lines listed.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items",
		strings.NewReader(`{"variant_id":"`+variantID+`","quantity":1}`))
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec := httptest.NewRecorder()
	AddCartItemHandler(svc)(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("add item = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(`{"shipping_address":{"city":"تهران"},"payment_provider":"mock"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsellable line = %d: %s", rec.Code, rec.Body.String())
	}
	var un struct {
		Code  string            `json:"code"`
		Items []UnavailableLine `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &un)
	if un.Code != "checkout_unavailable" || len(un.Items) == 0 {
		t.Fatalf("unavailable body = %s", rec.Body.String())
	}
}

// TestPG_CreateReturnRequestHandler covers the customer's return route through its handler:
// creation, the duplicate guard, a missing reason and a foreign order.
func TestPG_CreateReturnRequestHandler(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-RRH-1")
	customerID := seedCustomerRow(t, ctx, pool)

	orderID, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-RRH-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 3000, SupplyType: "OWNED_INVENTORY",
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
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: "REF-RRH-" + orderID[:8],
		Amount: 3000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + orderID[:8],
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, orderID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}

	call := func(body, id, cust string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		r = r.WithContext(authorization.WithCustomerID(r.Context(), cust))
		r.SetPathValue("id", id)
		rec := httptest.NewRecorder()
		CreateReturnRequestHandler(svc)(rec, r)
		return rec
	}

	if rec := call(`{"quantity":1}`, orderID, customerID); rec.Code != http.StatusBadRequest {
		t.Fatalf("no reason = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(`{"reason":"خرابی"}`, orderID, seedCustomerRow(t, ctx, pool)); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign customer = %d, want 404", rec.Code)
	}
	if rec := call(`{"reason":"خرابی"}`, orderID, customerID); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	// The partial unique index makes a second open claim a conflict.
	if rec := call(`{"reason":"دوباره"}`, orderID, customerID); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d: %s", rec.Code, rec.Body.String())
	}
}
