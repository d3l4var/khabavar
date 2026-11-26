package suppliers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// memStore is an in-memory suppliers.Store for unit tests. It enforces the same
// "price change writes history" semantics as the DB implementation so the §7.3
// rule is exercised without a database.
type memStore struct {
	mu        sync.Mutex
	suppliers map[string]*Supplier
	supProds  map[string]*SupplierProduct
	pairIndex map[string]string // "supplierID:productID" → id
	supVars   map[string]*memSupplierVariant
	varByVar  map[string][]string // variantID → []supplierVariantID
	history   map[string][]PriceHistoryEntry
	evals     map[string][]Evaluation
	seq       int
}

type memSupplierVariant struct {
	SupplierVariant
	historyOpen bool // true if there's an open (valid_until IS NULL) history row
}

func newMemStore() *memStore {
	return &memStore{
		suppliers: map[string]*Supplier{},
		supProds:  map[string]*SupplierProduct{},
		pairIndex: map[string]string{},
		supVars:   map[string]*memSupplierVariant{},
		varByVar:  map[string][]string{},
		history:   map[string][]PriceHistoryEntry{},
		evals:     map[string][]Evaluation{},
	}
}

func (m *memStore) CreateSupplier(_ context.Context, sup Supplier) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "sup-" + itoa(m.seq)
	sup.ID = id
	sup.CreatedAt = time.Now()
	sup.UpdatedAt = sup.CreatedAt
	m.suppliers[id] = &sup
	return id, nil
}

func (m *memStore) GetSupplier(_ context.Context, id string) (*Supplier, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suppliers[id]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (m *memStore) UpdateSupplierStatus(_ context.Context, id, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.suppliers[id]
	if !ok {
		return ErrSupplierNotFound
	}
	s.Status = status
	s.UpdatedAt = time.Now()
	return nil
}

func (m *memStore) CreateSupplierProduct(_ context.Context, sp SupplierProduct) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "sp-" + itoa(m.seq)
	sp.ID = id
	m.supProds[id] = &sp
	m.pairIndex[sp.SupplierID+":"+sp.ProductID] = id
	return id, nil
}

func (m *memStore) GetSupplierProduct(_ context.Context, id string) (*SupplierProduct, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sp, ok := m.supProds[id]
	if !ok {
		return nil, nil
	}
	cp := *sp
	return &cp, nil
}

func (m *memStore) GetSupplierProductByPair(_ context.Context, supplierID, productID string) (*SupplierProduct, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.pairIndex[supplierID+":"+productID]
	if !ok {
		return nil, nil
	}
	cp := *m.supProds[id]
	return &cp, nil
}

func (m *memStore) CreateSupplierVariant(_ context.Context, sv SupplierVariant) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "sv-" + itoa(m.seq)
	sv.ID = id
	m.supVars[id] = &memSupplierVariant{SupplierVariant: sv, historyOpen: false}
	m.varByVar[sv.VariantID] = append(m.varByVar[sv.VariantID], id)
	return id, nil
}

func (m *memStore) GetSupplierVariant(_ context.Context, id string) (*SupplierVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sv, ok := m.supVars[id]
	if !ok {
		return nil, nil
	}
	cp := sv.SupplierVariant
	return &cp, nil
}

func (m *memStore) ListSupplierVariantsByVariant(_ context.Context, variantID string) ([]SupplierVariant, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := m.varByVar[variantID]
	out := make([]SupplierVariant, 0, len(ids))
	for _, id := range ids {
		out = append(out, m.supVars[id].SupplierVariant)
	}
	return out, nil
}

