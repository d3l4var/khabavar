// Package inventory implements the inventory module (SAD §۴.۲, DB v2 §۸, R-04 §۶):
// warehouses, stock balances, the append-only stock movement ledger, and
// transactional inventory reservations.
//
// The signature business rules enforced here:
//   - Sellable quantity = on_hand - reserved; it must NEVER be negative. The DB
//     enforces on_hand >= 0 and reserved >= 0, but the Service enforces the
//     combined invariant first so callers get a domain error (not a constraint
//     violation) when a reservation would oversell.
//   - The stock_movements ledger is append-only and immutable (§۸.۳). Corrections
//     happen ONLY by posting a reverse movement (e.g. ADJUSTMENT with negative
//     quantity), never by UPDATE or DELETE. RecordMovement always INSERTs.
//   - Reservations are source-explicit (§۸.۴): exactly one of warehouse_id or
//     supplier_id is set (the DB CHECK enforces this; the Service checks first).
//     A reservation without an expiry is meaningless (§۱۴.۲ / PRD O-4), so
//     expires_at is required.
//   - ExpireReservations closes ACTIVE reservations whose expires_at is in the
//     past, releasing their reserved_quantity back to sellable. This is the
//     worker's periodic sweep target.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status / type enums (mirrors CHECK constraints in 000004_inventory.up.sql).
const (
	WarehouseActive   = "ACTIVE"
	WarehouseInactive = "INACTIVE"

	MovementReceipt     = "RECEIPT"
	MovementSale        = "SALE"
	MovementReservation = "RESERVATION"
	MovementRelease     = "RELEASE"
	MovementReturn      = "RETURN"
	MovementAdjustment  = "ADJUSTMENT"
	MovementDamage      = "DAMAGE"
	MovementTransfer    = "TRANSFER"

	ReservationActive    = "ACTIVE"
	ReservationConverted = "CONVERTED"
	ReservationReleased  = "RELEASED"
	ReservationExpired   = "EXPIRED"
)

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrWarehouseNotFound      = errors.New("inventory: warehouse not found")
	ErrStockBalanceNotFound   = errors.New("inventory: stock balance not found")
	ErrInsufficientStock      = errors.New("inventory: insufficient sellable stock")
	ErrReservationNotFound    = errors.New("inventory: reservation not found")
	ErrReservationSource      = errors.New("inventory: reservation must have exactly one of warehouse_id or supplier_id")
	ErrReservationExpiryReq   = errors.New("inventory: reservation expires_at required")
	ErrReservationNotActive   = errors.New("inventory: reservation is not active")
	ErrZeroQuantity           = errors.New("inventory: movement quantity must be non-zero")
	ErrWarehouseCodeReq       = errors.New("inventory: warehouse_code required")
	ErrWarehouseNameReq       = errors.New("inventory: warehouse name required")
	ErrDuplicateWarehouseCode = errors.New("inventory: warehouse_code already exists")
	ErrNegativeOnHand         = errors.New("inventory: on_hand_quantity cannot be negative")
	ErrNegativeReserved       = errors.New("inventory: reserved_quantity cannot be negative")
)

// Warehouse mirrors warehouses.
type Warehouse struct {
	ID            string
	Name          string
	WarehouseCode string
	Address       string
	Status        string
	CreatedAt     time.Time
}

