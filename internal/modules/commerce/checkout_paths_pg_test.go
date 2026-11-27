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

// TestPG_CheckoutPaths covers the three checkout entry shapes the service supports: a guest
// sending items directly (folded into a cart), an explicit CartID, and the guard for a guest
// with no phone. Checkout was at 70.7%.
func TestPG_CheckoutPaths(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-CHK-PATH")
	svc.SetCartStore(store)

	// An approved price is what makes a line sellable.
	var approverID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'تأییدکننده','ACTIVE') RETURNING id`,
		"chk-approver@khabavar.ir").Scan(&approverID); err != nil {
		t.Fatalf("seed approver: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO product_price_records (variant_id, selling_price, currency, valid_from, approved_by)
		 VALUES ($1, 50000, 'IRR', now() - interval '1 hour', $2)`, variantID, approverID); err != nil {
		t.Fatalf("seed price: %v", err)
	}

	addr := `{"city":"تهران","province":"تهران","postal_code":"1234567890",` +
		`"recipient_name":"مشتری","recipient_phone":"09120000001"}`

	// A guest with no phone is refused before anything is priced.
	if _, err := svc.Checkout(ctx, "", "", CheckoutInput{
		ShippingAddress: json.RawMessage(addr), PaymentProvider: "mock",
		Items: []CheckoutItem{{VariantID: variantID, Quantity: 1}},
	}); err != ErrGuestPhoneRequired {
		t.Fatalf("guest without a phone = %v, want ErrGuestPhoneRequired", err)
	}
	// A missing shipping address is refused too.
	if _, err := svc.Checkout(ctx, "", "guest-token-1", CheckoutInput{
		PaymentProvider: "mock",
	}); err != ErrShippingAddressReq {
		t.Fatalf("no address = %v, want ErrShippingAddressReq", err)
	}

	// A guest sending items directly: they are folded into a cart and priced once.
	res, err := svc.Checkout(ctx, "", "guest-token-2", CheckoutInput{
		ShippingAddress: json.RawMessage(addr),
		PaymentProvider: "mock",
		GuestPhone:      "09120000002",
		Items:           []CheckoutItem{{VariantID: variantID, Quantity: 2}},
	})
	if err != nil {
		t.Fatalf("guest checkout with items: %v", err)
	}
	if res == nil || res.Order == nil || res.Order.OrderNumber == "" {
		t.Fatalf("guest checkout result = %+v", res)
	}
	// 2 × 50000 plus shipping.
	if res.Order.GrandTotal < 100000 {
		t.Fatalf("grand total = %d, want at least 100000", res.Order.GrandTotal)
	}

	// An explicit CartID path: build a customer cart, then check out by id.
	customerID := seedCustomerRow(t, ctx, pool)
	if _, err := svc.AddToCart(ctx, customerID, "", variantID, 1); err != nil {
		t.Fatalf("add to cart: %v", err)
	}
	cart, err := svc.Cart(ctx, customerID, "")
	if err != nil || cart == nil || cart.ID == "" {
		t.Fatalf("cart = %+v, %v", cart, err)
	}
	res2, err := svc.Checkout(ctx, customerID, "", CheckoutInput{
		ShippingAddress: json.RawMessage(addr),
		PaymentProvider: "mock",
		CartID:          cart.ID,
	})
	if err != nil {
		t.Fatalf("checkout by cart id: %v", err)
	}
	if res2 == nil || res2.Order == nil || res2.Order.OrderNumber == "" {
		t.Fatalf("cart-id checkout result = %+v", res2)
	}
	// The two orders are distinct.
	if res2.Order.OrderNumber == res.Order.OrderNumber {
		t.Fatal("two checkouts produced the same order number")
	}

	// The handler path for a guest with no phone answers a 4xx, not a 500.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/checkout", strings.NewReader(
		`{"shipping_address":`+addr+`,"payment_provider":"mock"}`))
	rec := httptest.NewRecorder()
	CheckoutHandler(svc)(rec, r)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("guest checkout without a phone = %d, want a 4xx: %s", rec.Code, rec.Body.String())
	}
	_ = authorization.WithCustomerID
}
