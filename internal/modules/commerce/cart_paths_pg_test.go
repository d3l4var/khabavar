package commerce

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestPG_CartServicePaths covers the cart service end to end: guest carts, quantity updates,
// removal, the empty-cart read and the guest merge. cart.go was at 75% and cart_db.go at 71%.
func TestPG_CartServicePaths(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-CART-PATH")
	svc.SetCartStore(store)

	// A guest cart keyed by session token.
	guest, err := svc.AddToCart(ctx, "", "guest-session-1", variantID, 2)
	if err != nil {
		t.Fatalf("guest add: %v", err)
	}
	if guest == nil || len(guest.Items) != 1 || guest.Items[0].Quantity != 2 {
		t.Fatalf("guest cart = %+v", guest)
	}
	// Reading it back by the same token finds it.
	again, err := svc.Cart(ctx, "", "guest-session-1")
	if err != nil || again == nil || again.ID != guest.ID {
		t.Fatalf("cart read = %+v, %v", again, err)
	}
	// An unknown token does not create a cart.
	empty, err := svc.Cart(ctx, "", "never-seen-token")
	if err != nil {
		t.Fatalf("unknown token: %v", err)
	}
	if empty != nil && len(empty.Items) > 0 {
		t.Fatalf("an unknown token produced a non-empty cart: %+v", empty)
	}

	// Update the quantity.
	itemID := guest.Items[0].ItemID
	updated, err := svc.UpdateCartItem(ctx, "", "guest-session-1", itemID, 5)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Items[0].Quantity != 5 {
		t.Fatalf("quantity after update = %d, want 5", updated.Items[0].Quantity)
	}
	// Removing the line empties the cart.
	afterRemove, err := svc.RemoveCartItem(ctx, "", "guest-session-1", itemID)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(afterRemove.Items) != 0 {
		t.Fatalf("items after remove = %+v", afterRemove.Items)
	}

	// A customer cart, then a guest cart merged into it.
	customerID := seedCustomerRow(t, ctx, pool)
	if _, err := svc.AddToCart(ctx, customerID, "", variantID, 1); err != nil {
		t.Fatalf("customer add: %v", err)
	}
	if _, err := svc.AddToCart(ctx, "", "guest-session-2", variantID, 3); err != nil {
		t.Fatalf("guest add 2: %v", err)
	}
	merged, err := svc.MergeGuestCart(ctx, customerID, "guest-session-2")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged == nil || len(merged.Items) == 0 {
		t.Fatalf("merged cart = %+v", merged)
	}
	// The quantities are summed, not replaced.
	if merged.Items[0].Quantity != 4 {
		t.Fatalf("merged quantity = %d, want 4", merged.Items[0].Quantity)
	}
	// A second merge of the same guest cart is a no-op: the guest cart was CONVERTED.
	second, err := svc.MergeGuestCart(ctx, customerID, "guest-session-2")
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if second == nil || len(second.Items) == 0 || second.Items[0].Quantity != 4 {
		t.Fatalf("second merge changed the cart: %+v", second)
	}

	// A merge with no customer session is refused.
	if _, err := svc.MergeGuestCart(ctx, "", "guest-session-2"); err == nil {
		t.Fatal("a merge without a customer session was accepted")
	}
	// Updating an unknown item is an error, not a silent success.
	if _, err := svc.UpdateCartItem(ctx, "", "guest-session-1", "00000000-0000-0000-0000-000000000000", 1); err == nil {
		t.Fatal("updating an unknown cart item succeeded")
	}
}

