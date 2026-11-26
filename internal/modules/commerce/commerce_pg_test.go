// commerce_pg_test.go covers the order lifecycle that refund_pg_test.go does not:
// creation with amount matching, the business order number, cancellation rules and the
// admin order list. Together the two files exercise the whole commerce DBStore.
//
// Skipped unless KH_TEST_PG_DSN is set.
package commerce

import (
	"context"

	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupCommerce(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewDBStore(pool)
	svc := NewService(store, NewDBEventEnqueuer(pool))
	svc.SetRefundStore(store)
	svc.SetAttemptStore(store)
	return pool, svc, store
}

// seedProductVariant inserts a product + variant and returns both ids.
func seedProductVariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sku string) (string, string) {
	t.Helper()
	var productID, variantID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ($1,$2,'DRAFT') RETURNING id`,
		"محصول "+sku, "p-"+sku).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO product_variants (product_id, sku, status) VALUES ($1,$2,'ACTIVE') RETURNING id`,
		productID, sku).Scan(&variantID); err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	return productID, variantID
}

// orderInput builds a two-line guest order whose totals satisfy chk_orders_totals.
func orderInput(productID, variantID, number string) OrderInput {
	addr, _ := json.Marshal(map[string]string{"city": "تهران", "province": "تهران"})
	return OrderInput{
		GuestPhone:  "09120000055",
		OrderNumber: number,
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID,
			SKUSnapshot: "SKU-COM-1", TitleSnapshot: "بالش",
			Quantity: 2, UnitPrice: 50000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr:  addr,
		ShippingTotal: 10000,
	}
}

// TestPG_OrderCreationAndNumberLookup: the totals are computed from the items, the
// business number is unique, and both the UUID and the number resolve.
func TestPG_OrderCreationAndNumberLookup(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-COM-1")

	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000301"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	detail, err := svc.GetOrderDetail(ctx, orderID)
	if err != nil {
		t.Fatalf("GetOrderDetail: %v", err)
	}
	// 2 × 50000 + 10000 shipping = 110000
	if detail.Order.Subtotal != 100000 || detail.Order.GrandTotal != 110000 {
		t.Fatalf("totals: subtotal=%d grand=%d, want 100000/110000",
			detail.Order.Subtotal, detail.Order.GrandTotal)
	}
	if detail.Order.Status != OrderPendingPayment {
		t.Fatalf("status = %s, want PENDING_PAYMENT", detail.Order.Status)
	}
	if len(detail.Items) != 1 || detail.Items[0].Quantity != 2 {
		t.Fatalf("items = %+v", detail.Items)
	}

	byNumber, err := svc.GetOrderByNumber(ctx, "KHA-ORD-2026-000301")
	if err != nil || byNumber == nil {
		t.Fatalf("GetOrderByNumber: %v", err)
	}
	if byNumber.ID != orderID {
		t.Fatalf("by-number id = %s, want %s", byNumber.ID, orderID)
	}

	// The business number is unique (§۴.۱).
	if _, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000301")); err == nil {
		t.Fatal("a duplicate order_number must be refused")
	}
}

// TestPG_CancelRules: only a PENDING_PAYMENT order can be cancelled; a paid order must
// go through the refund flow instead.
func TestPG_CancelRules(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-COM-2")

	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000302"))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if err := svc.CancelOrder(ctx, orderID, "مشتری منصرف شد"); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	o, err := store.GetOrder(ctx, orderID)
	if err != nil || o == nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if o.Status != OrderCancelled {
		t.Fatalf("status = %s, want CANCELLED", o.Status)
	}
	// Cancelling twice is refused (terminal state).
	if err := svc.CancelOrder(ctx, orderID, "دوباره"); err == nil {
		t.Fatal("cancelling a CANCELLED order must be refused")
	}

	// A paid order cannot be cancelled.
	paidID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000303"))
	if err != nil {
		t.Fatalf("CreateOrder paid: %v", err)
	}
	if _, err := svc.CreatePaymentIntent(ctx, paidID); err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, paidID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}
	if err := svc.CancelOrder(ctx, paidID, "بعد از پرداخت"); err == nil {
		t.Fatal("a PAID order must not be cancellable — use the refund flow")
	}

	// The outbox received the expected events.
	rows, err := pool.Query(ctx, `SELECT event_type FROM domain_events ORDER BY occurred_at`)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	seen := map[string]int{}
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[et]++
	}
	if seen["OrderCreated"] != 2 {
		t.Errorf("OrderCreated events = %d, want 2", seen["OrderCreated"])
	}
	if seen["OrderCancelled"] != 1 {
		t.Errorf("OrderCancelled events = %d, want 1", seen["OrderCancelled"])
	}
	if seen["OrderPaid"] != 1 {
		t.Errorf("OrderPaid events = %d, want 1", seen["OrderPaid"])
	}
}

