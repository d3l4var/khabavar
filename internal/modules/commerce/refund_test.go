package commerce

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// memRefundStore is an in-memory RefundStore for unit tests.
type memRefundStore struct {
	mu       sync.Mutex
	refunds  map[string]*Refund
	byOrder  map[string][]string
	attempts map[string]int64 // orderID → sum of succeeded attempts
	seq      int
	// Assertion fields captured by UpdateOrderRefunded.
	lastRefundedTotal int64
	lastStatus        string
	lastOrderID       string
}

func newMemRefundStore() *memRefundStore {
	return &memRefundStore{
		refunds:  map[string]*Refund{},
		byOrder:  map[string][]string{},
		attempts: map[string]int64{},
	}
}

func (m *memRefundStore) CreateRefund(_ context.Context, r Refund) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "rf-" + itoa(m.seq)
	r.ID = id
	r.CreatedAt = time.Now()
	m.refunds[id] = &r
	m.byOrder[r.OrderID] = append(m.byOrder[r.OrderID], id)
	return id, nil
}

func (m *memRefundStore) ListRefunds(_ context.Context, orderID string) ([]Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byOrder[orderID]
	out := make([]Refund, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.refunds[id])
	}
	return out, nil
}

func (m *memRefundStore) UpdateOrderRefunded(_ context.Context, id string, refundedTotal int64, status string) error {
	// In tests, the stubOrderStore owns the orders; this records the call for
	// assertion (the test checks lastStatus/lastRefundedTotal).
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastRefundedTotal = refundedTotal
	m.lastStatus = status
	m.lastOrderID = id
	return nil
}

func (m *memRefundStore) SumSucceededAttempts(_ context.Context, orderID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attempts[orderID], nil
}

// SuccessfulAttemptID returns a synthetic attempt id when the order has a succeeded
// attempt (attempts[orderID] > 0), mirroring the DB rule that a refund must reference
// the payment it reverses.
func (m *memRefundStore) SuccessfulAttemptID(_ context.Context, orderID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.attempts[orderID] <= 0 {
		return "", nil
	}
	return "pa-" + orderID, nil
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestCreateRefund_PartialRefund(t *testing.T) {
	store := newMemRefundStore()
	store.attempts["o-1"] = 100000 // 100k IRR succeeded
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	svc.SetNow(func() time.Time { return time.Now() })
	// Seed an order in the memStore. We need a Store that returns the order;
	// use a stub.
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPaid, Currency: "IRR", PaidTotal: 100000, RefundedTotal: 0},
	}}
	svc.store = stubStore

	id, err := svc.CreateRefund(context.Background(), "o-1", 30000, "customer returned item")
	if err != nil {
		t.Fatalf("create refund: %v", err)
	}
	if id == "" {
		t.Fatal("empty refund id")
	}
	// A partial refund leaves the order status untouched (DB v2 §10.3 has no
	// PARTIALLY_REFUNDED status) but accumulates refunded_total.
	if store.lastStatus != OrderPaid {
		t.Fatalf("status=%s want %s (partial refund must not change order status)", store.lastStatus, OrderPaid)
	}
	if store.lastRefundedTotal != 30000 {
		t.Fatalf("refunded_total=%d want 30000", store.lastRefundedTotal)
	}
}

func TestCreateRefund_FullRefundClosesOrder(t *testing.T) {
	store := newMemRefundStore()
	store.attempts["o-1"] = 100000
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	svc.SetNow(func() time.Time { return time.Now() })
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderDelivered, Currency: "IRR", PaidTotal: 100000, RefundedTotal: 0},
	}}
	svc.store = stubStore

	_, err := svc.CreateRefund(context.Background(), "o-1", 100000, "order cancelled after delivery")
	if err != nil {
		t.Fatalf("full refund: %v", err)
	}
	if store.lastStatus != OrderClosed {
		t.Fatalf("status=%s want %s (full refund closes the order)", store.lastStatus, OrderClosed)
	}
	if store.lastRefundedTotal != 100000 {
		t.Fatalf("refunded_total=%d want 100000", store.lastRefundedTotal)
	}
}

func TestCreateRefund_RejectsExceedingAvailable(t *testing.T) {
	store := newMemRefundStore()
	store.attempts["o-1"] = 100000
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	svc.SetNow(func() time.Time { return time.Now() })
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPaid, Currency: "IRR", PaidTotal: 100000, RefundedTotal: 40000},
	}}
	svc.store = stubStore
	// Available = 100000 − 40000 = 60000. Refund 70000 → must fail.
	_, err := svc.CreateRefund(context.Background(), "o-1", 70000, "too much")
	if !errors.Is(err, ErrRefundExceedsPaid) {
		t.Fatalf("expected ErrRefundExceedsPaid, got %v", err)
	}
}

