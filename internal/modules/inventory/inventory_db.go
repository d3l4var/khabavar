// inventory_db.go provides the pgx implementation of inventory.Store. The critical
// methods are ReserveStock and ReleaseStock: they use SELECT ... FOR UPDATE on the
// stock_balances row to enforce the sellable invariant (on_hand - reserved >= 0)
// atomically under concurrent reservations.
package inventory

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed inventory.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateWarehouse(ctx context.Context, w Warehouse) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO warehouses (name, warehouse_code, address, status)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		w.Name, w.WarehouseCode, nullableStr(w.Address), w.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetWarehouseByCode(ctx context.Context, code string) (*Warehouse, error) {
	var w Warehouse
	var addr pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, warehouse_code, address, status, created_at
		   FROM warehouses WHERE warehouse_code = $1`, code).
		Scan(&w.ID, &w.Name, &w.WarehouseCode, &addr, &w.Status, &w.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.Address = textOr(addr)
	return &w, nil
}

func (s *DBStore) GetWarehouse(ctx context.Context, id string) (*Warehouse, error) {
	var w Warehouse
	var addr pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, warehouse_code, address, status, created_at
		   FROM warehouses WHERE id = $1`, id).
		Scan(&w.ID, &w.Name, &w.WarehouseCode, &addr, &w.Status, &w.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.Address = textOr(addr)
	return &w, nil
}

func (s *DBStore) GetStockBalance(ctx context.Context, warehouseID, variantID string) (*StockBalance, error) {
	var b StockBalance
	err := s.pool.QueryRow(ctx,
		`SELECT id, warehouse_id, variant_id, on_hand_quantity, reserved_quantity, updated_at
		   FROM stock_balances WHERE warehouse_id = $1 AND variant_id = $2`,
		warehouseID, variantID).
		Scan(&b.ID, &b.WarehouseID, &b.VariantID, &b.OnHandQuantity, &b.ReservedQuantity, &b.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *DBStore) UpsertStockBalance(ctx context.Context, b StockBalance) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO stock_balances (warehouse_id, variant_id, on_hand_quantity, reserved_quantity, updated_at)
		 VALUES ($1,$2,$3,$4,$5)
		 ON CONFLICT (warehouse_id, variant_id)
		 DO UPDATE SET on_hand_quantity = EXCLUDED.on_hand_quantity,
		               reserved_quantity = EXCLUDED.reserved_quantity,
		               updated_at = EXCLUDED.updated_at`,
		b.WarehouseID, b.VariantID, b.OnHandQuantity, b.ReservedQuantity, b.UpdatedAt)
	return err
}

// ReserveStock atomically increments reserved_quantity within a transaction,
// after locking the row with FOR UPDATE. The sellable invariant is checked
// before the increment: if (on_hand - reserved) < qty, the transaction rolls
// back and ErrInsufficientStock is returned. If no balance row exists, it is
// created with on_hand=0 (so an attempt to reserve against a non-existent
// balance correctly fails with ErrInsufficientStock).
func (s *DBStore) ReserveStock(ctx context.Context, warehouseID, variantID string, qty int, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var onHand, reserved int
	err = tx.QueryRow(ctx,
		`SELECT on_hand_quantity, reserved_quantity FROM stock_balances
		  WHERE warehouse_id = $1 AND variant_id = $2 FOR UPDATE`,
		warehouseID, variantID).Scan(&onHand, &reserved)
	if err == pgx.ErrNoRows {
		// No row yet: insert a zero balance so the invariant check below has numbers to
		// compare. The insert is rolled back when the check fails — a refused reservation
		// must not leave a row behind.
		_, err = tx.Exec(ctx,
			`INSERT INTO stock_balances (warehouse_id, variant_id, on_hand_quantity, reserved_quantity, updated_at)
			 VALUES ($1,$2,0,0,$3)`,
			warehouseID, variantID, now)
		if err != nil {
			return err
		}
		onHand, reserved = 0, 0
	} else if err != nil {
		return err
	}

	if onHand-reserved < qty {
		return ErrInsufficientStock
	}
	_, err = tx.Exec(ctx,
		`UPDATE stock_balances SET reserved_quantity = reserved_quantity + $3, updated_at = $4
		  WHERE warehouse_id = $1 AND variant_id = $2`,
		warehouseID, variantID, qty, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReleaseStock atomically decrements reserved_quantity (clamped at 0) within a
// transaction. If no balance row exists, it is a no-op (nothing to release).
func (s *DBStore) ReleaseStock(ctx context.Context, warehouseID, variantID string, qty int, now time.Time) error {
	if qty <= 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var reserved int
	err = tx.QueryRow(ctx,
		`SELECT reserved_quantity FROM stock_balances
		  WHERE warehouse_id = $1 AND variant_id = $2 FOR UPDATE`,
		warehouseID, variantID).Scan(&reserved)
	if err == pgx.ErrNoRows {
		return tx.Commit(ctx) // nothing to release
	}
	if err != nil {
		return err
	}
	// Clamp at 0 (defensive: should never go negative).
	newReserved := reserved - qty
	if newReserved < 0 {
		newReserved = 0
	}
	_, err = tx.Exec(ctx,
		`UPDATE stock_balances SET reserved_quantity = $3, updated_at = $4
		  WHERE warehouse_id = $1 AND variant_id = $2`,
		warehouseID, variantID, newReserved, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AdjustOnHand atomically adds delta to on_hand_quantity (delta may be negative),
// enforcing on_hand >= 0. If no balance row exists, it is created with on_hand=delta
// (must be >= 0 for a fresh row).
func (s *DBStore) AdjustOnHand(ctx context.Context, warehouseID, variantID string, delta int, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var onHand int
	err = tx.QueryRow(ctx,
		`SELECT on_hand_quantity FROM stock_balances
		  WHERE warehouse_id = $1 AND variant_id = $2 FOR UPDATE`,
		warehouseID, variantID).Scan(&onHand)
	if err == pgx.ErrNoRows {
		if delta < 0 {
			return ErrNegativeOnHand
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO stock_balances (warehouse_id, variant_id, on_hand_quantity, reserved_quantity, updated_at)
			 VALUES ($1,$2,$3,0,$4)`,
			warehouseID, variantID, delta, now)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	newOnHand := onHand + delta
	if newOnHand < 0 {
		return ErrNegativeOnHand
	}
	_, err = tx.Exec(ctx,
		`UPDATE stock_balances SET on_hand_quantity = $3, updated_at = $4
		  WHERE warehouse_id = $1 AND variant_id = $2`,
		warehouseID, variantID, newOnHand, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *DBStore) RecordMovement(ctx context.Context, m Movement) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO stock_movements (warehouse_id, variant_id, movement_type, quantity,
		         reference_type, reference_id, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		m.WarehouseID, m.VariantID, m.MovementType, m.Quantity,
		nullableStr(m.ReferenceType), nullableUUID(m.ReferenceID),
		nullableStr(m.CreatedBy)).Scan(&id)
	return id, err
}