// TestPG_AdminOrderList: the panel list filters by status and reports the right totals.
func TestPG_AdminOrderList(t *testing.T) {
	pool, svc, _ := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-COM-3")

	pendingID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000304"))
	if err != nil {
		t.Fatalf("CreateOrder pending: %v", err)
	}
	paidID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, "KHA-ORD-2026-000305"))
	if err != nil {
		t.Fatalf("CreateOrder paid: %v", err)
	}
	if _, err := svc.CreatePaymentIntent(ctx, paidID); err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, paidID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}

	list := NewOrderListStore(pool)
	all, total, err := list.List(ctx, "", 1, 20)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if total != 2 || len(all) != 2 {
		t.Fatalf("list all: total=%d len=%d", total, len(all))
	}
	// Newest first.
	if all[0].ID != paidID {
		t.Errorf("list order: first = %s, want the newest (%s)", all[0].ID, paidID)
	}

	paid, total, err := list.List(ctx, OrderPaid, 1, 20)
	if err != nil {
		t.Fatalf("List paid: %v", err)
	}
	if total != 1 || len(paid) != 1 || paid[0].ID != paidID {
		t.Fatalf("paid list: total=%d rows=%+v", total, paid)
	}
	if paid[0].PaidTotal != 110000 || paid[0].GrandTotal != 110000 {
		t.Fatalf("paid totals = %+v", paid[0])
	}
	if paid[0].CreatedAt == "" {
		t.Error("created_at must be rendered")
	}

	pending, total, err := list.List(ctx, OrderPendingPayment, 1, 20)
	if err != nil {
		t.Fatalf("List pending: %v", err)
	}
	if total != 1 || pending[0].ID != pendingID {
		t.Fatalf("pending list: total=%d rows=%+v", total, pending)
	}
}