func TestCreateRefund_RejectsNonRefundableStatus(t *testing.T) {
	store := newMemRefundStore()
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	svc.SetNow(func() time.Time { return time.Now() })
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPendingPayment, Currency: "IRR", PaidTotal: 0, RefundedTotal: 0},
	}}
	svc.store = stubStore
	_, err := svc.CreateRefund(context.Background(), "o-1", 1000, "x")
	if !errors.Is(err, ErrOrderNotRefundable) {
		t.Fatalf("expected ErrOrderNotRefundable, got %v", err)
	}
	// CANCELLED also not refundable.
	stubStore.orders["o-1"].Status = OrderCancelled
	_, err = svc.CreateRefund(context.Background(), "o-1", 1000, "x")
	if !errors.Is(err, ErrOrderNotRefundable) {
		t.Fatalf("cancelled not refundable: %v", err)
	}
}

func TestCreateRefund_RejectsZeroAmountAndMissingReason(t *testing.T) {
	svc := NewService(nil, nil)
	svc.SetRefundStore(newMemRefundStore())
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPaid, PaidTotal: 1000},
	}}
	svc.store = stubStore
	if _, err := svc.CreateRefund(context.Background(), "o-1", 0, "r"); !errors.Is(err, ErrRefundAmountInvalid) {
		t.Fatalf("zero amount: %v", err)
	}
	if _, err := svc.CreateRefund(context.Background(), "o-1", 100, ""); !errors.Is(err, ErrRefundReasonReq) {
		t.Fatalf("missing reason: %v", err)
	}
}

func TestReconcileTransaction_Reconciled(t *testing.T) {
	store := newMemRefundStore()
	store.attempts["o-1"] = 100000 // succeeded attempts
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPaid, PaidTotal: 100000, RefundedTotal: 20000},
	}}
	svc.store = stubStore
	// paid_total equals the captured payments (100000) → reconciled; refunds are a
	// separate accumulator, reported as NetPaid.
	res, err := svc.ReconcileTransaction(context.Background(), "o-1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !res.Reconciled {
		t.Fatalf("expected reconciled: paid=%d succeeded=%d", res.PaidTotal, res.SucceededAttempts)
	}
	if res.NetPaid != 80000 {
		t.Fatalf("net_paid=%d want 80000", res.NetPaid)
	}
}

func TestReconcileTransaction_NotReconciled(t *testing.T) {
	store := newMemRefundStore()
	store.attempts["o-1"] = 100000
	svc := NewService(nil, nil)
	svc.SetRefundStore(store)
	stubStore := &stubOrderStore{orders: map[string]*Order{
		"o-1": {ID: "o-1", Status: OrderPaid, PaidTotal: 70000, RefundedTotal: 20000},
	}}
	svc.store = stubStore
	// paid_total (70000) ≠ captured payments (100000) → NOT reconciled (money missing).
	res, _ := svc.ReconcileTransaction(context.Background(), "o-1")
	if res.Reconciled {
		t.Fatal("expected NOT reconciled")
	}
}

// NextOrderNumber satisfies Store for the refund tests, which never create orders.
func (s *stubOrderStore) NextOrderNumber(_ context.Context, year int) (string, error) {
	return fmt.Sprintf("KHA-ORD-%04d-000001", year), nil
}

// stubOrderStore is a minimal Store for refund tests (only GetOrder needed).
type stubOrderStore struct {
	orders map[string]*Order
}

func (s *stubOrderStore) GetOrder(_ context.Context, id string) (*Order, error) {
	o, ok := s.orders[id]
	if !ok {
		return nil, nil
	}
	cp := *o
	return &cp, nil
}

// The other Store methods are not used by refund tests; stub them as panics.
func (s *stubOrderStore) GetCart(context.Context, string) (*Cart, []CartItem, error) {
	panic("not used")
}
func (s *stubOrderStore) UpdateCartStatus(context.Context, string, string) error { panic("not used") }
func (s *stubOrderStore) CreateOrder(context.Context, Order, []OrderItem) (string, error) {
	panic("not used")
}
func (s *stubOrderStore) GetOrderByNumber(context.Context, string) (*Order, error) { panic("not used") }
func (s *stubOrderStore) ListOrderItems(context.Context, string) ([]OrderItem, error) {
	panic("not used")
}
func (s *stubOrderStore) UpdateOrderStatus(context.Context, string, string) error { panic("not used") }
func (s *stubOrderStore) OrderIDForIntent(context.Context, string) (string, error) {
	panic("not used")
}
func (s *stubOrderStore) UpdateOrderPaid(context.Context, string, int64, string) error {
	panic("not used")
}
func (s *stubOrderStore) CreatePaymentIntent(context.Context, PaymentIntent) (string, error) {
	panic("not used")
}
func (s *stubOrderStore) GetActivePaymentIntent(context.Context, string) (*PaymentIntent, error) {
	panic("not used")
}
func (s *stubOrderStore) UpdatePaymentIntentStatus(context.Context, string, string) error {
	panic("not used")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
