package fulfillment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory fulfillment.Store for unit tests.
type memStore struct {
	mu             sync.Mutex
	fulfill        map[string]*FulfillmentOrder
	byOrder        map[string][]string // orderID → []fulfillmentID
	items          map[string]*FulfillmentItem
	itemsByFO      map[string][]string // fulfillmentOrderID → []itemID
	shipments      map[string]*Shipment
	shipByFO       map[string][]string
	shipItems      map[string]*ShipmentItem
	shipItemByShip map[string][]string
	events         map[string][]ShipmentEvent // shipmentID → events
	seq            int
}

func newMemStore() *memStore {
	return &memStore{
		fulfill:        map[string]*FulfillmentOrder{},
		byOrder:        map[string][]string{},
		items:          map[string]*FulfillmentItem{},
		itemsByFO:      map[string][]string{},
		shipments:      map[string]*Shipment{},
		shipByFO:       map[string][]string{},
		shipItems:      map[string]*ShipmentItem{},
		shipItemByShip: map[string][]string{},
		events:         map[string][]ShipmentEvent{},
	}
}

func (m *memStore) CreateFulfillmentOrder(_ context.Context, fo FulfillmentOrder) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "fo-" + itoa(m.seq)
	fo.ID = id
	fo.CreatedAt = time.Now()
	fo.UpdatedAt = fo.CreatedAt
	m.fulfill[id] = &fo
	m.byOrder[fo.OrderID] = append(m.byOrder[fo.OrderID], id)
	return id, nil
}

func (m *memStore) GetFulfillmentOrder(_ context.Context, id string) (*FulfillmentOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fo, ok := m.fulfill[id]
	if !ok {
		return nil, nil
	}
	cp := *fo
	return &cp, nil
}

func (m *memStore) ListFulfillmentOrdersByOrder(_ context.Context, orderID string) ([]FulfillmentOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.byOrder[orderID]
	out := make([]FulfillmentOrder, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.fulfill[id])
	}
	return out, nil
}

func (m *memStore) UpdateFulfillmentOrderStatus(_ context.Context, id, status string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	fo, ok := m.fulfill[id]
	if !ok {
		return ErrFulfillmentNotFound
	}
	fo.Status = status
	fo.UpdatedAt = now
	return nil
}

func (m *memStore) CreateFulfillmentItem(_ context.Context, fi FulfillmentItem) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "fi-" + itoa(m.seq)
	fi.ID = id
	fi.CreatedAt = time.Now()
	m.items[id] = &fi
	m.itemsByFO[fi.FulfillmentOrderID] = append(m.itemsByFO[fi.FulfillmentOrderID], id)
	return id, nil
}

func (m *memStore) GetFulfillmentItem(_ context.Context, id string) (*FulfillmentItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fi, ok := m.items[id]
	if !ok {
		return nil, nil
	}
	cp := *fi
	return &cp, nil
}

func (m *memStore) SumAllocatedForOrderItem(_ context.Context, orderItemID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sum := 0
	for _, fi := range m.items {
		if fi.OrderItemID == orderItemID && fi.Status != ItemCancelled && fi.Status != ItemRejected {
			sum += fi.Quantity
		}
	}
	return sum, nil
}

func (m *memStore) ListFulfillmentItemsByFulfillment(_ context.Context, fulfillmentOrderID string) ([]FulfillmentItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.itemsByFO[fulfillmentOrderID]
	out := make([]FulfillmentItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.items[id])
	}
	return out, nil
}

func (m *memStore) UpdateFulfillmentItemStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	fi, ok := m.items[id]
	if !ok {
		return ErrFulfillmentItemNotFound
	}
	fi.Status = status
	return nil
}

func (m *memStore) CreateShipment(_ context.Context, sh Shipment) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "sh-" + itoa(m.seq)
	sh.ID = id
	sh.CreatedAt = time.Now()
	m.shipments[id] = &sh
	m.shipByFO[sh.FulfillmentOrderID] = append(m.shipByFO[sh.FulfillmentOrderID], id)
	return id, nil
}

