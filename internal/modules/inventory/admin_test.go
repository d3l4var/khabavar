package inventory

import (
	"context"
	"errors"
	"testing"
)

// TestRecordMovementAcceptsCorrectionTypes: the panel may post on-hand corrections.
func TestRecordMovementAcceptsCorrectionTypes(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	if _, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH1", Name: "انبار مرکزی"}); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	wh, err := store.GetWarehouseByCode(ctx, "WH1")
	if err != nil || wh == nil {
		t.Fatalf("load warehouse: %v", err)
	}

	for _, mt := range []string{MovementReceipt, MovementReturn, MovementAdjustment, MovementDamage, MovementTransfer} {
		if _, err := svc.RecordMovement(ctx, Movement{
			WarehouseID: wh.ID, VariantID: "v1", MovementType: mt, Quantity: 5,
		}); err != nil {
			t.Errorf("%s must be accepted, got %v", mt, err)
		}
	}
}

// TestRecordMovementRejectsOrderFlowTypes is the safety rule: RESERVATION, RELEASE and
// SALE move reserved_quantity and belong to the order flow. Accepting them from the
// panel would let an operator corrupt available = on_hand − reserved (DB §8.2).
func TestRecordMovementRejectsOrderFlowTypes(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	if _, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH1", Name: "x"}); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	wh, _ := store.GetWarehouseByCode(ctx, "WH1")

	for _, mt := range []string{MovementReservation, MovementRelease, MovementSale} {
		if _, err := svc.RecordMovement(ctx, Movement{
			WarehouseID: wh.ID, VariantID: "v1", MovementType: mt, Quantity: 1,
		}); !errors.Is(err, ErrMovementTypeNotAllowed) {
			t.Errorf("%s must be refused with ErrMovementTypeNotAllowed, got %v", mt, err)
		}
	}
}

func TestRecordMovementRejectsZeroQuantity(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	if _, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH1", Name: "x"}); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	wh, _ := store.GetWarehouseByCode(ctx, "WH1")
	if _, err := svc.RecordMovement(ctx, Movement{
		WarehouseID: wh.ID, VariantID: "v1", MovementType: MovementAdjustment, Quantity: 0,
	}); !errors.Is(err, ErrZeroQuantity) {
		t.Fatalf("want ErrZeroQuantity, got %v", err)
	}
}

// TestRecordMovementWritesLedgerRow: every correction must leave a ledger entry.
func TestRecordMovementWritesLedgerRow(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	if _, err := svc.CreateWarehouse(ctx, Warehouse{WarehouseCode: "WH1", Name: "x"}); err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	wh, _ := store.GetWarehouseByCode(ctx, "WH1")

	id, err := svc.RecordMovement(ctx, Movement{
		WarehouseID: wh.ID, VariantID: "v1", MovementType: MovementReceipt,
		Quantity: 10, ReferenceType: "MANUAL",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if id == "" {
		t.Fatal("empty movement id")
	}
	movements, err := svc.ListMovements(ctx, wh.ID, "v1", 10)
	if err != nil {
		t.Fatalf("list movements: %v", err)
	}
	if len(movements) != 1 {
		t.Fatalf("movements = %d, want 1", len(movements))
	}
	if movements[0].Quantity != 10 || movements[0].MovementType != MovementReceipt {
		t.Fatalf("movement = %+v", movements[0])
	}
}