// UpdateSupplierVariantPrice mirrors the DB transaction: close the previous open
// history row, update the price, insert a new open history row.
func (m *memStore) UpdateSupplierVariantPrice(_ context.Context, id string, newPrice int64, validFrom time.Time, validUntil *time.Time, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sv, ok := m.supVars[id]
	if !ok {
		return ErrSupplierVariantNotFound
	}
	// Close previous open history row.
	hist := m.history[id]
	for i := len(hist) - 1; i >= 0; i-- {
		if hist[i].ValidUntil == nil {
			v := now
			hist[i].ValidUntil = &v
			break
		}
	}
	// Update the variant price.
	sv.PurchasePrice = &newPrice
	sv.PriceValidUntil = validUntil
	sv.LastPriceCheckAt = &now
	// Insert new open history row.
	m.history[id] = append(hist, PriceHistoryEntry{
		ID:                "ph-" + itoa(m.seq+len(hist)+1),
		SupplierVariantID: id,
		PurchasePrice:     newPrice,
		Currency:          "IRR",
		ValidFrom:         validFrom,
		ValidUntil:        nil,
	})
	sv.historyOpen = true
	return nil
}

func (m *memStore) ListPriceHistory(_ context.Context, supplierVariantID string) ([]PriceHistoryEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hist := m.history[supplierVariantID]
	out := make([]PriceHistoryEntry, len(hist))
	copy(out, hist)
	// Reverse to newest-first (DB ORDER BY valid_from DESC).
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *memStore) CreateEvaluation(_ context.Context, ev Evaluation) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := "ev-" + itoa(m.seq)
	ev.ID = id
	m.evals[ev.SupplierID] = append(m.evals[ev.SupplierID], ev)
	return id, nil
}

func (m *memStore) ListEvaluationsBySupplier(_ context.Context, supplierID string) ([]Evaluation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	evs := m.evals[supplierID]
	out := make([]Evaluation, len(evs))
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

func TestCreateSupplier_RequiresName(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateSupplier(context.Background(), Supplier{}); !errors.Is(err, ErrNameRequired) {
		t.Fatalf("expected ErrNameRequired, got %v", err)
	}
}

func TestCreateSupplier_OnlyActiveInitial(t *testing.T) {
	svc := NewService(newMemStore())
	if _, err := svc.CreateSupplier(context.Background(), Supplier{Name: "S", Status: SupplierRetired}); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("non-ACTIVE initial must fail, got %v", err)
	}
	id, err := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	sup, _ := svc.store.(*memStore).GetSupplier(context.Background(), id)
	if sup.Status != SupplierActive {
		t.Fatalf("initial status=%s want ACTIVE", sup.Status)
	}
}

func TestUpdateSupplierStatus_AllowedTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		wantErr  error
	}{
		{SupplierActive, SupplierSuspended, nil},
		{SupplierSuspended, SupplierRetired, nil},
		{SupplierRetired, SupplierProbation, nil},
		{SupplierProbation, SupplierActive, nil},
		// Disallowed: RETIRED → ACTIVE must go through PROBATION.
		{SupplierRetired, SupplierActive, ErrInvalidStatus},
		// Disallowed: ACTIVE → RETIRED must go through SUSPENDED.
		{SupplierActive, SupplierRetired, ErrInvalidStatus},
	}
	for _, c := range cases {
		t.Run(c.from+"->"+c.to, func(t *testing.T) {
			store := newMemStore()
			svc := NewService(store)
			id, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
			// Force the from-status by mutating the store directly.
			store.suppliers[id].Status = c.from
			err := svc.UpdateSupplierStatus(context.Background(), id, c.to)
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				sup, _ := store.GetSupplier(context.Background(), id)
				if sup.Status != c.to {
					t.Fatalf("status=%s want %s", sup.Status, c.to)
				}
			} else {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("expected %v, got %v", c.wantErr, err)
				}
			}
		})
	}
}

func TestUpdateSupplierStatus_NotFound(t *testing.T) {
	svc := NewService(newMemStore())
	if err := svc.UpdateSupplierStatus(context.Background(), "nope", SupplierSuspended); !errors.Is(err, ErrSupplierNotFound) {
		t.Fatalf("expected ErrSupplierNotFound, got %v", err)
	}
}

func TestCreateSupplierProduct_DuplicatePair(t *testing.T) {
	svc := NewService(newMemStore())
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	if _, err := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"}); !errors.Is(err, ErrDuplicateSupplierProduct) {
		t.Fatalf("duplicate pair must fail, got %v", err)
	}
	// Same supplier, different product → ok.
	if _, err := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p2"}); err != nil {
		t.Fatalf("second product: %v", err)
	}
}

