package commerce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// CartStore is the persistence boundary for the server-side cart (DB §14.1).
//
// Guest carts are identified by a session token the client keeps; a signed-in customer
// has one ACTIVE cart per customer. The unique index on session_token is PARTIAL
// (uq_carts_active_session ... WHERE status = 'ACTIVE'), so every upsert repeats that
// predicate — without it Postgres cannot infer the index (42P10).
type CartStore interface {
	GetOrCreateCart(ctx context.Context, customerID, sessionToken string) (*Cart, error)
	AddCartItem(ctx context.Context, cartID, variantID string, quantity int) error
	SetCartItemQuantity(ctx context.Context, cartID, itemID string, quantity int) error
	DeleteCartItem(ctx context.Context, cartID, itemID string) error
	MergeCarts(ctx context.Context, guestCartID, customerCartID string) error
	CartView(ctx context.Context, cartID string) (*CartView, error)
}

// CartLine is one priced line of the cart as the storefront sees it.
type CartLine struct {
	ItemID            string `json:"id"`
	ProductID         string `json:"product_id"`
	VariantID         string `json:"variant_id"`
	SKU               string `json:"sku"`
	ProductTitle      string `json:"product_title"`
	Quantity          int    `json:"quantity"`
	UnitPrice         int64  `json:"unit_price"`
	LineTotal         int64  `json:"line_total"`
	Availability      string `json:"availability"`
	PriceIsApproved   bool   `json:"price_is_approved"`
	PriceRecordExists bool   `json:"-"`
}

// CartView is the cart with today's prices (a stale cart price is never guaranteed,
// DB §10.2: the price is re-read from the approved record).
type CartView struct {
	ID        string     `json:"id"`
	Currency  string     `json:"currency"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Items     []CartLine `json:"items"`
	Subtotal  int64      `json:"subtotal"`
}

// GetOrCreateCart returns the ACTIVE cart for the customer, or the guest cart for the
// session token, creating it when absent.
func (s *DBStore) GetOrCreateCart(ctx context.Context, customerID, sessionToken string) (*Cart, error) {
	if customerID == "" && sessionToken == "" {
		return nil, errors.New("commerce: customer_id or session_token is required")
	}
	var c Cart
	var custID *string
	var sess *string
	var expires *time.Time
	row := func() error {
		return s.pool.QueryRow(ctx,
			`SELECT id, customer_id::text, session_token, status, currency, expires_at
			   FROM carts
			  WHERE status = 'ACTIVE'
			    AND (($1 <> '' AND customer_id = $1::uuid) OR ($1 = '' AND session_token = $2))
			  ORDER BY created_at DESC LIMIT 1`, customerID, sessionToken).
			Scan(&c.ID, &custID, &sess, &c.Status, &c.Currency, &expires)
	}
	err := row()
	if errors.Is(err, pgx.ErrNoRows) {
		// Create it. The partial index needs the same predicate in ON CONFLICT.
		if customerID != "" {
			err = s.pool.QueryRow(ctx,
				`INSERT INTO carts (customer_id, status, currency, expires_at)
				 VALUES ($1::uuid, 'ACTIVE', 'IRR', now() + interval '30 days')
				 ON CONFLICT (session_token) WHERE status = 'ACTIVE' AND session_token IS NOT NULL
				 DO UPDATE SET updated_at = now()
				 RETURNING id`, customerID).Scan(&c.ID)
		} else {
			err = s.pool.QueryRow(ctx,
				`INSERT INTO carts (session_token, status, currency, expires_at)
				 VALUES ($1, 'ACTIVE', 'IRR', now() + interval '30 days')
				 ON CONFLICT (session_token) WHERE status = 'ACTIVE' AND session_token IS NOT NULL
				 DO UPDATE SET updated_at = now()
				 RETURNING id`, sessionToken).Scan(&c.ID)
		}
		if err != nil {
			return nil, fmt.Errorf("create cart: %w", err)
		}
		return s.GetOrCreateCart(ctx, customerID, sessionToken)
	}
	if err != nil {
		return nil, err
	}
	c.CustomerID = deref(custID)
	c.SessionToken = deref(sess)
	c.ExpiresAt = expires
	return &c, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// AddCartItem adds quantity to an existing line or inserts it. The unique index on
// (cart_id, variant_id) makes the upsert atomic, so two concurrent adds cannot create
// duplicate lines.
func (s *DBStore) AddCartItem(ctx context.Context, cartID, variantID string, quantity int) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO cart_items (cart_id, variant_id, quantity)
		 VALUES ($1,$2,$3)
		 ON CONFLICT (cart_id, variant_id)
		 DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, updated_at = now()`,
		cartID, variantID, quantity)
	return err
}

