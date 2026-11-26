package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// ── In-memory Store + EventEnqueuer for unit tests ────────────────────────────

type memStore struct {
	mu       sync.Mutex
	carts    map[string]*Cart
	cartItems map[string][]CartItem
	orders   map[string]*Order
	orderByNumber map[string]string // number → id
	items    map[string][]OrderItem
	intents  map[string][]*PaymentIntent // orderID → intents (newest last)
}

func newMemStore() *memStore {
	return &memStore{
		carts:    map[string]*Cart{},
		cartItems: map[string][]CartItem{},
		orders:   map[string]*Order{},
		orderByNumber: map[string]string{},
		items:    map[string][]OrderItem{},
		intents:  map[string][]*PaymentIntent{},
	}
}

func (m *memStore) GetCart(_ context.Context, id string) (*Cart, []CartItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.carts[id]
	if !ok {
		return nil, nil, nil
	}
	cp := *c
	items := append([]CartItem{}, m.cartItems[id]...)
	return &cp, items, nil
}

func (m *memStore) UpdateCartStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.carts[id]
	if !ok {
		return ErrCartNotFound
	}
	c.Status = status
	return nil
}

func (m *memStore) CreateOrder(_ context.Context, o Order, items []OrderItem) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.orders[o.ID]; ok {
		return "", errors.New("duplicate order id")
	}
	if _, ok := m.orderByNumber[o.OrderNumber]; ok {
		return "", errors.New("duplicate order_number")
	}
	oc := o
	m.orders[o.ID] = &oc
	m.orderByNumber[o.OrderNumber] = o.ID
	cp := append([]OrderItem{}, items...)
	for i := range cp {
		cp[i].OrderID = o.ID
	}
	m.items[o.ID] = cp
	return o.ID, nil
}

func (m *memStore) GetOrder(_ context.Context, id string) (*Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return nil, nil
	}
	cp := *o
	return &cp, nil
}

func (m *memStore) GetOrderByNumber(_ context.Context, number string) (*Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.orderByNumber[number]
	if !ok {
		return nil, nil
	}
	cp := *m.orders[id]
	return &cp, nil
}

func (m *memStore) ListOrderItems(_ context.Context, orderID string) ([]OrderItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]OrderItem{}, m.items[orderID]...)
	return cp, nil
}

func (m *memStore) UpdateOrderStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return ErrOrderNotFound
	}
	o.Status = status
	return nil
}

func (m *memStore) UpdateOrderPaid(_ context.Context, id string, paidTotal int64, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return ErrOrderNotFound
	}
	o.PaidTotal = paidTotal
	o.Status = status
	return nil
}

func (m *memStore) CreatePaymentIntent(_ context.Context, pi PaymentIntent) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.intents[pi.OrderID] = append(m.intents[pi.OrderID], &pi)
	return pi.ID, nil
}

func (m *memStore) GetActivePaymentIntent(_ context.Context, orderID string) (*PaymentIntent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.intents[orderID]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].Status == IntentRequiresPayment || list[i].Status == IntentProcessing {
			cp := *list[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *memStore) UpdatePaymentIntentStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, list := range m.intents {
		for _, pi := range list {
			if pi.ID == id {
				pi.Status = status
				return nil
			}
		}
	}
	return errors.New("intent not found")
}

// memQueue captures enqueued events for test assertions.
type memQueue struct {
	mu     sync.Mutex
	events []queuedEvent
}

type queuedEvent struct {
	eventType, aggType, aggID string
	payload                   any
}

func (q *memQueue) Enqueue(_ context.Context, eventType, aggType, aggID string, payload any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.events = append(q.events, queuedEvent{eventType, aggType, aggID, payload})
	return nil
}

func (q *memQueue) Events() []queuedEvent {
	q.mu.Lock()
	defer q.mu.Unlock()
	cp := append([]queuedEvent{}, q.events...)
	return cp
}

// ── Test fixtures ─────────────────────────────────────────────────────────────

func validOrderInput() OrderInput {
	return OrderInput{
		CustomerID:  "cust-1",
		OrderNumber: "KHA-ORD-2026-000001",
		Items: []OrderItemInput{
			{
				ProductID: "prod-1", VariantID: "var-1",
				SKUSnapshot: "SKU-001", TitleSnapshot: "عسل خواب‌آور ۵۰۰گ",
				Quantity: 2, UnitPrice: 500_000, SupplyType: "OWNED_INVENTORY",
			},
		},
		ShippingAddr: json.RawMessage(`{"city":"تهران"}`),
	}
}

func testService(t *testing.T) (*Service, *memStore, *memQueue) {
	t.Helper()
	store := newMemStore()
	q := &memQueue{}
	svc := NewService(store, q)
	return svc, store, q
}

