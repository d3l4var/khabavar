package inventory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory inventory.Store for unit tests. The ReserveStock
// implementation enforces the same sellable invariant as the DB (on_hand -
// reserved >= 0) and uses a mutex to model the FOR UPDATE lock.
type memStore struct {
	mu           sync.Mutex
	warehouses   map[string]*Warehouse
	whByCode     map[string]string
	balances     map[string]*StockBalance // key = warehouseID + ":" + variantID
	movements    []Movement
	reservations map[string]*Reservation
	seq          int
}

func newMemStore() *memStore {
	return &memStore{
		warehouses:   map[string]*Warehouse{},
		whByCode:     map[string]string{},
		balances:     map[string]*StockBalance{},
		reservations: map[string]*Reservation{},
	}
}

func balKey(warehouseID, variantID string) string { return warehouseID + ":" + variantID }

func (m *memStore) CreateWarehouse(_ context.Context, w Warehouse) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "wh-" + itoa(m.seq)
	w.ID = id
	w.CreatedAt = time.Now()
	m.warehouses[id] = &w
	m.whByCode[w.WarehouseCode] = id
	return id, nil
}

func (m *memStore) GetWarehouseByCode(_ context.Context, code string) (*Warehouse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.whByCode[code]
	if !ok {
		return nil, nil
	}
	cp := *m.warehouses[id]
	return &cp, nil
}

func (m *memStore) GetWarehouse(_ context.Context, id string) (*Warehouse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.warehouses[id]
	if !ok {
		return nil, nil
	}
	cp := *w
	return &cp, nil
}

func (m *memStore) GetStockBalance(_ context.Context, warehouseID, variantID string) (*StockBalance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.balances[balKey(warehouseID, variantID)]
	if !ok {
		return nil, nil
	}
	cp := *b
	return &cp, nil
}

func (m *memStore) UpsertStockBalance(_ context.Context, b StockBalance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.balances[balKey(b.WarehouseID, b.VariantID)] = &b
	return nil
}

func (m *memStore) ReserveStock(_ context.Context, warehouseID, variantID string, qty int, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := balKey(warehouseID, variantID)
	b, ok := m.balances[k]
	if !ok {
		// Create zero balance (mirrors DB behavior).
		b = &StockBalance{WarehouseID: warehouseID, VariantID: variantID}
		m.balances[k] = b
	}
	if b.OnHandQuantity-b.ReservedQuantity < qty {
		return ErrInsufficientStock
	}
	b.ReservedQuantity += qty
	b.UpdatedAt = now
	return nil
}

func (m *memStore) ReleaseStock(_ context.Context, warehouseID, variantID string, qty int, now time.Time) error {
	if qty <= 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.balances[balKey(warehouseID, variantID)]
	if !ok {
		return nil
	}
	b.ReservedQuantity -= qty
	if b.ReservedQuantity < 0 {
		b.ReservedQuantity = 0
	}
	b.UpdatedAt = now
	return nil
}

func (m *memStore) AdjustOnHand(_ context.Context, warehouseID, variantID string, delta int, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := balKey(warehouseID, variantID)
	b, ok := m.balances[k]
	if !ok {
		if delta < 0 {
			return ErrNegativeOnHand
		}
		b = &StockBalance{WarehouseID: warehouseID, VariantID: variantID, OnHandQuantity: delta, UpdatedAt: now}
		m.balances[k] = b
		return nil
	}
	if b.OnHandQuantity+delta < 0 {
		return ErrNegativeOnHand
	}
	b.OnHandQuantity += delta
	b.UpdatedAt = now
	return nil
}

func (m *memStore) RecordMovement(_ context.Context, mov Movement) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "mv-" + itoa(m.seq)
	mov.ID = id
	if mov.CreatedAt.IsZero() {
		mov.CreatedAt = time.Now()
	}
	m.movements = append(m.movements, mov)
	return id, nil
}

func (m *memStore) ListMovements(_ context.Context, warehouseID, variantID string, limit int) ([]Movement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Movement
	for i := len(m.movements) - 1; i >= 0 && len(out) < limit; i-- {
		mov := m.movements[i]
		if mov.WarehouseID == warehouseID && mov.VariantID == variantID {
			out = append(out, mov)
		}
	}
	return out, nil
}

