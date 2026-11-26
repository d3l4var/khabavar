// refund_pg_test.go exercises the refund path against the real schema (migration
// 000006). It exists because the refund store was written against an imagined table
// (currency, processed_at) and only failed at runtime with
// `column "currency" does not exist`. Skipped unless KH_TEST_PG_DSN is set.
package commerce

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/modules/shared"
	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// seedPaidOrder inserts the minimal graph a refund needs: customer → product →
// variant → order → payment intent → SUCCEEDED attempt. It returns the order id and
// the attempt id.
func seedPaidOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool, paid int64) (orderID, attemptID string) {
	t.Helper()
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\nSQL: %s", err, sql)
		}
	}
	var productID, variantID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('تست','test-product','DRAFT') RETURNING id`).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO product_variants (product_id, sku, status) VALUES ($1,'SKU-TEST','ACTIVE') RETURNING id`,
		productID).Scan(&variantID); err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO orders (order_number, guest_phone, status, subtotal, grand_total, paid_total,
		                     shipping_address_snapshot)
		 VALUES ('KHA-ORD-2026-000001','09120000009','PAID',$1,$1,$1,'{"city":"Tehran"}'::jsonb)
		 RETURNING id`, paid).Scan(&orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	mustExec(`INSERT INTO order_items (order_id, product_id, variant_id, sku_snapshot, title_snapshot,
	                                   quantity, unit_price, line_total, supply_type_snapshot)
	          VALUES ($1,$2,$3,'SKU-TEST','تست',1,$4,$4,'OWNED_INVENTORY')`, orderID, productID, variantID, paid)
	var intentID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO payment_intents (order_id, amount, status) VALUES ($1,$2,'SUCCEEDED') RETURNING id`,
		orderID, paid).Scan(&intentID); err != nil {
		t.Fatalf("seed intent: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO payment_attempts (payment_intent_id, provider, amount, status, completed_at)
		 VALUES ($1,'mock',$2,'SUCCEEDED', now()) RETURNING id`, intentID, paid).Scan(&attemptID); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}
	return orderID, attemptID
}

func TestPG_CreateRefundWritesSchemaColumns(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	orderID, attemptID := seedPaidOrder(t, ctx, pool, 100000)

	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)
	svc.SetNow(time.Now)

	adminID := seedAdmin(t, ctx, pool)
	ctx = shared.WithActorID(ctx, adminID)

	refundID, err := svc.CreateRefund(ctx, orderID, 30000, "کالای معیوب")
	if err != nil {
		t.Fatalf("CreateRefund: %v", err)
	}
	if refundID == "" {
		t.Fatal("empty refund id")
	}

	var (
		gotAttempt, gotStatus, gotApproved string
		gotAmount                          int64
		completedAt                        *time.Time
	)
	if err := pool.QueryRow(ctx,
		`SELECT payment_attempt_id, amount, status, coalesce(approved_by::text,''), completed_at
		   FROM refunds WHERE id = $1`, refundID).
		Scan(&gotAttempt, &gotAmount, &gotStatus, &gotApproved, &completedAt); err != nil {
		t.Fatalf("read refund: %v", err)
	}
	if gotAttempt != attemptID {
		t.Errorf("payment_attempt_id = %s, want %s (a refund must reference the payment it reverses)", gotAttempt, attemptID)
	}
	if gotAmount != 30000 {
		t.Errorf("amount = %d, want 30000", gotAmount)
	}
	if gotStatus != RefundCompleted {
		t.Errorf("status = %s, want %s", gotStatus, RefundCompleted)
	}
	if gotApproved != adminID {
		t.Errorf("approved_by = %q, want %q (IAM §۷: who approved the refund)", gotApproved, adminID)
	}
	if completedAt == nil {
		t.Error("completed_at must be set for a completed refund")
	}

	// Partial refund: order status untouched, refunded_total accumulated.
	var status string
	var refundedTotal int64
	if err := pool.QueryRow(ctx, `SELECT status, refunded_total FROM orders WHERE id = $1`, orderID).
		Scan(&status, &refundedTotal); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if status != OrderPaid {
		t.Errorf("order status = %s, want %s", status, OrderPaid)
	}
	if refundedTotal != 30000 {
		t.Errorf("refunded_total = %d, want 30000", refundedTotal)
	}

	// ListRefunds must read the same row back with the same fields.
	refunds, err := svc.ListRefunds(ctx, orderID)
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(refunds) != 1 {
		t.Fatalf("ListRefunds returned %d rows, want 1", len(refunds))
	}
	if refunds[0].PaymentAttemptID != attemptID || refunds[0].ApprovedBy != adminID {
		t.Errorf("ListRefunds row = %+v", refunds[0])
	}

	// The remaining 70000 completes the refund and closes the order.
	if _, err := svc.CreateRefund(ctx, orderID, 70000, "تسویه نهایی"); err != nil {
		t.Fatalf("second refund: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status, refunded_total FROM orders WHERE id = $1`, orderID).
		Scan(&status, &refundedTotal); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if status != OrderClosed {
		t.Errorf("order status = %s, want %s after a full refund", status, OrderClosed)
	}
	if refundedTotal != 100000 {
		t.Errorf("refunded_total = %d, want 100000", refundedTotal)
	}

	// Over-refunding is rejected by the application rule (BR-05).
	if _, err := svc.CreateRefund(ctx, orderID, 1, "بیش از سقف"); err == nil {
		t.Error("refunding more than paid − refunded must fail")
	}

	// Reconciliation: paid == succeeded attempts − refunds.
	res, err := svc.ReconcileTransaction(ctx, orderID)
	if err != nil {
		t.Fatalf("ReconcileTransaction: %v", err)
	}
	if !res.Reconciled {
		t.Errorf("reconciliation failed: %+v", res)
	}
}

func TestPG_CreateRefundRequiresSucceededPayment(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	// An order marked PAID with no payment attempt rows: the refund must be refused
	// rather than written with a dangling attempt reference.
	var orderID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO orders (order_number, guest_phone, status, subtotal, grand_total, paid_total,
		                     shipping_address_snapshot)
		 VALUES ('KHA-ORD-2026-000002','09120000010','PAID',5000,5000,5000,'{}'::jsonb)
		 RETURNING id`).Scan(&orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}

	store := NewDBStore(pool)
	svc := NewService(store, nil)
	svc.SetRefundStore(store)

	if _, err := svc.CreateRefund(ctx, orderID, 1000, "بدون پرداخت"); err == nil {
		t.Fatal("refund without a succeeded attempt must fail")
	}
}

// seedAdmin inserts an ACTIVE admin user and returns its id.
func seedAdmin(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ('refund@khabavar.ir','مالی','ACTIVE')
		 RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return id
}
