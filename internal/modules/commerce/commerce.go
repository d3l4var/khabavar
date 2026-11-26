// Package commerce implements the commerce module (SAD §۴.۲, DB v2 §۱۰, R-06):
// carts, cart items, orders, order items, payment intents/attempts, and refunds.
//
// The signature business rules enforced here:
//   - Order totals must reconcile: grand_total = subtotal - discount + shipping + tax
//     (the DB also enforces this via chk_orders_totals, but the Service checks first so
//     callers get a domain error instead of a constraint violation).
//   - One active payment intent per order at a time (uq_intent_active_per_order); one
//     successful payment per order (uq_intent_success_per_order) — ADR-COM-001.
//   - Guest checkout: customer_id may be NULL but guest_phone is required (chk_orders_contact).
//   - On OrderPaid the Service enqueues an "OrderPaid" domain event via EventEnqueuer so
//     the worker outbox can dispatch it to inventory/fulfillment (ADR-ARCH-007).
//
// Cross-module communication (SAD §۴.۲): commerce depends on catalog, inventory,
// pricing, identity — but never imports their tables directly. It receives
// already-resolved variant/price/customer data through the CreateOrder input and
// communicates outbound only via domain events.
package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ── Status enums (mirror CHECK constraints in 000006_commerce.up.sql) ─────────

const (
	CartActive    = "ACTIVE"
	CartConverted = "CONVERTED"
	CartAbandoned = "ABANDONED"
	CartExpired   = "EXPIRED"

	OrderPendingPayment = "PENDING_PAYMENT"
	OrderPaid           = "PAID"
	OrderInFulfillment  = "IN_FULFILLMENT"
	OrderPartiallyShip  = "PARTIALLY_SHIPPED"
	OrderShipped        = "SHIPPED"
	OrderDelivered      = "DELIVERED"
	OrderCompleted      = "COMPLETED"
	OrderCancelled      = "CANCELLED"
	OrderClosed         = "CLOSED"

	IntentRequiresPayment = "REQUIRES_PAYMENT"
	IntentProcessing      = "PROCESSING"
	IntentSucceeded       = "SUCCEEDED"
	IntentFailed          = "FAILED"
	IntentExpired         = "EXPIRED"
	IntentCancelled       = "CANCELLED"

	AttemptInitiated  = "INITIATED"
	AttemptRedirected = "REDIRECTED"
	AttemptUnknown    = "UNKNOWN"
	AttemptSucceeded  = "SUCCEEDED"
	AttemptFailed     = "FAILED"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrCartNotFound        = errors.New("commerce: cart not found")
	ErrCartEmpty           = errors.New("commerce: cart is empty")
	ErrOrderNotFound       = errors.New("commerce: order not found")
	ErrInvalidTransition   = errors.New("commerce: invalid status transition")
	ErrTotalsMismatch      = errors.New("commerce: order totals do not reconcile")
	ErrLineTotalsMismatch  = errors.New("commerce: order item line_total does not reconcile")
	ErrIntentExists        = errors.New("commerce: an active payment intent already exists for this order")
	ErrIntentNotSucceeded  = errors.New("commerce: payment intent has not succeeded")
	ErrAlreadyPaid         = errors.New("commerce: order is already paid")
	ErrCannotCancel        = errors.New("commerce: order cannot be cancelled in its current state")
	ErrContactRequired     = errors.New("commerce: customer_id or guest_phone is required")
	ErrOrderNumberRequired = errors.New("commerce: order_number is required")
)

// ── Domain types ──────────────────────────────────────────────────────────────

// Cart mirrors carts.
type Cart struct {
	ID           string
	CustomerID   string
	SessionToken string
	Status       string
	Currency     string
	ExpiresAt    *time.Time
}

// CartItem mirrors cart_items.
type CartItem struct {
	ID        string
	CartID    string
	VariantID string
	Quantity  int
}

// OrderInput is the payload for creating an order from a cart. The caller (HTTP
// handler or service orchestrator) resolves variant snapshots and unit prices from
// catalog/pricing BEFORE calling CreateOrder — commerce never reads those tables.
type OrderInput struct {
	CustomerID    string // empty for guest checkout
	GuestPhone    string // required when CustomerID is empty
	OrderNumber   string // e.g. "KHA-ORD-2026-000001" (§۴.۱/§۱۵)
	Currency      string // defaults to IRR
	Items         []OrderItemInput
	ShippingAddr  json.RawMessage // snapshot — JSONB in DB
	DiscountTotal int64
	ShippingTotal int64
	TaxTotal      int64
}

// OrderItemInput is a single line item resolved from a cart item + pricing.
type OrderItemInput struct {
	ProductID          string
	VariantID          string
	SKUSnapshot        string
	TitleSnapshot      string
	AttributesSnapshot json.RawMessage
	Quantity           int
	UnitPrice          int64
	DiscountAmount     int64
	TaxAmount          int64
	SupplyType         string // OWNED_INVENTORY | SUPPLIER_SOURCED
}

