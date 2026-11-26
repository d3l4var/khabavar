package commerce

import (
	"context"
	"errors"
	"fmt"
)

// SetCartStore wires the server-side cart (DB §14.1).
func (s *Service) SetCartStore(store CartStore) { s.carts = store }

// Cart returns the current cart for a guest session token or a customer.
func (s *Service) Cart(ctx context.Context, customerID, sessionToken string) (*CartView, error) {
	if s.carts == nil {
		return nil, errors.New("commerce: cart store not configured")
	}
	if customerID == "" && sessionToken == "" {
		return nil, ErrCartTokenRequired
	}
	c, err := s.carts.GetOrCreateCart(ctx, customerID, sessionToken)
	if err != nil {
		return nil, err
	}
	return s.carts.CartView(ctx, c.ID)
}

// AddToCart adds a variant to the cart and returns the priced cart.
func (s *Service) AddToCart(ctx context.Context, customerID, sessionToken, variantID string, quantity int) (*CartView, error) {
	if err := validateCartQuantity(quantity); err != nil {
		return nil, err
	}
	if variantID == "" {
		return nil, errors.New("commerce: variant_id is required")
	}
	c, err := s.Cart(ctx, customerID, sessionToken) // ensures the cart exists
	if err != nil {
		return nil, err
	}
	if err := s.carts.AddCartItem(ctx, c.ID, variantID, quantity); err != nil {
		return nil, fmt.Errorf("add cart item: %w", err)
	}
	return s.carts.CartView(ctx, c.ID)
}

// UpdateCartItem sets an absolute quantity (zero removes the line).
func (s *Service) UpdateCartItem(ctx context.Context, customerID, sessionToken, itemID string, quantity int) (*CartView, error) {
	if quantity < 0 || quantity > 10 {
		return nil, ErrCartQuantityInvalid
	}
	c, err := s.Cart(ctx, customerID, sessionToken)
	if err != nil {
		return nil, err
	}
	if err := s.carts.SetCartItemQuantity(ctx, c.ID, itemID, quantity); err != nil {
		return nil, err
	}
	return s.carts.CartView(ctx, c.ID)
}

// RemoveCartItem deletes a line from the cart.
func (s *Service) RemoveCartItem(ctx context.Context, customerID, sessionToken, itemID string) (*CartView, error) {
	c, err := s.Cart(ctx, customerID, sessionToken)
	if err != nil {
		return nil, err
	}
	if err := s.carts.DeleteCartItem(ctx, c.ID, itemID); err != nil {
		return nil, err
	}
	return s.carts.CartView(ctx, c.ID)
}

// validateCartQuantity enforces the contract bounds (1..10 per add).
func validateCartQuantity(q int) error {
	if q < 1 || q > 10 {
		return ErrCartQuantityInvalid
	}
	return nil
}
