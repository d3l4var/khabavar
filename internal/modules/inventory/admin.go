// admin.go exposes the contract's supply panel surface (R-07 admin-supply):
//
//	GET  /admin/stock             stock balances with the computed available quantity
//	POST /admin/stock/movements   record a stock movement (corrections only)
//	GET  /admin/reservations      reservations with status / expiry filters
//
// The write path is deliberately narrow. The contract says "correction only via a
// movement" (DB §8.3), so only on-hand-affecting types are accepted here: RESERVATION,
// RELEASE and SALE move reserved_quantity and belong to the order flow, and allowing
// them from the panel would let an operator corrupt the reservation invariant
// (available = on_hand − reserved, DB §8.2).
package inventory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// ErrMovementTypeNotAllowed is returned when a caller tries to post a movement type
// that belongs to the order flow rather than to a manual correction.
var ErrMovementTypeNotAllowed = errors.New("inventory: movement type is not allowed from the panel")

// onHandMovementTypes are the types a manual correction may post: they change
// on_hand_quantity only.
var onHandMovementTypes = map[string]bool{
	MovementReceipt:    true,
	MovementReturn:     true,
	MovementAdjustment: true,
	MovementDamage:     true,
	MovementTransfer:   true,
}

// RecordMovement applies a signed stock correction and appends the ledger row
// (DB §8.3). Positive quantity increases on_hand, negative decreases it; a negative
// resulting balance is refused by the store (DB §8.2).
func (s *Service) RecordMovement(ctx context.Context, mv Movement) (string, error) {
	if !onHandMovementTypes[mv.MovementType] {
		return "", ErrMovementTypeNotAllowed
	}
	if mv.Quantity == 0 {
		return "", ErrZeroQuantity
	}
	now := s.now()
	mv.CreatedAt = now
	if err := s.store.AdjustOnHand(ctx, mv.WarehouseID, mv.VariantID, mv.Quantity, now); err != nil {
		return "", err
	}
	return s.store.RecordMovement(ctx, mv)
}

// ── panel read views ───────────────────────────────────────────────────────────

// AdminStore reads the panel projections (independent of the Store interface so the
// unit-test fake stays minimal).
type AdminStore struct {
	pool *pgxpool.Pool
}

// NewAdminStore wires the reader.
func NewAdminStore(pool *pgxpool.Pool) *AdminStore { return &AdminStore{pool: pool} }

// StockBalanceView mirrors the contract's StockBalanceView.
type StockBalanceView struct {
	VariantID     string `json:"variant_id"`
	SKU           string `json:"sku"`
	WarehouseCode string `json:"warehouse_code"`
	OnHand        int    `json:"on_hand_quantity"`
	Reserved      int    `json:"reserved_quantity"`
	Available     int    `json:"available_quantity"`
	UpdatedAt     string `json:"updated_at"`
}

// ListStock returns a page of stock balances, optionally filtered by warehouse and by
// a SKU/title search term.
func (s *AdminStore) ListStock(ctx context.Context, warehouseID, q string, page, pageSize int) ([]StockBalanceView, int, error) {
	const where = `
	  WHERE ($1 = '' OR sb.warehouse_id::text = $1)
	    AND ($2 = '' OR v.sku ILIKE '%' || $2 || '%' OR p.title ILIKE '%' || $2 || '%')`
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*)
		   FROM stock_balances sb
		   JOIN warehouses w ON w.id = sb.warehouse_id
		   JOIN product_variants v ON v.id = sb.variant_id
		   JOIN products p ON p.id = v.product_id`+where, warehouseID, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT sb.variant_id, v.sku, w.warehouse_code, sb.on_hand_quantity, sb.reserved_quantity,
		        sb.on_hand_quantity - sb.reserved_quantity, sb.updated_at
		   FROM stock_balances sb
		   JOIN warehouses w ON w.id = sb.warehouse_id
		   JOIN product_variants v ON v.id = sb.variant_id
		   JOIN products p ON p.id = v.product_id`+where+`
		  ORDER BY w.warehouse_code, v.sku
		  LIMIT $3 OFFSET $4`, warehouseID, q, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]StockBalanceView, 0, pageSize)
	for rows.Next() {
		var v StockBalanceView
		var updated pgtype.Timestamptz
		if err := rows.Scan(&v.VariantID, &v.SKU, &v.WarehouseCode, &v.OnHand, &v.Reserved, &v.Available, &updated); err != nil {
			return nil, 0, err
		}
		if updated.Valid {
			v.UpdatedAt = updated.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// ReservationView mirrors the contract's ReservationView. order_number comes from the
// order item the reservation was created for.
type ReservationView struct {
	ID          string `json:"id"`
	OrderNumber string `json:"order_number,omitempty"`
	VariantID   string `json:"variant_id"`
	Source      string `json:"source"`
	Quantity    int    `json:"quantity"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expires_at"`
}

// ListReservations returns a page of reservations, optionally filtered by status and by
// an expiry ceiling (used by the sweep dashboard).
func (s *AdminStore) ListReservations(ctx context.Context, status string, expiresBefore *time.Time, page, pageSize int) ([]ReservationView, int, error) {
	const where = `
	  WHERE ($1 = '' OR r.status = $1)
	    AND ($2::timestamptz IS NULL OR r.expires_at <= $2)`
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM inventory_reservations r`+where, status, expiresBefore).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT r.id, COALESCE(o.order_number,''), r.variant_id,
		        CASE WHEN r.warehouse_id IS NOT NULL THEN 'WAREHOUSE' ELSE 'SUPPLIER' END,
		        r.quantity, r.status, r.expires_at
		   FROM inventory_reservations r
		   LEFT JOIN order_items oi ON oi.id = r.order_item_id
		   LEFT JOIN orders o ON o.id = oi.order_id`+where+`
		  ORDER BY r.expires_at
		  LIMIT $3 OFFSET $4`, status, expiresBefore, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]ReservationView, 0, pageSize)
	for rows.Next() {
		var v ReservationView
		var expires pgtype.Timestamptz
		if err := rows.Scan(&v.ID, &v.OrderNumber, &v.VariantID, &v.Source, &v.Quantity, &v.Status, &expires); err != nil {
			return nil, 0, err
		}
		if expires.Valid {
			v.ExpiresAt = expires.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// ── handlers ───────────────────────────────────────────────────────────────────

// ListStockHandler serves GET /api/v1/admin/stock.
func ListStockHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, size := pageParams(r)
		items, total, err := s.ListStock(r.Context(),
			r.URL.Query().Get("warehouse_id"), r.URL.Query().Get("q"), page, size)
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"items": items, "page": pageInfo(page, size, total)})
	}
}

