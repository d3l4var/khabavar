// Package fulfillment implements the fulfillment module (SAD §۴.۲, DB v2 §۱۱,
// R-06 §۱۱, ADR-COM-001): fulfillment orders (per-source-type), fulfillment items,
// shipments (multiple per fulfillment), shipment items (split across packages),
// and the append-only shipment event timeline.
//
// The signature business rules enforced here:
//   - A fulfillment order's source_type must match its source reference: OWNED_INVENTORY
//     requires warehouse_id (no supplier_id), SUPPLIER_SOURCED requires supplier_id
//     (no warehouse_id). The DB enforces this with chk_fulfillment_source; the
//     Service checks first and returns a domain error (§11.1).
//   - Item allocation invariant (§11.2): the sum of quantities allocated to
//     fulfillment_items for one order_item must NOT exceed the order_item's
//     quantity. The Service checks this before creating each fulfillment_item.
//   - Shipment status transitions follow a forward-only path:
//     LABEL_CREATED → IN_TRANSIT → OUT_FOR_DELIVERY → DELIVERED
//     ↘ FAILED → RETURNED
//     A DELIVERED shipment cannot move back; a FAILED shipment can only go to
//     RETURNED. The Service enforces this state machine.
//   - Shipment events are append-only (§11.5): RecordEvent always INSERTs, never
//     UPDATEs or DELETEs. The timeline is the audit trail.
package fulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// Status / type enums (mirrors CHECK constraints in 000007_fulfillment.up.sql).
const (
	// Fulfillment order source types (§11.1).
	SourceOwnedInventory  = "OWNED_INVENTORY"
	SourceSupplierSourced = "SUPPLIER_SOURCED"

	// Fulfillment order status.
	FulfillmentPending    = "PENDING"
	FulfillmentConfirmed  = "CONFIRMED"
	FulfillmentProcessing = "PROCESSING"
	FulfillmentReady      = "READY"
	FulfillmentShipped    = "SHIPPED"
	FulfillmentDelivered  = "DELIVERED"
	FulfillmentCancelled  = "CANCELLED"
	FulfillmentRejected   = "REJECTED"

	// Fulfillment item status (subset of order status).
	ItemPending    = "PENDING"
	ItemConfirmed  = "CONFIRMED"
	ItemProcessing = "PROCESSING"
	ItemShipped    = "SHIPPED"
	ItemDelivered  = "DELIVERED"
	ItemCancelled  = "CANCELLED"
	ItemRejected   = "REJECTED"

	// Shipment status (§11.3).
	ShipmentLabelCreated   = "LABEL_CREATED"
	ShipmentInTransit      = "IN_TRANSIT"
	ShipmentOutForDelivery = "OUT_FOR_DELIVERY"
	ShipmentDelivered      = "DELIVERED"
	ShipmentFailed         = "FAILED"
	ShipmentReturned       = "RETURNED"

	// Shipment event source (§11.5).
	EventSourceManual  = "MANUAL"
	EventSourceCarrier = "CARRIER_API"
	EventSourceWebhook = "WEBHOOK"
)

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrFulfillmentNotFound      = errors.New("fulfillment: fulfillment order not found")
	ErrFulfillmentItemNotFound  = errors.New("fulfillment: fulfillment item not found")
	ErrShipmentNotFound         = errors.New("fulfillment: shipment not found")
	ErrSourceTypeMismatch       = errors.New("fulfillment: source_type and source reference mismatch (§11.1)")
	ErrInvalidFulfillmentStatus = errors.New("fulfillment: invalid fulfillment status transition")
	ErrInvalidShipmentStatus    = errors.New("fulfillment: invalid shipment status transition")
	ErrInvalidEventSource       = errors.New("fulfillment: invalid shipment event source")
	ErrAllocationExceedsOrder   = errors.New("fulfillment: item allocation exceeds order_item quantity (§11.2)")
	ErrQuantityMustBePositive   = errors.New("fulfillment: quantity must be > 0")
	ErrCarrierRequired          = errors.New("fulfillment: carrier required for shipment")
	ErrOrderIDRequired          = errors.New("fulfillment: order_id required")
)

