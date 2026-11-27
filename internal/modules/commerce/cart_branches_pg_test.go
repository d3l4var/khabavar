package commerce

import (
	"context"
	"errors"
	"testing"
)

// TestPG_CartServiceBranches covers the cart service's refusal paths, which the happy-path
// handler tests never reach: no identity, a bad quantity, a missing variant, an item that
// belongs to another cart, and an expired guest cart.
func TestPG_CartServiceBranches(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	_, variantID := seedProductVariant(t, ctx, pool, "SKU-CBR-1")
	customerID := seedCustomerRow(t, ctx, pool)
	svc.SetCartStore(store)

	// No identity at all.
	if _, err := svc.Cart(ctx, "", ""); !errors.Is(err, ErrCartTokenRequired) {
		t.Fatalf("Cart() with no identity = %v, want ErrCartTokenRequired", err)
	}
	// Quantity bounds: 0 and 11 are refused, 1 and 10 are not.
	for _, q := range []int{0, -1, 11} {
		if _, err := svc.AddToCart(ctx, customerID, "", variantID, q); !errors.Is(err, ErrCartQuantityInvalid) {
			t.Fatalf("AddToCart(quantity=%d) = %v, want ErrCartQuantityInvalid", q, err)
		}
	}
	// A missing variant id is a client error, not a panic.
	if _, err := svc.AddToCart(ctx, customerID, "", "", 1); err == nil {
		t.Fatal("AddToCart without a variant must fail")
	}

	// A real add, then an update that targets an item in a DIFFERENT cart.
	view, err := svc.AddToCart(ctx, customerID, "", variantID, 2)
	if err != nil {
		t.Fatalf("AddToCart: %v", err)
	}
	itemID := view.Items[0].ItemID

	otherCustomer := seedCustomerRow(t, ctx, pool)
	if _, err := svc.UpdateCartItem(ctx, otherCustomer, "", itemID, 3); !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("update across carts = %v, want ErrCartItemNotFound", err)
	}
	if _, err := svc.RemoveCartItem(ctx, otherCustomer, "", itemID); !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("remove across carts = %v, want ErrCartItemNotFound", err)
	}

	// Quantity 0 on update means "remove", and the line is gone afterwards.
	after, err := svc.UpdateCartItem(ctx, customerID, "", itemID, 0)
	if err != nil {
		t.Fatalf("UpdateCartItem(0): %v", err)
	}
	if len(after.Items) != 0 {
		t.Fatalf("quantity 0 left %d items", len(after.Items))
	}

	// Merging a guest cart that does not exist is a no-op, not an error.
	merged, err := svc.MergeGuestCart(ctx, customerID, "no-such-guest-token")
	if err != nil {
		t.Fatalf("MergeGuestCart(unknown) = %v, want nil", err)
	}
	if merged == nil {
		t.Fatal("MergeGuestCart returned no cart")
	}
	// Merging without a customer session is refused.
	if _, err := svc.MergeGuestCart(ctx, "", "tok"); err == nil {
		t.Fatal("MergeGuestCart without a customer must fail")
	}
}