// TestPG_OrderTotalsConstraint: chk_orders_totals rejects an inconsistent order even if
// the service is bypassed.
func TestPG_OrderTotalsConstraint(t *testing.T) {
	pool, _, _ := setupCommerce(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO orders (order_number, guest_phone, status, subtotal, grand_total,
		                     shipping_address_snapshot)
		 VALUES ('KHA-ORD-2026-000399','09120000001','PENDING_PAYMENT',1000,9999,'{}'::jsonb)`)
	if err == nil {
		t.Fatal("chk_orders_totals must reject grand_total != subtotal - discount + shipping + tax")
	}
	_ = errors.Is(err, ErrOrderNotFound)
	_ = time.Now
}

// TestPG_OrderNumberIsMintedByTheServer: the contract makes the business order number
// server-owned (§۴.۱), so a checkout that omits it must still succeed and produce a
// KHA-ORD-YYYY-NNNNNN number from the sequence.
func TestPG_OrderNumberIsMintedByTheServer(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-COM-9")

	in := orderInput(productID, variantID, "")
	id, err := svc.CreateOrder(ctx, in)
	if err != nil {
		t.Fatalf("CreateOrder without a number: %v", err)
	}
	o, err := store.GetOrder(ctx, id)
	if err != nil || o == nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if o.OrderNumber == "" {
		t.Fatal("the server must mint an order number")
	}
	if len(o.OrderNumber) != len("KHA-ORD-2026-000001") || o.OrderNumber[:8] != "KHA-ORD-" {
		t.Fatalf("order_number = %q, want KHA-ORD-YYYY-NNNNNN", o.OrderNumber)
	}

	// Two checkouts must not collide on the unique number.
	second, err := svc.CreateOrder(ctx, in)
	if err != nil {
		t.Fatalf("second CreateOrder: %v", err)
	}
	o2, _ := store.GetOrder(ctx, second)
	if o2.OrderNumber == o.OrderNumber {
		t.Fatalf("both orders got %q — the sequence must advance", o.OrderNumber)
	}
}

// TestPG_StatusUpdatesRejectUnknownOrder: a zero-row update must be an error, not a
// silent success — otherwise a caller believes a payment or cancellation happened.
func TestPG_StatusUpdatesRejectUnknownOrder(t *testing.T) {
	_, _, store := setupCommerce(t)
	ctx := context.Background()
	missing := "00000000-0000-0000-0000-000000000000"

	if err := store.UpdateOrderStatus(ctx, missing, OrderCancelled); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("UpdateOrderStatus on a missing order: %v, want ErrOrderNotFound", err)
	}
	if err := store.UpdateOrderPaid(ctx, missing, 1000, OrderPaid); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("UpdateOrderPaid on a missing order: %v, want ErrOrderNotFound", err)
	}
}

// TestPG_PaymentIntentAndAttemptReads covers the money-path store methods that had no
// test against the real schema: the active intent lookup, the attempt aggregates that
// reconciliation relies on, and the refunded-total write.
func TestPG_PaymentIntentAndAttemptReads(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-PAY-1")

	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, ""))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	intentID := uuid.NewString()
	if _, err := store.CreatePaymentIntent(ctx, PaymentIntent{
		ID: intentID, OrderID: orderID, Amount: 900000, Currency: "IRR",
		Status: IntentRequiresPayment, ExpiresAt: ptrTime(time.Now().Add(time.Hour)),
	}); err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}

	active, err := store.GetActivePaymentIntent(ctx, orderID)
	if err != nil || active == nil {
		t.Fatalf("GetActivePaymentIntent: %v", err)
	}
	if active.ID != intentID || active.Amount != 900000 {
		t.Fatalf("active intent = %+v, want id %s amount 900000", active, intentID)
	}

	attemptID, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "MOCK", Amount: 900000,
		Status: "SUCCEEDED", IdempotencyKey: "idem-" + uuid.NewString(),
		CompletedAt: ptrTime(time.Now()),
	})
	if err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}

	gotAttempt, err := store.SuccessfulAttemptID(ctx, orderID)
	if err != nil {
		t.Fatalf("SuccessfulAttemptID: %v", err)
	}
	if gotAttempt != attemptID {
		t.Fatalf("SuccessfulAttemptID = %q, want %q", gotAttempt, attemptID)
	}

	sum, err := store.SumSucceededAttempts(ctx, orderID)
	if err != nil {
		t.Fatalf("SumSucceededAttempts: %v", err)
	}
	if sum != 900000 {
		t.Fatalf("SumSucceededAttempts = %d, want 900000", sum)
	}

	if err := store.UpdateOrderRefunded(ctx, orderID, 200000, OrderPaid); err != nil {
		t.Fatalf("UpdateOrderRefunded: %v", err)
	}
	o, err := store.GetOrder(ctx, orderID)
	if err != nil || o == nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if o.RefundedTotal != 200000 {
		t.Fatalf("refunded_total = %d, want 200000", o.RefundedTotal)
	}

	// A zero-row update must not be reported as success.
	if err := store.UpdateOrderRefunded(ctx, "00000000-0000-0000-0000-000000000000", 1, OrderPaid); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("UpdateOrderRefunded on a missing order: %v, want ErrOrderNotFound", err)
	}
	if err := store.UpdatePaymentIntentStatus(ctx, "00000000-0000-0000-0000-000000000000", "CANCELLED"); !errors.Is(err, ErrPaymentIntentNotFound) {
		t.Fatalf("UpdatePaymentIntentStatus on a missing intent: %v, want ErrPaymentIntentNotFound", err)
	}
}

// TestPG_CartAndOrderItemReads covers the cart lifecycle and the order-item read that
// the storefront and the panel both depend on.
func TestPG_CartAndOrderItemReads(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-CART-1")

	var cartID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO carts (session_token, status, expires_at) VALUES ('sess-cart-1','ACTIVE', now() + interval '1 hour') RETURNING id`).Scan(&cartID); err != nil {
		t.Fatalf("seed cart: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO cart_items (cart_id, variant_id, quantity) VALUES ($1,$2,2)`, cartID, variantID); err != nil {
		t.Fatalf("seed cart item: %v", err)
	}

	cart, items, err := store.GetCart(ctx, cartID)
	if err != nil || cart == nil {
		t.Fatalf("GetCart: %v", err)
	}
	if len(items) != 1 || items[0].Quantity != 2 {
		t.Fatalf("cart items = %+v, want one line of 2", items)
	}

	if err := store.UpdateCartStatus(ctx, cartID, CartAbandoned); err != nil {
		t.Fatalf("UpdateCartStatus: %v", err)
	}
	cart, _, err = store.GetCart(ctx, cartID)
	if err != nil || cart == nil {
		t.Fatalf("GetCart after update: %v", err)
	}
	if cart.Status != CartAbandoned {
		t.Fatalf("cart status = %q, want %q", cart.Status, CartAbandoned)
	}

	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, ""))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	orderItems, err := store.ListOrderItems(ctx, orderID)
	if err != nil {
		t.Fatalf("ListOrderItems: %v", err)
	}
	if len(orderItems) != 1 || orderItems[0].VariantID != variantID {
		t.Fatalf("order items = %+v, want the one line", orderItems)
	}
}