// FulfillmentOrder mirrors fulfillment_orders. One order may have multiple
// fulfillment orders (e.g. one OWNED_INVENTORY + one SUPPLIER_SOURCED).
type FulfillmentOrder struct {
	ID                  string     `json:"id"`
	OrderID             string     `json:"order_id"`
	SourceType          string     `json:"source_type"`
	WarehouseID         string     `json:"warehouse_id"` // set iff source_type = OWNED_INVENTORY
	SupplierID          string     `json:"supplier_id"`  // set iff source_type = SUPPLIER_SOURCED
	Status              string     `json:"status"`
	EstimatedReadyAt    *time.Time `json:"estimated_ready_at"`
	EstimatedDeliveryAt *time.Time `json:"estimated_delivery_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// FulfillmentItem mirrors fulfillment_items. The allocation invariant (§11.2)
// says Σ(fulfillment_items.quantity for one order_item) ≤ order_item.quantity.
type FulfillmentItem struct {
	ID                 string
	FulfillmentOrderID string
	OrderItemID        string
	Quantity           int
	Status             string
	CreatedAt          time.Time
}

// Shipment mirrors shipments. A fulfillment order may have multiple shipments
// (e.g. partial shipments, split across packages — §11.3).
type Shipment struct {
	ID                 string     `json:"id"`
	FulfillmentOrderID string     `json:"fulfillment_order_id"`
	Carrier            string     `json:"carrier"`
	ServiceType        string     `json:"service_type"`
	TrackingNumber     string     `json:"tracking_number"`
	ShippingCost       int64      `json:"shipping_cost"` // IRR
	Status             string     `json:"status"`
	ShippedAt          *time.Time `json:"shipped_at"`
	DeliveredAt        *time.Time `json:"delivered_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

// ShipmentItem mirrors shipment_items — a fulfillment_item split across packages (§11.4).
type ShipmentItem struct {
	ID                string
	ShipmentID        string
	FulfillmentItemID string
	Quantity          int
}

// ShipmentEvent mirrors shipment_events — the append-only timeline (§11.5).
type ShipmentEvent struct {
	ID                string
	ShipmentID        string
	EventCode         string
	Description       string
	OccurredAt        time.Time
	Source            string
	ExternalReference string
	CreatedAt         time.Time
}

// Store is the persistence boundary for fulfillment. The DB implementation uses
// pgx; unit tests use an in-memory implementation.
type Store interface {
	CreateFulfillmentOrder(ctx context.Context, fo FulfillmentOrder) (string, error)
	GetFulfillmentOrder(ctx context.Context, id string) (*FulfillmentOrder, error)
	ListFulfillmentOrdersByOrder(ctx context.Context, orderID string) ([]FulfillmentOrder, error)
	UpdateFulfillmentOrderStatus(ctx context.Context, id, status string, now time.Time) error

	CreateFulfillmentItem(ctx context.Context, fi FulfillmentItem) (string, error)
	GetFulfillmentItem(ctx context.Context, id string) (*FulfillmentItem, error)
	// SumAllocatedForOrderItem returns the total quantity already allocated to
	// fulfillment_items for a given order_item (the §11.2 invariant check).
	SumAllocatedForOrderItem(ctx context.Context, orderItemID string) (int, error)
	ListFulfillmentItemsByFulfillment(ctx context.Context, fulfillmentOrderID string) ([]FulfillmentItem, error)
	UpdateFulfillmentItemStatus(ctx context.Context, id, status string) error

	CreateShipment(ctx context.Context, sh Shipment) (string, error)
	GetShipment(ctx context.Context, id string) (*Shipment, error)
	ListShipmentsByFulfillment(ctx context.Context, fulfillmentOrderID string) ([]Shipment, error)
	UpdateShipmentStatus(ctx context.Context, id, status string, now time.Time) error

	CreateShipmentItem(ctx context.Context, si ShipmentItem) (string, error)
	ListShipmentItemsByShipment(ctx context.Context, shipmentID string) ([]ShipmentItem, error)

	RecordShipmentEvent(ctx context.Context, ev ShipmentEvent) (string, error)
	ListShipmentEvents(ctx context.Context, shipmentID string) ([]ShipmentEvent, error)
}

// Service is the fulfillment module boundary (SAD §۴.۲).
type Service struct {
	store   Store
	now     func() time.Time
	auditor AuditRecorder // optional; nil → no audit writes
}

// AuditRecorder records a sensitive action (IAM §۷). Declared here so fulfillment stays
// free of an audit import, the same shape the other modules use.
type AuditRecorder interface {
	Record(ctx context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error
}

// SetAuditor wires the audit recorder (nil = no audit writes).
func (s *Service) SetAuditor(a AuditRecorder) { s.auditor = a }

// NewService wires the store. now is injected for deterministic tests.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// CreateFulfillmentOrder creates a new fulfillment order for a commerce order.
// The source_type must be consistent with the source reference (§11.1):
//   - OWNED_INVENTORY → warehouse_id set, supplier_id empty
//   - SUPPLIER_SOURCED → supplier_id set, warehouse_id empty
func (s *Service) CreateFulfillmentOrder(ctx context.Context, fo FulfillmentOrder) (string, error) {
	if fo.OrderID == "" {
		return "", ErrOrderIDRequired
	}
	if !validSourceType(fo.SourceType) {
		return "", ErrSourceTypeMismatch
	}
	if !sourceConsistent(fo) {
		return "", ErrSourceTypeMismatch
	}
	if fo.Status == "" {
		fo.Status = FulfillmentPending
	}
	return s.store.CreateFulfillmentOrder(ctx, fo)
}

// validSourceType checks the source_type enum.
func validSourceType(t string) bool {
	return t == SourceOwnedInventory || t == SourceSupplierSourced
}

// sourceConsistent enforces the §11.1 rule: source_type must match the reference.
func sourceConsistent(fo FulfillmentOrder) bool {
	if fo.SourceType == SourceOwnedInventory {
		return fo.WarehouseID != "" && fo.SupplierID == ""
	}
	if fo.SourceType == SourceSupplierSourced {
		return fo.SupplierID != "" && fo.WarehouseID == ""
	}
	return false
}

// ConfirmFulfillmentOrder moves PENDING → CONFIRMED. This is the point where
// inventory reservations are converted (the caller — commerce module — does
// that via the inventory module's API; fulfillment just records the status).
func (s *Service) ConfirmFulfillmentOrder(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentConfirmed, []string{FulfillmentPending})
}

// StartProcessing moves CONFIRMED → PROCESSING.
func (s *Service) StartProcessing(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentProcessing, []string{FulfillmentConfirmed})
}

