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

	"github.com/khabavar/khabavar/internal/modules/shared"
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
	ErrCartNotFound = errors.New("commerce: cart not found")
	// ErrCustomerSessionRequired is a missing authentication, not a server fault: it used to
	// be a bare error and answered 500 on POST /cart/merge.
	ErrCustomerSessionRequired = errors.New("commerce: a customer session is required to merge a cart")
	// ErrVariantRequired is a client mistake: an empty variant_id used to answer 500.
	ErrVariantRequired       = errors.New("commerce: variant_id is required")
	ErrCartEmpty             = errors.New("commerce: cart is empty")
	ErrCartItemNotFound      = errors.New("commerce: cart item not found")
	ErrCartTokenRequired     = errors.New("commerce: X-Guest-Cart-Token is required")
	ErrShippingAddressReq    = errors.New("commerce: shipping_address is required")
	ErrGuestPhoneRequired    = errors.New("commerce: guest_phone is required for a guest checkout (MA-8)")
	ErrIdempotencyKeyMissing = errors.New("commerce: Idempotency-Key header is required")
	// ErrOrderItemInvalid covers a line the caller sent without the snapshots an order needs.
	ErrOrderItemInvalid = errors.New("commerce: order item is invalid")
	// ErrIdempotencyKeyTooLong guards the varchar(100) column: an over-long key used to reach
	// the database and surface as a 500 for what is a bad request.
	ErrIdempotencyKeyTooLong = errors.New("commerce: Idempotency-Key is longer than 100 characters")
	ErrIdempotencyConflict   = errors.New("commerce: Idempotency-Key reused with a different body")
	ErrIdempotencyInFlight   = errors.New("commerce: the same Idempotency-Key is still in flight")
	ErrCartQuantityInvalid   = errors.New("commerce: quantity must be between 1 and 10")
	ErrOrderNotFound         = errors.New("commerce: order not found")
	ErrPaymentIntentNotFound = errors.New("commerce: payment intent not found")
	ErrTotalsMismatch        = errors.New("commerce: order totals do not reconcile")
	ErrIntentExists          = errors.New("commerce: an active payment intent already exists for this order")
	ErrIntentNotSucceeded    = errors.New("commerce: payment intent has not succeeded")
	ErrAlreadyPaid           = errors.New("commerce: order is already paid")
	ErrCannotCancel          = errors.New("commerce: order cannot be cancelled in its current state")
	ErrContactRequired       = errors.New("commerce: customer_id or guest_phone is required")
	ErrOrderNumberRequired   = errors.New("commerce: order_number is required")
)

// ── Domain types ──────────────────────────────────────────────────────────────

// Cart mirrors carts.
type Cart struct {
	ID           string     `json:"id"`
	CustomerID   string     `json:"customer_id"`
	SessionToken string     `json:"session_token"`
	Status       string     `json:"status"`
	Currency     string     `json:"currency"`
	ExpiresAt    *time.Time `json:"expires_at"`
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
	CustomerID    string           `json:"customer_id,omitempty"`  // empty for guest checkout
	GuestPhone    string           `json:"guest_phone,omitempty"`  // required when CustomerID is empty
	OrderNumber   string           `json:"order_number,omitempty"` // e.g. "KHA-ORD-2026-000001" (§۴.۱/§۱۵)
	Currency      string           `json:"currency,omitempty"`     // defaults to IRR
	Items         []OrderItemInput `json:"items"`
	ShippingAddr  json.RawMessage  `json:"shipping_address"` // snapshot — JSONB in DB
	DiscountTotal int64            `json:"discount_total"`
	ShippingTotal int64            `json:"shipping_total"`
	TaxTotal      int64            `json:"tax_total"`
}

// OrderItemInput is a single line item resolved from a cart item + pricing.
type OrderItemInput struct {
	ProductID          string          `json:"product_id"`
	VariantID          string          `json:"variant_id"`
	SKUSnapshot        string          `json:"sku_snapshot"`
	TitleSnapshot      string          `json:"title_snapshot"`
	AttributesSnapshot json.RawMessage `json:"attributes_snapshot,omitempty"`
	Quantity           int             `json:"quantity"`
	UnitPrice          int64           `json:"unit_price"`
	DiscountAmount     int64           `json:"discount_amount"`
	TaxAmount          int64           `json:"tax_amount"`
	SupplyType         string          `json:"supply_type"` // OWNED_INVENTORY | SUPPLIER_SOURCED
}

