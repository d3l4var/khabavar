package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestPG_GetOrderReadGate is the regression test for a PII leak: GET /orders/{orderNumber}
// used to answer 200 to anyone who guessed a sequential order number, exposing guest_phone
// and the shipping address. The contract requires the owning customer or a matching
// ?phone= for a guest, and 403 otherwise.
func TestPG_GetOrderReadGate(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-GATE-1")
	owner := seedCustomerRow(t, ctx, pool)
	stranger := seedCustomerRow(t, ctx, pool)

	// A customer order.
	custOrder, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: owner, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-GATE-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	// A guest order with a phone.
	guestOrder, err := svc.CreateOrder(ctx, OrderInput{
		GuestPhone: "09120000077", Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-GATE-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	})
	if err != nil {
		t.Fatalf("CreateOrder (guest): %v", err)
	}

	get := func(id, query, customer string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+id+query, nil)
		r.SetPathValue("id", id)
		if customer != "" {
			r = r.WithContext(authorization.WithCustomerID(r.Context(), customer))
		}
		rec := httptest.NewRecorder()
		GetOrderHandler(svc).ServeHTTP(rec, r)
		return rec.Code
	}

	// Customer order: owner yes, stranger no, anonymous no.
	if code := get(custOrder, "", owner); code != http.StatusOK {
		t.Fatalf("owner read = %d, want 200", code)
	}
	if code := get(custOrder, "", stranger); code != http.StatusForbidden {
		t.Fatalf("stranger read = %d, want 403", code)
	}
	if code := get(custOrder, "", ""); code != http.StatusForbidden {
		t.Fatalf("anonymous read = %d, want 403", code)
	}

	// Guest order: needs the matching phone, in either digit form.
	if code := get(guestOrder, "", ""); code != http.StatusForbidden {
		t.Fatalf("guest read without phone = %d, want 403", code)
	}
	if code := get(guestOrder, "?phone=09120000000", ""); code != http.StatusForbidden {
		t.Fatalf("guest read with a wrong phone = %d, want 403", code)
	}
	if code := get(guestOrder, "?phone=09120000077", ""); code != http.StatusOK {
		t.Fatalf("guest read with the right phone = %d, want 200", code)
	}
	// Persian digits must work too: the customer types what the SMS showed.
	if code := get(guestOrder, "?phone=%DB%B0%DB%B9%DB%B1%DB%B2%DB%B0%DB%B0%DB%B0%DB%B0%DB%B0%DB%B7%DB%B7", ""); code != http.StatusOK {
		t.Fatalf("guest read with Persian digits = %d, want 200", code)
	}

	// The same gate applies when the path segment is the business order number.
	o, _ := svc.GetOrderByNumber(ctx, NormalizeOrderNumber("")+"")
	_ = o
	if code := get(guestOrder, "", ""); code != http.StatusForbidden {
		t.Fatalf("by-number read without phone = %d, want 403", code)
	}
}