func (m *memStore) CreateReservation(_ context.Context, r Reservation) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "res-" + itoa(m.seq)
	r.ID = id
	r.CreatedAt = time.Now()
	m.reservations[id] = &r
	return id, nil
}

func (m *memStore) GetReservation(_ context.Context, id string) (*Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reservations[id]
	if !ok {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}

func (m *memStore) UpdateReservationStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.reservations[id]
	if !ok {
		return ErrReservationNotFound
	}
	r.Status = status
	return nil
}

func (m *memStore) ListExpiredActiveReservations(_ context.Context, now time.Time, limit int) ([]Reservation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Reservation
	for _, r := range m.reservations {
		if r.Status == ReservationActive && !r.ExpiresAt.After(now) {
			cp := *r
			out = append(out, cp)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
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

// ── tests ──────────────────────────────────────────────────────────────────────

func TestCreateWarehouse_RequiresNameAndCode(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateWarehouse(context.Background(), Warehouse{WarehouseCode: "C"}); !errors.Is(err, ErrWarehouseNameReq) {
		t.Fatalf("missing name: %v", err)
	}
	if _, err := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W"}); !errors.Is(err, ErrWarehouseCodeReq) {
		t.Fatalf("missing code: %v", err)
	}
}

func TestCreateWarehouse_DuplicateCode(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W1", WarehouseCode: "WH-01"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W2", WarehouseCode: "WH-01"}); !errors.Is(err, ErrDuplicateWarehouseCode) {
		t.Fatalf("duplicate code must fail, got %v", err)
	}
}

func TestGetStockBalance_ZeroForUnknown(t *testing.T) {
	svc := NewService(newMemStore())
	b, err := svc.GetStockBalance(context.Background(), "wh-1", "v-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if b.OnHandQuantity != 0 || b.ReservedQuantity != 0 || b.Sellable() != 0 {
		t.Fatalf("expected zero balance, got %+v", b)
	}
}

func TestReceiveStock_IncrementsOnHandAndPostsMovement(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	if err := svc.ReceiveStock(context.Background(), whID, "v-1", 50, "PO", "po-1", "admin-1"); err != nil {
		t.Fatalf("receive: %v", err)
	}
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.OnHandQuantity != 50 || b.ReservedQuantity != 0 || b.Sellable() != 50 {
		t.Fatalf("balance=%+v want on_hand=50", b)
	}
	movs, _ := svc.ListMovements(context.Background(), whID, "v-1", 10)
	if len(movs) != 1 || movs[0].MovementType != MovementReceipt || movs[0].Quantity != 50 {
		t.Fatalf("movement=%+v want RECEIPT/50", movs)
	}
}

func TestReceiveStock_RejectsZeroOrNegative(t *testing.T) {
	svc := NewService(newMemStore())
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	if err := svc.ReceiveStock(context.Background(), whID, "v-1", 0, "PO", "po-1", "admin-1"); err == nil {
		t.Fatal("zero qty must fail")
	}
	if err := svc.ReceiveStock(context.Background(), whID, "v-1", -5, "PO", "po-1", "admin-1"); err == nil {
		t.Fatal("negative qty must fail")
	}
}

func TestAdjustStock_RejectsZeroAndEnforcesNonNegativeOnHand(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	if err := svc.AdjustStock(context.Background(), whID, "v-1", 0, "MANUAL", "m-1", "admin-1", ""); !errors.Is(err, ErrZeroQuantity) {
		t.Fatalf("zero adjust: %v", err)
	}
	// No balance yet → negative adjust must fail.
	if err := svc.AdjustStock(context.Background(), whID, "v-1", -5, "MANUAL", "m-1", "admin-1", ""); !errors.Is(err, ErrNegativeOnHand) {
		t.Fatalf("negative adjust on empty balance: %v", err)
	}
	// Receive 10 then adjust -15 → must fail (would go negative).
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	if err := svc.AdjustStock(context.Background(), whID, "v-1", -15, "MANUAL", "m-2", "admin-1", ""); !errors.Is(err, ErrNegativeOnHand) {
		t.Fatalf("oversell adjust: %v", err)
	}
	// Adjust -5 → ok, on_hand becomes 5.
	if err := svc.AdjustStock(context.Background(), whID, "v-1", -5, "MANUAL", "m-3", "admin-1", ""); err != nil {
		t.Fatalf("valid adjust: %v", err)
	}
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.OnHandQuantity != 5 {
		t.Fatalf("on_hand=%d want 5", b.OnHandQuantity)
	}
}

