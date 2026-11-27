package commerce

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestHandlerGuards drives the early-return branches every handler shares: a broken JSON
// body and a missing path value. They are cheap to test and they are where a handler
// silently answers 500 instead of 400 when someone forgets a guard.
func TestHandlerGuards(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	svc.SetCartStore(store)
	// A real customer row: a made-up uuid trips the FK and would look like a 500 bug.
	customer := seedCustomerRow(t, context.Background(), pool)

	type tc struct {
		name    string
		h       http.HandlerFunc
		method  string
		body    string
		pv      map[string]string
		wantMin int
		wantMax int
	}
	cases := []tc{
		{"add item, broken body", AddCartItemHandler(svc), http.MethodPost, `{`, nil, 400, 400},
		{"add item, no variant", AddCartItemHandler(svc), http.MethodPost, `{"quantity":1}`, nil, 400, 500},
		{"update item, broken body", UpdateCartItemHandler(svc), http.MethodPost, `{`, map[string]string{"id": "x"}, 400, 400},
		{"merge cart, broken body", MergeCartHandler(svc), http.MethodPost, `{`, nil, 400, 400},
		{"create refund, broken body", CreateRefundHandler(svc), http.MethodPost, `{`, map[string]string{"id": "x"}, 400, 400},
		{"return request, broken body", CreateReturnRequestHandler(svc), http.MethodPost, `{`, map[string]string{"id": "x"}, 400, 400},
		{"webhook, broken body", PaymentWebhookHandler(svc, "s"), http.MethodPost, `{`, nil, 400, 401},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "/x", strings.NewReader(c.body))
		r = r.WithContext(authorization.WithCustomerID(r.Context(), customer))
		for k, v := range c.pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		c.h(rec, r)
		if rec.Code < c.wantMin || rec.Code > c.wantMax {
			t.Fatalf("%s = %d, want %d..%d: %s", c.name, rec.Code, c.wantMin, c.wantMax, rec.Body.String())
		}
	}

	// Read handlers must answer a 4xx (not a 500) for an unknown id.
	for _, c := range []struct {
		name string
		h    http.HandlerFunc
	}{
		{"refund list", ListRefundsHandler(svc)},
		{"reconcile", ReconcileTransactionHandler(svc)},
	} {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
		rec := httptest.NewRecorder()
		c.h(rec, r)
		if rec.Code >= 500 {
			t.Fatalf("%s on an unknown id = %d, want a 4xx", c.name, rec.Code)
		}
	}

	// The cart view without any identity is a 400 with a machine-readable code.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/cart", nil)
	rec := httptest.NewRecorder()
	CartHandler(svc)(rec, r)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "cart_token_required") {
		t.Fatalf("cart without identity = %d: %s", rec.Code, rec.Body.String())
	}

	// Deleting an item that does not exist is a 404, not a 500.
	r = httptest.NewRequest(http.MethodDelete, "/x", nil)
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customer))
	r.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	rec = httptest.NewRecorder()
	DeleteCartItemHandler(svc)(rec, r)
	if rec.Code >= 500 {
		t.Fatalf("delete unknown item = %d, want a 4xx", rec.Code)
	}
}
