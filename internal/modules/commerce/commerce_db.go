// commerce_db.go provides the pgx implementation of commerce.Store and a DB
// EventEnqueuer that INSERTs into domain_events (the transactional outbox).
package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx implementation of Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) GetCart(ctx context.Context, id string) (*Cart, []CartItem, error) {
	var c Cart
	var custID pgtype.Text
	var sessTok pgtype.Text
	var expires pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, customer_id, session_token, status, currency, expires_at
		   FROM carts WHERE id = $1`, id).
		Scan(&c.ID, &custID, &sessTok, &c.Status, &c.Currency, &expires)
	if err == pgx.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if custID.Valid {
		c.CustomerID = custID.String
	}
	if sessTok.Valid {
		c.SessionToken = sessTok.String
	}
	if expires.Valid {
		t := expires.Time
		c.ExpiresAt = &t
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, cart_id, variant_id, quantity FROM cart_items WHERE cart_id = $1 ORDER BY created_at`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []CartItem
	for rows.Next() {
		var it CartItem
		if err := rows.Scan(&it.ID, &it.CartID, &it.VariantID, &it.Quantity); err != nil {
			return nil, nil, err
		}
		items = append(items, it)
	}
	return &c, items, rows.Err()
}

func (s *DBStore) UpdateCartStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE carts SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

func (s *DBStore) CreateOrder(ctx context.Context, o Order, items []OrderItem) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`INSERT INTO orders
		   (id, order_number, customer_id, guest_phone, status, currency,
		    subtotal, discount_total, shipping_total, tax_total, grand_total,
		    shipping_address_snapshot)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		o.ID, o.OrderNumber, nullableStr(o.CustomerID), nullableStr(o.GuestPhone),
		o.Status, o.Currency,
		o.Subtotal, o.DiscountTotal, o.ShippingTotal, o.TaxTotal, o.GrandTotal,
		// json.RawMessage is a []byte, which pgx sends as bytea for a jsonb column and the
		// driver turns into NULL. Send the JSON text and let Postgres cast it.
		string(o.ShippingAddrSnap)); err != nil {
		return "", err
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO order_items
			   (id, order_id, product_id, variant_id, sku_snapshot, title_snapshot,
			    attributes_snapshot, quantity, unit_price, discount_amount, tax_amount,
			    line_total, supply_type_snapshot)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			it.ID, o.ID, it.ProductID, it.VariantID, it.SKUSnapshot, it.TitleSnapshot,
			it.AttributesSnapshot, it.Quantity, it.UnitPrice, it.DiscountAmount, it.TaxAmount,
			it.LineTotal, it.SupplyTypeSnapshot); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return o.ID, nil
}

func (s *DBStore) GetOrder(ctx context.Context, id string) (*Order, error) {
	var o Order
	var custID, guestPhone pgtype.Text
	var shipAddr []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_number, customer_id, guest_phone, status, currency,
		        subtotal, discount_total, shipping_total, tax_total, grand_total,
		        paid_total, refunded_total, shipping_address_snapshot, created_at, updated_at
		   FROM orders WHERE id = $1`, id).
		Scan(&o.ID, &o.OrderNumber, &custID, &guestPhone, &o.Status, &o.Currency,
			&o.Subtotal, &o.DiscountTotal, &o.ShippingTotal, &o.TaxTotal, &o.GrandTotal,
			&o.PaidTotal, &o.RefundedTotal, &shipAddr, &o.CreatedAt, &o.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if custID.Valid {
		o.CustomerID = custID.String
	}
	if guestPhone.Valid {
		o.GuestPhone = guestPhone.String
	}
	o.ShippingAddrSnap = shipAddr
	return &o, nil
}