// ── CreateOrder ───────────────────────────────────────────────────────────────

func TestCreateOrder_PersistsOrderAndItems(t *testing.T) {
	svc, store, q := testService(t)
	in := validOrderInput()

	oid, err := svc.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	o, err := store.GetOrder(context.Background(), oid)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if o == nil {
		t.Fatal("order not persisted")
	}
	if o.Status != OrderPendingPayment {
		t.Fatalf("status=%s want PENDING_PAYMENT", o.Status)
	}
	// grand_total = subtotal - discount + shipping + tax
	// subtotal = 2 * 500000 = 1000000; discount=0; shipping=0; tax=0
	if o.GrandTotal != 1_000_000 {
		t.Fatalf("grand_total=%d want 1000000", o.GrandTotal)
	}
	if o.Subtotal != 1_000_000 {
		t.Fatalf("subtotal=%d want 1000000", o.Subtotal)
	}

	items, _ := store.ListOrderItems(context.Background(), oid)
	if len(items) != 1 {
		t.Fatalf("items=%d want 1", len(items))
	}
	if items[0].LineTotal != 1_000_000 {
		t.Fatalf("line_total=%d want 1000000", items[0].LineTotal)
	}

	// Event enqueued.
	evs := q.Events()
	if len(evs) != 1 || evs[0].eventType != EventOrderCreated {
		t.Fatalf("events=%v", evs)
	}
}

func TestCreateOrder_RejectsEmptyItems(t *testing.T) {
	svc, _, _ := testService(t)
	in := validOrderInput()
	in.Items = nil
	_, err := svc.CreateOrder(context.Background(), in)
	if !errors.Is(err, ErrCartEmpty) {
		t.Fatalf("expected ErrCartEmpty, got %v", err)
	}
}

func TestCreateOrder_RejectsMissingContact(t *testing.T) {
	svc, _, _ := testService(t)
	in := validOrderInput()
	in.CustomerID = ""
	in.GuestPhone = ""
	_, err := svc.CreateOrder(context.Background(), in)
	if !errors.Is(err, ErrContactRequired) {
		t.Fatalf("expected ErrContactRequired, got %v", err)
	}
}

func TestCreateOrder_AllowsGuestWithPhone(t *testing.T) {
	svc, store, _ := testService(t)
	in := validOrderInput()
	in.CustomerID = ""
	in.GuestPhone = "09120000000"
	oid, err := svc.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	o, _ := store.GetOrder(context.Background(), oid)
	if o.CustomerID != "" || o.GuestPhone != "09120000000" {
		t.Fatalf("guest order = %+v", o)
	}
}

func TestCreateOrder_RejectsMissingOrderNumber(t *testing.T) {
	svc, _, _ := testService(t)
	in := validOrderInput()
	in.OrderNumber = ""
	_, err := svc.CreateOrder(context.Background(), in)
	if !errors.Is(err, ErrOrderNumberRequired) {
		t.Fatalf("expected ErrOrderNumberRequired, got %v", err)
	}
}

