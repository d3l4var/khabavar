package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestPG_CheckoutHandlerUsesCustomerCart is the regression test for the checkout handler
// discarding the signed-in customer: before the fix it passed "" as the customer id, so a
// signed-in customer's cart looked empty and checkout answered 409 cart_empty.
func TestPG_CheckoutHandlerUsesCustomerCart(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-COH-1")
	customerID := seedCustomerRow(t, ctx, pool)
	svc.SetCartStore(store)

	// Checkout refuses a line with no approved price (price_unavailable), so the variant
	// needs a real approved price record — the same gate the storefront relies on.
	// approved_by references admin_users, so the approval needs a real staff account.
	var approverID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'تأییدکننده','ACTIVE') RETURNING id`,
		"approver-"+variantID[:8]+"@khabavar.ir").Scan(&approverID); err != nil {
		t.Fatalf("seed approver: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO product_price_records (variant_id, selling_price, currency, valid_from, approved_by)
		 VALUES ($1, 50000, 'IRR', now() - interval '1 hour', $2)`,
		variantID, approverID); err != nil {
		t.Fatalf("seed price record: %v", err)
	}

	// Fill the customer's cart through the handler (no guest token at all).
	r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/items",
		strings.NewReader(`{"variant_id":"`+variantID+`","quantity":1}`))
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec := httptest.NewRecorder()
	AddCartItemHandler(svc)(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("add item = %d: %s", rec.Code, rec.Body.String())
	}

	// Checkout must see that cart.
	body := `{"shipping_address":{"city":"تهران","province":"تهران","postal_code":"1234567890",` +
		`"recipient_name":"مشتری","recipient_phone":"09120000001"},"payment_provider":"mock"}`
	r = httptest.NewRequest(http.MethodPost, "/api/v1/checkout", strings.NewReader(body))
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec = httptest.NewRecorder()
	CheckoutHandler(svc)(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Order struct {
			ID          string `json:"id"`
			OrderNumber string `json:"order_number"`
		} `json:"order"`
		Payment struct {
			IntentID string `json:"intent_id"`
		} `json:"payment"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if out.Order.ID == "" || out.Payment.IntentID == "" {
		t.Fatalf("checkout response = %s", rec.Body.String())
	}

	// The order must belong to the signed-in customer, not to a guest.
	o, err := store.GetOrder(ctx, out.Order.ID)
	if err != nil || o == nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if o.CustomerID != customerID {
		t.Fatalf("order customer = %q, want %q (checkout dropped the identity)", o.CustomerID, customerID)
	}

	// A second checkout on the now-empty cart is a conflict, not a second order.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/checkout", strings.NewReader(body))
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec = httptest.NewRecorder()
	CheckoutHandler(svc)(rec, r)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second checkout = %d, want 409", rec.Code)
	}
}