func (m *memStore) GetShipment(_ context.Context, id string) (*Shipment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sh, ok := m.shipments[id]
	if !ok {
		return nil, nil
	}
	cp := *sh
	return &cp, nil
}

func (m *memStore) ListShipmentsByFulfillment(_ context.Context, fulfillmentOrderID string) ([]Shipment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.shipByFO[fulfillmentOrderID]
	out := make([]Shipment, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.shipments[id])
	}
	return out, nil
}

func (m *memStore) UpdateShipmentStatus(_ context.Context, id, status string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sh, ok := m.shipments[id]
	if !ok {
		return ErrShipmentNotFound
	}
	sh.Status = status
	if status == ShipmentInTransit && sh.ShippedAt == nil {
		sh.ShippedAt = &now
	}
	if status == ShipmentDelivered {
		sh.DeliveredAt = &now
	}
	return nil
}

func (m *memStore) CreateShipmentItem(_ context.Context, si ShipmentItem) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "si-" + itoa(m.seq)
	si.ID = id
	m.shipItems[id] = &si
	m.shipItemByShip[si.ShipmentID] = append(m.shipItemByShip[si.ShipmentID], id)
	return id, nil
}

func (m *memStore) ListShipmentItemsByShipment(_ context.Context, shipmentID string) ([]ShipmentItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.shipItemByShip[shipmentID]
	out := make([]ShipmentItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, *m.shipItems[id])
	}
	return out, nil
}

func (m *memStore) RecordShipmentEvent(_ context.Context, ev ShipmentEvent) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "ev-" + itoa(m.seq)
	ev.ID = id
	ev.CreatedAt = time.Now()
	m.events[ev.ShipmentID] = append(m.events[ev.ShipmentID], ev)
	return id, nil
}

func (m *memStore) ListShipmentEvents(_ context.Context, shipmentID string) ([]ShipmentEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	evs := m.events[shipmentID]
	out := make([]ShipmentEvent, len(evs))
	copy(out, evs)
	// Reverse to newest-first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
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

func TestCreateFulfillmentOrder_SourceConsistency(t *testing.T) {
	svc := NewService(newMemStore())
	// OWNED_INVENTORY without warehouse_id → error.
	if _, err := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory,
	}); !errors.Is(err, ErrSourceTypeMismatch) {
		t.Fatalf("owned without warehouse: %v", err)
	}
	// SUPPLIER_SOURCED without supplier_id → error.
	if _, err := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceSupplierSourced,
	}); !errors.Is(err, ErrSourceTypeMismatch) {
		t.Fatalf("supplier without supplier_id: %v", err)
	}
	// OWNED_INVENTORY with both warehouse and supplier → error.
	if _, err := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory,
		WarehouseID: "wh-1", SupplierID: "sup-1",
	}); !errors.Is(err, ErrSourceTypeMismatch) {
		t.Fatalf("both sources: %v", err)
	}
	// OWNED_INVENTORY with warehouse only → ok.
	id, err := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	if err != nil {
		t.Fatalf("valid owned: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
}

func TestCreateFulfillmentOrder_RequiresOrderID(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	}); !errors.Is(err, ErrOrderIDRequired) {
		t.Fatalf("missing order_id: %v", err)
	}
}

