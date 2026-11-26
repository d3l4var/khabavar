package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// recordAuditor captures the audit calls a service makes.
type recordAuditor struct {
	actions []string
	actors  []string
	err     error
}

func (a *recordAuditor) Record(_ context.Context, actorType, actorID, action, entityType, entityID string, _, _ any, _ string) error {
	a.actions = append(a.actions, action)
	a.actors = append(a.actors, actorID)
	return a.err
}

// TestPG_OrderStatusAudit: an order status change is one of the seven audited categories
// (IAM §۷), so both the paid and the cancelled transitions must leave a trail — and an
// audit failure must not be reported as success.
func TestPG_OrderStatusAudit(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-OAUD-1")
	customerID := seedCustomerRow(t, ctx, pool)

	aud := &recordAuditor{}
	svc.SetAuditor(aud)

	newOrder := func() string {
		id, err := svc.CreateOrder(ctx, OrderInput{
			CustomerID: customerID, Currency: "IRR",
			Items: []OrderItemInput{{
				ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-OAUD-1",
				TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
			}},
			ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
		})
		if err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
		return id
	}

	// Cancelled.
	cancelID := newOrder()
	if err := svc.CancelOrder(ctx, cancelID, "انصراف مشتری"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	// Paid.
	paidID := newOrder()
	intentID, err := svc.CreatePaymentIntent(ctx, paidID)
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: "REF-OAUD-" + paidID[:8],
		Amount: 1000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + paidID[:8],
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, paidID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}

	got := map[string]bool{}
	for _, a := range aud.actions {
		got[a] = true
	}
	if !got["order.cancelled"] || !got["order.paid"] {
		t.Fatalf("audited actions = %v, want order.cancelled and order.paid", aud.actions)
	}

	// An audit failure is surfaced, not swallowed.
	aud.err = errors.New("audit down")
	thirdID := newOrder()
	if err := svc.CancelOrder(ctx, thirdID, "دوباره"); err == nil {
		t.Fatal("an audit failure must be returned from CancelOrder")
	}
}
