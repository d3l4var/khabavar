package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory pricing.Store for unit tests.
type memStore struct {
	mu     sync.Mutex
	costs  []*CostRecord
	prices []*PriceRecord
	seq    int
}

func newMemStore() *memStore { return &memStore{} }

func (m *memStore) CreateCostRecord(_ context.Context, c CostRecord) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "cost-" + itoaP(m.seq)
	c.ID = id
	m.costs = append(m.costs, &c)
	return id, nil
}

func (m *memStore) ListCostRecordsByVariant(_ context.Context, variantID string) ([]CostRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []CostRecord
	for _, c := range m.costs {
		if c.VariantID == variantID {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ValidFrom.After(out[j].ValidFrom) })
	return out, nil
}

func (m *memStore) GetValidCostRecord(_ context.Context, variantID string, now time.Time) (*CostRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *CostRecord
	for _, c := range m.costs {
		if c.VariantID != variantID {
			continue
		}
		if !inWindow(c.ValidFrom, c.ValidUntil, now) {
			continue
		}
		if best == nil || c.ValidFrom.After(best.ValidFrom) {
			best = c
		}
	}
	if best == nil {
		return nil, nil
	}
	c := *best
	return &c, nil
}

func (m *memStore) CreatePriceRecord(_ context.Context, p PriceRecord) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "price-" + itoaP(m.seq)
	p.ID = id
	m.prices = append(m.prices, &p)
	return id, nil
}

func (m *memStore) ApprovePriceRecord(_ context.Context, id, approverID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.prices {
		if p.ID == id {
			if p.ApprovedBy != "" {
				return ErrPriceAlreadyApproved
			}
			p.ApprovedBy = approverID
			return nil
		}
	}
	return ErrVariantNotFound
}

func (m *memStore) GetValidPriceRecord(_ context.Context, variantID string, now time.Time) (*PriceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *PriceRecord
	for _, p := range m.prices {
		if p.VariantID != variantID || p.ApprovedBy == "" {
			continue
		}
		if !inWindow(p.ValidFrom, p.ValidUntil, now) {
			continue
		}
		if best == nil || p.ValidFrom.After(best.ValidFrom) {
			best = p
		}
	}
	if best == nil {
		return nil, nil
	}
	p := *best
	return &p, nil
}

func (m *memStore) ListPriceRecordsByVariant(_ context.Context, variantID string) ([]PriceRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []PriceRecord
	for _, p := range m.prices {
		if p.VariantID == variantID {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ValidFrom.After(out[j].ValidFrom) })
	return out, nil
}

func inWindow(from time.Time, until *time.Time, now time.Time) bool {
	if now.Before(from) {
		return false
	}
	if until != nil && !now.Before(*until) {
		return false
	}
	return true
}