// Order mirrors orders.
type Order struct {
	ID               string          `json:"id"`
	OrderNumber      string          `json:"order_number"`
	CustomerID       string          `json:"customer_id,omitempty"`
	GuestPhone       string          `json:"guest_phone,omitempty"`
	Status           string          `json:"status"`
	Currency         string          `json:"currency"`
	Subtotal         int64           `json:"subtotal"`
	DiscountTotal    int64           `json:"discount_total"`
	ShippingTotal    int64           `json:"shipping_total"`
	TaxTotal         int64           `json:"tax_total"`
	GrandTotal       int64           `json:"grand_total"`
	PaidTotal        int64           `json:"paid_total"`
	RefundedTotal    int64           `json:"refunded_total"`
	ShippingAddrSnap json.RawMessage `json:"shipping_address,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// OrderItem mirrors order_items.
type OrderItem struct {
	ID                 string          `json:"id"`
	OrderID            string          `json:"order_id"`
	ProductID          string          `json:"product_id"`
	VariantID          string          `json:"variant_id"`
	SKUSnapshot        string          `json:"sku_snapshot"`
	TitleSnapshot      string          `json:"title_snapshot"`
	AttributesSnapshot json.RawMessage `json:"attributes_snapshot,omitempty"`
	Quantity           int             `json:"quantity"`
	UnitPrice          int64           `json:"unit_price"`
	DiscountAmount     int64           `json:"discount_amount"`
	TaxAmount          int64           `json:"tax_amount"`
	LineTotal          int64           `json:"line_total"`
	SupplyTypeSnapshot string
}

// OrderDetail is an order with its items (the order-detail payload).
type OrderDetail struct {
	Order
	Items []OrderItem `json:"items"`
}

// PaymentIntent mirrors payment_intents (DB v2 §10.5).
type PaymentIntent struct {
	ID        string
	OrderID   string
	Amount    int64
	Currency  string
	Status    string
	ExpiresAt *time.Time
	CreatedAt time.Time
}

// PaymentAttempt mirrors payment_attempts (DB v2 §10.5): one provider interaction.
// A refund must reference the SUCCEEDED attempt it reverses, so every captured payment
// has to produce one — otherwise refunds cannot be issued.
type PaymentAttempt struct {
	ID                string
	PaymentIntentID   string
	Provider          string
	ProviderReference string
	Amount            int64
	Status            string
	FailureCode       string
	IdempotencyKey    string
	CreatedAt         time.Time
	CompletedAt       *time.Time
}

// PaymentAttemptStore records provider attempts. DBStore implements it; the Service
// takes it as an optional dependency so unit tests with a minimal Store keep working.
type PaymentAttemptStore interface {
	CreateAttempt(ctx context.Context, a PaymentAttempt) (string, error)
}

// SetAttemptStore wires the payment-attempt recorder.
func (s *Service) SetAttemptStore(as PaymentAttemptStore) { s.attempts = as }

// FindIntentByProviderReference is used by the payment webhook to resolve the intent
// from the gateway's transaction id.
func (s *Service) FindIntentByProviderReference(ctx context.Context, reference string) (intentID, orderID string, err error) {
	store, ok := s.store.(interface {
		FindIntentByProviderReference(ctx context.Context, reference string) (string, string, error)
	})
	if !ok {
		return "", "", errors.New("commerce: provider reference lookup not supported by this store")
	}
	return store.FindIntentByProviderReference(ctx, reference)
}

// SetPaymentProvider sets the provider name stamped on payment attempts (the
// mock-first adapter name comes from KH_PAYMENT_ADAPTER, ADR-ARCH-011).
func (s *Service) SetPaymentProvider(name string) {
	if name != "" {
		s.provider = name
	}
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
	OrderIDForIntent(ctx context.Context, intentID string) (string, error)
	UpdateOrderPaid(ctx context.Context, id string, paidTotal int64, status string) error

	// Payment intent
	// NextOrderNumber mints the next business order number (KHA-ORD-YYYY-NNNNNN).
	// A sequence, not a count, so concurrent checkouts cannot collide.
	NextOrderNumber(ctx context.Context, year int) (string, error)

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
	carts    CartStore
	store    Store
	queue    EventEnqueuer
	refunds  RefundStore         // optional; wired via SetRefundStore (nil → refund methods error)
	attempts PaymentAttemptStore // optional; wired via SetAttemptStore (nil → no attempt rows)
	auditor  AuditRecorder       // optional; wired via SetAuditor (nil → no audit writes)
	provider string              // payment provider stamped on attempts (default mock)
	now      func() time.Time
}

// NewService wires the store and the optional outbox event enqueuer. queue may be nil
// (events are silently dropped — useful for unit tests that don't assert events).
func NewService(store Store, queue EventEnqueuer) *Service {
	return &Service{store: store, queue: queue, provider: "mock", now: time.Now}
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
	// The business order number is server-owned (§۴.۱): mint it BEFORE validation so a
	// caller that omits it is not rejected, and a web client never has to invent a
	// sequence value.
	if in.OrderNumber == "" {
		num, err := s.store.NextOrderNumber(ctx, s.now().Year())
		if err != nil {
			return "", fmt.Errorf("mint order number: %w", err)
		}
		in.OrderNumber = num
	}
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
		// An empty or malformed id used to reach the uuid column and answer 500. Validate here,
		// where it is plainly the caller's mistake.
		if !validUUID(it.VariantID) {
			return fmt.Errorf("%w: item %d variant_id is not a uuid", ErrOrderItemInvalid, i)
		}
		if it.ProductID != "" && !validUUID(it.ProductID) {
			return fmt.Errorf("%w: item %d product_id is not a uuid", ErrOrderItemInvalid, i)
		}
		if it.SupplyType != "OWNED_INVENTORY" && it.SupplyType != "SUPPLIER_SOURCED" {
			return fmt.Errorf("%w: item %d invalid supply_type %q", ErrOrderItemInvalid, i, it.SupplyType)
		}
		if it.SKUSnapshot == "" || it.TitleSnapshot == "" {
			// A missing snapshot is the caller's mistake, so it must map to 400, not 500.
			return fmt.Errorf("%w: item %d missing sku/title snapshot", ErrOrderItemInvalid, i)
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
// ConfirmPayment confirms an intent by its own id. It resolves the intent to its order and
// delegates to ConfirmPaymentForOrder, so there is one confirmation path, not two.
func (s *Service) ConfirmPayment(ctx context.Context, intentID string) error {
	if intentID == "" {
		return ErrPaymentIntentNotFound
	}
	orderID, err := s.store.OrderIDForIntent(ctx, intentID)
	if err != nil {
		return err
	}
	if orderID == "" {
		return ErrPaymentIntentNotFound
	}
	return s.ConfirmPaymentForOrder(ctx, orderID)
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

	// Record the provider attempt (DB v2 §10.5). A refund must reference the
	// SUCCEEDED attempt it reverses (refunds.payment_attempt_id NOT NULL), so a
	// captured payment without an attempt row makes the order unrefundable. The
	// idempotency key is derived from the intent, so a replayed webhook cannot
	// create a second attempt (payment_attempts.idempotency_key is UNIQUE).
	if s.attempts != nil {
		now := s.now()
		if _, err := s.attempts.CreateAttempt(ctx, PaymentAttempt{
			PaymentIntentID: pi.ID,
			Provider:        s.provider,
			Amount:          pi.Amount,
			Status:          AttemptSucceeded,
			IdempotencyKey:  "confirm:" + pi.ID,
			CreatedAt:       now,
			CompletedAt:     &now,
		}); err != nil {
			return fmt.Errorf("record payment attempt: %w", err)
		}
	}

	s.enqueue(ctx, "OrderPaid", "order", orderID, map[string]any{
		"order_id":     orderID,
		"order_number": o.OrderNumber,
		"paid_total":   o.GrandTotal,
		"currency":     o.Currency,
	})
	// A payment confirmation changes the order status, which is an audited category (IAM §۷).
	if aerr := s.auditOrder(ctx, "order.paid", orderID, OrderPendingPayment, OrderPaid, ""); aerr != nil {
		return aerr
	}
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
	// An order status change is one of the seven audited categories (IAM §۷).
	if aerr := s.auditOrder(ctx, "order.cancelled", orderID, o.Status, OrderCancelled, reason); aerr != nil {
		return aerr
	}
	s.enqueue(ctx, "OrderCancelled", "order", orderID, map[string]any{
		"order_id":     orderID,
		"order_number": o.OrderNumber,
		"reason":       reason,
	})
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// auditOrder records an order status change (IAM §۷). A nil auditor is a no-op; a real
// failure is returned so a status change without a trail is never reported as success.
func (s *Service) auditOrder(ctx context.Context, action, orderID, from, to, reason string) error {
	if s.auditor == nil {
		return nil
	}
	payload := map[string]any{"from": from, "to": to}
	if reason != "" {
		payload["reason"] = reason
	}
	if err := s.auditor.Record(ctx, "ADMIN", shared.ActorID(ctx), action, "order", orderID, nil, payload, ""); err != nil {
		return fmt.Errorf("audit %s: %w", action, err)
	}
	return nil
}

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

// validUUID reports whether s parses as a UUID, so a malformed id is a 400 before it reaches a
// uuid column and comes back as a 500.
func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
