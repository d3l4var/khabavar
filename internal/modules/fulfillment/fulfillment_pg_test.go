// fulfillment_pg_test.go exercises the fulfillment DBStore against the real schema: the
// state machine (PENDING → CONFIRMED → PROCESSING → READY → SHIPPED → DELIVERED), the
// §11.1 source rule, the §11.2 allocation invariant, split shipments (§11.4) and the
// append-only shipment timeline (§11.5).
//
// Skipped unless KH_TEST_PG_DSN is set.
package fulfillment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupFulfillment(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	store := NewDBStore(pool)
	return pool, NewService(store), store
}

// seedOrderItemGraph inserts order → order_item and returns (orderID, orderItemID).
func seedOrderItemGraph(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderNumber, sku string) (string, string) {
	t.Helper()
	var productID, variantID, orderID, itemID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ($1,$2,'DRAFT') RETURNING id`,
		"محصول "+sku, "p-"+sku).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO product_variants (product_id, sku, status) VALUES ($1,$2,'ACTIVE') RETURNING id`,
		productID, sku).Scan(&variantID); err != nil {
		t.Fatalf("seed variant: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO orders (order_number, guest_phone, status, subtotal, grand_total,
		                     shipping_address_snapshot)
		 VALUES ($1,'09120000002','PAID',1000,1000,'{}'::jsonb) RETURNING id`,
		orderNumber).Scan(&orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO order_items (order_id, product_id, variant_id, sku_snapshot, title_snapshot,
		                          quantity, unit_price, line_total, supply_type_snapshot)
		 VALUES ($1,$2,$3,$4,'x',3,1000,3000,'OWNED_INVENTORY') RETURNING id`,
		orderID, productID, variantID, sku).Scan(&itemID); err != nil {
		t.Fatalf("seed order item: %v", err)
	}
	return orderID, itemID
}