// TestPG_CartLifecycle covers the server-side cart store: guest cart creation (the
// partial unique index needs the same predicate in ON CONFLICT), item upsert, quantity
// set, delete, merge and the priced view.
func TestPG_CartLifecycle(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-CART-PG")
	_ = productID

	// Guest cart is created once and returned again for the same token.
	c1, err := store.GetOrCreateCart(ctx, "", "guest-token-1")
	if err != nil {
		t.Fatalf("GetOrCreateCart: %v", err)
	}
	c2, err := store.GetOrCreateCart(ctx, "", "guest-token-1")
	if err != nil {
		t.Fatalf("GetOrCreateCart (again): %v", err)
	}
	if c1.ID != c2.ID {
		t.Fatalf("same token produced two carts: %s vs %s", c1.ID, c2.ID)
	}

	// Adding the same variant twice must sum, not duplicate (unique cart_id+variant_id).
	if err := store.AddCartItem(ctx, c1.ID, variantID, 2); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	if err := store.AddCartItem(ctx, c1.ID, variantID, 3); err != nil {
		t.Fatalf("AddCartItem (again): %v", err)
	}
	view, err := store.CartView(ctx, c1.ID)
	if err != nil {
		t.Fatalf("CartView: %v", err)
	}
	if len(view.Items) != 1 || view.Items[0].Quantity != 5 {
		t.Fatalf("items = %+v, want one line of 5", view.Items)
	}
	// No approved price yet: the line must be reported unavailable, not with a stale price.
	if view.Items[0].Availability != "UNAVAILABLE" || view.Items[0].UnitPrice != 0 {
		t.Fatalf("unpriced line = %+v, want UNAVAILABLE with zero price", view.Items[0])
	}

	// An approved price makes the line priced and the subtotal add up.
	var adminID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name) VALUES ($1,'تأییدکننده') RETURNING id`,
		"price-approver-"+uuid.NewString()[:8]+"@khabavar.ir").Scan(&adminID); err != nil {
		t.Fatalf("seed approver: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO product_price_records (variant_id, selling_price, currency, valid_from, approved_by)
		 VALUES ($1, 900000, 'IRR', now() - interval '1 minute', $2)`, variantID, adminID); err != nil {
		t.Fatalf("seed price: %v", err)
	}
	view, err = store.CartView(ctx, c1.ID)
	if err != nil {
		t.Fatalf("CartView (priced): %v", err)
	}
	if view.Items[0].UnitPrice != 900000 || view.Subtotal != 4500000 {
		t.Fatalf("priced line = %+v subtotal=%d, want 900000 x5", view.Items[0], view.Subtotal)
	}

	// Absolute quantity, then zero removes the line.
	itemID := view.Items[0].ItemID
	if err := store.SetCartItemQuantity(ctx, c1.ID, itemID, 1); err != nil {
		t.Fatalf("SetCartItemQuantity: %v", err)
	}
	view, _ = store.CartView(ctx, c1.ID)
	if view.Items[0].Quantity != 1 {
		t.Fatalf("quantity = %d, want 1", view.Items[0].Quantity)
	}
	if err := store.SetCartItemQuantity(ctx, c1.ID, itemID, 0); err != nil {
		t.Fatalf("SetCartItemQuantity(0): %v", err)
	}
	view, _ = store.CartView(ctx, c1.ID)
	if len(view.Items) != 0 {
		t.Fatalf("items = %+v, want none after quantity 0", view.Items)
	}

	// Deleting a line from another cart must not work (scoped by cart_id).
	if err := store.AddCartItem(ctx, c1.ID, variantID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	view, _ = store.CartView(ctx, c1.ID)
	other, _ := store.GetOrCreateCart(ctx, "", "guest-token-2")
	if err := store.DeleteCartItem(ctx, other.ID, view.Items[0].ItemID); !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("cross-cart delete = %v, want ErrCartItemNotFound", err)
	}

	// Merge folds the guest cart into the customer cart and retires the guest one.
	custCart, err := store.GetOrCreateCart(ctx, seedCustomerRow(t, ctx, pool), "")
	if err != nil {
		t.Fatalf("customer cart: %v", err)
	}
	if err := store.MergeCarts(ctx, c1.ID, custCart.ID); err != nil {
		t.Fatalf("MergeCarts: %v", err)
	}
	merged, _ := store.CartView(ctx, custCart.ID)
	if len(merged.Items) != 1 || merged.Items[0].Quantity != 1 {
		t.Fatalf("merged items = %+v, want the guest line", merged.Items)
	}
	guestAfter, _ := store.CartView(ctx, c1.ID)
	if len(guestAfter.Items) != 0 {
		t.Fatalf("guest cart still has items after merge: %+v", guestAfter.Items)
	}
	_ = svc
}

