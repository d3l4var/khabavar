package commerce

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPG_MergeCartsEdgeCases covers the branches of MergeCarts and the cart-expiry path
// that a single happy-path merge never reaches: the same variant in both carts, an empty
// guest cart, merging a cart into itself, and an expired guest cart.
func TestPG_MergeCartsEdgeCases(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantA := seedProductVariant(t, ctx, pool, "SKU-MG-A")
	_, variantB := seedProductVariant(t, ctx, pool, "SKU-MG-B")
	customerID := seedCustomerRow(t, ctx, pool)
	svc.SetCartStore(store)

	// Customer cart already holds variant A.
	custCart, err := store.GetOrCreateCart(ctx, customerID, "")
	if err != nil {
		t.Fatalf("customer cart: %v", err)
	}
	if err := store.AddCartItem(ctx, custCart.ID, variantA, 1); err != nil {
		t.Fatalf("AddCartItem (customer): %v", err)
	}
	// Guest cart holds A (overlapping) and B (new).
	guest, err := store.GetOrCreateCart(ctx, "", "merge-edge-1")
	if err != nil {
		t.Fatalf("guest cart: %v", err)
	}
	if err := store.AddCartItem(ctx, guest.ID, variantA, 2); err != nil {
		t.Fatalf("AddCartItem (guest A): %v", err)
	}
	if err := store.AddCartItem(ctx, guest.ID, variantB, 3); err != nil {
		t.Fatalf("AddCartItem (guest B): %v", err)
	}

	// Merging a cart into itself is a no-op, not an error.
	if err := store.MergeCarts(ctx, custCart.ID, custCart.ID); err != nil {
		t.Fatalf("MergeCarts(self) = %v, want nil", err)
	}

	view, err := svc.MergeGuestCart(ctx, customerID, "merge-edge-1")
	if err != nil {
		t.Fatalf("MergeGuestCart: %v", err)
	}
	// The overlapping variant is summed (1+2=3), the new one is added.
	qty := map[string]int{}
	for _, it := range view.Items {
		qty[it.VariantID] = it.Quantity
	}
	if qty[variantA] != 3 || qty[variantB] != 3 {
		t.Fatalf("merged quantities = %v, want A=3 B=3", qty)
	}

	// The guest cart is retired: a second merge adds nothing.
	again, err := svc.MergeGuestCart(ctx, customerID, "merge-edge-1")
	if err != nil {
		t.Fatalf("MergeGuestCart (again): %v", err)
	}
	if len(again.Items) != 2 {
		t.Fatalf("second merge produced %d lines, want 2", len(again.Items))
	}

	// An empty guest cart merges to a no-op.
	emptyGuest, err := store.GetOrCreateCart(ctx, "", "merge-edge-empty")
	if err != nil {
		t.Fatalf("empty guest cart: %v", err)
	}
	if err := store.MergeCarts(ctx, emptyGuest.ID, custCart.ID); err != nil {
		t.Fatalf("MergeCarts(empty guest) = %v", err)
	}
	afterEmpty, _ := store.CartView(ctx, custCart.ID)
	if len(afterEmpty.Items) != 2 {
		t.Fatalf("merging an empty cart changed the line count to %d", len(afterEmpty.Items))
	}

	// An expired cart is not returned by FindCartBySession, so a merge of it is a no-op.
	expired, err := store.GetOrCreateCart(ctx, "", "merge-edge-expired")
	if err != nil {
		t.Fatalf("expired guest cart: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE carts SET expires_at = now() - interval '1 hour' WHERE id = $1`, expired.ID); err != nil {
		t.Fatalf("expire cart: %v", err)
	}
	found, err := store.FindCartBySession(ctx, "merge-edge-expired")
	if err != nil {
		t.Fatalf("FindCartBySession: %v", err)
	}
	if found != nil {
		t.Fatalf("an expired cart came back from FindCartBySession: %+v", found)
	}
	if _, err := svc.MergeGuestCart(ctx, customerID, "merge-edge-expired"); err != nil {
		t.Fatalf("merging an expired cart = %v, want nil", err)
	}

	// A merge without a customer session is refused.
	if _, err := svc.MergeGuestCart(ctx, "", "merge-edge-1"); err == nil {
		t.Fatal("MergeGuestCart without a customer must fail")
	}
	_ = errors.Is
	_ = time.Now
}
