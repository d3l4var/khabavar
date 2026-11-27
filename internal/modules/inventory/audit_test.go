package inventory

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeAuditor records what the service asked it to audit.
type fakeAuditor struct {
	calls int
	last  struct{ actorType, actorID, action, entityType, entityID string }
	err   error
}

func (f *fakeAuditor) Record(_ context.Context, actorType, actorID, action, entityType, entityID string, _, _ any, _ string) error {
	f.calls++
	f.last.actorType, f.last.actorID, f.last.action = actorType, actorID, action
	f.last.entityType, f.last.entityID = entityType, entityID
	return f.err
}

// TestAdjustStockWritesAudit: a stock change is one of the seven audited categories
// (IAM §۷), so the service must record it — and must surface an audit failure instead of
// pretending the change succeeded.
func TestAdjustStockWritesAudit(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	svc.SetNow(func() time.Time { return time.Unix(0, 0) })
	aud := &fakeAuditor{}
	svc.SetAuditor(aud)

	// The warehouse must exist now that AdjustStock validates it (a 404 instead of an FK 500).
	if _, err := svc.CreateWarehouse(context.Background(), Warehouse{WarehouseCode: "wh-1", Name: "انبار تست"}); err != nil {
		t.Fatalf("CreateWarehouse: %v", err)
	}
	// Seed on-hand first: a negative adjustment off an empty shelf is refused.
	if err := svc.AdjustStock(context.Background(), "wh-1", "var-1", 10, "MANUAL", "", "admin-9", "ورود اولیه"); err != nil {
		t.Fatalf("seed on-hand: %v", err)
	}
	aud.calls = 0
	if err := svc.AdjustStock(context.Background(), "wh-1", "var-1", -3, "MANUAL", "", "admin-9", "شمارش انبار"); err != nil {
		t.Fatalf("AdjustStock: %v", err)
	}
	if aud.calls != 1 {
		t.Fatalf("audit calls = %d, want 1", aud.calls)
	}
	if aud.last.action != "inventory.adjust" || aud.last.actorID != "admin-9" || aud.last.entityID != "var-1" {
		t.Fatalf("audit = %+v", aud.last)
	}

	// A zero delta changes nothing, so it must not be audited either.
	aud.calls = 0
	if err := svc.AdjustStock(context.Background(), "wh-1", "var-1", 0, "MANUAL", "", "admin-9", ""); !errors.Is(err, ErrZeroQuantity) {
		t.Fatalf("zero delta = %v, want ErrZeroQuantity", err)
	}
	if aud.calls != 0 {
		t.Fatalf("a rejected adjustment was audited (%d calls)", aud.calls)
	}

	// An audit failure is returned, not swallowed.
	aud.err = errors.New("audit down")
	if err := svc.AdjustStock(context.Background(), "wh-1", "var-1", 1, "MANUAL", "", "admin-9", ""); err == nil {
		t.Fatal("an audit failure must be returned")
	}
}
