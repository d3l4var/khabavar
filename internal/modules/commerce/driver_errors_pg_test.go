package commerce

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
)

// TestPG_ServiceSurfacesDriverErrors walks the service entry points against a closed pool.
// Almost every uncovered block in this package is a `return err` arm after a store call;
// a dead pool reaches all of them at once and proves the errors are returned, not swallowed
// or turned into a panic.
func TestPG_ServiceSurfacesDriverErrors(t *testing.T) {
	pool, _ := testpg.Setup(t)
	// Closed on purpose: the schema cleanup is skipped because a closed pool cannot drop it.
	pool.Close()
	svc := NewService(NewDBStore(pool), nil)
	ctx := context.Background()
	now := time.Now()

	calls := map[string]func() error{
		"get order": func() error {
			_, err := svc.GetOrderDetail(ctx, "00000000-0000-0000-0000-000000000001")
			return err
		},
		"get by number": func() error {
			_, err := svc.GetOrderByNumber(ctx, "KHA-ORD-2026-000001")
			return err
		},
		"cart": func() error {
			_, err := svc.Cart(ctx, "00000000-0000-0000-0000-000000000001", "")
			return err
		},
		"add to cart": func() error {
			_, err := svc.AddToCart(ctx, "00000000-0000-0000-0000-000000000001", "",
				"00000000-0000-0000-0000-000000000002", 1)
			return err
		},
		"checkout": func() error {
			_, err := svc.Checkout(ctx, "00000000-0000-0000-0000-000000000001", "", CheckoutInput{
				ShippingAddress: json.RawMessage(`{"city":"تهران"}`),
				PaymentProvider: "mock",
			})
			return err
		},
		"create intent": func() error {
			_, err := svc.CreatePaymentIntent(ctx, "00000000-0000-0000-0000-000000000001")
			return err
		},
		"confirm": func() error {
			return svc.ConfirmPaymentForOrder(ctx, "00000000-0000-0000-0000-000000000001")
		},
		"find intent": func() error {
			_, _, err := svc.FindIntentByProviderReference(ctx, "ref-1")
			return err
		},
		"cancel": func() error {
			return svc.CancelOrder(ctx, "00000000-0000-0000-0000-000000000001", "reason")
		},
		"admin list": func() error {
			_, _, err := NewOrderListStore(pool).List(ctx, "", "", 1, 20)
			return err
		},
		"refunds": func() error {
			_, err := svc.ListRefunds(ctx, "00000000-0000-0000-0000-000000000001")
			return err
		},
		"customer orders": func() error {
			_, _, err := NewOrderListStore(pool).ListByCustomer(ctx, "00000000-0000-0000-0000-000000000001", 1, 20)
			return err
		},
		"merge cart": func() error {
			_, err := svc.MergeGuestCart(ctx, "00000000-0000-0000-0000-000000000001", "guest-token")
			return err
		},
		"return request": func() error {
			_, err := svc.CreateReturnRequest(ctx,
				"00000000-0000-0000-0000-000000000001",
				"00000000-0000-0000-0000-000000000002", 1, "خرابی")
			return err
		},
		"next order number": func() error {
			_, err := NewDBStore(pool).NextOrderNumber(ctx, now.Year())
			return err
		},
	}
	for name, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("%s against a closed pool returned no error", name)
		}
	}
}

var _ = pgxpool.Pool{}

// TestPG_StoreSurfacesDriverErrors calls the DBStore methods directly against a closed pool.
// Every store method has a `return err` arm that a happy-path test never reaches; this
// reaches them all and proves none of them swallows the failure.
func TestPG_StoreSurfacesDriverErrors(t *testing.T) {
	pool, _ := testpg.Setup(t)
	pool.Close()
	store := NewDBStore(pool)
	ctx := context.Background()
	now := time.Now()
	id := "00000000-0000-0000-0000-000000000001"

	calls := map[string]func() error{
		"get cart": func() error { _, _, err := store.GetCart(ctx, id); return err },
		"update cart status": func() error {
			return store.UpdateCartStatus(ctx, id, "ACTIVE")
		},
		"create order": func() error {
			_, err := store.CreateOrder(ctx, Order{OrderNumber: "N", Status: OrderPendingPayment}, nil)
			return err
		},
		"get order":     func() error { _, err := store.GetOrder(ctx, id); return err },
		"get by number": func() error { _, err := store.GetOrderByNumber(ctx, "N"); return err },
		"list items":    func() error { _, err := store.ListOrderItems(ctx, id); return err },
		"update status": func() error { return store.UpdateOrderStatus(ctx, id, OrderPaid) },
		"update paid":   func() error { return store.UpdateOrderPaid(ctx, id, 1, OrderPaid) },
		"create intent": func() error {
			_, err := store.CreatePaymentIntent(ctx, PaymentIntent{OrderID: id})
			return err
		},
		"active intent": func() error { _, err := store.GetActivePaymentIntent(ctx, id); return err },
		"intent status": func() error {
			return store.UpdatePaymentIntentStatus(ctx, id, IntentSucceeded)
		},
		"next number": func() error { _, err := store.NextOrderNumber(ctx, now.Year()); return err },
		"create refund": func() error {
			_, err := store.CreateRefund(ctx, Refund{OrderID: id})
			return err
		},
		"list refunds":  func() error { _, err := store.ListRefunds(ctx, id); return err },
		"successful":    func() error { _, err := store.SuccessfulAttemptID(ctx, id); return err },
		"update refund": func() error { return store.UpdateOrderRefunded(ctx, id, 1, OrderClosed) },
		"create attempt": func() error {
			_, err := store.CreateAttempt(ctx, PaymentAttempt{PaymentIntentID: id})
			return err
		},
		"sum attempts": func() error { _, err := store.SumSucceededAttempts(ctx, id); return err },
		"get or create cart": func() error {
			_, err := store.GetOrCreateCart(ctx, id, "")
			return err
		},
		"add item":    func() error { return store.AddCartItem(ctx, id, id, 1) },
		"set qty":     func() error { return store.SetCartItemQuantity(ctx, id, id, 2) },
		"delete item": func() error { return store.DeleteCartItem(ctx, id, id) },
	}
	for name, fn := range calls {
		if err := fn(); err == nil {
			t.Fatalf("%s against a closed pool returned no error", name)
		}
	}
}
