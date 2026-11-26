// commerce_db.go provides the pgx implementation of commerce.Store and a DB
// EventEnqueuer that INSERTs into domain_events (the transactional outbox).
package commerce

import (
	"context"
	"encoding/json"
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
		o.ShippingAddrSnap); err != nil {
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
	var out []OrderItem
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

func (s *DBStore) UpdateOrderStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

func (s *DBStore) UpdateOrderPaid(ctx context.Context, id string, paidTotal int64, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE orders SET paid_total = $2, status = $3, updated_at = now() WHERE id = $1`,
		id, paidTotal, status)
	return err
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

func (s *DBStore) UpdatePaymentIntentStatus(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE payment_intents SET status = $2 WHERE id = $1`, id, status)
	return err
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

// ── RefundStore implementation ────────────────────────────────────────────────
//
// The refunds table would be created by a future migration; for now these
// methods query a `refunds` table schema that matches the Refund struct. The
// Service interface is stable — adding the table later doesn't change callers.

func (s *DBStore) CreateRefund(ctx context.Context, r Refund) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO refunds (order_id, amount, currency, reason, status)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		r.OrderID, r.Amount, r.Currency, r.Reason, r.Status).Scan(&id)
	return id, err
}

func (s *DBStore) ListRefunds(ctx context.Context, orderID string) ([]Refund, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, amount, currency, reason, status, created_at, processed_at
		   FROM refunds WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Refund
	for rows.Next() {
		var r Refund
		var processedAt pgtype.Timestamptz
		if err := rows.Scan(&r.ID, &r.OrderID, &r.Amount, &r.Currency, &r.Reason,
			&r.Status, &r.CreatedAt, &processedAt); err != nil {
			return nil, err
		}
		if processedAt.Valid {
			t := processedAt.Time
			r.ProcessedAt = &t
		}
		out = append(out, r)
	}
	return out, rows.Err()
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
func (s *DBStore) SumSucceededAttempts(ctx context.Context, orderID string) (int64, error) {
	// Payment intents that SUCCEEDED carry the captured amount.
	var sum int64
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM payment_intents
		  WHERE order_id = $1 AND status = 'SUCCEEDED'`, orderID).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// Compile-time assertion that DBStore satisfies RefundStore.
var _ RefundStore = (*DBStore)(nil)