// ListReservationsHandler serves GET /api/v1/admin/reservations.
func ListReservationsHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, size := pageParams(r)
		var expiresBefore *time.Time
		if raw := r.URL.Query().Get("expires_before"); raw != "" {
			t, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeAdminJSON(w, http.StatusBadRequest, map[string]string{
					"error": "expires_before must be RFC3339", "code": "invalid_request",
				})
				return
			}
			expiresBefore = &t
		}
		items, total, err := s.ListReservations(r.Context(),
			r.URL.Query().Get("status"), expiresBefore, page, size)
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"items": items, "page": pageInfo(page, size, total)})
	}
}

// RecordMovementHandler serves POST /api/v1/admin/stock/movements (inventory.adjust).
func RecordMovementHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			WarehouseID   string `json:"warehouse_id"`
			VariantID     string `json:"variant_id"`
			MovementType  string `json:"movement_type"`
			Quantity      int    `json:"quantity"`
			ReferenceType string `json:"reference_type"`
			Note          string `json:"note"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAdminJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid JSON body", "code": "invalid_request",
			})
			return
		}
		if req.WarehouseID == "" || req.VariantID == "" || req.MovementType == "" {
			writeAdminJSON(w, http.StatusBadRequest, map[string]string{
				"error": "warehouse_id, variant_id and movement_type are required", "code": "invalid_request",
			})
			return
		}
		if req.ReferenceType == "" {
			req.ReferenceType = "MANUAL"
		}
		id, err := svc.RecordMovement(r.Context(), Movement{
			WarehouseID:   req.WarehouseID,
			VariantID:     req.VariantID,
			MovementType:  req.MovementType,
			Quantity:      req.Quantity,
			ReferenceType: req.ReferenceType,
			CreatedBy:     shared.ActorID(r.Context()),
		})
		if err != nil {
			writeMovementErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// writeMovementErr maps movement errors; a negative resulting balance is a 409 because
// the operator must correct the ledger, not the request (DB §8.2).
func writeMovementErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrZeroQuantity), errors.Is(err, ErrMovementTypeNotAllowed):
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error(), "code": "invalid_request"})
	case errors.Is(err, ErrNegativeOnHand), errors.Is(err, ErrInsufficientStock):
		writeAdminJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "code": "insufficient_stock"})
	default:
		writeAdminJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "internal error", "code": "internal_error",
		})
	}
}

// pageParams clamps page/page_size to the contract defaults (page_size 20, max 100).
func pageParams(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// pageInfo builds the contract's PageInfo envelope.
func pageInfo(page, size, total int) map[string]any {
	pages := 0
	if size > 0 {
		pages = (total + size - 1) / size
	}
	return map[string]any{"page": page, "page_size": size, "total": total, "total_pages": pages}
}

func writeAdminErr(w http.ResponseWriter, err error) {
	writeAdminJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "internal error", "code": "internal_error",
	})
}

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WarehouseView is one row of the panel's warehouse list.
type WarehouseView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	WarehouseCode string `json:"warehouse_code"`
	Address       string `json:"address,omitempty"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

// ListWarehouses returns a page of warehouses, optionally filtered by status and a
// name/code term. The panel needs this to discover the {wid} that every stock operation
// takes: without it a warehouse could be created but never read back.
func (s *AdminStore) ListWarehouses(ctx context.Context, status, q string, page, pageSize int) ([]WarehouseView, int, error) {
	const where = `
	  WHERE ($1 = '' OR status = $1)
	    AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR warehouse_code ILIKE '%' || $2 || '%')`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM warehouses`+where, status, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, warehouse_code, COALESCE(address,''), status, created_at
		   FROM warehouses`+where+`
		  ORDER BY created_at DESC
		  LIMIT $3 OFFSET $4`, status, q, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]WarehouseView, 0, pageSize)
	for rows.Next() {
		var v WarehouseView
		var created pgtype.Timestamptz
		if err := rows.Scan(&v.ID, &v.Name, &v.WarehouseCode, &v.Address, &v.Status, &created); err != nil {
			return nil, 0, err
		}
		if created.Valid {
			v.CreatedAt = created.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// ListWarehousesHandler serves GET /api/v1/admin/warehouses (inventory.manage).
func ListWarehousesHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, size := pageParams(r)
		items, total, err := s.ListWarehouses(r.Context(),
			r.URL.Query().Get("status"), r.URL.Query().Get("q"), page, size)
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{
			"items": items, "page": pageInfo(page, size, total),
		})
	}
}