// MarkReady moves PROCESSING → READY (goods packed, awaiting shipment).
func (s *Service) MarkReady(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentReady, []string{FulfillmentProcessing})
}

// MarkShipped moves READY → SHIPPED. All shipments under this fulfillment must
// have been created; the caller is responsible for creating them first.
func (s *Service) MarkShipped(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentShipped, []string{FulfillmentReady})
}

// MarkDelivered moves SHIPPED → DELIVERED. Terminal status.
func (s *Service) MarkDelivered(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentDelivered, []string{FulfillmentShipped})
}

// CancelFulfillmentOrder moves any non-terminal status → CANCELLED. PENDING,
// CONFIRMED, PROCESSING, READY can be cancelled; SHIPPED/DELIVERED cannot.
func (s *Service) CancelFulfillmentOrder(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentCancelled,
		[]string{FulfillmentPending, FulfillmentConfirmed, FulfillmentProcessing, FulfillmentReady})
}

// RejectFulfillmentOrder moves PENDING → REJECTED (supplier refused, or
// inventory unavailable). Terminal status.
func (s *Service) RejectFulfillmentOrder(ctx context.Context, id string) error {
	return s.transitionFulfillment(ctx, id, FulfillmentRejected, []string{FulfillmentPending, FulfillmentConfirmed})
}

// transitionFulfillment is the shared status-transition guard. It loads the
// current status, checks it's in the allowed `from` set, then updates.
func (s *Service) transitionFulfillment(ctx context.Context, id, to string, allowedFrom []string) error {
	fo, err := s.store.GetFulfillmentOrder(ctx, id)
	if err != nil {
		return err
	}
	if fo == nil {
		return ErrFulfillmentNotFound
	}
	if !statusInList(fo.Status, allowedFrom) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidFulfillmentStatus, fo.Status, to)
	}
	return s.store.UpdateFulfillmentOrderStatus(ctx, id, to, s.now())
}

