// inventory_pg_test.go exercises the inventory DBStore against the real schema: the
// stock ledger, the sellable invariant (available = on_hand − reserved, DB §8.2), the
// reservation source rule (§8.4) and the expiry sweep.
//
// Skipped unless KH_TEST_PG_DSN is set.
package inventory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupInventory(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
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

// seedOrderItem inserts the minimal order graph and returns (orderItemID, variantID).
func seedOrderItem(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orderNumber, sku string) (string, string) {
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
		 VALUES ($1,'09120000001','PENDING_PAYMENT',1000,1000,'{}'::jsonb) RETURNING id`,
		orderNumber).Scan(&orderID); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO order_items (order_id, product_id, variant_id, sku_snapshot, title_snapshot,
		                          quantity, unit_price, line_total, supply_type_snapshot)
		 VALUES ($1,$2,$3,$4,'x',5,1000,5000,'OWNED_INVENTORY') RETURNING id`,
		orderID, productID, variantID, sku).Scan(&itemID); err != nil {
		t.Fatalf("seed order item: %v", err)
	}
	return itemID, variantID
}

// TestPG_StockLedgerAndSellableInvariant is the core §8.2 rule: reserved stock is not
// sellable, and an oversell attempt must not move either counter.
func TestPG_StockLedgerAndSellableInvariant(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000101", "SKU-INV-1")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH-INV", Name: "انبار تست"})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}

	if err := svc.ReceiveStock(ctx, whID, variantID, 10, "MANUAL", "", ""); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	bal, err := svc.GetStockBalance(ctx, whID, variantID)
	if err != nil || bal == nil {
		t.Fatalf("GetStockBalance: %v", err)
	}
	if bal.OnHandQuantity != 10 || bal.ReservedQuantity != 0 || bal.Sellable() != 10 {
		t.Fatalf("balance = %+v", bal)
	}

	// Reserve 4 → available 6.
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, WarehouseID: whID,
		Quantity: 4, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveForOrder: %v", err)
	}
	bal, _ = svc.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 4 || bal.Sellable() != 6 {
		t.Fatalf("after reserve: %+v (sellable %d)", bal, bal.Sellable())
	}

	// Oversell: reserving 7 more must fail and leave the counters untouched.
	if _, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, WarehouseID: whID,
		Quantity: 7, ExpiresAt: time.Now().Add(time.Hour),
	}); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("oversell: want ErrInsufficientStock, got %v", err)
	}
	bal, _ = svc.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 4 || bal.OnHandQuantity != 10 {
		t.Fatalf("a refused reservation must not move stock: %+v", bal)
	}

	// Release returns the stock to the sellable pool.
	if err := svc.ReleaseReservation(ctx, resID); err != nil {
		t.Fatalf("ReleaseReservation: %v", err)
	}
	bal, _ = svc.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 0 || bal.Sellable() != 10 {
		t.Fatalf("after release: %+v", bal)
	}

	// The ledger recorded every step.
	movements, err := store.ListMovements(ctx, whID, variantID, 50)
	if err != nil {
		t.Fatalf("ListMovements: %v", err)
	}
	seen := map[string]int{}
	for _, m := range movements {
		seen[m.MovementType] += m.Quantity
	}
	if seen[MovementReceipt] != 10 {
		t.Errorf("RECEIPT total = %d, want 10", seen[MovementReceipt])
	}
	if seen[MovementReservation] != 4 {
		t.Errorf("RESERVATION total = %d, want 4", seen[MovementReservation])
	}
	if seen[MovementRelease] != -4 {
		t.Errorf("RELEASE total = %d, want -4", seen[MovementRelease])
	}
}

// TestPG_ReservationConvertedToSale: converting a reservation consumes the stock
// (on_hand down, reserved down) and posts a SALE movement.
func TestPG_ReservationConvertedToSale(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000102", "SKU-INV-2")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH-INV2", Name: "انبار ۲"})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	if err := svc.ReceiveStock(ctx, whID, variantID, 5, "MANUAL", "", ""); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, WarehouseID: whID,
		Quantity: 2, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveForOrder: %v", err)
	}
	if err := svc.ConvertReservation(ctx, resID); err != nil {
		t.Fatalf("ConvertReservation: %v", err)
	}
	bal, _ := svc.GetStockBalance(ctx, whID, variantID)
	if bal.OnHandQuantity != 3 || bal.ReservedQuantity != 0 {
		t.Fatalf("after convert: %+v, want on_hand=3 reserved=0", bal)
	}
	movements, _ := store.ListMovements(ctx, whID, variantID, 50)
	var sale int
	for _, m := range movements {
		if m.MovementType == MovementSale {
			sale += m.Quantity
		}
	}
	if sale != -2 {
		t.Fatalf("SALE movement total = %d, want -2", sale)
	}
}