// seedWarehouse inserts a warehouse directly (fulfillment only needs the id).
func seedWarehouse(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO warehouses (warehouse_code, name) VALUES ($1,'انبار') RETURNING id`, code).Scan(&id); err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	return id
}

// TestPG_FulfillmentStateMachine walks the happy path and proves an illegal jump is
// refused without changing the row.
func TestPG_FulfillmentStateMachine(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000201", "SKU-FUL-1")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	fo, err := store.GetFulfillmentOrder(ctx, foID)
	if err != nil || fo == nil {
		t.Fatalf("GetFulfillmentOrder: %v", err)
	}
	if fo.Status != FulfillmentPending {
		t.Fatalf("initial status = %s, want PENDING", fo.Status)
	}

	// PENDING → SHIPPED is illegal: it must skip CONFIRMED/PROCESSING/READY.
	if err := svc.MarkShipped(ctx, foID); !errors.Is(err, ErrInvalidFulfillmentStatus) {
		t.Fatalf("PENDING→SHIPPED: want ErrInvalidFulfillmentStatus, got %v", err)
	}
	fo, _ = store.GetFulfillmentOrder(ctx, foID)
	if fo.Status != FulfillmentPending {
		t.Fatalf("a refused transition must not change the row, got %s", fo.Status)
	}

	for _, step := range []struct {
		name string
		fn   func(context.Context, string) error
		want string
	}{
		{"confirm", svc.ConfirmFulfillmentOrder, FulfillmentConfirmed},
		{"processing", svc.StartProcessing, FulfillmentProcessing},
		{"ready", svc.MarkReady, FulfillmentReady},
		{"shipped", svc.MarkShipped, FulfillmentShipped},
		{"delivered", svc.MarkDelivered, FulfillmentDelivered},
	} {
		if err := step.fn(ctx, foID); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		fo, _ = store.GetFulfillmentOrder(ctx, foID)
		if fo.Status != step.want {
			t.Fatalf("after %s: status = %s, want %s", step.name, fo.Status, step.want)
		}
	}

	// A delivered fulfillment cannot be cancelled.
	if err := svc.CancelFulfillmentOrder(ctx, foID); !errors.Is(err, ErrInvalidFulfillmentStatus) {
		t.Fatalf("DELIVERED→CANCELLED: want ErrInvalidFulfillmentStatus, got %v", err)
	}

	// The allocation invariant: the item fits within the order item's quantity.
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 3,
	}, 3); err != nil {
		t.Fatalf("AddFulfillmentItem: %v", err)
	}
	items, err := store.ListFulfillmentItemsByFulfillment(ctx, foID)
	if err != nil {
		t.Fatalf("ListFulfillmentItems: %v", err)
	}
	if len(items) != 1 || items[0].Quantity != 3 {
		t.Fatalf("items = %+v", items)
	}
}

// TestPG_FulfillmentSourceRule: source_type must match the reference (§11.1) and the DB
// CHECK must agree.
func TestPG_FulfillmentSourceRule(t *testing.T) {
	pool, svc, _ := setupFulfillment(t)
	ctx := context.Background()
	orderID, _ := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000202", "SKU-FUL-2")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL2")

	// OWNED_INVENTORY without a warehouse.
	if _, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory,
	}); !errors.Is(err, ErrSourceTypeMismatch) {
		t.Fatalf("missing warehouse: want ErrSourceTypeMismatch, got %v", err)
	}
	// SUPPLIER_SOURCED with a warehouse instead of a supplier.
	if _, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceSupplierSourced, WarehouseID: whID,
	}); !errors.Is(err, ErrSourceTypeMismatch) {
		t.Fatalf("wrong reference: want ErrSourceTypeMismatch, got %v", err)
	}
	// The DB CHECK rejects an inconsistent row inserted directly.
	if _, err := pool.Exec(ctx,
		`INSERT INTO fulfillment_orders (order_id, source_type, warehouse_id, status)
		 VALUES ($1,'SUPPLIER_SOURCED',$2,'PENDING')`, orderID, whID); err == nil {
		t.Fatal("chk_fulfillment_source must reject SUPPLIER_SOURCED with a warehouse")
	}
}

// TestPG_ShipmentSplitAndTimeline: one shipment per package, split across fulfillment
// items, with an append-only event timeline.
func TestPG_ShipmentSplitAndTimeline(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000203", "SKU-FUL-3")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL3")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 3,
	}, 3); err != nil {
		t.Fatalf("AddFulfillmentItem: %v", err)
	}
	items, _ := store.ListFulfillmentItemsByFulfillment(ctx, foID)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}

	// A shipment may only be created once the fulfillment order is confirmed.
	if _, err := svc.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "TIPAX",
	}); err == nil {
		t.Fatal("a shipment on a PENDING fulfillment order must be refused")
	}
	// A shipment needs a shippable fulfillment order: PROCESSING or READY (§11.3).
	if err := svc.ConfirmFulfillmentOrder(ctx, foID); err != nil {
		t.Fatalf("ConfirmFulfillmentOrder: %v", err)
	}
	if err := svc.StartProcessing(ctx, foID); err != nil {
		t.Fatalf("StartProcessing: %v", err)
	}

	// Two shipments for one fulfillment order (partial dispatch, §11.3).
	sh1, err := svc.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "TIPAX", ServiceType: "EXPRESS", ShippingCost: 50000,
	})
	if err != nil {
		t.Fatalf("CreateShipment 1: %v", err)
	}
	sh2, err := svc.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "POST", ServiceType: "STANDARD", ShippingCost: 30000,
	})
	if err != nil {
		t.Fatalf("CreateShipment 2: %v", err)
	}
	shipments, err := store.ListShipmentsByFulfillment(ctx, foID)
	if err != nil {
		t.Fatalf("ListShipmentsByFulfillment: %v", err)
	}
	if len(shipments) != 2 {
		t.Fatalf("shipments = %d, want 2", len(shipments))
	}

	// Split the 3 units across the two packages: 2 + 1.
	if _, err := svc.AddShipmentItem(ctx, ShipmentItem{
		ShipmentID: sh1, FulfillmentItemID: items[0].ID, Quantity: 2,
	}); err != nil {
		t.Fatalf("AddShipmentItem 1: %v", err)
	}
	if _, err := svc.AddShipmentItem(ctx, ShipmentItem{
		ShipmentID: sh2, FulfillmentItemID: items[0].ID, Quantity: 1,
	}); err != nil {
		t.Fatalf("AddShipmentItem 2: %v", err)
	}

	// The timeline is append-only and ordered.
	// An unknown event source is refused before the status changes.
	if err := svc.TransitionShipmentStatus(ctx, sh1, ShipmentInTransit, "DISPATCHED", "x", "SYSTEM"); !errors.Is(err, ErrInvalidEventSource) {
		t.Fatalf("invalid source: want ErrInvalidEventSource, got %v", err)
	}
	sh, _ := store.GetShipment(ctx, sh1)
	if sh.Status != ShipmentLabelCreated {
		t.Fatalf("a refused transition must not change the shipment, got %s", sh.Status)
	}
	if err := svc.TransitionShipmentStatus(ctx, sh1, ShipmentInTransit, "DISPATCHED", "تحویل به حامل", EventSourceCarrier); err != nil {
		t.Fatalf("TransitionShipmentStatus: %v", err)
	}
	events, err := store.ListShipmentEvents(ctx, sh1)
	if err != nil {
		t.Fatalf("ListShipmentEvents: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("a shipment transition must append an event")
	}
	if events[0].EventCode == "" {
		t.Fatalf("event = %+v", events[0])
	}
}

// TestPG_AllocationInvariantRejectsOverAllocation: Σ fulfillment_items per order_item
// may not exceed the order item's quantity (§11.2).
func TestPG_AllocationInvariantRejectsOverAllocation(t *testing.T) {
	pool, svc, _ := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000204", "SKU-FUL-4")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL4")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 2,
	}, 3); err != nil {
		t.Fatalf("first allocation: %v", err)
	}
	// 2 + 2 > 3 → refused.
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 2,
	}, 3); err == nil {
		t.Fatal("over-allocation must be refused")
	}
}

var _ = time.Now

// TestPG_ShipmentWrites covers the fulfillment writes that had no test on the real
// schema: the shipment, its status change, the event log and the allocation sum.
func TestPG_ShipmentWrites(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000401", "SKU-FUL-9")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL-9")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 2,
	}, 3); err != nil {
		t.Fatalf("AddFulfillmentItem: %v", err)
	}

	allocated, err := store.SumAllocatedForOrderItem(ctx, itemID)
	if err != nil {
		t.Fatalf("SumAllocatedForOrderItem: %v", err)
	}
	if allocated != 2 {
		t.Fatalf("allocated = %d, want 2", allocated)
	}

	shID, err := store.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "POST", ServiceType: "EXPRESS",
		TrackingNumber: "TRK-1", ShippingCost: 50000, Status: ShipmentLabelCreated,
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}

	if err := store.UpdateShipmentStatus(ctx, shID, ShipmentInTransit, time.Now()); err != nil {
		t.Fatalf("UpdateShipmentStatus: %v", err)
	}

	if _, err := store.RecordShipmentEvent(ctx, ShipmentEvent{
		ShipmentID: shID, EventCode: "IN_TRANSIT", Description: "خروج از انبار",
		OccurredAt: time.Now(), Source: EventSourceCarrier,
	}); err != nil {
		t.Fatalf("RecordShipmentEvent: %v", err)
	}
	events, err := store.ListShipmentEvents(ctx, shID)
	if err != nil {
		t.Fatalf("ListShipmentEvents: %v", err)
	}
	if len(events) != 1 || events[0].EventCode != "IN_TRANSIT" {
		t.Fatalf("events = %+v, want one IN_TRANSIT", events)
	}
}

// TestPG_FulfillmentReadsAndItemStatus covers the reads and the item-level status change
// that the panel and the shipment flow rely on.
func TestPG_FulfillmentReadsAndItemStatus(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000501", "SKU-FUL-11")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL-11")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 1,
	}, 3); err != nil {
		t.Fatalf("AddFulfillmentItem: %v", err)
	}

	orders, err := store.ListFulfillmentOrdersByOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("ListFulfillmentOrdersByOrder: %v", err)
	}
	if len(orders) != 1 || orders[0].ID != foID {
		t.Fatalf("fulfillment orders = %+v, want the one just created", orders)
	}

	items, err := store.ListFulfillmentItemsByFulfillment(ctx, foID)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListFulfillmentItems: %v (n=%d)", err, len(items))
	}
	item, err := store.GetFulfillmentItem(ctx, items[0].ID)
	if err != nil || item == nil {
		t.Fatalf("GetFulfillmentItem: %v", err)
	}
	if err := store.UpdateFulfillmentItemStatus(ctx, item.ID, ItemProcessing); err != nil {
		t.Fatalf("UpdateFulfillmentItemStatus: %v", err)
	}
	item, _ = store.GetFulfillmentItem(ctx, item.ID)
	if item.Status != ItemProcessing {
		t.Fatalf("item status = %q, want %q", item.Status, ItemProcessing)
	}

	if err := store.UpdateFulfillmentOrderStatus(ctx, foID, FulfillmentConfirmed, time.Now()); err != nil {
		t.Fatalf("UpdateFulfillmentOrderStatus: %v", err)
	}
	fo, _ := store.GetFulfillmentOrder(ctx, foID)
	if fo.Status != FulfillmentConfirmed {
		t.Fatalf("fulfillment order status = %q, want %q", fo.Status, FulfillmentConfirmed)
	}
}

// TestPG_ListShipmentItemsByShipment: the shipment detail view reads this and it had no
// test on the real schema.
func TestPG_ListShipmentItemsByShipment(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, itemID := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000601", "SKU-FUL-12")
	whID := seedWarehouse(t, ctx, pool, "WH-FUL-12")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	if err := svc.AddFulfillmentItem(ctx, FulfillmentItem{
		FulfillmentOrderID: foID, OrderItemID: itemID, Quantity: 1,
	}, 3); err != nil {
		t.Fatalf("AddFulfillmentItem: %v", err)
	}
	items, err := store.ListFulfillmentItemsByFulfillment(ctx, foID)
	if err != nil || len(items) != 1 {
		t.Fatalf("ListFulfillmentItemsByFulfillment: %v (n=%d)", err, len(items))
	}
	shID, err := store.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "POST", ServiceType: "EXPRESS",
		TrackingNumber: "TRK-2", ShippingCost: 1000, Status: ShipmentLabelCreated,
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO shipment_items (shipment_id, fulfillment_item_id, quantity) VALUES ($1,$2,1)`,
		shID, items[0].ID); err != nil {
		t.Fatalf("seed shipment item: %v", err)
	}

	si, err := store.ListShipmentItemsByShipment(ctx, shID)
	if err != nil {
		t.Fatalf("ListShipmentItemsByShipment: %v", err)
	}
	if len(si) != 1 || si[0].FulfillmentItemID != items[0].ID {
		t.Fatalf("shipment items = %+v, want the one line", si)
	}
}
