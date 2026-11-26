package fulfillment

import (
	"context"
	"errors"
	"testing"
)

type fakeAuditor struct {
	actions []string
	err     error
}

func (f *fakeAuditor) Record(_ context.Context, _, _, action, _, _ string, _, _ any, _ string) error {
	f.actions = append(f.actions, action)
	return f.err
}

// TestShipmentTransitionAudit: a shipment status change is an audited category (IAM §۷),
// and an audit failure must not be reported as a successful transition.
func TestShipmentTransitionAudit(t *testing.T) {
	pool, svc, store := setupFulfillment(t)
	ctx := context.Background()
	orderID, _ := seedOrderItemGraph(t, ctx, pool, "KHA-ORD-2026-000801", "SKU-AUD-1")
	whID := seedWarehouse(t, ctx, pool, "WH-AUD-1")

	foID, err := svc.CreateFulfillmentOrder(ctx, FulfillmentOrder{
		OrderID: orderID, SourceType: SourceOwnedInventory, WarehouseID: whID,
	})
	if err != nil {
		t.Fatalf("CreateFulfillmentOrder: %v", err)
	}
	shID, err := store.CreateShipment(ctx, Shipment{
		FulfillmentOrderID: foID, Carrier: "POST", ServiceType: "EXPRESS",
		TrackingNumber: "TRK-AUD", ShippingCost: 1000, Status: ShipmentLabelCreated,
	})
	if err != nil {
		t.Fatalf("CreateShipment: %v", err)
	}

	aud := &fakeAuditor{}
	svc.SetAuditor(aud)
	if err := svc.TransitionShipmentStatus(ctx, shID, ShipmentInTransit, "PICKED_UP", "تحویل به پست", EventSourceCarrier); err != nil {
		t.Fatalf("TransitionShipmentStatus: %v", err)
	}
	if len(aud.actions) != 1 || aud.actions[0] != "shipment.status" {
		t.Fatalf("audit = %v, want shipment.status", aud.actions)
	}

	// An illegal transition is refused before any audit write.
	aud.actions = nil
	if err := svc.TransitionShipmentStatus(ctx, shID, ShipmentLabelCreated, "BACK", "", EventSourceManual); err == nil {
		t.Fatal("a backwards transition must be refused")
	}
	if len(aud.actions) != 0 {
		t.Fatalf("a refused transition was audited: %v", aud.actions)
	}

	// Audit failure surfaces.
	aud.err = errors.New("audit down")
	if err := svc.TransitionShipmentStatus(ctx, shID, ShipmentOutForDelivery, "OFD", "", EventSourceCarrier); err == nil {
		t.Fatal("an audit failure must be returned")
	}
}
