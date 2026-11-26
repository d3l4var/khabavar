package commerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// CheckoutInput is the POST /checkout body (contract CheckoutInput).
type CheckoutInput struct {
	CartID          string          `json:"cart_id"`
	Items           []CheckoutItem  `json:"items"`
	ShippingAddress json.RawMessage `json:"shipping_address"`
	GuestPhone      string          `json:"guest_phone"`
	PaymentProvider string          `json:"payment_provider"`
	DiscountTotal   int64           `json:"discount_total"`
	ShippingTotal   int64           `json:"shipping_total"`
	TaxTotal        int64           `json:"tax_total"`
}

// CheckoutItem is one requested line when the client sends items instead of a cart id.
type CheckoutItem struct {
	VariantID string `json:"variant_id"`
	Quantity  int    `json:"quantity"`
}

// CheckoutPayment is the payment half of the result.
type CheckoutPayment struct {
	IntentID    string `json:"intent_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

// CheckoutResult is the POST /checkout response (contract CheckoutResult).
type CheckoutResult struct {
	Order   *Order          `json:"order"`
	Payment CheckoutPayment `json:"payment"`
}

// UnavailableLine reports a line that changed between the cart and the checkout.
type UnavailableLine struct {
	VariantID string `json:"variant_id"`
	Reason    string `json:"reason"`
}

// ErrCheckoutUnavailable carries the lines that cannot be sold right now (422).
type ErrCheckoutUnavailable struct{ Lines []UnavailableLine }

func (e *ErrCheckoutUnavailable) Error() string {
	return fmt.Sprintf("commerce: %d line(s) unavailable at checkout", len(e.Lines))
}

// Checkout turns a cart into an order plus a payment intent (DB §14.1):
// validate the cart and today's prices → create the order with snapshots → mark the
// cart CONVERTED → issue the payment intent. Stock is reserved after payment is
// confirmed (PRD O-4), not here.
func (s *Service) Checkout(ctx context.Context, customerID, sessionToken string, in CheckoutInput) (*CheckoutResult, error) {
	if len(in.ShippingAddress) == 0 {
		return nil, ErrShippingAddressReq
	}
	if customerID == "" && in.GuestPhone == "" {
		return nil, ErrGuestPhoneRequired
	}

	// A client may send items directly; they are folded into the cart first so there is
	// exactly one pricing path.
	if in.CartID == "" && len(in.Items) > 0 {
		for _, it := range in.Items {
			if _, err := s.AddToCart(ctx, customerID, sessionToken, it.VariantID, it.Quantity); err != nil {
				return nil, err
			}
		}
	}

	cart, err := s.Cart(ctx, customerID, sessionToken)
	if err != nil {
		return nil, err
	}
	if in.CartID != "" {
		cart, err = s.carts.CartView(ctx, in.CartID)
		if err != nil {
			return nil, err
		}
	}
	if len(cart.Items) == 0 {
		return nil, ErrCartEmpty
	}

	// Every line must have an approved price valid today; a stale or missing price is a
	// 422 with the affected lines, never a silent zero.
	unavailable := make([]UnavailableLine, 0)
	for _, l := range cart.Items {
		if !l.PriceIsApproved {
			unavailable = append(unavailable, UnavailableLine{VariantID: l.VariantID, Reason: "price_unavailable"})
		}
	}
	if len(unavailable) > 0 {
		return nil, &ErrCheckoutUnavailable{Lines: unavailable}
	}

	items := make([]OrderItemInput, 0, len(cart.Items))
	for _, l := range cart.Items {
		items = append(items, OrderItemInput{
			ProductID:     l.ProductID,
			VariantID:     l.VariantID,
			SKUSnapshot:   l.SKU,
			TitleSnapshot: l.ProductTitle,
			Quantity:      l.Quantity,
			UnitPrice:     l.UnitPrice,
			SupplyType:    "OWNED_INVENTORY",
		})
	}

	orderID, err := s.CreateOrder(ctx, OrderInput{
		CustomerID:    customerID,
		GuestPhone:    in.GuestPhone,
		Currency:      cart.Currency,
		Items:         items,
		ShippingAddr:  in.ShippingAddress,
		DiscountTotal: in.DiscountTotal,
		ShippingTotal: in.ShippingTotal,
		TaxTotal:      in.TaxTotal,
	})
	if err != nil {
		return nil, err
	}

	// The cart is retired only after the order exists, so a failed order leaves the cart
	// intact for a retry.
	if err := s.ConvertCart(ctx, cart.ID); err != nil {
		return nil, fmt.Errorf("convert cart: %w", err)
	}

	intentID, err := s.CreatePaymentIntent(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}
	detail, err := s.GetOrderDetail(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order := &detail.Order

	return &CheckoutResult{
		Order: order,
		Payment: CheckoutPayment{
			IntentID: intentID,
			Amount:   order.GrandTotal,
			Currency: order.Currency,
		},
	}, nil
}