func TestCreateSupplierVariant_RejectsNegativePrice(t *testing.T) {
	svc := NewService(newMemStore())
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	spID, _ := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"})
	neg := int64(-100)
	if _, err := svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PurchasePrice: &neg,
	}); !errors.Is(err, ErrPriceNegative) {
		t.Fatalf("negative price must fail, got %v", err)
	}
}

func TestCreateSupplierVariant_RejectsExpiredPriceValidity(t *testing.T) {
	svc := NewService(newMemStore())
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	spID, _ := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"})
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	expired := now.Add(-1 * time.Hour)
	if _, err := svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PriceValidUntil: &expired,
	}); !errors.Is(err, ErrPriceExpired) {
		t.Fatalf("expired price_valid_until must fail, got %v", err)
	}
}

func TestUpdateSupplierVariantPrice_RecordsHistory(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	spID, _ := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"})
	p1 := int64(100000)
	svID, _ := svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PurchasePrice: &p1,
	})

	// First price change: 100000 → 120000.
	if err := svc.UpdateSupplierVariantPrice(context.Background(), svID, 120000, nil); err != nil {
		t.Fatalf("price update 1: %v", err)
	}
	hist, err := svc.ListPriceHistory(context.Background(), svID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(hist) != 1 {
		t.Fatalf("history rows=%d want 1", len(hist))
	}
	if hist[0].PurchasePrice != 120000 {
		t.Fatalf("history price=%d want 120000", hist[0].PurchasePrice)
	}
	if hist[0].ValidUntil != nil {
		t.Fatalf("newest history row must have open valid_until, got %v", hist[0].ValidUntil)
	}

	// Second price change: 120000 → 90000.
	later := now.Add(1 * time.Hour)
	svc.SetNow(func() time.Time { return later })
	if err := svc.UpdateSupplierVariantPrice(context.Background(), svID, 90000, nil); err != nil {
		t.Fatalf("price update 2: %v", err)
	}
	hist, _ = svc.ListPriceHistory(context.Background(), svID)
	if len(hist) != 2 {
		t.Fatalf("history rows=%d want 2", len(hist))
	}
	// Newest first: hist[0] = 90000 (open), hist[1] = 120000 (closed).
	if hist[0].PurchasePrice != 90000 || hist[0].ValidUntil != nil {
		t.Fatalf("newest row wrong: price=%d valid_until=%v", hist[0].PurchasePrice, hist[0].ValidUntil)
	}
	if hist[1].PurchasePrice != 120000 || hist[1].ValidUntil == nil {
		t.Fatalf("previous row must be closed: price=%d valid_until=%v", hist[1].PurchasePrice, hist[1].ValidUntil)
	}
	if !hist[1].ValidUntil.Equal(later) {
		t.Fatalf("previous row valid_until=%v want %v", *hist[1].ValidUntil, later)
	}

	// The variant's current price must be the latest.
	sv, _ := store.GetSupplierVariant(context.Background(), svID)
	if sv.PurchasePrice == nil || *sv.PurchasePrice != 90000 {
		t.Fatalf("variant price=%v want 90000", sv.PurchasePrice)
	}
}

func TestListSupplierVariantsForVariant_FiltersExpiredAndInactive(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	now := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	svc.SetNow(func() time.Time { return now })
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	spID, _ := svc.CreateSupplierProduct(context.Background(), SupplierProduct{SupplierID: supID, ProductID: "p1"})

	// Three offers for the same variant:
	//  - active, valid price
	//  - active, expired price (price_valid_until in past)
	//  - inactive
	p := int64(100000)
	id1, _ := svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PurchasePrice: &p, Status: SupplierVariantActive,
	})
	expired := now.Add(-1 * time.Hour)
	svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PurchasePrice: &p, PriceValidUntil: &expired,
		Status: SupplierVariantActive,
	})
	svc.CreateSupplierVariant(context.Background(), SupplierVariant{
		SupplierProductID: spID, VariantID: "v1", PurchasePrice: &p, Status: SupplierVariantInactive,
	})

	offers, err := svc.ListSupplierVariantsForVariant(context.Background(), "v1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(offers) != 1 {
		t.Fatalf("offers=%d want 1 (only active+valid)", len(offers))
	}
	if offers[0].ID != id1 {
		t.Fatalf("returned id=%s want %s", offers[0].ID, id1)
	}
}

