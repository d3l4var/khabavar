// Package suppliers implements the suppliers module (SAD §۴.۲, DB v2 §۷, R-05):
// suppliers, supplier_products, supplier_variants (multi-source with price
// validity), price history, and weighted evaluations.
//
// The signature business rules enforced here:
//   - Weighted evaluation total_score is computed from 8 criteria using the
//     R-05 §۳ weights (quality 25%, pricing 20%, availability 15%, warranty 10%,
//     documentation 10%, flexibility 10%, logistics 5%, communication 5%).
//     The weights_version column records which weight policy produced the score
//     so historical evaluations remain reproducible (§7.4).
//   - Purchase price changes MUST be recorded in supplier_variant_price_history
//     (§7.3). UpdateSupplierVariantPrice writes the new price AND inserts a
//     history row with valid_from=now, valid_until=NULL and closes the previous
//     history row's valid_until in one transaction.
//   - Supplier status transitions are constrained: a RETIRED supplier cannot be
//     reactivated except through PROBATION (review workflow).
package suppliers

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status enums (mirrors CHECK constraints in 000003_supplier.up.sql).
const (
	SupplierActive    = "ACTIVE"
	SupplierProbation = "PROBATION"
	SupplierSuspended = "SUSPENDED"
	SupplierRetired   = "RETIRED"

	SupplierProductActive   = "ACTIVE"
	SupplierProductInactive = "INACTIVE"

	SupplierVariantActive   = "ACTIVE"
	SupplierVariantInactive = "INACTIVE"
)

// WeightsVersion is the current evaluation weight policy version (R-05 §۳).
// Changing the weights bumps this version; historical evaluations keep their
// original weights_version so the total_score stays reproducible.
const WeightsVersion = "v1.0"

// EvalWeights are the R-05 §۳ weights (sum = 1.0).
var EvalWeights = map[string]float64{
	"quality":       0.25,
	"pricing":       0.20,
	"availability":  0.15,
	"warranty":      0.10,
	"documentation": 0.10,
	"flexibility":   0.10,
	"logistics":     0.05,
	"communication": 0.05,
}

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrSupplierNotFound      = errors.New("suppliers: supplier not found")
	ErrSupplierProductNotFound = errors.New("suppliers: supplier product not found")
	ErrSupplierVariantNotFound = errors.New("suppliers: supplier variant not found")
	ErrInvalidStatus         = errors.New("suppliers: invalid status transition")
	ErrDuplicateSupplierProduct = errors.New("suppliers: supplier+product link already exists")
	ErrScoreOutOfRange       = errors.New("suppliers: score must be 0-100")
	ErrNameRequired          = errors.New("suppliers: name required")
	ErrPriceNegative         = errors.New("suppliers: purchase price cannot be negative")
	ErrPriceExpired          = errors.New("suppliers: supplier price expired (price_valid_until in past)")
)