// TestPG_ReservationSourceRule: exactly one of warehouse_id / supplier_id (§8.4), and
// the DB CHECK agrees with the service.
func TestPG_ReservationSourceRule(t *testing.T) {
	pool, svc, _ := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000103", "SKU-INV-3")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH-INV3", Name: "انبار ۳"})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	// Neither source.
	if _, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, Quantity: 1,
		ExpiresAt: time.Now().Add(time.Hour),
	}); !errors.Is(err, ErrReservationSource) {
		t.Fatalf("no source: want ErrReservationSource, got %v", err)
	}
	// Both sources.
	if _, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, WarehouseID: whID,
		SupplierID: "00000000-0000-0000-0000-000000000001",
		Quantity:   1, ExpiresAt: time.Now().Add(time.Hour),
	}); !errors.Is(err, ErrReservationSource) {
		t.Fatalf("both sources: want ErrReservationSource, got %v", err)
	}
	// The DB CHECK rejects a both-sources row inserted directly.
	if _, err := pool.Exec(ctx,
		`INSERT INTO inventory_reservations (order_item_id, variant_id, warehouse_id, supplier_id,
		     quantity, status, expires_at)
		 VALUES ($1,$2,$3,'00000000-0000-0000-0000-000000000001',1,'ACTIVE',now()+interval '1 hour')`,
		orderItemID, variantID, whID); err == nil {
		t.Fatal("chk_reservation_source must reject a reservation with two sources")
	}
}

// TestPG_ExpireReservationsSweep: the worker's sweep releases expired ACTIVE rows.
func TestPG_ExpireReservationsSweep(t *testing.T) {
	pool, svc, _ := setupInventory(t)
	ctx := context.Background()
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000104", "SKU-INV-4")

	whID, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH-INV4", Name: "انبار ۴"})
	if err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	if err := svc.ReceiveStock(ctx, whID, variantID, 3, "MANUAL", "", ""); err != nil {
		t.Fatalf("ReceiveStock: %v", err)
	}
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		OrderItemID: orderItemID, VariantID: variantID, WarehouseID: whID,
		Quantity: 3, ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ReserveForOrder: %v", err)
	}
	// Age the reservation past its expiry.
	if _, err := pool.Exec(ctx,
		`UPDATE inventory_reservations SET expires_at = now() - interval '1 minute' WHERE id=$1`, resID); err != nil {
		t.Fatalf("age reservation: %v", err)
	}

	n, err := svc.ExpireReservations(ctx, 10)
	if err != nil {
		t.Fatalf("ExpireReservations: %v", err)
	}
	if n != 1 {
		t.Fatalf("expired %d reservations, want 1", n)
	}
	bal, _ := svc.GetStockBalance(ctx, whID, variantID)
	if bal.ReservedQuantity != 0 || bal.Sellable() != 3 {
		t.Fatalf("after sweep: %+v, want reserved=0 sellable=3", bal)
	}
}

// TestPG_ReservationAndBalanceWrites covers the inventory writes that had no test on
// the real schema: the balance upsert (twice, to prove it updates rather than
// duplicates), a reservation, its read-back and its status change.
func TestPG_ReservationAndBalanceWrites(t *testing.T) {
	pool, _, store := setupInventory(t)
	ctx := context.Background()
	itemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000301", "SKU-INV-9")
	whID := seedWarehouseRow(t, ctx, pool, "WH-INV-9")

	bal := StockBalance{WarehouseID: whID, VariantID: variantID, OnHandQuantity: 10, ReservedQuantity: 0}
	if err := store.UpsertStockBalance(ctx, bal); err != nil {
		t.Fatalf("UpsertStockBalance insert: %v", err)
	}
	bal.OnHandQuantity = 15
	if err := store.UpsertStockBalance(ctx, bal); err != nil {
		t.Fatalf("UpsertStockBalance update: %v", err)
	}
	var onHand, rows int
	if err := pool.QueryRow(ctx,
		`SELECT on_hand_quantity, count(*) OVER () FROM stock_balances WHERE warehouse_id=$1 AND variant_id=$2`,
		whID, variantID).Scan(&onHand, &rows); err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if onHand != 15 || rows != 1 {
		t.Fatalf("balance on_hand=%d rows=%d, want 15 and 1 (upsert must update, not duplicate)", onHand, rows)
	}

	resID, err := store.CreateReservation(ctx, Reservation{
		OrderItemID: itemID, VariantID: variantID, WarehouseID: whID,
		Quantity: 2, Status: "ACTIVE", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateReservation: %v", err)
	}
	res, err := store.GetReservation(ctx, resID)
	if err != nil || res == nil {
		t.Fatalf("GetReservation: %v", err)
	}
	if res.Quantity != 2 || res.Status != "ACTIVE" {
		t.Fatalf("reservation = %+v, want qty 2 ACTIVE", res)
	}
	if err := store.UpdateReservationStatus(ctx, resID, "RELEASED"); err != nil {
		t.Fatalf("UpdateReservationStatus: %v", err)
	}
	res, _ = store.GetReservation(ctx, resID)
	if res.Status != "RELEASED" {
		t.Fatalf("reservation status = %q, want RELEASED", res.Status)
	}
}

// seedWarehouseRow inserts a warehouse for the inventory tests.
func seedWarehouseRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, code string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO warehouses (warehouse_code, name) VALUES ($1,'انبار') RETURNING id`, code).Scan(&id); err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	return id
}