func TestCreateEvaluation_EnforcesScoreRange(t *testing.T) {
	svc := NewService(newMemStore())
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	// Score > 100 must fail.
	if _, err := svc.CreateEvaluation(context.Background(), Evaluation{
		SupplierID: supID, QualityScore: 101,
	}); !errors.Is(err, ErrScoreOutOfRange) {
		t.Fatalf("score > 100 must fail, got %v", err)
	}
	// Score < 0 must fail.
	if _, err := svc.CreateEvaluation(context.Background(), Evaluation{
		SupplierID: supID, QualityScore: -1,
	}); !errors.Is(err, ErrScoreOutOfRange) {
		t.Fatalf("score < 0 must fail, got %v", err)
	}
}

func TestCreateEvaluation_ComputesWeightedTotal(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	supID, _ := svc.CreateSupplier(context.Background(), Supplier{Name: "S"})
	// All scores = 100 → total = 100.00.
	evID, err := svc.CreateEvaluation(context.Background(), Evaluation{
		SupplierID:   supID,
		QualityScore: 100, PricingScore: 100, AvailabilityScore: 100,
		WarrantyScore: 100, DocumentationScore: 100, FlexibilityScore: 100,
		LogisticsScore: 100, CommunicationScore: 100,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	evs, _ := svc.ListEvaluationsBySupplier(context.Background(), supID)
	if len(evs) != 1 || evs[0].ID != evID {
		t.Fatalf("eval not found")
	}
	if evs[0].TotalScore != 100.0 {
		t.Fatalf("all-100 total=%v want 100", evs[0].TotalScore)
	}
	if evs[0].WeightsVersion != WeightsVersion {
		t.Fatalf("weights_version=%s want %s", evs[0].WeightsVersion, WeightsVersion)
	}

	// Mixed scores: quality=80 (×0.25=20), pricing=60 (×0.20=12), availability=40 (×0.15=6),
	//   warranty=100 (×0.10=10), documentation=50 (×0.10=5), flexibility=70 (×0.10=7),
	//   logistics=90 (×0.05=4.5), communication=30 (×0.05=1.5)
	//   total = 20+12+6+10+5+7+4.5+1.5 = 66.0
	_, err = svc.CreateEvaluation(context.Background(), Evaluation{
		SupplierID:   supID,
		QualityScore: 80, PricingScore: 60, AvailabilityScore: 40,
		WarrantyScore: 100, DocumentationScore: 50, FlexibilityScore: 70,
		LogisticsScore: 90, CommunicationScore: 30,
	})
	if err != nil {
		t.Fatalf("create mixed: %v", err)
	}
	evs, _ = svc.ListEvaluationsBySupplier(context.Background(), supID)
	if evs[0].TotalScore != 66.0 {
		t.Fatalf("mixed total=%v want 66.0", evs[0].TotalScore)
	}
}

func TestComputeWeightedTotal_RoundsToTwoDecimals(t *testing.T) {
	// quality=33 (×0.25=8.25), pricing=33 (×0.20=6.6), availability=33 (×0.15=4.95),
	//   warranty=33 (×0.10=3.3), documentation=33 (×0.10=3.3), flexibility=33 (×0.10=3.3),
	//   logistics=33 (×0.05=1.65), communication=33 (×0.05=1.65)
	//   raw = 8.25+6.6+4.95+3.3+3.3+3.3+1.65+1.65 = 32.999999... → round to 33.00
	ev := Evaluation{
		QualityScore: 33, PricingScore: 33, AvailabilityScore: 33,
		WarrantyScore: 33, DocumentationScore: 33, FlexibilityScore: 33,
		LogisticsScore: 33, CommunicationScore: 33,
	}
	got := computeWeightedTotal(ev)
	if got != 33.0 {
		t.Fatalf("computeWeightedTotal=%v want 33.0", got)
	}
}