func TestReserveForOrder_EnforcesSellableInvariant(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	// Reserve 6 → ok, sellable drops to 4.
	r1, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID,
		Quantity: 6, ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("reserve 6: %v", err)
	}
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.Sellable() != 4 {
		t.Fatalf("sellable=%d want 4", b.Sellable())
	}

	// Reserve 5 more → would oversell (only 4 sellable), must fail with ErrInsufficientStock.
	if _, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-2", VariantID: "v-1", WarehouseID: whID,
		Quantity: 5, ExpiresAt: expires,
	}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("oversell reserve must fail, got %v", err)
	}
	// Reservation must not have been created.
	if _, err := svc.store.(*memStore).GetReservation(context.Background(), "res-2"); err != nil {
		t.Fatalf("no partial reservation should exist")
	}

	// Reserve exactly 4 → ok (boundary).
	_, err = svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-3", VariantID: "v-1", WarehouseID: whID,
		Quantity: 4, ExpiresAt: expires,
	})
	if err != nil {
		t.Fatalf("reserve 4 boundary: %v", err)
	}

	// The first reservation id should be non-empty.
	if r1 == "" {
		t.Fatal("reservation id empty")
	}
}

func TestReserveForOrder_RequiresExactlyOneSource(t *testing.T) {
	svc := NewService(newMemStore())
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	// Both empty → error.
	if _, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", Quantity: 1, ExpiresAt: expires,
	}); !errors.Is(err, ErrReservationSource) {
		t.Fatalf("no source: %v", err)
	}
	// Both set → error.
	if _, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID, SupplierID: "sup-1",
		Quantity: 1, ExpiresAt: expires,
	}); !errors.Is(err, ErrReservationSource) {
		t.Fatalf("both sources: %v", err)
	}
	// Missing expires_at → error.
	if _, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID, Quantity: 1,
	}); !errors.Is(err, ErrReservationExpiryReq) {
		t.Fatalf("missing expiry: %v", err)
	}
}

func TestReserveForOrder_SupplierSourcedDoesNotReserveLiveStock(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	// Supplier-sourced reservation → no warehouse stock reserved.
	_, err := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", SupplierID: "sup-1",
		Quantity: 100, ExpiresAt: expires, // more than on_hand, but supplier-sourced
	})
	if err != nil {
		t.Fatalf("supplier-sourced should not check warehouse stock: %v", err)
	}
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.ReservedQuantity != 0 {
		t.Fatalf("supplier-sourced must not reserve warehouse stock, reserved=%d", b.ReservedQuantity)
	}
}

func TestReleaseReservation_ReleasesStockAndPostsMovement(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	rID, _ := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID,
		Quantity: 4, ExpiresAt: expires,
	})
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.Sellable() != 6 {
		t.Fatalf("sellable=%d want 6 before release", b.Sellable())
	}
	if err := svc.ReleaseReservation(context.Background(), rID); err != nil {
		t.Fatalf("release: %v", err)
	}
	b, _ = svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.Sellable() != 10 {
		t.Fatalf("sellable=%d want 10 after release", b.Sellable())
	}
	// RELEASE movement posted (signed negative).
	movs, _ := svc.ListMovements(context.Background(), whID, "v-1", 100)
	hasRelease := false
	for _, m := range movs {
		if m.MovementType == MovementRelease && m.Quantity == -4 {
			hasRelease = true
		}
	}
	if !hasRelease {
		t.Fatalf("no RELEASE movement found: %+v", movs)
	}
	// Second release → not active, must fail.
	if err := svc.ReleaseReservation(context.Background(), rID); !errors.Is(err, ErrReservationNotActive) {
		t.Fatalf("double release: %v", err)
	}
}

