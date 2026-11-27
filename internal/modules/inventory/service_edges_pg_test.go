package inventory

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPG_ReservationServiceEdges covers the reservation service branches: the not-found and
// not-active guards, the release path (which posts a negative movement) and the limit clamp
// on the movement list.
func TestPG_ReservationServiceEdges(t *testing.T) {
	pool, svc, store := setupInventory(t)
	ctx := context.Background()

	// Guards first: an unknown reservation and a double release.
	if err := svc.ReleaseReservation(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("release unknown = %v", err)
	}
	if err := svc.ConvertReservation(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, ErrReservationNotFound) {
		t.Fatalf("convert unknown = %v", err)
	}

	whID, err := svc.CreateWarehouse(ctx, Warehouse{
		WarehouseCode: "WH-EDGE", Name: "انبار لبه", Status: WarehouseActive,
	})
	if err != nil {
		t.Fatalf("warehouse: %v", err)
	}
	// variant_id is a FK to product_variants, so seed a real one.
	orderItemID, variantID := seedOrderItem(t, ctx, pool, "KHA-ORD-2026-000950", "SKU-INV-EDGE")
	_ = orderItemID
	if err := store.UpsertStockBalance(ctx, StockBalance{
		WarehouseID: whID, VariantID: variantID, OnHandQuantity: 10,
	}); err != nil {
		t.Fatalf("balance: %v", err)
	}

	// Reserve, then release: the reserved count goes back and a negative movement is posted.
	resID, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: whID, VariantID: variantID, Quantity: 4,
		OrderItemID: orderItemID,
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := svc.ReleaseReservation(ctx, resID); err != nil {
		t.Fatalf("release: %v", err)
	}
	bal, err := store.GetStockBalance(ctx, whID, variantID)
	if err != nil || bal == nil {
		t.Fatalf("balance after release = %+v, %v", bal, err)
	}
	if bal.ReservedQuantity != 0 {
		t.Fatalf("reserved after release = %d, want 0", bal.ReservedQuantity)
	}
	// A second release is refused: the reservation is no longer ACTIVE.
	if err := svc.ReleaseReservation(ctx, resID); !errors.Is(err, ErrReservationNotActive) {
		t.Fatalf("double release = %v, want ErrReservationNotActive", err)
	}

	// The release posted a movement with a negative quantity.
	movements, err := svc.ListMovements(ctx, whID, variantID, 0)
	if err != nil {
		t.Fatalf("movements: %v", err)
	}
	var sawRelease bool
	for _, m := range movements {
		if m.MovementType == MovementRelease {
			sawRelease = true
			if m.Quantity >= 0 {
				t.Fatalf("release movement quantity = %d, want negative", m.Quantity)
			}
		}
	}
	if !sawRelease {
		t.Fatal("no RELEASE movement was recorded")
	}

	// The limit clamp: 0 and a huge value both land inside the allowed window.
	if _, err := svc.ListMovements(ctx, whID, variantID, 5000); err != nil {
		t.Fatalf("clamped movements: %v", err)
	}

	// Convert a fresh reservation: on_hand drops and the reservation becomes CONVERTED.
	res2, err := svc.ReserveForOrder(ctx, Reservation{
		WarehouseID: whID, VariantID: variantID, Quantity: 3,
		OrderItemID: orderItemID,
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("reserve 2: %v", err)
	}
	if err := svc.ConvertReservation(ctx, res2); err != nil {
		t.Fatalf("convert: %v", err)
	}
	bal, _ = store.GetStockBalance(ctx, whID, variantID)
	if bal.OnHandQuantity != 7 {
		t.Fatalf("on_hand after convert = %d, want 7", bal.OnHandQuantity)
	}
	if bal.ReservedQuantity != 0 {
		t.Fatalf("reserved after convert = %d, want 0", bal.ReservedQuantity)
	}
	// Converting twice is refused.
	if err := svc.ConvertReservation(ctx, res2); !errors.Is(err, ErrReservationNotActive) {
		t.Fatalf("double convert = %v, want ErrReservationNotActive", err)
	}
}