func TestFulfillmentOrderStatusTransitions(t *testing.T) {
	cases := []struct {
		name   string
		from   string
		action func(*Service, context.Context, string) error
		wantOK bool
	}{
		{"PENDING→CONFIRMED", FulfillmentPending, func(s *Service, ctx context.Context, id string) error { return s.ConfirmFulfillmentOrder(ctx, id) }, true},
		{"CONFIRMED→PROCESSING", FulfillmentConfirmed, func(s *Service, ctx context.Context, id string) error { return s.StartProcessing(ctx, id) }, true},
		{"PROCESSING→READY", FulfillmentProcessing, func(s *Service, ctx context.Context, id string) error { return s.MarkReady(ctx, id) }, true},
		{"READY→SHIPPED", FulfillmentReady, func(s *Service, ctx context.Context, id string) error { return s.MarkShipped(ctx, id) }, true},
		{"SHIPPED→DELIVERED", FulfillmentShipped, func(s *Service, ctx context.Context, id string) error { return s.MarkDelivered(ctx, id) }, true},
		// Invalid transitions:
		{"PENDING→SHIPPED (skip)", FulfillmentPending, func(s *Service, ctx context.Context, id string) error { return s.MarkShipped(ctx, id) }, false},
		{"DELIVERED→SHIPPED (terminal)", FulfillmentDelivered, func(s *Service, ctx context.Context, id string) error { return s.MarkShipped(ctx, id) }, false},
		{"SHIPPED→CONFIRMED (backward)", FulfillmentShipped, func(s *Service, ctx context.Context, id string) error { return s.ConfirmFulfillmentOrder(ctx, id) }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newMemStore()
			svc := NewService(store)
			id, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
				OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
			})
			// Force the from-status.
			store.fulfill[id].Status = c.from
			err := c.action(svc, context.Background(), id)
			if c.wantOK {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
			} else {
				if !errors.Is(err, ErrInvalidFulfillmentStatus) {
					t.Fatalf("expected ErrInvalidFulfillmentStatus, got %v", err)
				}
			}
		})
	}
}

func TestCancelFulfillmentOrder(t *testing.T) {
	cases := []struct {
		from   string
		wantOK bool
	}{
		{FulfillmentPending, true},
		{FulfillmentConfirmed, true},
		{FulfillmentProcessing, true},
		{FulfillmentReady, true},
		{FulfillmentShipped, false}, // shipped cannot be cancelled
		{FulfillmentDelivered, false},
	}
	for _, c := range cases {
		t.Run("cancel_"+c.from, func(t *testing.T) {
			store := newMemStore()
			svc := NewService(store)
			id, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
				OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
			})
			store.fulfill[id].Status = c.from
			err := svc.CancelFulfillmentOrder(context.Background(), id)
			if c.wantOK && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !c.wantOK && !errors.Is(err, ErrInvalidFulfillmentStatus) {
				t.Fatalf("expected ErrInvalidFulfillmentStatus, got %v", err)
			}
		})
	}
}

func TestAddFulfillmentItem_AllocationInvariant(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	// Order item has quantity 10.
	if err := svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: 6,
	}, 10); err != nil {
		t.Fatalf("first allocation 6/10: %v", err)
	}
	// Allocate 5 more → total 11 > 10, must fail.
	if err := svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: 5,
	}, 10); !errors.Is(err, ErrAllocationExceedsOrder) {
		t.Fatalf("over-allocation must fail, got %v", err)
	}
	// Allocate exactly 4 more → total 10 = 10, ok (boundary).
	if err := svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: 4,
	}, 10); err != nil {
		t.Fatalf("boundary allocation 4/4: %v", err)
	}
	// Verify sum = 10.
	store.mu.Lock()
	sum := 0
	for _, fi := range store.items {
		if fi.OrderItemID == "oi-1" {
			sum += fi.Quantity
		}
	}
	store.mu.Unlock()
	if sum != 10 {
		t.Fatalf("total allocated=%d want 10", sum)
	}
}

func TestAddFulfillmentItem_RejectsZeroOrNegative(t *testing.T) {
	svc := NewService(newMemStore())
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	if err := svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: 0,
	}, 10); !errors.Is(err, ErrQuantityMustBePositive) {
		t.Fatalf("zero qty: %v", err)
	}
	if err := svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: -1,
	}, 10); !errors.Is(err, ErrQuantityMustBePositive) {
		t.Fatalf("negative qty: %v", err)
	}
}