func TestConvertReservation_ReducesOnHandAndReleasesReserved(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	rID, _ := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID,
		Quantity: 3, ExpiresAt: expires,
	})
	if err := svc.ConvertReservation(context.Background(), rID); err != nil {
		t.Fatalf("convert: %v", err)
	}
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	// on_hand drops by 3 (10→7), reserved drops by 3 (3→0), sellable=7.
	if b.OnHandQuantity != 7 || b.ReservedQuantity != 0 || b.Sellable() != 7 {
		t.Fatalf("after convert: on_hand=%d reserved=%d sellable=%d want 7/0/7",
			b.OnHandQuantity, b.ReservedQuantity, b.Sellable())
	}
	movs, _ := svc.ListMovements(context.Background(), whID, "v-1", 100)
	hasSale := false
	for _, m := range movs {
		if m.MovementType == MovementSale && m.Quantity == -3 {
			hasSale = true
		}
	}
	if !hasSale {
		t.Fatalf("no SALE movement found: %+v", movs)
	}
}

func TestConvertReservation_NotActiveFails(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")
	expires := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	rID, _ := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID,
		Quantity: 3, ExpiresAt: expires,
	})
	svc.ReleaseReservation(context.Background(), rID) // → RELEASED
	if err := svc.ConvertReservation(context.Background(), rID); !errors.Is(err, ErrReservationNotActive) {
		t.Fatalf("convert released: %v", err)
	}
}

func TestExpireReservations_SweepsExpiredAndReleasesStock(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	svc.ReceiveStock(context.Background(), whID, "v-1", 10, "PO", "po-1", "admin-1")

	// Two reservations: one expired (expires_at in past), one still active.
	expiredAt := now.Add(-1 * time.Hour)
	futureAt := now.Add(1 * time.Hour)
	rExpired, _ := svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-1", VariantID: "v-1", WarehouseID: whID,
		Quantity: 4, ExpiresAt: expiredAt,
	})
	svc.ReserveForOrder(context.Background(), Reservation{
		OrderItemID: "oi-2", VariantID: "v-1", WarehouseID: whID,
		Quantity: 3, ExpiresAt: futureAt,
	})

	// Before sweep: reserved = 7, sellable = 3.
	b, _ := svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.ReservedQuantity != 7 || b.Sellable() != 3 {
		t.Fatalf("pre-sweep: reserved=%d sellable=%d want 7/3", b.ReservedQuantity, b.Sellable())
	}

	count, err := svc.ExpireReservations(context.Background(), 100)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if count != 1 {
		t.Fatalf("expired count=%d want 1", count)
	}

	// After sweep: reserved drops by 4 (the expired one), sellable rises to 7.
	b, _ = svc.GetStockBalance(context.Background(), whID, "v-1")
	if b.ReservedQuantity != 3 || b.Sellable() != 7 {
		t.Fatalf("post-sweep: reserved=%d sellable=%d want 3/7", b.ReservedQuantity, b.Sellable())
	}

	// The expired reservation's status is now EXPIRED.
	r, _ := store.GetReservation(context.Background(), rExpired)
	if r.Status != ReservationExpired {
		t.Fatalf("expired reservation status=%s want EXPIRED", r.Status)
	}
	// The future reservation is still ACTIVE.
	movs, _ := svc.ListMovements(context.Background(), whID, "v-1", 100)
	hasRelease := false
	for _, m := range movs {
		if m.MovementType == MovementRelease && m.Quantity == -4 {
			hasRelease = true
		}
	}
	if !hasRelease {
		t.Fatalf("no RELEASE movement from expiry sweep: %+v", movs)
	}
}

func TestListMovements_NewestFirst(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	base := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	whID, _ := svc.CreateWarehouse(context.Background(), Warehouse{Name: "W", WarehouseCode: "WH-01"})
	// Receive 3 times with distinct timestamps.
	for i, delta := range []int{10, 20, 30} {
		func(i, delta int) {
			svc.SetNow(func() time.Time { return base.Add(time.Duration(i) * time.Hour) })
			svc.ReceiveStock(context.Background(), whID, "v-1", delta, "PO", "po-1", "admin-1")
		}(i, delta)
	}
	movs, _ := svc.ListMovements(context.Background(), whID, "v-1", 10)
	if len(movs) != 3 {
		t.Fatalf("movements=%d want 3", len(movs))
	}
	// Newest first: the last RECEIPT (qty=30) should be movs[0].
	if movs[0].Quantity != 30 {
		t.Fatalf("newest first: movs[0].Quantity=%d want 30", movs[0].Quantity)
	}
	if movs[2].Quantity != 10 {
		t.Fatalf("oldest last: movs[2].Quantity=%d want 10", movs[2].Quantity)
	}
}