// SetCartItemQuantity sets an absolute quantity; zero deletes the line (contract:
// "صفر = حذف آیتم").
func (s *DBStore) SetCartItemQuantity(ctx context.Context, cartID, itemID string, quantity int) error {
	if quantity <= 0 {
		return s.DeleteCartItem(ctx, cartID, itemID)
	}
	ct, err := s.pool.Exec(ctx,
		`UPDATE cart_items SET quantity = $3, updated_at = now() WHERE id = $1 AND cart_id = $2`,
		itemID, cartID, quantity)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrCartItemNotFound
	}
	return nil
}

// DeleteCartItem removes a line, scoped to the cart so an id from another cart cannot
// be deleted.
func (s *DBStore) DeleteCartItem(ctx context.Context, cartID, itemID string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM cart_items WHERE id = $1 AND cart_id = $2`, itemID, cartID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrCartItemNotFound
	}
	return nil
}

// MergeCarts folds the guest cart into the customer cart (MA-8): quantities are summed
// and the guest cart is marked CONVERTED so it is never merged twice.
func (s *DBStore) MergeCarts(ctx context.Context, guestCartID, customerCartID string) error {
	if guestCartID == customerCartID {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`INSERT INTO cart_items (cart_id, variant_id, quantity)
		 SELECT $2, variant_id, quantity FROM cart_items WHERE cart_id = $1
		 ON CONFLICT (cart_id, variant_id)
		 DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, updated_at = now()`,
		guestCartID, customerCartID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, guestCartID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE carts SET status = 'CONVERTED', updated_at = now() WHERE id = $1`, guestCartID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CartView returns the cart with today's approved price per line. A line without an
// approved price is reported with availability UNAVAILABLE and zero price, so the
// storefront can say "not for sale" instead of showing a stale number.
func (s *DBStore) CartView(ctx context.Context, cartID string) (*CartView, error) {
	var v CartView
	var expires *time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT id, currency, expires_at FROM carts WHERE id = $1`, cartID).
		Scan(&v.ID, &v.Currency, &expires); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCartNotFound
		}
		return nil, err
	}
	v.ExpiresAt = expires

	rows, err := s.pool.Query(ctx,
		`SELECT ci.id, p.id, ci.variant_id, pv.sku, p.title, ci.quantity,
		        COALESCE(pr.selling_price, 0) AS unit_price,
		        pr.id IS NOT NULL AS has_price
		   FROM cart_items ci
		   JOIN product_variants pv ON pv.id = ci.variant_id
		   JOIN products p ON p.id = pv.product_id
		   LEFT JOIN LATERAL (
		        SELECT id, selling_price FROM product_price_records
		         WHERE variant_id = ci.variant_id
		           AND approved_by IS NOT NULL
		           AND valid_from <= now()
		           AND (valid_until IS NULL OR valid_until > now())
		         ORDER BY valid_from DESC LIMIT 1) pr ON true
		  WHERE ci.cart_id = $1
		  ORDER BY ci.created_at`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v.Items = make([]CartLine, 0, 4)
	for rows.Next() {
		var l CartLine
		if err := rows.Scan(&l.ItemID, &l.ProductID, &l.VariantID, &l.SKU, &l.ProductTitle, &l.Quantity,
			&l.UnitPrice, &l.PriceRecordExists); err != nil {
			return nil, err
		}
		l.LineTotal = l.UnitPrice * int64(l.Quantity)
		l.PriceIsApproved = l.PriceRecordExists
		if l.PriceRecordExists {
			l.Availability = "AVAILABLE"
		} else {
			l.Availability = "UNAVAILABLE"
		}
		v.Subtotal += l.LineTotal
		v.Items = append(v.Items, l)
	}
	return &v, rows.Err()
}
