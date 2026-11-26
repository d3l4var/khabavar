// fulfillment_db.go provides the pgx implementation of fulfillment.Store.
package fulfillment

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed fulfillment.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateFulfillmentOrder(ctx context.Context, fo FulfillmentOrder) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO fulfillment_orders (order_id, source_type, warehouse_id, supplier_id,
		         status, estimated_ready_at, estimated_delivery_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		fo.OrderID, fo.SourceType, nullableStr(fo.WarehouseID), nullableStr(fo.SupplierID),
		fo.Status, nullableTime(fo.EstimatedReadyAt), nullableTime(fo.EstimatedDeliveryAt)).Scan(&id)
	return id, err
}

func (s *DBStore) GetFulfillmentOrder(ctx context.Context, id string) (*FulfillmentOrder, error) {
	var fo FulfillmentOrder
	var warehouseID, supplierID pgtype.Text
	var estReady, estDelivery pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, order_id, source_type, warehouse_id, supplier_id, status,
		         estimated_ready_at, estimated_delivery_at, created_at, updated_at
		   FROM fulfillment_orders WHERE id = $1`, id).
		Scan(&fo.ID, &fo.OrderID, &fo.SourceType, &warehouseID, &supplierID, &fo.Status,
			&estReady, &estDelivery, &fo.CreatedAt, &fo.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fo.WarehouseID = textOr(warehouseID)
	fo.SupplierID = textOr(supplierID)
	fo.EstimatedReadyAt = timePtr(estReady)
	fo.EstimatedDeliveryAt = timePtr(estDelivery)
	return &fo, nil
}

func (s *DBStore) ListFulfillmentOrdersByOrder(ctx context.Context, orderID string) ([]FulfillmentOrder, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, order_id, source_type, warehouse_id, supplier_id, status,
		         estimated_ready_at, estimated_delivery_at, created_at, updated_at
		   FROM fulfillment_orders WHERE order_id = $1 ORDER BY created_at`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FulfillmentOrder, 0)
	for rows.Next() {
		var fo FulfillmentOrder
		var warehouseID, supplierID pgtype.Text
		var estReady, estDelivery pgtype.Timestamptz
		if err := rows.Scan(&fo.ID, &fo.OrderID, &fo.SourceType, &warehouseID, &supplierID,
			&fo.Status, &estReady, &estDelivery, &fo.CreatedAt, &fo.UpdatedAt); err != nil {
			return nil, err
		}
		fo.WarehouseID = textOr(warehouseID)
		fo.SupplierID = textOr(supplierID)
		fo.EstimatedReadyAt = timePtr(estReady)
		fo.EstimatedDeliveryAt = timePtr(estDelivery)
		out = append(out, fo)
	}
	return out, rows.Err()
}

func (s *DBStore) UpdateFulfillmentOrderStatus(ctx context.Context, id, status string, now time.Time) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE fulfillment_orders SET status = $2, updated_at = $3 WHERE id = $1`,
		id, status, now)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrFulfillmentNotFound
	}
	return nil
}

func (s *DBStore) CreateFulfillmentItem(ctx context.Context, fi FulfillmentItem) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO fulfillment_items (fulfillment_order_id, order_item_id, quantity, status)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		fi.FulfillmentOrderID, fi.OrderItemID, fi.Quantity, fi.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetFulfillmentItem(ctx context.Context, id string) (*FulfillmentItem, error) {
	var fi FulfillmentItem
	err := s.pool.QueryRow(ctx,
		`SELECT id, fulfillment_order_id, order_item_id, quantity, status, created_at
		   FROM fulfillment_items WHERE id = $1`, id).
		Scan(&fi.ID, &fi.FulfillmentOrderID, &fi.OrderItemID, &fi.Quantity, &fi.Status, &fi.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &fi, nil
}

// SumAllocatedForOrderItem returns the total quantity already allocated to
// fulfillment_items for a given order_item (the §11.2 invariant check). Only
// counts non-cancelled/rejected items.
func (s *DBStore) SumAllocatedForOrderItem(ctx context.Context, orderItemID string) (int, error) {
	var sum int
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(quantity), 0) FROM fulfillment_items
		  WHERE order_item_id = $1 AND status NOT IN ('CANCELLED','REJECTED')`,
		orderItemID).Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