func TestCreateShipment_RequiresCarrierAndReadyFulfillment(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	// Missing carrier → error.
	if _, err := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID,
	}); !errors.Is(err, ErrCarrierRequired) {
		t.Fatalf("missing carrier: %v", err)
	}
	// Fulfillment in PENDING → cannot ship yet.
	if _, err := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID, Carrier: "Tipax",
	}); !errors.Is(err, ErrInvalidFulfillmentStatus) {
		t.Fatalf("ship on PENDING fulfillment: %v", err)
	}
	// Move to READY → can ship.
	store.fulfill[foID].Status = FulfillmentReady
	shID, err := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID, Carrier: "Tipax", TrackingNumber: "TPX-001", ShippingCost: 50000,
	})
	if err != nil {
		t.Fatalf("create shipment: %v", err)
	}
	if shID == "" {
		t.Fatal("empty shipment id")
	}
}

func TestShipmentStatusTransitions(t *testing.T) {
	cases := []struct {
		name   string
		from   string
		to     string
		wantOK bool
	}{
		{"LABEL→IN_TRANSIT", ShipmentLabelCreated, ShipmentInTransit, true},
		{"IN_TRANSIT→OUT_FOR_DELIVERY", ShipmentInTransit, ShipmentOutForDelivery, true},
		{"OUT_FOR_DELIVERY→DELIVERED", ShipmentOutForDelivery, ShipmentDelivered, true},
		{"OUT_FOR_DELIVERY→FAILED", ShipmentOutForDelivery, ShipmentFailed, true},
		{"FAILED→RETURNED", ShipmentFailed, ShipmentReturned, true},
		// Invalid:
		{"LABEL→DELIVERED (skip)", ShipmentLabelCreated, ShipmentDelivered, false},
		{"DELIVERED→IN_TRANSIT (terminal)", ShipmentDelivered, ShipmentInTransit, false},
		{"RETURNED→IN_TRANSIT (terminal)", ShipmentReturned, ShipmentInTransit, false},
		{"DELIVERED→FAILED (terminal)", ShipmentDelivered, ShipmentFailed, false},
		{"LABEL→RETURNED (skip)", ShipmentLabelCreated, ShipmentReturned, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := newMemStore()
			svc := NewService(store)
			foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
				OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
			})
			store.fulfill[foID].Status = FulfillmentReady
			shID, _ := svc.CreateShipment(context.Background(), Shipment{
				FulfillmentOrderID: foID, Carrier: "Tipax",
			})
			// Force the from-status.
			store.shipments[shID].Status = c.from
			err := svc.TransitionShipmentStatus(context.Background(), shID, c.to,
				"event_"+c.to, "test transition", EventSourceManual)
			if c.wantOK {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				sh, _ := store.GetShipment(context.Background(), shID)
				if sh.Status != c.to {
					t.Fatalf("status=%s want %s", sh.Status, c.to)
				}
			} else {
				if !errors.Is(err, ErrInvalidShipmentStatus) {
					t.Fatalf("expected ErrInvalidShipmentStatus, got %v", err)
				}
			}
		})
	}
}

func TestTransitionShipmentStatus_RecordsEvent(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	store.fulfill[foID].Status = FulfillmentReady
	shID, _ := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID, Carrier: "Tipax",
	})
	// Transition LABEL_CREATED → IN_TRANSIT, should record an event.
	if err := svc.TransitionShipmentStatus(context.Background(), shID, ShipmentInTransit,
		"picked_up", "Carrier picked up package", EventSourceCarrier); err != nil {
		t.Fatalf("transition: %v", err)
	}
	events, err := svc.ListShipmentEvents(context.Background(), shID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	if events[0].EventCode != "picked_up" || events[0].Source != EventSourceCarrier {
		t.Fatalf("event=%+v want picked_up/CARRIER_API", events[0])
	}
	// Transition IN_TRANSIT → OUT_FOR_DELIVERY, should record a second event.
	svc.TransitionShipmentStatus(context.Background(), shID, ShipmentOutForDelivery,
		"out_for_delivery", "Out for delivery", EventSourceWebhook)
	events, _ = svc.ListShipmentEvents(context.Background(), shID)
	if len(events) != 2 {
		t.Fatalf("events=%d want 2", len(events))
	}
	// Newest first: events[0] = out_for_delivery.
	if events[0].EventCode != "out_for_delivery" {
		t.Fatalf("newest event=%s want out_for_delivery", events[0].EventCode)
	}
}

