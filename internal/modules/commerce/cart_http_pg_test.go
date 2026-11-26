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

// TestPG_CartHTTPHandlers drives the cart surface through its HTTP handlers against the
// real schema: add, update, remove, view, and the customer-scoped reads. Handler-level
// tests are what catch a wrong status code or a missing auth check, which a service test
// cannot see.
func TestPG_CartHTTPHandlers(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-CARTH-1")
	customerID := seedCustomerRow(t, ctx, pool)
	svc.SetCartStore(store)

	call := func(h http.HandlerFunc, method, target, body, customer string, pathValues map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, target, nil)
		} else {
			r = httptest.NewRequest(method, target, strings.NewReader(body))
		}
		if customer != "" {
			r = r.WithContext(authorization.WithCustomerID(r.Context(), customer))
		}
		for k, v := range pathValues {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Add a line.
	rec := call(AddCartItemHandler(svc), http.MethodPost, "/api/v1/cart/items",
		`{"variant_id":"`+variantID+`","quantity":2}`, customerID, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("add item = %d: %s", rec.Code, rec.Body.String())
	}
	var added struct {
		CartItemID string `json:"cart_item_id"`
		CartID     string `json:"cart_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &added)

	// The cart view must show the line.
	rec = call(CartHandler(svc), http.MethodGet, "/api/v1/cart", "", customerID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cart view = %d: %s", rec.Code, rec.Body.String())
	}
	var view CartView
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if len(view.Items) != 1 || view.Items[0].Quantity != 2 {
		t.Fatalf("cart view = %+v", view.Items)
	}
	itemID := view.Items[0].ItemID

	// Update the quantity.
	rec = call(UpdateCartItemHandler(svc), http.MethodPatch, "/api/v1/cart/items/"+itemID,
		`{"quantity":5}`, customerID, map[string]string{"id": itemID})
	if rec.Code != http.StatusOK {
		t.Fatalf("update item = %d: %s", rec.Code, rec.Body.String())
	}
	dbView, err := store.CartView(ctx, view.ID)
	if err != nil {
		t.Fatalf("CartView: %v", err)
	}
	if dbView.Items[0].Quantity != 5 {
		t.Fatalf("quantity = %d, want 5", dbView.Items[0].Quantity)
	}

	// A negative quantity is refused by the handler, not by the DB. (Zero is legal: the
	// contract defines quantity 0 as "remove this line".)
	rec = call(UpdateCartItemHandler(svc), http.MethodPatch, "/api/v1/cart/items/"+itemID,
		`{"quantity":-1}`, customerID, map[string]string{"id": itemID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("negative quantity = %d, want 400", rec.Code)
	}

	// Remove it.
	rec = call(DeleteCartItemHandler(svc), http.MethodDelete, "/api/v1/cart/items/"+itemID,
		"", customerID, map[string]string{"id": itemID})
	if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
		t.Fatalf("delete item = %d: %s", rec.Code, rec.Body.String())
	}
	dbView, _ = store.CartView(ctx, view.ID)
	if len(dbView.Items) != 0 {
		t.Fatalf("cart still has %d items", len(dbView.Items))
	}

	// The customer-scoped reads must answer even with no data, and must not 500.
	rec = call(MyOrdersHandler(NewOrderListStore(pool)), http.MethodGet, "/api/v1/me/orders", "", customerID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("my orders = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(MergeCartHandler(svc), http.MethodPost, "/api/v1/cart/merge", `{"guest_cart_token":"nope"}`, customerID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("merge with an unknown token = %d: %s", rec.Code, rec.Body.String())
	}
}
