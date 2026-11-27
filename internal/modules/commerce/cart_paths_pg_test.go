package commerce

import (
	"context"
	"testing"
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