// StockBalance mirrors stock_balances. Sellable = OnHand - Reserved (computed).
// StockBalance carries snake_case JSON tags: the rest of the API is snake_case, and a
// PascalCase key leaking from a struct without tags is a client-visible inconsistency.
type StockBalance struct {
	ID               string    `json:"id"`
	WarehouseID      string    `json:"warehouse_id"`
	VariantID        string    `json:"variant_id"`
	OnHandQuantity   int       `json:"on_hand_quantity"`
	ReservedQuantity int       `json:"reserved_quantity"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Sellable returns the sellable quantity (on_hand - reserved). It can be negative
// only if data is corrupted; the Service guards against oversell before it happens.
func (b StockBalance) Sellable() int { return b.OnHandQuantity - b.ReservedQuantity }

// Movement mirrors a stock_movements row (append-only ledger, §۸.۳).
type Movement struct {
	ID            string
	WarehouseID   string
	VariantID     string
	MovementType  string
	Quantity      int // signed: positive in, negative out
	ReferenceType string
	ReferenceID   string
	CreatedAt     time.Time
	CreatedBy     string
}

// Reservation mirrors inventory_reservations (§۸.۴ — source-explicit).
type Reservation struct {
	ID          string
	OrderItemID string
	VariantID   string
	WarehouseID string // set iff source = warehouse
	SupplierID  string // set iff source = supplier
	Quantity    int
	Status      string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

// Store is the persistence boundary for inventory. The DB implementation uses pgx;
// unit tests use an in-memory implementation.
type Store interface {
	CreateWarehouse(ctx context.Context, w Warehouse) (string, error)
	GetWarehouseByCode(ctx context.Context, code string) (*Warehouse, error)
	GetWarehouse(ctx context.Context, id string) (*Warehouse, error)

	GetStockBalance(ctx context.Context, warehouseID, variantID string) (*StockBalance, error)
	UpsertStockBalance(ctx context.Context, b StockBalance) error
	// ReserveStock atomically: increments reserved_quantity by qty (within a tx),
	// checking the sellable invariant first. Returns ErrInsufficientStock if
	// on_hand - reserved < qty after the increment.
	ReserveStock(ctx context.Context, warehouseID, variantID string, qty int, now time.Time) error
	// ReleaseStock decrements reserved_quantity by qty (clamped at 0).
	ReleaseStock(ctx context.Context, warehouseID, variantID string, qty int, now time.Time) error
	// AdjustOnHand adds delta to on_hand_quantity (delta may be negative). Used by
	// RECEIPT (positive) and ADJUSTMENT/DAMAGE/RETURN (any sign). Enforces >= 0.
	AdjustOnHand(ctx context.Context, warehouseID, variantID string, delta int, now time.Time) error

	RecordMovement(ctx context.Context, m Movement) (string, error)
	ListMovements(ctx context.Context, warehouseID, variantID string, limit int) ([]Movement, error)

	CreateReservation(ctx context.Context, r Reservation) (string, error)
	GetReservation(ctx context.Context, id string) (*Reservation, error)
	UpdateReservationStatus(ctx context.Context, id, status string) error
	// ListExpiredActiveReservations returns ACTIVE reservations whose expires_at <= now,
	// for the periodic sweep (worker unit).
	ListExpiredActiveReservations(ctx context.Context, now time.Time, limit int) ([]Reservation, error)
}

// Service is the inventory module boundary (SAD §۴.۲).
type Service struct {
	store   Store
	now     func() time.Time
	auditor AuditRecorder // optional; nil → no audit writes
}

// AuditRecorder records a sensitive action (IAM §۷). Declaring the interface here keeps
// inventory free of an audit import, the same shape commerce and catalog use.
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

// CreateWarehouse persists a new warehouse. Warehouse code is unique (§۱۵).
func (s *Service) CreateWarehouse(ctx context.Context, w Warehouse) (string, error) {
	if w.Name == "" {
		return "", ErrWarehouseNameReq
	}
	if w.WarehouseCode == "" {
		return "", ErrWarehouseCodeReq
	}
	if w.Status == "" {
		w.Status = WarehouseActive
	}
	existing, err := s.store.GetWarehouseByCode(ctx, w.WarehouseCode)
	if err != nil {
		return "", fmt.Errorf("check code: %w", err)
	}
	if existing != nil {
		return "", ErrDuplicateWarehouseCode
	}
	return s.store.CreateWarehouse(ctx, w)
}

// GetStockBalance returns the on-hand/reserved state for a variant in a warehouse.
// Returns a zero-value balance (not an error) if no row exists yet.
func (s *Service) GetStockBalance(ctx context.Context, warehouseID, variantID string) (*StockBalance, error) {
	if warehouseID == "" || variantID == "" {
		return nil, ErrStockBalanceNotFound
	}
	b, err := s.store.GetStockBalance(ctx, warehouseID, variantID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		// No row yet → treat as zero balance (sellable=0).
		return &StockBalance{WarehouseID: warehouseID, VariantID: variantID}, nil
	}
	return b, nil
}

// ReceiveStock records a goods receipt: increments on_hand by qty and posts a
// RECEIPT movement. qty must be > 0. Both writes happen in the caller's
// transaction at the DB layer (the Store methods are individually atomic; the
// caller is responsible for wrapping them if it needs cross-table atomicity —
// here ReceiveStock calls AdjustOnHand then RecordMovement, and AdjustOnHand
// failing first means no orphan movement is written).
func (s *Service) ReceiveStock(ctx context.Context, warehouseID, variantID string, qty int, referenceType, referenceID, createdBy string) error {
	if qty <= 0 {
		return fmt.Errorf("inventory: receive qty must be > 0")
	}
	now := s.now()
	if err := s.store.AdjustOnHand(ctx, warehouseID, variantID, qty, now); err != nil {
		return fmt.Errorf("adjust on_hand: %w", err)
	}
	_, err := s.store.RecordMovement(ctx, Movement{
		WarehouseID:   warehouseID,
		VariantID:     variantID,
		MovementType:  MovementReceipt,
		Quantity:      qty,
		ReferenceType: referenceType,
		ReferenceID:   referenceID,
		CreatedAt:     now,
		CreatedBy:     createdBy,
	})
	if err != nil {
		return err
	}
	return nil
}

// AdjustStock records a manual adjustment: applies delta (any sign) to on_hand
// and posts an ADJUSTMENT movement. delta=0 is rejected (movements must be non-zero).
func (s *Service) AdjustStock(ctx context.Context, warehouseID, variantID string, delta int, referenceType, referenceID, createdBy, notes string) error {
	if delta == 0 {
		return ErrZeroQuantity
	}
	now := s.now()
	if err := s.store.AdjustOnHand(ctx, warehouseID, variantID, delta, now); err != nil {
		return fmt.Errorf("adjust on_hand: %w", err)
	}
	_, err := s.store.RecordMovement(ctx, Movement{
		WarehouseID:   warehouseID,
		VariantID:     variantID,
		MovementType:  MovementAdjustment,
		Quantity:      delta,
		ReferenceType: referenceType,
		ReferenceID:   referenceID,
		CreatedAt:     now,
		CreatedBy:     createdBy,
	})
	if err != nil {
		return err
	}
	// A stock change is one of the seven audited categories (IAM §۷). The audit follows the
	// movement row, so the trail never claims a change that was not recorded, and an audit
	// failure is returned rather than swallowed.
	if s.auditor != nil {
		if aerr := s.auditor.Record(ctx, "ADMIN", createdBy, "inventory.adjust", "variant", variantID,
			nil, map[string]any{
				"warehouse_id": warehouseID, "delta": delta,
				"reference_type": referenceType, "reference_id": referenceID, "notes": notes,
			}, ""); aerr != nil {
			return fmt.Errorf("audit stock adjustment: %w", aerr)
		}
	}
	return nil
}

// ReserveForOrder creates a reservation for an order item, reserves the stock in
// the warehouse, and posts a RESERVATION movement. The reservation is the
// durable record; the reserved_quantity increment is the live invariant.
// If the warehouse does not have enough sellable stock, ReserveStock returns
// ErrInsufficientStock and no reservation is created.
func (s *Service) ReserveForOrder(ctx context.Context, r Reservation) (string, error) {
	if r.OrderItemID == "" {
		return "", fmt.Errorf("inventory: order_item_id required")
	}
	if r.VariantID == "" {
		return "", ErrStockBalanceNotFound
	}
	if (r.WarehouseID == "") == (r.SupplierID == "") {
		// Both empty OR both set → invalid (§۸.۴: exactly one).
		return "", ErrReservationSource
	}
	if r.Quantity <= 0 {
		return "", fmt.Errorf("inventory: reservation quantity must be > 0")
	}
	if r.ExpiresAt.IsZero() {
		return "", ErrReservationExpiryReq
	}
	if r.Status == "" {
		r.Status = ReservationActive
	}
	now := s.now()
	// Warehouse-sourced reservation → reserve live stock.
	if r.WarehouseID != "" {
		if err := s.store.ReserveStock(ctx, r.WarehouseID, r.VariantID, r.Quantity, now); err != nil {
			return "", err
		}
		// Post the RESERVATION movement (signed: positive reserved count).
		s.store.RecordMovement(ctx, Movement{
			WarehouseID:   r.WarehouseID,
			VariantID:     r.VariantID,
			MovementType:  MovementReservation,
			Quantity:      r.Quantity,
			ReferenceType: "ORDER",
			ReferenceID:   r.OrderItemID,
			CreatedAt:     now,
		})
	}
	// Supplier-sourced reservation → no live stock to reserve (the supplier's
	// availability is tracked in suppliers.supplier_variants.available_quantity,
	// a different module's table). We only record the reservation row here.
	return s.store.CreateReservation(ctx, r)
}

// ReleaseReservation marks a reservation as RELEASED and releases the reserved
// stock back to sellable (warehouse-sourced only). Posts a RELEASE movement.
// Idempotent: if the reservation is already RELEASED/EXPIRED/CONVERTED, returns
// ErrReservationNotActive without touching stock.
func (s *Service) ReleaseReservation(ctx context.Context, id string) error {
	r, err := s.store.GetReservation(ctx, id)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrReservationNotFound
	}
	if r.Status != ReservationActive {
		return ErrReservationNotActive
	}
	if err := s.store.UpdateReservationStatus(ctx, id, ReservationReleased); err != nil {
		return err
	}
	if r.WarehouseID != "" {
		now := s.now()
		if err := s.store.ReleaseStock(ctx, r.WarehouseID, r.VariantID, r.Quantity, now); err != nil {
			return err
		}
		s.store.RecordMovement(ctx, Movement{
			WarehouseID:   r.WarehouseID,
			VariantID:     r.VariantID,
			MovementType:  MovementRelease,
			Quantity:      -r.Quantity, // release reduces the reserved count
			ReferenceType: "ORDER",
			ReferenceID:   r.OrderItemID,
			CreatedAt:     now,
		})
	}
	return nil
}

// ConvertReservation marks a reservation as CONVERTED (the order item has been
// fulfilled/shipped). For a warehouse-sourced reservation, it also reduces
// on_hand by the reserved quantity (the goods leave the warehouse) and posts a
// SALE movement, then releases the reserved_quantity (now that on_hand has
// dropped, reserved must drop too or sellable would be wrong).
func (s *Service) ConvertReservation(ctx context.Context, id string) error {
	r, err := s.store.GetReservation(ctx, id)
	if err != nil {
		return err
	}
	if r == nil {
		return ErrReservationNotFound
	}
	if r.Status != ReservationActive {
		return ErrReservationNotActive
	}
	if err := s.store.UpdateReservationStatus(ctx, id, ReservationConverted); err != nil {
		return err
	}
	if r.WarehouseID != "" {
		now := s.now()
		// Reduce on_hand (goods leave the warehouse).
		if err := s.store.AdjustOnHand(ctx, r.WarehouseID, r.VariantID, -r.Quantity, now); err != nil {
			return fmt.Errorf("reduce on_hand on convert: %w", err)
		}
		// Release the reserved quantity (now that on_hand dropped).
		if err := s.store.ReleaseStock(ctx, r.WarehouseID, r.VariantID, r.Quantity, now); err != nil {
			return fmt.Errorf("release reserved on convert: %w", err)
		}
		// Post SALE movement (signed: negative, goods out).
		s.store.RecordMovement(ctx, Movement{
			WarehouseID:   r.WarehouseID,
			VariantID:     r.VariantID,
			MovementType:  MovementSale,
			Quantity:      -r.Quantity,
			ReferenceType: "ORDER",
			ReferenceID:   r.OrderItemID,
			CreatedAt:     now,
		})
	}
	return nil
}

// ExpireReservations is the periodic sweep: finds ACTIVE reservations whose
// expires_at <= now and releases them. Returns the count expired. This is the
// target the worker unit calls on its ticker (§۱۴.۲ / PRD O-4).
func (s *Service) ExpireReservations(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	now := s.now()
	expired, err := s.store.ListExpiredActiveReservations(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, r := range expired {
		// UpdateReservationStatus to EXPIRED, then release warehouse stock.
		if err := s.store.UpdateReservationStatus(ctx, r.ID, ReservationExpired); err != nil {
			continue // best-effort; a partial sweep is acceptable
		}
		if r.WarehouseID != "" {
			s.store.ReleaseStock(ctx, r.WarehouseID, r.VariantID, r.Quantity, now)
			s.store.RecordMovement(ctx, Movement{
				WarehouseID:   r.WarehouseID,
				VariantID:     r.VariantID,
				MovementType:  MovementRelease,
				Quantity:      -r.Quantity,
				ReferenceType: "ORDER",
				ReferenceID:   r.OrderItemID,
				CreatedAt:     now,
			})
		}
		count++
	}
	return count, nil
}

// ListMovements returns the movement history for a variant in a warehouse
// (newest first). The ledger is append-only; this is a pure read.
func (s *Service) ListMovements(ctx context.Context, warehouseID, variantID string, limit int) ([]Movement, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return s.store.ListMovements(ctx, warehouseID, variantID, limit)
}