func (s *DBStore) ListFulfillmentItemsByFulfillment(ctx context.Context, fulfillmentOrderID string) ([]FulfillmentItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, fulfillment_order_id, order_item_id, quantity, status, created_at
		   FROM fulfillment_items WHERE fulfillment_order_id = $1 ORDER BY created_at`,
		fulfillmentOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FulfillmentItem, 0)
	for rows.Next() {
		var fi FulfillmentItem
		if err := rows.Scan(&fi.ID, &fi.FulfillmentOrderID, &fi.OrderItemID, &fi.Quantity,
			&fi.Status, &fi.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, fi)
	}
	return out, rows.Err()
}

func (s *DBStore) UpdateFulfillmentItemStatus(ctx context.Context, id, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE fulfillment_items SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrFulfillmentItemNotFound
	}
	return nil
}

func (s *DBStore) CreateShipment(ctx context.Context, sh Shipment) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO shipments (fulfillment_order_id, carrier, service_type, tracking_number,
		         shipping_cost, status)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sh.FulfillmentOrderID, sh.Carrier, nullableStr(sh.ServiceType),
		nullableStr(sh.TrackingNumber), sh.ShippingCost, sh.Status).Scan(&id)
	return id, err
}

func (s *DBStore) GetShipment(ctx context.Context, id string) (*Shipment, error) {
	var sh Shipment
	var serviceType, tracking pgtype.Text
	var shippedAt, deliveredAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, fulfillment_order_id, carrier, service_type, tracking_number,
		         shipping_cost, status, shipped_at, delivered_at, created_at
		   FROM shipments WHERE id = $1`, id).
		Scan(&sh.ID, &sh.FulfillmentOrderID, &sh.Carrier, &serviceType, &tracking,
			&sh.ShippingCost, &sh.Status, &shippedAt, &deliveredAt, &sh.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sh.ServiceType = textOr(serviceType)
	sh.TrackingNumber = textOr(tracking)
	sh.ShippedAt = timePtr(shippedAt)
	sh.DeliveredAt = timePtr(deliveredAt)
	return &sh, nil
}

func (s *DBStore) ListShipmentsByFulfillment(ctx context.Context, fulfillmentOrderID string) ([]Shipment, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, fulfillment_order_id, carrier, service_type, tracking_number,
		         shipping_cost, status, shipped_at, delivered_at, created_at
		   FROM shipments WHERE fulfillment_order_id = $1 ORDER BY created_at`,
		fulfillmentOrderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Shipment, 0)
	for rows.Next() {
		var sh Shipment
		var serviceType, tracking pgtype.Text
		var shippedAt, deliveredAt pgtype.Timestamptz
		if err := rows.Scan(&sh.ID, &sh.FulfillmentOrderID, &sh.Carrier, &serviceType,
			&tracking, &sh.ShippingCost, &sh.Status, &shippedAt, &deliveredAt, &sh.CreatedAt); err != nil {
			return nil, err
		}
		sh.ServiceType = textOr(serviceType)
		sh.TrackingNumber = textOr(tracking)
		sh.ShippedAt = timePtr(shippedAt)
		sh.DeliveredAt = timePtr(deliveredAt)
		out = append(out, sh)
	}
	return out, rows.Err()
}

// UpdateShipmentStatus updates the status and, for terminal transitions, sets
// shipped_at (on IN_TRANSIT) or delivered_at (on DELIVERED).
func (s *DBStore) UpdateShipmentStatus(ctx context.Context, id, status string, now time.Time) error {
	var ct interface{ RowsAffected() int64 }
	var err error
	switch status {
	case ShipmentInTransit:
		ct, err = s.pool.Exec(ctx,
			`UPDATE shipments SET status = $2, shipped_at = COALESCE(shipped_at, $3)
			  WHERE id = $1`, id, status, now)
	case ShipmentDelivered:
		ct, err = s.pool.Exec(ctx,
			`UPDATE shipments SET status = $2, delivered_at = $3
			  WHERE id = $1`, id, status, now)
	default:
		ct, err = s.pool.Exec(ctx,
			`UPDATE shipments SET status = $2 WHERE id = $1`, id, status)
	}
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrShipmentNotFound
	}
	return nil
}

func (s *DBStore) CreateShipmentItem(ctx context.Context, si ShipmentItem) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO shipment_items (shipment_id, fulfillment_item_id, quantity)
		 VALUES ($1,$2,$3) RETURNING id`,
		si.ShipmentID, si.FulfillmentItemID, si.Quantity).Scan(&id)
	return id, err
}

func (s *DBStore) ListShipmentItemsByShipment(ctx context.Context, shipmentID string) ([]ShipmentItem, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shipment_id, fulfillment_item_id, quantity
		   FROM shipment_items WHERE shipment_id = $1 ORDER BY id`, shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ShipmentItem, 0)
	for rows.Next() {
		var si ShipmentItem
		if err := rows.Scan(&si.ID, &si.ShipmentID, &si.FulfillmentItemID, &si.Quantity); err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *DBStore) RecordShipmentEvent(ctx context.Context, ev ShipmentEvent) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO shipment_events (shipment_id, event_code, description, occurred_at,
		         source, external_reference)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		ev.ShipmentID, ev.EventCode, nullableStr(ev.Description), ev.OccurredAt,
		ev.Source, nullableStr(ev.ExternalReference)).Scan(&id)
	return id, err
}

func (s *DBStore) ListShipmentEvents(ctx context.Context, shipmentID string) ([]ShipmentEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, shipment_id, event_code, description, occurred_at, source,
		         external_reference, created_at
		   FROM shipment_events WHERE shipment_id = $1 ORDER BY occurred_at DESC`,
		shipmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ShipmentEvent, 0)
	for rows.Next() {
		var ev ShipmentEvent
		var desc, extRef pgtype.Text
		if err := rows.Scan(&ev.ID, &ev.ShipmentID, &ev.EventCode, &desc, &ev.OccurredAt,
			&ev.Source, &extRef, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Description = textOr(desc)
		ev.ExternalReference = textOr(extRef)
		out = append(out, ev)
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

func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

// Compile-time assertion that DBStore satisfies Store.
var _ Store = (*DBStore)(nil)
