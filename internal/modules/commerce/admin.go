// admin.go exposes the staff order list (R-07 /admin/orders, order.view). It reads a
// narrow projection so the panel never depends on the full order aggregate.
package commerce

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrderSummary is one row of the admin order list.
type OrderSummary struct {
	ID            string `json:"id"`
	OrderNumber   string `json:"order_number"`
	Status        string `json:"status"`
	Currency      string `json:"currency"`
	GrandTotal    int64  `json:"grand_total"`
	PaidTotal     int64  `json:"paid_total"`
	RefundedTotal int64  `json:"refunded_total"`
	GuestPhone    string `json:"guest_phone,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// OrderListStore reads the order list for the panel.
type OrderListStore struct {
	pool *pgxpool.Pool
}

// NewOrderListStore wires the reader.
func NewOrderListStore(pool *pgxpool.Pool) *OrderListStore { return &OrderListStore{pool: pool} }

// List returns a page of orders, newest first, optionally filtered by status.
func (s *OrderListStore) List(ctx context.Context, status, q string, page, pageSize int) ([]OrderSummary, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders
		  WHERE ($1 = '' OR status = $1)
		    AND ($2 = '' OR order_number ILIKE '%'||$2||'%' OR COALESCE(guest_phone,'') LIKE '%'||$2||'%')`,
		status, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_number, status, currency, grand_total, paid_total, refunded_total,
		        COALESCE(guest_phone,''), created_at
		   FROM orders
		  WHERE ($1 = '' OR status = $1)
		    AND ($2 = '' OR order_number ILIKE '%'||$2||'%' OR COALESCE(guest_phone,'') LIKE '%'||$2||'%')
		  ORDER BY created_at DESC
		  LIMIT $3 OFFSET $4`, status, q, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []OrderSummary
	for rows.Next() {
		var o OrderSummary
		var created pgtype.Timestamptz
		if err := rows.Scan(&o.ID, &o.OrderNumber, &o.Status, &o.Currency, &o.GrandTotal,
			&o.PaidTotal, &o.RefundedTotal, &o.GuestPhone, &created); err != nil {
			return nil, 0, err
		}
		if created.Valid {
			o.CreatedAt = created.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// ListOrdersHandler serves GET /api/v1/admin/orders (order.view).
func ListOrdersHandler(s *OrderListStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if size < 1 || size > 100 {
			size = 20
		}
		orders, total, err := s.List(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("q"), page, size)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error", "code": "internal_error"})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": orders, "total": total, "page": page, "page_size": size,
		})
	}
}