func TestTransitionShipmentStatus_SetsShippedAtOnFirstInTransit(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc := NewService(store)
	svc.SetNow(func() time.Time { return now })
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	store.fulfill[foID].Status = FulfillmentReady
	shID, _ := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID, Carrier: "Tipax",
	})
	svc.TransitionShipmentStatus(context.Background(), shID, ShipmentInTransit, "x", "", "")
	sh, _ := store.GetShipment(context.Background(), shID)
	if sh.ShippedAt == nil || !sh.ShippedAt.Equal(now) {
		t.Fatalf("shipped_at=%v want %v", sh.ShippedAt, now)
	}
	// Transition back is blocked, so shipped_at stays.
}

func TestListShipmentEvents_EmptyForUnknown(t *testing.T) {
	svc := NewService(newMemStore())
	events, err := svc.ListShipmentEvents(context.Background(), "sh-unknown")
	if err != nil {
		t.Fatalf("list events for unknown: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events=%d want 0", len(events))
	}
}

func TestListFulfillmentOrdersByOrder(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	// Two fulfillment orders for one order (one owned, one supplier-sourced).
	svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceSupplierSourced, SupplierID: "sup-1",
	})
	// One fulfillment for a different order.
	svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-2", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	fos, err := svc.ListFulfillmentOrdersByOrder(context.Background(), "o-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(fos) != 2 {
		t.Fatalf("fulfillments for o-1=%d want 2", len(fos))
	}
}

func TestAddShipmentItem(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceOwnedInventory, WarehouseID: "wh-1",
	})
	store.fulfill[foID].Status = FulfillmentReady
	shID, _ := svc.CreateShipment(context.Background(), Shipment{
		FulfillmentOrderID: foID, Carrier: "Tipax",
	})
	// Add a fulfillment item first.
	svc.AddFulfillmentItem(context.Background(), FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: "oi-1", Quantity: 10,
	}, 10)
	fiID := ""
	store.mu.Lock()
	for id := range store.items {
		fiID = id
		break
	}
	store.mu.Unlock()
	// Split into 2 shipment items (6 + 4).
	if _, err := svc.AddShipmentItem(context.Background(), ShipmentItem{
		ShipmentID: shID, FulfillmentItemID: fiID, Quantity: 6,
	}); err != nil {
		t.Fatalf("first shipment item: %v", err)
	}
	if _, err := svc.AddShipmentItem(context.Background(), ShipmentItem{
		ShipmentID: shID, FulfillmentItemID: fiID, Quantity: 4,
	}); err != nil {
		t.Fatalf("second shipment item: %v", err)
	}
	items, _ := store.ListShipmentItemsByShipment(context.Background(), shID)
	if len(items) != 2 {
		t.Fatalf("shipment items=%d want 2", len(items))
	}
	// Reject zero/negative.
	if _, err := svc.AddShipmentItem(context.Background(), ShipmentItem{
		ShipmentID: shID, FulfillmentItemID: fiID, Quantity: 0,
	}); !errors.Is(err, ErrQuantityMustBePositive) {
		t.Fatalf("zero qty: %v", err)
	}
}

func TestRejectFulfillmentOrder(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	foID, _ := svc.CreateFulfillmentOrder(context.Background(), FulfillmentOrder{
		OrderID: "o-1", SourceType: SourceSupplierSourced, SupplierID: "sup-1",
	})
	// PENDING → REJECTED is ok (supplier refused).
	if err := svc.RejectFulfillmentOrder(context.Background(), foID); err != nil {
		t.Fatalf("reject from PENDING: %v", err)
	}
	// SHIPPED → REJECTED must fail (too late).
	store.fulfill[foID].Status = FulfillmentShipped
	if err := svc.RejectFulfillmentOrder(context.Background(), foID); !errors.Is(err, ErrInvalidFulfillmentStatus) {
		t.Fatalf("reject from SHIPPED must fail: %v", err)
	}
}