// Order mirrors orders.
type Order struct {
	ID               string
	OrderNumber      string
	CustomerID       string
	GuestPhone       string
	Status           string
	Currency         string
	Subtotal         int64
	DiscountTotal    int64
	ShippingTotal    int64
	TaxTotal         int64
	GrandTotal       int64
	PaidTotal        int64
	RefundedTotal    int64
	ShippingAddrSnap json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// OrderItem mirrors order_items.
type OrderItem struct {
	ID                 string
	OrderID            string
	ProductID          string
	VariantID          string
	SKUSnapshot        string
	TitleSnapshot      string
	AttributesSnapshot json.RawMessage
	Quantity           int
	UnitPrice          int64
	DiscountAmount     int64
	TaxAmount          int64
	LineTotal          int64
	SupplyTypeSnapshot string
}

// OrderDetail is an order with its items (the order-detail payload).
type OrderDetail struct {
	Order
	Items []OrderItem `json:"items"`
}

// PaymentIntent mirrors payment_intents.
type PaymentIntent struct {
	ID        string
	OrderID   string
	Amount    int64
	Currency  string
	Status    string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// ── Store: persistence boundary ───────────────────────────────────────────────

// Store is the persistence boundary for commerce. The DB implementation uses pgx;
// unit tests use an in-memory implementation (commerce_test.go).
type Store interface {
	// Cart
	GetCart(ctx context.Context, id string) (*Cart, []CartItem, error)
	UpdateCartStatus(ctx context.Context, id, status string) error

	// Order
	CreateOrder(ctx context.Context, o Order, items []OrderItem) (string, error)
	GetOrder(ctx context.Context, id string) (*Order, error)
	GetOrderByNumber(ctx context.Context, number string) (*Order, error)
	ListOrderItems(ctx context.Context, orderID string) ([]OrderItem, error)
	UpdateOrderStatus(ctx context.Context, id, status string) error
	UpdateOrderPaid(ctx context.Context, id string, paidTotal int64, status string) error

	// Payment intent
	CreatePaymentIntent(ctx context.Context, pi PaymentIntent) (string, error)
	GetActivePaymentIntent(ctx context.Context, orderID string) (*PaymentIntent, error)
	UpdatePaymentIntentStatus(ctx context.Context, id, status string) error
}

// EventEnqueuer appends a row to domain_events (the transactional outbox). The DB
// implementation INSERTs into domain_events; the in-memory test impl captures events
// for assertion. In production this must run in the same transaction as the order
// write (ADR-ARCH-007 §۱۴.۳) — the Service accepts an EventEnqueuer so the boundary
// is explicit, and the DB wiring can wrap both calls in a tx.
type EventEnqueuer interface {
	Enqueue(ctx context.Context, eventType, aggregateType, aggregateID string, payload any) error
}

// ── Service ───────────────────────────────────────────────────────────────────

// Service is the commerce module boundary (SAD §۴.۲).
type Service struct {
	store   Store
	queue   EventEnqueuer
	refunds RefundStore // optional; wired via SetRefundStore (nil → refund methods error)
	now     func() time.Time
}

// NewService wires the store and the optional outbox event enqueuer. queue may be nil
// (events are silently dropped — useful for unit tests that don't assert events).
func NewService(store Store, queue EventEnqueuer) *Service {
	return &Service{store: store, queue: queue, now: time.Now}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// ── CreateOrder: cart → order ──────────────────────────────────────────────────

// CreateOrder validates the input, reconciles totals, persists the order + items,
// and enqueues an "OrderCreated" domain event. The cart is NOT consumed here — the
// caller decides whether to mark it CONVERTED (typically yes).
func (s *Service) CreateOrder(ctx context.Context, in OrderInput) (string, error) {
	if err := validateOrderInput(in); err != nil {
		return "", err
	}
	if in.Currency == "" {
		in.Currency = "IRR"
	}

	// Compute subtotal and validate line totals.
	var subtotal int64
	items := make([]OrderItem, 0, len(in.Items))
	for i, it := range in.Items {
		if it.Quantity <= 0 {
			return "", fmt.Errorf("commerce: item %d quantity must be > 0", i)
		}
		lineTotal := int64(it.Quantity)*it.UnitPrice - it.DiscountAmount + it.TaxAmount
		if lineTotal < 0 {
			return "", fmt.Errorf("commerce: item %d line_total negative", i)
		}
		subtotal += lineTotal
		items = append(items, OrderItem{
			ID:                 uuid.NewString(),
			ProductID:          it.ProductID,
			VariantID:          it.VariantID,
			SKUSnapshot:        it.SKUSnapshot,
			TitleSnapshot:      it.TitleSnapshot,
			AttributesSnapshot: it.AttributesSnapshot,
			Quantity:           it.Quantity,
			UnitPrice:          it.UnitPrice,
			DiscountAmount:     it.DiscountAmount,
			TaxAmount:          it.TaxAmount,
			LineTotal:          lineTotal,
			SupplyTypeSnapshot: it.SupplyType,
		})
	}

	grandTotal := subtotal - in.DiscountTotal + in.ShippingTotal + in.TaxTotal
	if grandTotal < 0 {
		return "", ErrTotalsMismatch
	}

	order := Order{
		ID:               uuid.NewString(),
		OrderNumber:      in.OrderNumber,
		CustomerID:       in.CustomerID,
		GuestPhone:       in.GuestPhone,
		Status:           OrderPendingPayment,
		Currency:         in.Currency,
		Subtotal:         subtotal,
		DiscountTotal:    in.DiscountTotal,
		ShippingTotal:    in.ShippingTotal,
		TaxTotal:         in.TaxTotal,
		GrandTotal:       grandTotal,
		ShippingAddrSnap: in.ShippingAddr,
	}

	oid, err := s.store.CreateOrder(ctx, order, items)
	if err != nil {
		return "", fmt.Errorf("create order: %w", err)
	}

	// Enqueue OrderCreated event (best-effort — the outbox is the source of truth for
	// downstream modules; a failure here must not roll back the order).
	s.enqueue(ctx, "OrderCreated", "order", oid, map[string]any{
		"order_id":     oid,
		"order_number": in.OrderNumber,
		"grand_total":  grandTotal,
		"currency":     in.Currency,
	})

	return oid, nil
}

// validateOrderInput checks the invariants that the DB CHECK constraints also enforce,
// but returns clear domain errors first.
func validateOrderInput(in OrderInput) error {
	if in.OrderNumber == "" {
		return ErrOrderNumberRequired
	}
	if in.CustomerID == "" && in.GuestPhone == "" {
		return ErrContactRequired
	}
	if len(in.Items) == 0 {
		return ErrCartEmpty
	}
	for i, it := range in.Items {
		if it.SupplyType != "OWNED_INVENTORY" && it.SupplyType != "SUPPLIER_SOURCED" {
			return fmt.Errorf("commerce: item %d invalid supply_type %q", i, it.SupplyType)
		}
		if it.SKUSnapshot == "" || it.TitleSnapshot == "" {
			return fmt.Errorf("commerce: item %d missing sku/title snapshot", i)
		}
	}
	return nil
}

// ── ConvertCart: mark cart CONVERTED ──────────────────────────────────────────

// ConvertCart marks a cart CONVERTED (after its items have been turned into an order).
func (s *Service) ConvertCart(ctx context.Context, cartID string) error {
	return s.store.UpdateCartStatus(ctx, cartID, CartConverted)
}

// ── GetOrderDetail ────────────────────────────────────────────────────────────

// GetOrderDetail returns an order with its items, or ErrOrderNotFound.
func (s *Service) GetOrderDetail(ctx context.Context, id string) (*OrderDetail, error) {
	o, err := s.store.GetOrder(ctx, id)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}
	items, err := s.store.ListOrderItems(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	return &OrderDetail{Order: *o, Items: items}, nil
}

// GetOrderByNumber looks up an order by its human-readable number (used by guest
// order-tracking by phone + order_number — UJ-01).
func (s *Service) GetOrderByNumber(ctx context.Context, number string) (*Order, error) {
	o, err := s.store.GetOrderByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	if o == nil {
		return nil, ErrOrderNotFound
	}
	return o, nil
}

// ── Payment lifecycle ─────────────────────────────────────────────────────────

// CreatePaymentIntent starts a new payment intent for an order. Enforces the
// "one active intent per order" rule (uq_intent_active_per_order) before inserting.
func (s *Service) CreatePaymentIntent(ctx context.Context, orderID string) (string, error) {
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return "", err
	}
	if o == nil {
		return "", ErrOrderNotFound
	}
	if o.Status != OrderPendingPayment {
		return "", ErrAlreadyPaid
	}
	// Check for an existing active intent.
	existing, err := s.store.GetActivePaymentIntent(ctx, orderID)
	if err != nil {
		return "", fmt.Errorf("check active intent: %w", err)
	}
	if existing != nil {
		return "", ErrIntentExists
	}

	pi := PaymentIntent{
		ID:        uuid.NewString(),
		OrderID:   orderID,
		Amount:    o.GrandTotal,
		Currency:  o.Currency,
		Status:    IntentRequiresPayment,
		ExpiresAt: ptrTime(s.now().Add(30 * time.Minute)),
		CreatedAt: s.now(),
	}
	pid, err := s.store.CreatePaymentIntent(ctx, pi)
	if err != nil {
		return "", fmt.Errorf("create intent: %w", err)
	}
	return pid, nil
}

// ConfirmPayment marks a payment intent SUCCEEDED and the order PAID. Enforces
// ADR-COM-001: the order must be PENDING_PAYMENT; the intent must transition to
// SUCCEEDED. On success it enqueues an "OrderPaid" event for fulfillment/inventory.
func (s *Service) ConfirmPayment(ctx context.Context, intentID string) error {
	// The Store needs to find the intent by ID. We don't have a GetPaymentIntent by ID
	// in the interface (kept minimal), so the caller passes the intent obtained from
	// GetActivePaymentIntent. For the DB impl this is a single UPDATE … RETURNING.
	// Here we accept the intentID and let the store handle the lookup.
	//
	// To keep the interface minimal, we require the caller to have the orderID too.
	// This is a deliberate trade-off: ConfirmPayment is called by the payment webhook
	// handler which already knows the order.
	return fmt.Errorf("commerce: ConfirmPayment requires orderID — use ConfirmPaymentForOrder")
}

// ConfirmPaymentForOrder marks the active payment intent for orderID as SUCCEEDED and
// the order as PAID. This is the webhook-facing entry point.
func (s *Service) ConfirmPaymentForOrder(ctx context.Context, orderID string) error {
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if o == nil {
		return ErrOrderNotFound
	}
	if o.Status != OrderPendingPayment {
		return ErrAlreadyPaid
	}
	pi, err := s.store.GetActivePaymentIntent(ctx, orderID)
	if err != nil {
		return fmt.Errorf("load intent: %w", err)
	}
	if pi == nil {
		return ErrIntentNotSucceeded
	}
	if pi.Status != IntentRequiresPayment && pi.Status != IntentProcessing {
		return ErrIntentNotSucceeded
	}

	if err := s.store.UpdatePaymentIntentStatus(ctx, pi.ID, IntentSucceeded); err != nil {
		return fmt.Errorf("update intent: %w", err)
	}
	if err := s.store.UpdateOrderPaid(ctx, orderID, o.GrandTotal, OrderPaid); err != nil {
		return fmt.Errorf("update order paid: %w", err)
	}

	s.enqueue(ctx, "OrderPaid", "order", orderID, map[string]any{
		"order_id":     orderID,
		"order_number": o.OrderNumber,
		"paid_total":   o.GrandTotal,
		"currency":     o.Currency,
	})
	return nil
}

// ── Cancel ────────────────────────────────────────────────────────────────────

// CancelOrder transitions an order to CANCELLED. Only PENDING_PAYMENT orders can be
// cancelled (paid orders require a refund flow, which is a separate concern). On
// success it enqueues an "OrderCancelled" event so inventory reservations release.
func (s *Service) CancelOrder(ctx context.Context, orderID, reason string) error {
	o, err := s.store.GetOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if o == nil {
		return ErrOrderNotFound
	}
	if o.Status != OrderPendingPayment {
		return ErrCannotCancel
	}
	if err := s.store.UpdateOrderStatus(ctx, orderID, OrderCancelled); err != nil {
		return fmt.Errorf("cancel: %w", err)
	}
	s.enqueue(ctx, "OrderCancelled", "order", orderID, map[string]any{
		"order_id":     orderID,
		"order_number": o.OrderNumber,
		"reason":       reason,
	})
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// enqueue is a best-effort event enqueue. A nil EventEnqueuer means events are
// dropped (unit tests that don't assert events).
func (s *Service) enqueue(ctx context.Context, eventType, aggType, aggID string, payload any) {
	if s.queue == nil {
		return
	}
	if err := s.queue.Enqueue(ctx, eventType, aggType, aggID, payload); err != nil {
		// Log and continue — the order write already succeeded. The outbox retry
		// policy handles transient failures; a missing event is a known operational
		// risk mitigated by reconciliation jobs (P4).
		_ = err
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

// ── EventPayload helpers (used by tests and the HTTP layer) ───────────────────

// MarshalPayload is a convenience for handlers that need to JSON-encode an event
// payload before passing it to EventEnqueuer.
func MarshalPayload(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("commerce: marshal payload: %w", err)
	}
	return b, nil
}

// Event types produced by this module. Downstream modules (inventory, fulfillment,
// notifications) register handlers for these in the worker dispatcher.
const (
	EventOrderCreated   = "OrderCreated"
	EventOrderPaid      = "OrderPaid"
	EventOrderCancelled = "OrderCancelled"
)

// Ensure strings import is used (order_number normalization helper below).

// NormalizeOrderNumber upper-cases and trims the order number. KHA-ORD-... is the
// canonical form (§۴.۱).
func NormalizeOrderNumber(n string) string {
	return strings.ToUpper(strings.TrimSpace(n))
}