func itoaP(n int) string {
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

func ptrTime(t time.Time) *time.Time { return &t }

// ── pure arithmetic, anchored on R-05 §۱۹ worked example (×10 toman→rial) ──────

func TestTotalCost_AnchoredOnR05Example(t *testing.T) {
	c := CostRecord{
		PurchaseCost:             4_000_000, // هزینه خرید ۴۰۰٬۰۰۰ toman
		InboundFreight:           200_000,   // حمل ورودی + کنترل کیفیت = ۳۰۰٬۰۰۰
		InspectionCost:           100_000,
		PackagingCost:            250_000, // بسته‌بندی + پردازش = ۴۰۰٬۰۰۰
		FulfillmentCost:          150_000,
		EstimatedPaymentFee:      100_000, // کارمزد پرداخت ۱۰٬۰۰۰
		EstimatedShippingSubsidy: 500_000, // یارانه ارسال ۵۰٬۰۰۰ (additive)
		ExpectedReturnCost:       200_000, // هزینه مرجوعی ۲۰٬۰۰۰
	}
	// R-05 §۱۹: total variable cost = ۵۵۰٬۰۰۰ toman = ۵٬۵۰۰٬۰۰۰ rial.
	if got := TotalCost(c); got != 5_500_000 {
		t.Fatalf("TotalCost=%d want 5_500_000", got)
	}
}

func TestNetRevenueContribution_AnchoredOnR05Example(t *testing.T) {
	// فروش بدون مالیات ۱٬۰۰۰٬۰۰۰ toman، تخفیف ۵۰٬۰۰۰ → درآمد خالص ۹۵۰٬۰۰۰.
	selling := int64(10_000_000)
	discount := int64(500_000)
	net := NetRevenue(selling, discount)
	if net != 9_500_000 {
		t.Fatalf("NetRevenue=%d want 9_500_000", net)
	}
	totalCost := int64(5_500_000)
	contrib := Contribution(net, totalCost)
	if contrib != 4_000_000 { // سود مشارکت ۴۰۰٬۰۰۰ toman
		t.Fatalf("Contribution=%d want 4_000_000", contrib)
	}
	rate := ContributionRate(contrib, net)
	// ۴٬۰۰۰٬۰۰۰ / ۹٬۵۰۰٬۰۰۰ ≈ ۰٫۴۲۱۰۵
	if math.Abs(rate-0.4210526) > 1e-5 {
		t.Fatalf("ContributionRate=%.6f want ~0.4210526", rate)
	}
	// Zero net revenue → rate 0 (no division by zero).
	if r := ContributionRate(100, 0); r != 0 {
		t.Fatalf("ContributionRate(100,0)=%v want 0", r)
	}
}

func TestDiscountAmount_ExtractsFromPolicy(t *testing.T) {
	policy, _ := json.Marshal(map[string]any{"amount": 750_000, "type": "flat"})
	if d := DiscountAmount(policy); d != 750_000 {
		t.Fatalf("discount=%d want 750_000", d)
	}
	if d := DiscountAmount(nil); d != 0 {
		t.Fatalf("nil policy discount=%d want 0", d)
	}
	if d := DiscountAmount(json.RawMessage(`{}`)); d != 0 {
		t.Fatalf("empty policy discount=%d want 0", d)
	}
	// Negative or malformed → 0.
	if d := DiscountAmount(json.RawMessage(`{"amount":-5}`)); d != 0 {
		t.Fatalf("negative discount=%d want 0", d)
	}
}

func TestTargetPrice_FormulaAndValidation(t *testing.T) {
	// TotalCost=5_500_000, target rate 0.4210526 → TargetPrice = 5_500_000 / (1-0.4210526) ≈ 9_500_000
	// (the selling price that yields the target contribution rate at this cost).
	tp, err := TargetPrice(5_500_000, 0.4210526)
	if err != nil {
		t.Fatalf("TargetPrice: %v", err)
	}
	if math.Abs(float64(tp)-9_500_000) > 1 {
		t.Fatalf("TargetPrice=%d want ~9_500_000", tp)
	}
	// Out-of-range rate → error.
	if _, err := TargetPrice(1_000_000, 1.0); !errors.Is(err, ErrInvalidTargetRate) {
		t.Fatalf("rate=1.0 should fail, got %v", err)
	}
	if _, err := TargetPrice(1_000_000, -0.1); !errors.Is(err, ErrInvalidTargetRate) {
		t.Fatalf("negative rate should fail, got %v", err)
	}
}

// ── Service: cost record validation ────────────────────────────────────────────

func TestCreateCostRecord_Validation(t *testing.T) {
	svc := NewService(newMemStore())
	// Bad supply type.
	if _, err := svc.CreateCostRecord(context.Background(), CostRecord{VariantID: "v1", SupplyType: "BOGUS"}); !errors.Is(err, ErrInvalidSupplyType) {
		t.Fatalf("bad supply type: %v", err)
	}
	// SUPPLIER_SOURCED without supplier_id.
	if _, err := svc.CreateCostRecord(context.Background(), CostRecord{VariantID: "v1", SupplyType: SupplySupplierSourced}); !errors.Is(err, ErrSupplierRequired) {
		t.Fatalf("missing supplier: %v", err)
	}
	// OWNED ok.
	id, err := svc.CreateCostRecord(context.Background(), CostRecord{VariantID: "v1", SupplyType: SupplyOwnedInventory, PurchaseCost: 1000})
	if err != nil || id == "" {
		t.Fatalf("owned create: %v", err)
	}
}

// ── Service: price record + contribution + approval gate ───────────────────────

func newTestService(t *testing.T) (*Service, *memStore, time.Time, string) {
	t.Helper()
	store := newMemStore()
	svc := NewService(store)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	costID, err := svc.CreateCostRecord(context.Background(), CostRecord{
		VariantID: "v1", SupplyType: SupplyOwnedInventory,
		PurchaseCost: 4_000_000, InboundFreight: 200_000, InspectionCost: 100_000,
		PackagingCost: 250_000, FulfillmentCost: 150_000, EstimatedPaymentFee: 100_000,
		EstimatedShippingSubsidy: 500_000, ExpectedReturnCost: 200_000,
		ValidFrom: now.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed cost: %v", err)
	}
	return svc, store, now, costID
}

func TestCreatePriceRecord_ComputesContributionAndAnchorsR05(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	policy, _ := json.Marshal(map[string]any{"amount": 500_000})
	id, contrib, err := svc.CreatePriceRecord(context.Background(), PriceRecord{
		VariantID: "v1", SellingPrice: 10_000_000, DiscountPolicy: policy,
	})
	if err != nil {
		t.Fatalf("create price: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	// Contribution must equal the R-05 §۱۹ anchor: ۴٬۰۰۰٬۰۰۰ rial.
	if contrib != 4_000_000 {
		t.Fatalf("contribution=%d want 4_000_000", contrib)
	}
}

func TestCreatePriceRecord_NoValidCost(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	// Cost record whose window has already closed (valid_until before now).
	svc.CreateCostRecord(context.Background(), CostRecord{
		VariantID: "v1", SupplyType: SupplyOwnedInventory, PurchaseCost: 1_000_000,
		ValidFrom: now.Add(-48 * time.Hour), ValidUntil: ptrTime(now.Add(-24 * time.Hour)),
	})
	_, _, err := svc.CreatePriceRecord(context.Background(), PriceRecord{VariantID: "v1", SellingPrice: 2_000_000})
	if !errors.Is(err, ErrNoValidCost) {
		t.Fatalf("expected ErrNoValidCost, got %v", err)
	}
}

func TestCreatePriceRecordWithWarning_BR06NegativeContribution(t *testing.T) {
	svc, store, _, _ := newTestService(t)
	// Selling price below total cost (5_500_000) → negative contribution.
	res, err := svc.CreatePriceRecordWithWarning(context.Background(), PriceRecord{
		VariantID: "v1", SellingPrice: 4_000_000, // < 5_500_000 cost
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !res.NegativeContri {
		t.Fatal("expected BR-06 negative-contribution warning")
	}
	if len(res.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
	// The stored record must carry the computed (negative) contribution.
	store.mu.Lock()
	defer store.mu.Unlock()
	var stored *PriceRecord
	for _, p := range store.prices {
		if p.ID == res.ID {
			stored = p
		}
	}
	if stored == nil || stored.CalculatedContribution == nil {
		t.Fatal("stored contribution missing")
	}
	if *stored.CalculatedContribution >= 0 {
		t.Fatalf("stored contribution=%d must be negative", *stored.CalculatedContribution)
	}
}

func TestApprovePriceRecord_Gate(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	id, _, _ := svc.CreatePriceRecord(context.Background(), PriceRecord{VariantID: "v1", SellingPrice: 10_000_000})
	// First approval ok.
	if err := svc.ApprovePriceRecord(context.Background(), id, "admin-1"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// Second approval → already approved.
	if err := svc.ApprovePriceRecord(context.Background(), id, "admin-2"); !errors.Is(err, ErrPriceAlreadyApproved) {
		t.Fatalf("expected ErrPriceAlreadyApproved, got %v", err)
	}
	// Empty approver → error.
	if err := svc.ApprovePriceRecord(context.Background(), id, ""); err == nil {
		t.Fatal("empty approver should fail")
	}
	// Nonexistent → ErrVariantNotFound.
	if err := svc.ApprovePriceRecord(context.Background(), "nope", "admin-1"); !errors.Is(err, ErrVariantNotFound) {
		t.Fatalf("expected ErrVariantNotFound, got %v", err)
	}
}

// ── Service: UnitEconomics ─────────────────────────────────────────────────────

func TestUnitEconomics_AnchoredAndWarnings(t *testing.T) {
	svc, _, now, _ := newTestService(t)
	policy, _ := json.Marshal(map[string]any{"amount": 500_000})
	pid, _, _ := svc.CreatePriceRecord(context.Background(), PriceRecord{
		VariantID: "v1", SellingPrice: 10_000_000, DiscountPolicy: policy,
	})
	// Before approval: UnitEconomics warns "no approved price".
	ue, err := svc.UnitEconomics(context.Background(), "v1")
	if err != nil {
		t.Fatalf("ue pre-approval: %v", err)
	}
	if !contains(ue.Warnings, "no approved price record in validity window") {
		t.Fatalf("pre-approval warnings=%v", ue.Warnings)
	}
	// Approve and re-check → full economics, R-05 §۱۹ anchors.
	svc.ApprovePriceRecord(context.Background(), pid, "admin-1")
	ue, err = svc.UnitEconomics(context.Background(), "v1")
	if err != nil {
		t.Fatalf("ue: %v", err)
	}
	if ue.NetRevenue != 9_500_000 {
		t.Fatalf("net=%d want 9_500_000", ue.NetRevenue)
	}
	if ue.TotalCost != 5_500_000 {
		t.Fatalf("cost=%d want 5_500_000", ue.TotalCost)
	}
	if ue.Contribution != 4_000_000 {
		t.Fatalf("contribution=%d want 4_000_000", ue.Contribution)
	}
	if math.Abs(ue.ContributionRate-0.4210526) > 1e-5 {
		t.Fatalf("rate=%.6f want ~0.4210526", ue.ContributionRate)
	}
	if len(ue.Warnings) != 0 {
		t.Fatalf("expected no warnings for healthy variant, got %v", ue.Warnings)
	}
	// Variant with no cost at all → warns "no currently-valid cost".
	ue2, _ := svc.UnitEconomics(context.Background(), "no-cost-variant")
	if !contains(ue2.Warnings, "no currently-valid cost record") {
		t.Fatalf("missing-cost warnings=%v", ue2.Warnings)
	}
	_ = now
}

func TestUnitEconomics_BR06NegativeContributionWarning(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	pid, _, _ := svc.CreatePriceRecord(context.Background(), PriceRecord{
		VariantID: "v1", SellingPrice: 4_000_000, // below 5_500_000 cost
	})
	svc.ApprovePriceRecord(context.Background(), pid, "admin-1")
	ue, _ := svc.UnitEconomics(context.Background(), "v1")
	if !contains(ue.Warnings, "BR-06: non-positive contribution") {
		t.Fatalf("expected BR-06 warning, got %v", ue.Warnings)
	}
	if ue.Contribution >= 0 {
		t.Fatalf("contribution=%d must be negative", ue.Contribution)
	}
}

// ── validity window ────────────────────────────────────────────────────────────

func TestGetValidCostRecord_WindowExcludesExpired(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	// Expired record.
	store.CreateCostRecord(context.Background(), CostRecord{
		VariantID: "v1", SupplyType: SupplyOwnedInventory, PurchaseCost: 1_000_000,
		ValidFrom: now.Add(-48 * time.Hour), ValidUntil: ptrTime(now.Add(-24 * time.Hour)),
	})
	// Current record.
	store.CreateCostRecord(context.Background(), CostRecord{
		VariantID: "v1", SupplyType: SupplyOwnedInventory, PurchaseCost: 2_000_000,
		ValidFrom: now.Add(-12 * time.Hour),
	})
	got, err := store.GetValidCostRecord(context.Background(), "v1", now)
	if err != nil || got == nil {
		t.Fatalf("expected valid cost, got %v %v", got, err)
	}
	if got.PurchaseCost != 2_000_000 {
		t.Fatalf("picked wrong record: cost=%d", got.PurchaseCost)
	}
	// Before any record's valid_from → nil.
	got, _ = store.GetValidCostRecord(context.Background(), "v1", now.Add(-72*time.Hour))
	if got != nil {
		t.Fatalf("expected nil before window, got %+v", got)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