// TestPG_MergeCartsStoreEdge covers MergeCarts' early return and CartView's not-found arm,
// plus the merge handler's failure path. MergeCarts was at 71%, CartView at 81%.
func TestPG_MergeCartsStoreEdge(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-MRG-1")
	svc.SetCartStore(store)

	// Merging a cart into itself is a no-op, not an error.
	guest, err := svc.AddToCart(ctx, "", "guest-merge-edge", variantID, 1)
	if err != nil {
		t.Fatalf("guest add: %v", err)
	}
	if err := store.MergeCarts(ctx, guest.ID, guest.ID); err != nil {
		t.Fatalf("self merge = %v, want nil", err)
	}
	// The cart survives a self-merge.
	still, err := store.CartView(ctx, guest.ID)
	if err != nil || still == nil || len(still.Items) != 1 {
		t.Fatalf("cart after self merge = %+v, %v", still, err)
	}

	// CartView on an unknown id is ErrCartNotFound.
	if _, err := store.CartView(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrCartNotFound) {
		t.Fatalf("unknown cart = %v, want ErrCartNotFound", err)
	}
	// RemoveCartItem on an unknown line is an error, not a silent success.
	if _, err := svc.RemoveCartItem(ctx, "", "guest-merge-edge", "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("removing an unknown line succeeded")
	}

	// The merge handler: a customer merging an unknown guest token is a no-op 200.
	customerID := seedCustomerRow(t, ctx, pool)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/cart/merge",
		strings.NewReader(`{"session_token":"no-such-guest"}`))
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec := httptest.NewRecorder()
	MergeCartHandler(svc)(rec, r)
	if rec.Code >= 500 {
		t.Fatalf("merge with an unknown token = %d: %s", rec.Code, rec.Body.String())
	}
	// Without a customer session the merge is refused.
	r = httptest.NewRequest(http.MethodPost, "/api/v1/cart/merge",
		strings.NewReader(`{"session_token":"guest-merge-edge"}`))
	rec = httptest.NewRecorder()
	MergeCartHandler(svc)(rec, r)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("merge without a session = %d, want a 4xx", rec.Code)
	}
}

// TestPG_OrderInputValidationAndMyOrders covers validateOrderInput's arms and the customer
// order list handler. validateOrderInput was at 83%, MyOrdersHandler at 85%.
func TestPG_OrderInputValidationAndMyOrders(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-VAL-1")

	base := orderInput(productID, variantID, "KHA-ORD-2026-000901")

	// Each validation arm, one at a time.
	// An empty order number is NOT an error here: CreateOrder generates one. The
	// ErrOrderNumberRequired arm of validateOrderInput is therefore unreachable from this
	// entry point — pinned so the difference between the two is visible.
	noNumber := base
	noNumber.OrderNumber = ""
	generated, err := svc.CreateOrder(ctx, noNumber)
	if err != nil {
		t.Fatalf("empty number should be generated, got %v", err)
	}
	if generated == "" {
		t.Fatal("no id returned for a generated order")
	}
	noContact := base
	noContact.CustomerID, noContact.GuestPhone = "", ""
	if _, err := svc.CreateOrder(ctx, noContact); !errors.Is(err, ErrContactRequired) {
		t.Fatalf("no contact = %v, want ErrContactRequired", err)
	}
	noItems := base
	noItems.Items = nil
	if _, err := svc.CreateOrder(ctx, noItems); !errors.Is(err, ErrCartEmpty) {
		t.Fatalf("no items = %v, want ErrCartEmpty", err)
	}
	badSupply := base
	badSupply.Items = append([]OrderItemInput(nil), base.Items...)
	badSupply.Items[0].SupplyType = "BOGUS"
	if _, err := svc.CreateOrder(ctx, badSupply); err == nil {
		t.Fatal("an invalid supply_type was accepted")
	}
	noSnapshot := base
	noSnapshot.Items = append([]OrderItemInput(nil), base.Items...)
	noSnapshot.Items[0].SKUSnapshot = ""
	if _, err := svc.CreateOrder(ctx, noSnapshot); err == nil {
		t.Fatal("a missing sku snapshot was accepted")
	}

	// The customer order list handler answers 200 with paging metadata.
	customerID := seedCustomerRow(t, ctx, pool)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me/orders?page=1&page_size=5", nil)
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec := httptest.NewRecorder()
	MyOrdersHandler(NewOrderListStore(pool))(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("my orders = %d: %s", rec.Code, rec.Body.String())
	}
	// Nonsense paging falls back to the defaults.
	r = httptest.NewRequest(http.MethodGet, "/api/v1/me/orders?page=abc&page_size=9999", nil)
	r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
	rec = httptest.NewRecorder()
	MyOrdersHandler(NewOrderListStore(pool))(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("my orders with bad paging = %d", rec.Code)
	}
}
