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
