package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestCartErrorCodes pins the machine-readable codes the storefront switches on. A code
// that silently changes shape breaks the client without breaking the build.
func TestCartErrorCodes(t *testing.T) {
	cases := []struct {
		err  error
		code int
		name string
	}{
		{ErrCartTokenRequired, http.StatusBadRequest, "cart_token_required"},
		{ErrShippingAddressReq, http.StatusBadRequest, "shipping_address_required"},
		{ErrGuestPhoneRequired, http.StatusBadRequest, "guest_phone_required"},
		{ErrCartNotFound, http.StatusNotFound, "cart_not_found"},
		{ErrCartItemNotFound, http.StatusNotFound, "cart_item_not_found"},
		{ErrCartQuantityInvalid, http.StatusBadRequest, "invalid_quantity"},
		{errors.New("something else"), 0, ""},
	}
	for _, tc := range cases {
		code, name := cartErrorCode(tc.err)
		if code != tc.code || name != tc.name {
			t.Fatalf("cartErrorCode(%v) = %d/%q, want %d/%q", tc.err, code, name, tc.code, tc.name)
		}
	}
}

// TestPG_MyOrdersHandler covers the customer order list, including the paging guards and
// the fact that another customer's order never appears.
func TestPG_MyOrdersHandler(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-MOH-1")
	mine := seedCustomerRow(t, ctx, pool)
	theirs := seedCustomerRow(t, ctx, pool)

	if _, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: mine, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-MOH-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	h := MyOrdersHandler(NewOrderListStore(pool))
	get := func(customer, query string) (*httptest.ResponseRecorder, []OrderSummary, int) {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders"+query, nil)
		r = r.WithContext(authorization.WithCustomerID(r.Context(), customer))
		rec := httptest.NewRecorder()
		h(rec, r)
		var out struct {
			Data  []OrderSummary `json:"data"`
			Total int            `json:"total"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out.Data, out.Total
	}

	rec, orders, total := get(mine, "")
	if rec.Code != http.StatusOK || total != 1 || len(orders) != 1 {
		t.Fatalf("my orders = %d total %d: %s", rec.Code, total, rec.Body.String())
	}
	// Nonsense paging falls back to the defaults instead of erroring.
	if rec, _, _ := get(mine, "?page=-5&page_size=1000"); rec.Code != http.StatusOK {
		t.Fatalf("bad paging = %d: %s", rec.Code, rec.Body.String())
	}
	// Another customer sees nothing.
	if _, otherOrders, otherTotal := get(theirs, ""); otherTotal != 0 || len(otherOrders) != 0 {
		t.Fatalf("another customer saw %d orders", otherTotal)
	}
	// No identity at all: an empty list, never everyone's orders.
	if _, anonOrders, anonTotal := get("", ""); anonTotal != 0 || len(anonOrders) != 0 {
		t.Fatalf("an unauthenticated read returned %d orders", anonTotal)
	}
}

// TestPG_WebhookBranches covers the webhook's refusal branches that a happy-path test
// misses: a broken body, a missing reference, and a reference nobody knows.
func TestPG_WebhookBranches(t *testing.T) {
	_, svc, _ := setupCommerce(t)
	secret := PaymentWebhookSecret("whsec-branches")
	h := PaymentWebhookHandler(svc, secret)

	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook", strings.NewReader(body))
		r.Header.Set("X-Provider-Signature", signForTest(secret, []byte(body)))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	if rec := post(`{not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := post(`{"provider":"mock"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing reference = %d, want 400", rec.Code)
	}
	// An unknown but well-formed reference is accepted-and-ignored so the provider stops
	// retrying, without confirming anything.
	if rec := post(`{"provider":"mock","provider_reference":"NOPE","amount":1}`); rec.Code != http.StatusAccepted {
		t.Fatalf("unknown reference = %d, want 202", rec.Code)
	}
	// A webhook with no secret configured must fail closed.
	closed := PaymentWebhookHandler(svc, "")
	r := httptest.NewRequest(http.MethodPost, "/api/v1/payments/webhook",
		strings.NewReader(`{"provider":"mock","provider_reference":"X","amount":1}`))
	r.Header.Set("X-Provider-Signature", signForTest("", []byte(`{"provider":"mock","provider_reference":"X","amount":1}`)))
	rec := httptest.NewRecorder()
	closed(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty secret = %d, want 401", rec.Code)
	}
}