func statusInList(s string, list []string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// AddFulfillmentItem allocates an order_item's quantity to this fulfillment.
// Enforces the §11.2 invariant: Σ(allocation for order_item) + new qty ≤ order_item.qty.
// The caller passes the order_item's total quantity (the fulfillment module does
// NOT read the commerce module's tables — SAD §۴.۲ dependency rule).
func (s *Service) AddFulfillmentItem(ctx context.Context, fi FulfillmentItem, orderItemQuantity int) error {
	if fi.FulfillmentOrderID == "" {
		return ErrFulfillmentNotFound
	}
	if fi.OrderItemID == "" {
		return ErrFulfillmentItemNotFound
	}
	if fi.Quantity <= 0 {
		return ErrQuantityMustBePositive
	}
	if fi.Status == "" {
		fi.Status = ItemPending
	}
	// Check the allocation invariant (§11.2).
	alreadyAllocated, err := s.store.SumAllocatedForOrderItem(ctx, fi.OrderItemID)
	if err != nil {
		return fmt.Errorf("sum allocated: %w", err)
	}
	if alreadyAllocated+fi.Quantity > orderItemQuantity {
		return fmt.Errorf("%w: allocated=%d + new=%d > order_item=%d",
			ErrAllocationExceedsOrder, alreadyAllocated, fi.Quantity, orderItemQuantity)
	}
	_, err = s.store.CreateFulfillmentItem(ctx, fi)
	return err
}

// CreateShipment creates a shipment under a fulfillment order. The fulfillment
// order must be in READY or PROCESSING status (goods must exist before shipping).
func (s *Service) CreateShipment(ctx context.Context, sh Shipment) (string, error) {
	if sh.FulfillmentOrderID == "" {
		return "", ErrFulfillmentNotFound
	}
	if sh.Carrier == "" {
		return "", ErrCarrierRequired
	}
	if sh.Status == "" {
		sh.Status = ShipmentLabelCreated
	}
	// Verify the fulfillment order exists and is in a shippable status.
	fo, err := s.store.GetFulfillmentOrder(ctx, sh.FulfillmentOrderID)
	if err != nil {
		return "", err
	}
	if fo == nil {
		return "", ErrFulfillmentNotFound
	}
	if fo.Status != FulfillmentReady && fo.Status != FulfillmentProcessing && fo.Status != FulfillmentShipped {
		return "", fmt.Errorf("%w: fulfillment status=%s", ErrInvalidFulfillmentStatus, fo.Status)
	}
	return s.store.CreateShipment(ctx, sh)
}

// TransitionShipmentStatus moves a shipment through its lifecycle. The allowed
// transitions (§11.3):
//
//	LABEL_CREATED → IN_TRANSIT
//	IN_TRANSIT → OUT_FOR_DELIVERY
//	OUT_FOR_DELIVERY → DELIVERED | FAILED
//	FAILED → RETURNED
//
// DELIVERED and RETURNED are terminal (cannot transition further).
// Each transition also records a shipment_event (§11.5 audit trail).
func (s *Service) TransitionShipmentStatus(ctx context.Context, id, to string, eventCode, description, source string) error {
	sh, err := s.store.GetShipment(ctx, id)
	if err != nil {
		return err
	}
	if sh == nil {
		return ErrShipmentNotFound
	}
	if !allowedShipmentTransition(sh.Status, to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidShipmentStatus, sh.Status, to)
	}
	// Validate the event source BEFORE mutating anything: a bad source would otherwise
	// change the shipment status and then fail the timeline insert (the CHECK allows
	// MANUAL / CARRIER_API / WEBHOOK only), leaving a transition with no audit trail.
	if source == "" {
		source = EventSourceManual
	}
	if !validEventSource(source) {
		return ErrInvalidEventSource
	}
	now := s.now()
	if err := s.store.UpdateShipmentStatus(ctx, id, to, now); err != nil {
		return err
	}
	// Record the audit event (§11.5). A failure here is returned, never swallowed: a
	// silent gap in the timeline is worse than a visible error.
	if _, err := s.store.RecordShipmentEvent(ctx, ShipmentEvent{
		ShipmentID:  id,
		EventCode:   eventCode,
		Description: description,
		OccurredAt:  now,
		Source:      source,
	}); err != nil {
		return fmt.Errorf("record shipment event: %w", err)
	}
	// Shipping terms / shipment status is an audited category (IAM §۷). Written after the
	// event row so the trail never claims a transition that was not recorded.
	if s.auditor != nil {
		if aerr := s.auditor.Record(ctx, "ADMIN", shared.ActorID(ctx), "shipment.status", "shipment", id,
			map[string]any{"status": sh.Status}, map[string]any{"status": to, "source": source}, ""); aerr != nil {
			return fmt.Errorf("audit shipment transition: %w", aerr)
		}
	}
	return nil
}

