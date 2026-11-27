package suppliers

import (
	"context"
	"errors"
	"testing"
)

type fakeAuditor struct {
	actions  []string
	entities []string
	err      error
}

func (f *fakeAuditor) Record(_ context.Context, _, _, action, _, entityID string, _, _ any, _ string) error {
	f.actions = append(f.actions, action)
	f.entities = append(f.entities, entityID)
	return f.err
}

// TestSupplierAudit: supplier approval and purchase-cost changes are two of the seven
// audited categories (IAM §۷), and an audit failure must reach the caller.
func TestSupplierAudit(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	aud := &fakeAuditor{}
	svc.SetAuditor(aud)

	id, err := svc.CreateSupplier(ctx, Supplier{Name: "تأمین‌کننده", SupplierType: "MANUFACTURER"})
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	// APPROVED is the approval step; the seeded status may already be ACTIVE, so walk the
	// transition the state machine allows.
	if err := svc.UpdateSupplierStatus(ctx, id, SupplierProbation); err != nil {
		t.Fatalf("UpdateSupplierStatus: %v", err)
	}
	if len(aud.actions) != 1 || aud.actions[0] != "supplier.status" {
		t.Fatalf("audit = %v, want supplier.status", aud.actions)
	}

	// An invalid transition must not be audited.
	aud.actions = nil
	if err := svc.UpdateSupplierStatus(ctx, id, "NONSENSE"); !errors.Is(err, ErrUnknownStatus) {
		t.Fatalf("invalid status = %v, want ErrUnknownStatus", err)
	}
	if len(aud.actions) != 0 {
		t.Fatalf("a rejected transition was audited: %v", aud.actions)
	}

	// Audit failure is surfaced.
	aud.err = errors.New("audit down")
	if err := svc.UpdateSupplierStatus(ctx, id, SupplierActive); err == nil {
		t.Fatal("an audit failure must be returned")
	}
}