// Supplier mirrors suppliers.
type Supplier struct {
	ID                 string
	Name               string
	SupplierType       string
	RegistrationNumber string
	ContactName        string
	Phone              string
	Email              string
	Address            string
	Website            string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SupplierProduct mirrors supplier_products (a supplier→product link).
type SupplierProduct struct {
	ID                  string
	SupplierID          string
	ProductID           string
	SupplierProductCode string
	MinimumOrderQty     *int
	LeadTimeDays        *int
	Status              string
}

// SupplierVariant mirrors supplier_variants — a supply offer for one variant from
// one supplier. purchase_price is in IRR (int64, ADR-DATA-001). price_valid_until
// enforces the §7.3 rule that supplier prices have a validity period.
type SupplierVariant struct {
	ID                string
	SupplierProductID string
	VariantID         string
	SupplierSKU       string
	PurchasePrice     *int64 // IRR; nil = unknown
	Currency          string
	AvailableQuantity int
	LastStockCheckAt  *time.Time
	PriceValidUntil   *time.Time
	LastPriceCheckAt  *time.Time
	Status            string
}

// PriceHistoryEntry mirrors a supplier_variant_price_history row.
type PriceHistoryEntry struct {
	ID                string
	SupplierVariantID string
	PurchasePrice     int64
	Currency          string
	ValidFrom         time.Time
	ValidUntil        *time.Time
}

// Evaluation mirrors supplier_evaluations. Each criterion score is 0-100;
// total_score is the weighted sum (R-05 §۳).
type Evaluation struct {
	ID                 string
	SupplierID         string
	QualityScore       int
	PricingScore       int
	AvailabilityScore  int
	WarrantyScore      int
	DocumentationScore int
	FlexibilityScore   int
	LogisticsScore     int
	CommunicationScore int
	TotalScore         float64
	EvaluationDate     time.Time
	ReviewerID         string
	WeightsVersion     string
	Notes              string
}

// Store is the persistence boundary for suppliers. The DB implementation uses
// pgx; unit tests use an in-memory implementation.
type Store interface {
	CreateSupplier(ctx context.Context, s Supplier) (string, error)
	GetSupplier(ctx context.Context, id string) (*Supplier, error)
	UpdateSupplierStatus(ctx context.Context, id, status string) error

	CreateSupplierProduct(ctx context.Context, sp SupplierProduct) (string, error)
	GetSupplierProduct(ctx context.Context, id string) (*SupplierProduct, error)
	GetSupplierProductByPair(ctx context.Context, supplierID, productID string) (*SupplierProduct, error)

	CreateSupplierVariant(ctx context.Context, sv SupplierVariant) (string, error)
	GetSupplierVariant(ctx context.Context, id string) (*SupplierVariant, error)
	ListSupplierVariantsByVariant(ctx context.Context, variantID string) ([]SupplierVariant, error)
	UpdateSupplierVariantPrice(ctx context.Context, id string, newPrice int64, validFrom time.Time, validUntil *time.Time, now time.Time) error
	ListPriceHistory(ctx context.Context, supplierVariantID string) ([]PriceHistoryEntry, error)

	CreateEvaluation(ctx context.Context, ev Evaluation) (string, error)
	ListEvaluationsBySupplier(ctx context.Context, supplierID string) ([]Evaluation, error)
}

// Service is the suppliers module boundary (SAD §۴.۲).
type Service struct {
	store Store
	now   func() time.Time
}

// NewService wires the store. now is injected for deterministic tests.
func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// CreateSupplier persists a new supplier in ACTIVE status.
func (s *Service) CreateSupplier(ctx context.Context, sup Supplier) (string, error) {
	if sup.Name == "" {
		return "", ErrNameRequired
	}
	if sup.Status == "" {
		sup.Status = SupplierActive
	}
	if sup.Status != SupplierActive {
		return "", ErrInvalidStatus
	}
	return s.store.CreateSupplier(ctx, sup)
}

// UpdateSupplierStatus transitions a supplier's status. The allowed transitions:
//   ACTIVE → PROBATION | SUSPENDED
//   PROBATION → ACTIVE | SUSPENDED
//   SUSPENDED → PROBATION | RETIRED
//   RETIRED → PROBATION (review-reactivate only)
// A RETIRED→ACTIVE jump is rejected (must go through PROBATION review).
func (s *Service) UpdateSupplierStatus(ctx context.Context, id, newStatus string) error {
	if newStatus != SupplierActive && newStatus != SupplierProbation &&
		newStatus != SupplierSuspended && newStatus != SupplierRetired {
		return ErrInvalidStatus
	}
	existing, err := s.store.GetSupplier(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrSupplierNotFound
	}
	if !allowedTransition(existing.Status, newStatus) {
		return ErrInvalidStatus
	}
	return s.store.UpdateSupplierStatus(ctx, id, newStatus)
}

// allowedTransition enforces the state machine above.
func allowedTransition(from, to string) bool {
	if from == to {
		return true // idempotent
	}
	allowed := map[string][]string{
		SupplierActive:    {SupplierProbation, SupplierSuspended},
		SupplierProbation: {SupplierActive, SupplierSuspended},
		SupplierSuspended: {SupplierProbation, SupplierRetired},
		SupplierRetired:   {SupplierProbation},
	}
	for _, t := range allowed[from] {
		if t == to {
			return true
		}
	}
	return false
}

// CreateSupplierProduct links a supplier to a product. The (supplier_id, product_id)
// pair must be unique (§۱۵) — the Service checks for an existing link first.
func (s *Service) CreateSupplierProduct(ctx context.Context, sp SupplierProduct) (string, error) {
	if sp.SupplierID == "" {
		return "", ErrSupplierNotFound
	}
	if sp.ProductID == "" {
		return "", ErrSupplierProductNotFound
	}
	if sp.Status == "" {
		sp.Status = SupplierProductActive
	}
	existing, err := s.store.GetSupplierProductByPair(ctx, sp.SupplierID, sp.ProductID)
	if err != nil {
		return "", fmt.Errorf("check pair: %w", err)
	}
	if existing != nil {
		return "", ErrDuplicateSupplierProduct
	}
	return s.store.CreateSupplierProduct(ctx, sp)
}

// CreateSupplierVariant registers a supply offer for a variant. Price validity is
// enforced: a price with price_valid_until in the past is rejected (§7.3).
func (s *Service) CreateSupplierVariant(ctx context.Context, sv SupplierVariant) (string, error) {
	if sv.SupplierProductID == "" {
		return "", ErrSupplierProductNotFound
	}
	if sv.VariantID == "" {
		return "", ErrSupplierVariantNotFound
	}
	if sv.PurchasePrice != nil && *sv.PurchasePrice < 0 {
		return "", ErrPriceNegative
	}
	if sv.Currency == "" {
		sv.Currency = "IRR"
	}
	if sv.Status == "" {
		sv.Status = SupplierVariantActive
	}
	now := s.now()
	if sv.PriceValidUntil != nil && sv.PriceValidUntil.Before(now) {
		return "", ErrPriceExpired
	}
	return s.store.CreateSupplierVariant(ctx, sv)
}

// UpdateSupplierVariantPrice changes the purchase price of a supplier variant. Per
// §7.3 every price change MUST be recorded in history: this method writes the new
// price on supplier_variants AND inserts a price_history row with valid_from=now
// and valid_until=NULL, while closing the previously-open history row's valid_until.
// Both writes happen in one transaction (the DB implementation does this).
func (s *Service) UpdateSupplierVariantPrice(ctx context.Context, id string, newPrice int64, validUntil *time.Time) error {
	if newPrice < 0 {
		return ErrPriceNegative
	}
	now := s.now()
	if validUntil != nil && validUntil.Before(now) {
		return ErrPriceExpired
	}
	return s.store.UpdateSupplierVariantPrice(ctx, id, newPrice, now, validUntil, now)
}

// ListSupplierVariantsForVariant returns all active supply offers for a variant
// (the multi-source comparison used by the order-split decision, R-05 §۷).
func (s *Service) ListSupplierVariantsForVariant(ctx context.Context, variantID string) ([]SupplierVariant, error) {
	if variantID == "" {
		return nil, ErrSupplierVariantNotFound
	}
	all, err := s.store.ListSupplierVariantsByVariant(ctx, variantID)
	if err != nil {
		return nil, err
	}
	out := make([]SupplierVariant, 0, len(all))
	now := s.now()
	for _, sv := range all {
		if sv.Status != SupplierVariantActive {
			continue
		}
		// Exclude expired-price offers (§7.3): a price whose validity ended is stale.
		if sv.PriceValidUntil != nil && sv.PriceValidUntil.Before(now) {
			continue
		}
		out = append(out, sv)
	}
	return out, nil
}

// ListPriceHistory returns the full price history for a supplier variant (audit).
func (s *Service) ListPriceHistory(ctx context.Context, supplierVariantID string) ([]PriceHistoryEntry, error) {
	if supplierVariantID == "" {
		return nil, ErrSupplierVariantNotFound
	}
	return s.store.ListPriceHistory(ctx, supplierVariantID)
}

// CreateEvaluation records a weighted supplier evaluation. Each criterion score
// must be 0-100; total_score is computed from the R-05 §۳ weights and stored
// alongside the weights_version so the score is reproducible later.
func (s *Service) CreateEvaluation(ctx context.Context, ev Evaluation) (string, error) {
	if ev.SupplierID == "" {
		return "", ErrSupplierNotFound
	}
	for _, sc := range []int{ev.QualityScore, ev.PricingScore, ev.AvailabilityScore,
		ev.WarrantyScore, ev.DocumentationScore, ev.FlexibilityScore,
		ev.LogisticsScore, ev.CommunicationScore} {
		if sc < 0 || sc > 100 {
			return "", ErrScoreOutOfRange
		}
	}
	if ev.EvaluationDate.IsZero() {
		ev.EvaluationDate = s.now()
	}
	if ev.WeightsVersion == "" {
		ev.WeightsVersion = WeightsVersion
	}
	ev.TotalScore = computeWeightedTotal(ev)
	return s.store.CreateEvaluation(ctx, ev)
}

// computeWeightedTotal applies the R-05 §۳ weights to the criterion scores.
// total = Σ (criterion_score × weight). Result is rounded to 2 decimals.
func computeWeightedTotal(ev Evaluation) float64 {
	raw := float64(ev.QualityScore)*EvalWeights["quality"] +
		float64(ev.PricingScore)*EvalWeights["pricing"] +
		float64(ev.AvailabilityScore)*EvalWeights["availability"] +
		float64(ev.WarrantyScore)*EvalWeights["warranty"] +
		float64(ev.DocumentationScore)*EvalWeights["documentation"] +
		float64(ev.FlexibilityScore)*EvalWeights["flexibility"] +
		float64(ev.LogisticsScore)*EvalWeights["logistics"] +
		float64(ev.CommunicationScore)*EvalWeights["communication"]
	// Round to 2 decimals (NUMERIC(5,2) in DB).
	return float64(int(raw*100+0.5)) / 100
}

// ListEvaluationsBySupplier returns all evaluations for a supplier, newest first.
func (s *Service) ListEvaluationsBySupplier(ctx context.Context, supplierID string) ([]Evaluation, error) {
	if supplierID == "" {
		return nil, ErrSupplierNotFound
	}
	return s.store.ListEvaluationsBySupplier(ctx, supplierID)
}