// validEventSource mirrors the shipment_events.source CHECK constraint.
func validEventSource(source string) bool {
	return source == EventSourceManual || source == EventSourceCarrier || source == EventSourceWebhook
}

// allowedShipmentTransition enforces the §11.3 state machine.
func allowedShipmentTransition(from, to string) bool {
	if from == to {
		return true // idempotent
	}
	allowed := map[string][]string{
		ShipmentLabelCreated:   {ShipmentInTransit},
		ShipmentInTransit:      {ShipmentOutForDelivery, ShipmentFailed},
		ShipmentOutForDelivery: {ShipmentDelivered, ShipmentFailed},
		ShipmentFailed:         {ShipmentReturned},
		ShipmentDelivered:      {}, // terminal
		ShipmentReturned:       {}, // terminal
	}
	for _, t := range allowed[from] {
		if t == to {
			return true
		}
	}
	return false
}

// AddShipmentItem splits a fulfillment_item across packages (§11.4). The sum of
// shipment_items for one fulfillment_item must not exceed the fulfillment_item's
// quantity — the caller is responsible for this check (the Store can enforce it
// with a CHECK or the Service can be extended; for now we trust the caller and
// rely on the audit trail).
func (s *Service) AddShipmentItem(ctx context.Context, si ShipmentItem) (string, error) {
	if si.ShipmentID == "" {
		return "", ErrShipmentNotFound
	}
	if si.FulfillmentItemID == "" {
		return "", ErrFulfillmentItemNotFound
	}
	if si.Quantity <= 0 {
		return "", ErrQuantityMustBePositive
	}
	return s.store.CreateShipmentItem(ctx, si)
}

// ListShipmentEvents returns the append-only event timeline for a shipment (§11.5).
func (s *Service) ListShipmentEvents(ctx context.Context, shipmentID string) ([]ShipmentEvent, error) {
	if shipmentID == "" {
		return nil, ErrShipmentNotFound
	}
	return s.store.ListShipmentEvents(ctx, shipmentID)
}

// ListShipmentsByFulfillment returns all shipments under a fulfillment order.
func (s *Service) ListShipmentsByFulfillment(ctx context.Context, fulfillmentOrderID string) ([]Shipment, error) {
	if fulfillmentOrderID == "" {
		return nil, ErrFulfillmentNotFound
	}
	return s.store.ListShipmentsByFulfillment(ctx, fulfillmentOrderID)
}

// ListFulfillmentOrdersByOrder returns all fulfillment orders for a commerce order.
func (s *Service) ListFulfillmentOrdersByOrder(ctx context.Context, orderID string) ([]FulfillmentOrder, error) {
	if orderID == "" {
		return nil, ErrOrderIDRequired
	}
	return s.store.ListFulfillmentOrdersByOrder(ctx, orderID)
}

// AddShipmentEvent appends a carrier/manual event to a shipment timeline without
// changing its status. A status change must go through TransitionShipment, which keeps
// the state machine honest; this is for tracking updates only.
func (s *Service) AddShipmentEvent(ctx context.Context, shipmentID, eventCode, description, source string, occurredAt time.Time) (string, error) {
	if shipmentID == "" {
		return "", ErrShipmentNotFound
	}
	if eventCode == "" {
		return "", ErrInvalidEventSource // reuse: an event without a code is malformed
	}
	if source == "" {
		source = EventSourceManual
	}
	if !validEventSource(source) {
		return "", ErrInvalidEventSource
	}
	if occurredAt.IsZero() {
		occurredAt = s.now()
	}
	return s.store.RecordShipmentEvent(ctx, ShipmentEvent{
		ShipmentID:  shipmentID,
		EventCode:   eventCode,
		Description: description,
		OccurredAt:  occurredAt,
		Source:      source,
	})
}
