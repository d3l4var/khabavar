package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/khabavar/khabavar/internal/modules/authorization"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ListByCustomer returns the customer's orders, newest first. It is scoped by
// customer_id in SQL, so a caller cannot read another customer's history by guessing.
func (s *OrderListStore) ListByCustomer(ctx context.Context, customerID string, page, pageSize int) ([]OrderSummary, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE customer_id = $1`, customerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_number, status, currency, grand_total, paid_total, refunded_total,
		        COALESCE(guest_phone,''), created_at
		   FROM orders WHERE customer_id = $1
		  ORDER BY created_at DESC
		  LIMIT $2 OFFSET $3`, customerID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]OrderSummary, 0, pageSize)
	for rows.Next() {
		var o OrderSummary
		var created pgtype.Timestamptz // created_at is timestamptz; OrderSummary keeps a string
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.Status, &o.Currency, &o.GrandTotal,
			&o.PaidTotal, &o.RefundedTotal, &o.GuestPhone, &created); err != nil {
			return nil, 0, err
		}
		if created.Valid {
			o.CreatedAt = created.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// FindCartBySession returns the ACTIVE guest cart for a token, or nil when there is none.
// Unlike GetOrCreateCart it never creates: a merge of a cart that does not exist is a
// no-op, not a new empty cart.
func (s *DBStore) FindCartBySession(ctx context.Context, sessionToken string) (*Cart, error) {
	if sessionToken == "" {
		return nil, nil
	}
	var c Cart
	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(customer_id::text,''), COALESCE(session_token,''), status, currency
		   FROM carts WHERE session_token = $1 AND status = 'ACTIVE' LIMIT 1`, sessionToken).
		Scan(&c.ID, &c.CustomerID, &c.SessionToken, &c.Status, &c.Currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateReturnRequestHandler serves POST /api/v1/orders/{id}/return-requests (customer
// auth). It is mounted behind Idempotent: a double-clicked "return" must not create two
// claims.
func CreateReturnRequestHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Quantity int    `json:"quantity"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		if req.Quantity == 0 {
			req.Quantity = 1 // the whole-order return is the common case
		}
		rr, err := svc.CreateReturnRequest(r.Context(), r.PathValue("id"),
			authorization.CustomerID(r.Context()), req.Quantity, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, rr)
	}
}