func (s *DBStore) ListMovements(ctx context.Context, warehouseID, variantID string, limit int) ([]Movement, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, warehouse_id, variant_id, movement_type, quantity,
		         reference_type, reference_id, created_at, created_by
		   FROM stock_movements
		  WHERE warehouse_id = $1 AND variant_id = $2
		  ORDER BY created_at DESC
		  LIMIT $3`, warehouseID, variantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Movement, 0)
	for rows.Next() {
		var m Movement
		var refType, refID, createdBy pgtype.Text
		if err := rows.Scan(&m.ID, &m.WarehouseID, &m.VariantID, &m.MovementType,
			&m.Quantity, &refType, &refID, &m.CreatedAt, &createdBy); err != nil {
			return nil, err
		}
		m.ReferenceType = textOr(refType)
		m.ReferenceID = textOr(refID)
		m.CreatedBy = textOr(createdBy)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *DBStore) CreateReservation(ctx context.Context, r Reservation) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO inventory_reservations (order_item_id, variant_id, warehouse_id,
		         supplier_id, quantity, status, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		r.OrderItemID, r.VariantID, nullableStr(r.WarehouseID), nullableStr(r.SupplierID),
		r.Quantity, r.Status, r.ExpiresAt).Scan(&id)
	return id, err
}

func (s *DBStore) GetReservation(ctx context.Context, id string) (*Reservation, error) {
	var r Reservation
	var warehouseID, supplierID pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_item_id, variant_id, warehouse_id, supplier_id, quantity,
		         status, expires_at, created_at
		   FROM inventory_reservations WHERE id = $1`, id).
		Scan(&r.ID, &r.OrderItemID, &r.VariantID, &warehouseID, &supplierID,
			&r.Quantity, &r.Status, &r.ExpiresAt, &r.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.WarehouseID = textOr(warehouseID)
	r.SupplierID = textOr(supplierID)
	return &r, nil
}

func (s *DBStore) UpdateReservationStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE inventory_reservations SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrReservationNotFound
	}
	return nil
}

func (s *DBStore) ListExpiredActiveReservations(ctx context.Context, now time.Time, limit int) ([]Reservation, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_item_id, variant_id, warehouse_id, supplier_id, quantity,
		         status, expires_at, created_at
		   FROM inventory_reservations
		  WHERE status = 'ACTIVE' AND expires_at <= $1
		  ORDER BY expires_at ASC
		  LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Reservation, 0)
	for rows.Next() {
		var r Reservation
		var warehouseID, supplierID pgtype.Text
		if err := rows.Scan(&r.ID, &r.OrderItemID, &r.VariantID, &warehouseID, &supplierID,
			&r.Quantity, &r.Status, &r.ExpiresAt, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.WarehouseID = textOr(warehouseID)
		r.SupplierID = textOr(supplierID)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ── nullable helpers ──────────────────────────────────────────────────────────

func textOr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nullableUUID passes a UUID string through as text; pgx will cast. Empty → NULL.
func nullableUUID(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)