func (s *DBStore) GetOrderByNumber(ctx context.Context, number string) (*Order, error) {
	var o Order
	var custID, guestPhone pgtype.Text
	var shipAddr []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_number, customer_id, guest_phone, status, currency,
		        subtotal, discount_total, shipping_total, tax_total, grand_total,
		        paid_total, refunded_total, shipping_address_snapshot, created_at, updated_at
		   FROM orders WHERE order_number = $1`, number).
		Scan(&o.ID, &o.OrderNumber, &custID, &guestPhone, &o.Status, &o.Currency,
			&o.Subtotal, &o.DiscountTotal, &o.ShippingTotal, &o.TaxTotal, &o.GrandTotal,
			&o.PaidTotal, &o.RefundedTotal, &shipAddr, &o.CreatedAt, &o.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if custID.Valid {
		o.CustomerID = custID.String
	}
	if guestPhone.Valid {
		o.GuestPhone = guestPhone.String
	}
	o.ShippingAddrSnap = shipAddr
	return &o, nil
}

func (s *DBStore) ListOrderItems(ctx context.Context, orderID string) ([]OrderItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, product_id, variant_id, sku_snapshot, title_snapshot,
		        attributes_snapshot, quantity, unit_price, discount_amount, tax_amount,
		        line_total, supply_type_snapshot
		   FROM order_items WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OrderItem, 0)
	for rows.Next() {
		var it OrderItem
		var attrs []byte
		if err := rows.Scan(&it.ID, &it.OrderID, &it.ProductID, &it.VariantID,
			&it.SKUSnapshot, &it.TitleSnapshot, &attrs, &it.Quantity, &it.UnitPrice,
			&it.DiscountAmount, &it.TaxAmount, &it.LineTotal, &it.SupplyTypeSnapshot); err != nil {
			return nil, err
		}
		it.AttributesSnapshot = attrs
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpdateOrderStatus moves an order to a new status. A zero-row update means the order
// does not exist: reporting success there would let a caller believe a transition
// happened (the same silent-write class as the category bug), so it is an error.
func (s *DBStore) UpdateOrderStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// UpdateOrderPaid records a payment against an order. Money must never be "recorded"
// against a row that does not exist, so a zero-row update is an error.
func (s *DBStore) UpdateOrderPaid(ctx context.Context, id string, paidTotal int64, status string) error {
	// The status guard is what makes confirmation safe under concurrency: only the caller that
	// moves PENDING_PAYMENT to PAID affects a row. A second callback affects none, which means
	// the order was already settled — not that it is missing.
	ct, err := s.pool.Exec(ctx,
		`UPDATE orders SET paid_total = $2, status = $3, updated_at = now()
		  WHERE id = $1 AND status = 'PENDING_PAYMENT'`,
		id, paidTotal, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

func (s *DBStore) CreatePaymentIntent(ctx context.Context, pi PaymentIntent) (string, error) {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO payment_intents (id, order_id, amount, currency, status, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		pi.ID, pi.OrderID, pi.Amount, pi.Currency, pi.Status, pi.ExpiresAt)
	return pi.ID, err
}

func (s *DBStore) GetActivePaymentIntent(ctx context.Context, orderID string) (*PaymentIntent, error) {
	var pi PaymentIntent
	var expires pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, amount, currency, status, expires_at, created_at
		   FROM payment_intents
		  WHERE order_id = $1 AND status IN ('REQUIRES_PAYMENT','PROCESSING')
		  ORDER BY created_at DESC LIMIT 1`, orderID).
		Scan(&pi.ID, &pi.OrderID, &pi.Amount, &pi.Currency, &pi.Status, &expires, &pi.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if expires.Valid {
		t := expires.Time
		pi.ExpiresAt = &t
	}
	return &pi, nil
}

// UpdatePaymentIntentStatus moves an intent to a new status. A zero-row update means
// the intent does not exist and must not be reported as success.
func (s *DBStore) UpdatePaymentIntentStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE payment_intents SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrPaymentIntentNotFound
	}
	return nil
}

// DBEventEnqueuer INSERTs domain_events rows. In production this should run inside
// the same transaction as the order write (ADR-ARCH-007 §۱۴.۳); for P3 it shares
// the pool. The P4 UnitOfWork will wrap both in a single tx.
type DBEventEnqueuer struct {
	pool *pgxpool.Pool
}

func NewDBEventEnqueuer(pool *pgxpool.Pool) *DBEventEnqueuer { return &DBEventEnqueuer{pool: pool} }

func (q *DBEventEnqueuer) Enqueue(ctx context.Context, eventType, aggType, aggID string, payload any) error {
	var raw []byte
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		raw = b
	}
	_, err := q.pool.Exec(ctx,
		`INSERT INTO domain_events (aggregate_type, aggregate_id, event_type, payload)
		 VALUES ($1,$2,$3,$4)`, aggType, aggID, eventType, raw)
	return err
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Compile-time assertions.
var (
	_ Store         = (*DBStore)(nil)
	_ EventEnqueuer = (*DBEventEnqueuer)(nil)
)

// guard against unused time import if test trimmed later
var _ = time.Now

// NextOrderNumber mints KHA-ORD-YYYY-NNNNNN from the order_number_seq sequence.
func (s *DBStore) NextOrderNumber(ctx context.Context, year int) (string, error) {
	var n int64
	if err := s.pool.QueryRow(ctx, `SELECT nextval('order_number_seq')`).Scan(&n); err != nil {
		return "", err
	}
	return fmt.Sprintf("KHA-ORD-%04d-%06d", year, n), nil
}