// seedCustomerRow inserts a customer and returns its id.
func seedCustomerRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO customers (phone) VALUES ($1) RETURNING id`,
		"+9891200"+uuid.NewString()[:6]).Scan(&id); err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	return id
}

// TestPG_IdempotencyClaim covers ADR-ARCH-008 at the store level: the first claim owns
// the key, a second claim sees it, a completed key replays, and a different body is
// detectable from the stored hash.
func TestPG_IdempotencyClaim(t *testing.T) {
	_, _, store := setupCommerce(t)
	ctx := context.Background()
	key := "idem-" + uuid.NewString()
	scope := "POST /api/v1/checkout"
	hash := HashRequest([]byte(`{"a":1}`))

	owned, _, err := store.Claim(ctx, key, scope, hash, time.Hour)
	if err != nil || !owned {
		t.Fatalf("first claim: owned=%v err=%v", owned, err)
	}

	// Same key again: not owned, no response yet (in flight).
	owned, stored, err := store.Claim(ctx, key, scope, hash, time.Hour)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if owned {
		t.Fatal("the second claim must not own the key")
	}
	if stored == nil || stored.StatusCode != 0 {
		t.Fatalf("stored = %+v, want an in-flight marker", stored)
	}

	// Complete it, then a retry must replay the stored body.
	if err := store.Complete(ctx, key, 201, []byte(`{"order":"KHA-ORD-1"}`)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	_, stored, err = store.Claim(ctx, key, scope, hash, time.Hour)
	if err != nil {
		t.Fatalf("third claim: %v", err)
	}
	// The body is stored as JSONB, so compare it semantically: Postgres normalises
	// whitespace and key order on the way in.
	var replayed map[string]string
	if err := json.Unmarshal(stored.Body, &replayed); err != nil {
		t.Fatalf("replay body is not JSON: %v (%s)", err, stored.Body)
	}
	if stored.StatusCode != 201 || replayed["order"] != "KHA-ORD-1" {
		t.Fatalf("replay = %d %s", stored.StatusCode, stored.Body)
	}

	// The stored hash lets the caller spot a different body under the same key.
	if stored.RequestHash != hash {
		t.Fatalf("hash = %q, want %q", stored.RequestHash, hash)
	}
	if HashRequest([]byte(`{"a":2}`)) == hash {
		t.Fatal("different bodies must hash differently")
	}

	// Completing an unknown key must fail rather than silently succeed.
	if err := store.Complete(ctx, "idem-does-not-exist", 200, nil); err == nil {
		t.Fatal("Complete on a missing key must error")
	}
}

// TestHashRequestIgnoresFormatting: key order and whitespace must not look like a
// different request, or a harmless retry would be rejected as a conflict.
func TestHashRequestIgnoresFormatting(t *testing.T) {
	a := HashRequest([]byte(`{"a":1,"b":2}`))
	b := HashRequest([]byte("{\n  \"b\": 2,\n  \"a\": 1\n}"))
	if a != b {
		t.Fatalf("canonical hash differs: %s vs %s", a, b)
	}
}

// TestPG_MergeGuestCartAndCustomerOrders covers the two customer-facing reads/writes:
// merging a guest cart into the signed-in one, and listing a customer's own orders.
func TestPG_MergeGuestCartAndCustomerOrders(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-MERGE-1")
	customerID := seedCustomerRow(t, ctx, pool)

	// Guest cart with one line.
	guest, err := store.GetOrCreateCart(ctx, "", "merge-guest-1")
	if err != nil {
		t.Fatalf("guest cart: %v", err)
	}
	if err := store.AddCartItem(ctx, guest.ID, variantID, 2); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	// FindCartBySession must not create anything for an unknown token.
	if c, err := store.FindCartBySession(ctx, "no-such-token"); err != nil || c != nil {
		t.Fatalf("FindCartBySession(unknown) = %v, %v; want nil, nil", c, err)
	}

	svc.SetCartStore(store)
	merged, err := svc.MergeGuestCart(ctx, customerID, "merge-guest-1")
	if err != nil {
		t.Fatalf("MergeGuestCart: %v", err)
	}
	if len(merged.Items) != 1 || merged.Items[0].Quantity != 2 {
		t.Fatalf("merged view = %+v, want the guest line", merged.Items)
	}
	// The guest cart is retired, so a second merge adds nothing.
	again, err := svc.MergeGuestCart(ctx, customerID, "merge-guest-1")
	if err != nil {
		t.Fatalf("MergeGuestCart (again): %v", err)
	}
	if len(again.Items) != 1 || again.Items[0].Quantity != 2 {
		t.Fatalf("second merge duplicated the line: %+v", again.Items)
	}

	// A customer order must appear in their own list and not in another's.
	if _, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID,
		Currency:   "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-MERGE-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	}); err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	list := NewOrderListStore(pool)
	orders, total, err := list.ListByCustomer(ctx, customerID, 1, 10)
	if err != nil {
		t.Fatalf("ListByCustomer: %v", err)
	}
	if total != 1 || len(orders) != 1 {
		t.Fatalf("orders = %d total = %d, want 1", len(orders), total)
	}
	other, totalOther, err := list.ListByCustomer(ctx, seedCustomerRow(t, ctx, pool), 1, 10)
	if err != nil {
		t.Fatalf("ListByCustomer (other): %v", err)
	}
	if len(other) != 0 || totalOther != 0 {
		t.Fatalf("another customer sees %d orders, want 0", len(other))
	}
}

// TestPG_FindIntentByProviderReference: the webhook resolves the intent from the
// gateway's transaction id, so this lookup is the only bridge between them.
func TestPG_FindIntentByProviderReference(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-WH-1")

	orderID, err := svc.CreateOrder(ctx, orderInput(productID, variantID, ""))
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	intentID, err := svc.CreatePaymentIntent(ctx, orderID)
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	ref := "REF-" + uuid.NewString()
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: ref,
		Amount: 1000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + uuid.NewString(),
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}

	gotIntent, gotOrder, err := store.FindIntentByProviderReference(ctx, ref)
	if err != nil {
		t.Fatalf("FindIntentByProviderReference: %v", err)
	}
	if gotIntent != intentID || gotOrder != orderID {
		t.Fatalf("intent/order = %q/%q, want %q/%q", gotIntent, gotOrder, intentID, orderID)
	}

	// An unknown reference must be empty, not an error: the webhook answers 202 for it.
	missingIntent, missingOrder, err := store.FindIntentByProviderReference(ctx, "REF-does-not-exist")
	if err != nil {
		t.Fatalf("unknown reference errored: %v", err)
	}
	if missingIntent != "" || missingOrder != "" {
		t.Fatalf("unknown reference = %q/%q, want empty", missingIntent, missingOrder)
	}
}

// TestPG_ReturnRequest covers the customer's return claim: ownership scoping, the status
// gate, the one-open-request rule, and the audit write.
func TestPG_ReturnRequest(t *testing.T) {
	pool, svc, store := setupCommerce(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-RET-1")
	customerID := seedCustomerRow(t, ctx, pool)

	orderID, err := svc.CreateOrder(ctx, OrderInput{
		CustomerID: customerID, Currency: "IRR",
		Items: []OrderItemInput{{
			ProductID: productID, VariantID: variantID, SKUSnapshot: "SKU-RET-1",
			TitleSnapshot: "x", Quantity: 1, UnitPrice: 1000, SupplyType: "OWNED_INVENTORY",
		}},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	// A return needs the money taken first.
	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "خرابی"); !errors.Is(err, ErrReturnNotAllowed) {
		t.Fatalf("return on PENDING_PAYMENT = %v, want ErrReturnNotAllowed", err)
	}
	// Pay it: an intent plus a SUCCEEDED attempt is what ConfirmPaymentForOrder requires.
	intentID, err := svc.CreatePaymentIntent(ctx, orderID)
	if err != nil {
		t.Fatalf("CreatePaymentIntent: %v", err)
	}
	if _, err := store.CreateAttempt(ctx, PaymentAttempt{
		PaymentIntentID: intentID, Provider: "mock", ProviderReference: "REF-RET-" + uuid.NewString(),
		Amount: 1000, Status: "SUCCEEDED", IdempotencyKey: "idem-" + uuid.NewString(),
		CompletedAt: ptrTime(time.Now()),
	}); err != nil {
		t.Fatalf("CreateAttempt: %v", err)
	}
	if err := svc.ConfirmPaymentForOrder(ctx, orderID); err != nil {
		t.Fatalf("ConfirmPaymentForOrder: %v", err)
	}

	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, ""); !errors.Is(err, ErrReturnReasonRequired) {
		t.Fatalf("empty reason = %v, want ErrReturnReasonRequired", err)
	}
	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 0, "خرابی"); !errors.Is(err, ErrReturnQuantityInvalid) {
		t.Fatalf("zero quantity = %v, want ErrReturnQuantityInvalid", err)
	}
	// Another customer must not be able to file against this order.
	if _, err := svc.CreateReturnRequest(ctx, orderID, seedCustomerRow(t, ctx, pool), 1, "خرابی"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("foreign customer = %v, want ErrOrderNotFound", err)
	}

	rr, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "خرابی")
	if err != nil {
		t.Fatalf("CreateReturnRequest: %v", err)
	}
	if rr.Status != ReturnRequested || rr.Quantity != 1 {
		t.Fatalf("request = %+v", rr)
	}
	// The partial unique index must stop a second open claim.
	if _, err := svc.CreateReturnRequest(ctx, orderID, customerID, 1, "دوباره"); !errors.Is(err, ErrReturnAlreadyOpen) {
		t.Fatalf("second open request = %v, want ErrReturnAlreadyOpen", err)
	}

	list, err := store.ListReturnRequestsByOrder(ctx, orderID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
}