func TestCreateOrder_ReconcilesTotalsWithDiscountShippingTax(t *testing.T) {
	svc, store, _ := testService(t)
	in := validOrderInput()
	in.DiscountTotal = 100_000
	in.ShippingTotal = 50_000
	in.TaxTotal = 90_000 // 9% VAT on (1000000 - 100000)
	oid, err := svc.CreateOrder(context.Background(), in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	o, _ := store.GetOrder(context.Background(), oid)
	// grand = 1000000 - 100000 + 50000 + 90000 = 1040000
	if o.GrandTotal != 1_040_000 {
		t.Fatalf("grand_total=%d want 1040000", o.GrandTotal)
	}
}

func TestCreateOrder_RejectsInvalidSupplyType(t *testing.T) {
	svc, _, _ := testService(t)
	in := validOrderInput()
	in.Items[0].SupplyType = "INVALID"
	_, err := svc.CreateOrder(context.Background(), in)
	if err == nil {
		t.Fatal("expected error for invalid supply_type")
	}
}

// ── ConfirmPaymentForOrder ────────────────────────────────────────────────────

func TestConfirmPayment_MarksOrderPaidAndEnqueuesEvent(t *testing.T) {
	svc, store, q := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	if _, err := svc.CreatePaymentIntent(context.Background(), oid); err != nil {
		t.Fatalf("intent: %v", err)
	}

	if err := svc.ConfirmPaymentForOrder(context.Background(), oid); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	o, _ := store.GetOrder(context.Background(), oid)
	if o.Status != OrderPaid {
		t.Fatalf("status=%s want PAID", o.Status)
	}
	if o.PaidTotal != o.GrandTotal {
		t.Fatalf("paid_total=%d want %d", o.PaidTotal, o.GrandTotal)
	}

	evs := q.Events()
	// OrderCreated + OrderPaid
	if len(evs) != 2 {
		t.Fatalf("events=%d want 2", len(evs))
	}
	if evs[1].eventType != EventOrderPaid {
		t.Fatalf("second event=%s want OrderPaid", evs[1].eventType)
	}
}

func TestConfirmPayment_RejectsAlreadyPaid(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	svc.CreatePaymentIntent(context.Background(), oid)
	if err := svc.ConfirmPaymentForOrder(context.Background(), oid); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	err := svc.ConfirmPaymentForOrder(context.Background(), oid)
	if !errors.Is(err, ErrAlreadyPaid) {
		t.Fatalf("expected ErrAlreadyPaid, got %v", err)
	}
}

func TestConfirmPayment_RejectsWhenNoIntent(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	err := svc.ConfirmPaymentForOrder(context.Background(), oid)
	if !errors.Is(err, ErrIntentNotSucceeded) {
		t.Fatalf("expected ErrIntentNotSucceeded, got %v", err)
	}
}

func TestCreatePaymentIntent_RejectsDuplicateActive(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	if _, err := svc.CreatePaymentIntent(context.Background(), oid); err != nil {
		t.Fatalf("first intent: %v", err)
	}
	_, err := svc.CreatePaymentIntent(context.Background(), oid)
	if !errors.Is(err, ErrIntentExists) {
		t.Fatalf("expected ErrIntentExists, got %v", err)
	}
}

// ── CancelOrder ───────────────────────────────────────────────────────────────

func TestCancelOrder_PendingPaymentSucceeds(t *testing.T) {
	svc, store, q := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	if err := svc.CancelOrder(context.Background(), oid, "customer request"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	o, _ := store.GetOrder(context.Background(), oid)
	if o.Status != OrderCancelled {
		t.Fatalf("status=%s want CANCELLED", o.Status)
	}
	evs := q.Events()
	// OrderCreated + OrderCancelled
	if len(evs) != 2 || evs[1].eventType != EventOrderCancelled {
		t.Fatalf("events=%v", evs)
	}
}

func TestCancelOrder_PaidOrderFails(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	svc.CreatePaymentIntent(context.Background(), oid)
	svc.ConfirmPaymentForOrder(context.Background(), oid)
	err := svc.CancelOrder(context.Background(), oid, "test")
	if !errors.Is(err, ErrCannotCancel) {
		t.Fatalf("expected ErrCannotCancel for paid order, got %v", err)
	}
}

func TestCancelOrder_NotFoundFails(t *testing.T) {
	svc, _, _ := testService(t)
	err := svc.CancelOrder(context.Background(), "no-such-order", "test")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

// ── GetOrderDetail ────────────────────────────────────────────────────────────

func TestGetOrderDetail_ReturnsOrderWithItems(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	d, err := svc.GetOrderDetail(context.Background(), oid)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(d.Items) != 1 {
		t.Fatalf("items=%d want 1", len(d.Items))
	}
	if d.Items[0].SKUSnapshot != "SKU-001" {
		t.Fatalf("sku=%s", d.Items[0].SKUSnapshot)
	}
}

func TestGetOrderDetail_NotFound(t *testing.T) {
	svc, _, _ := testService(t)
	_, err := svc.GetOrderDetail(context.Background(), "no-such")
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestGetOrderByNumber_FindsOrder(t *testing.T) {
	svc, _, _ := testService(t)
	oid, _ := svc.CreateOrder(context.Background(), validOrderInput())
	o, err := svc.GetOrderByNumber(context.Background(), "KHA-ORD-2026-000001")
	if err != nil {
		t.Fatalf("by number: %v", err)
	}
	if o.ID != oid {
		t.Fatalf("id=%s want %s", o.ID, oid)
	}
}

// ── Nil EventEnqueuer ─────────────────────────────────────────────────────────

func TestService_NilQueueDoesNotPanic(t *testing.T) {
	store := newMemStore()
	svc := NewService(store, nil)
	oid, err := svc.CreateOrder(context.Background(), validOrderInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if oid == "" {
		t.Fatal("empty oid")
	}
}

// ── NormalizeOrderNumber ──────────────────────────────────────────────────────

func TestNormalizeOrderNumber(t *testing.T) {
	if got := NormalizeOrderNumber(" kha-ord-2026-000001 "); got != "KHA-ORD-2026-000001" {
		t.Fatalf("got %q", got)
	}
}

// guard against unused time import
var _ = time.Now