// ── RefundStore implementation ────────────────────────────────────────────────
// Column set follows migration 000006 (DB v2 §10.6): a refund references the
// SUCCEEDED payment attempt it reverses and stores who approved it. There is no
// currency column — an order is single-currency.

func (s *DBStore) CreateRefund(ctx context.Context, r Refund) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO refunds (order_id, payment_attempt_id, order_item_id, amount, reason,
		                      status, provider_reference, approved_by, completed_at)
		 VALUES ($1,$2,NULLIF($3,'')::uuid,$4,$5,$6,NULLIF($7,''),NULLIF($8,'')::uuid,$9)
		 RETURNING id`,
		r.OrderID, r.PaymentAttemptID, r.OrderItemID, r.Amount, r.Reason,
		r.Status, r.ProviderReference, r.ApprovedBy, r.CompletedAt).Scan(&id)
	return id, err
}

func (s *DBStore) ListRefunds(ctx context.Context, orderID string) ([]Refund, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, payment_attempt_id,
		        coalesce(order_item_id::text,''), amount, reason, status,
		        coalesce(provider_reference,''), coalesce(approved_by::text,''),
		        created_at, completed_at
		   FROM refunds WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Refund, 0)
	for rows.Next() {
		var r Refund
		var completedAt pgtype.Timestamptz
		if err := rows.Scan(&r.ID, &r.OrderID, &r.PaymentAttemptID, &r.OrderItemID,
			&r.Amount, &r.Reason, &r.Status, &r.ProviderReference, &r.ApprovedBy,
			&r.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			t := completedAt.Time
			r.CompletedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SuccessfulAttemptID returns the SUCCEEDED payment attempt of an order ("" when the
// order was never paid).
func (s *DBStore) SuccessfulAttemptID(ctx context.Context, orderID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT pa.id
		   FROM payment_attempts pa
		   JOIN payment_intents pi ON pi.id = pa.payment_intent_id
		  WHERE pi.order_id = $1 AND pa.status = 'SUCCEEDED'
		  ORDER BY pa.completed_at DESC NULLS LAST, pa.created_at DESC
		  LIMIT 1`, orderID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *DBStore) UpdateOrderRefunded(ctx context.Context, id string, refundedTotal int64, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE orders SET refunded_total = $2, status = $3, updated_at = now() WHERE id = $1`,
		id, refundedTotal, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOrderNotFound
	}
	return nil
}

// SumSucceededAttempts returns the total amount of succeeded payment attempts
// for an order. Used by ReconcileTransaction: PaidTotal should = this − refunds.
// CreateAttempt records a provider attempt. The idempotency key is unique, so a
// replayed webhook updates nothing and returns the existing row's id.
func (s *DBStore) CreateAttempt(ctx context.Context, a PaymentAttempt) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO payment_attempts (payment_intent_id, provider, provider_reference, amount,
		                               status, failure_code, idempotency_key, completed_at)
		 VALUES ($1,$2,NULLIF($3,''),$4,$5,NULLIF($6,''),NULLIF($7,''),$8)
		 ON CONFLICT (idempotency_key) DO UPDATE SET provider_reference = EXCLUDED.provider_reference
		 RETURNING id`,
		a.PaymentIntentID, a.Provider, a.ProviderReference, a.Amount,
		a.Status, a.FailureCode, a.IdempotencyKey, a.CompletedAt).Scan(&id)
	return id, err
}

// SumSucceededAttempts totals the captured money for an order. It sums the SUCCEEDED
// payment attempts (the money actually taken) and falls back to SUCCEEDED intents for
// any order paid before attempts were recorded.
func (s *DBStore) SumSucceededAttempts(ctx context.Context, orderID string) (int64, error) {
	var sum int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(pa.amount), 0)
		   FROM payment_attempts pa
		   JOIN payment_intents pi ON pi.id = pa.payment_intent_id
		  WHERE pi.order_id = $1 AND pa.status = 'SUCCEEDED'`, orderID).Scan(&sum)
	if err != nil {
		return 0, err
	}
	if sum > 0 {
		return sum, nil
	}
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM payment_intents
		  WHERE order_id = $1 AND status = 'SUCCEEDED'`, orderID).Scan(&sum)
	return sum, err
}

// Compile-time assertion that DBStore satisfies RefundStore.
var _ RefundStore = (*DBStore)(nil)
