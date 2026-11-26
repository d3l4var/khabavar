// Package pricing implements the pricing module (SAD §۴.۲, DB v2 §۹, R-05): landed
// cost of goods (product_cost_records), approved selling prices (product_price_records),
// per-variant unit economics, and the order financial snapshot basis.
//
// All monetary amounts are BIGINT rials (IRR); the UI may display toman but the
// conversion happens in a single presentation layer (R-05 §۲۲). Arithmetic is done in
// code, never in the head.
//
// Signature rules implemented here:
//   - TotalCost sums all eight landed-cost components (additive, including shipping
//     subsidy — see R-05 §۱۹ worked example).
//   - NetRevenue = SellingPrice − Discount; Contribution = NetRevenue − TotalCost;
//     ContributionRate = Contribution / NetRevenue (zero-division guarded).
//   - A cost/price record is "currently valid" only inside its [valid_from, valid_until)
//     window; expired records are never used as the live price (R-05 §۲۲).
//   - A selling price requires approval by an authorised role before it is live
//     (R-05 §۹.۲ — approved_by must be set).
//   - BR-06: a non-positive contribution is flagged as a warning (not blocked) for
//     review; the record is still created pending approval.
package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Supply types (mirrors product_cost_records.supply_type CHECK).
const (
	SupplyOwnedInventory  = "OWNED_INVENTORY"
	SupplySupplierSourced = "SUPPLIER_SOURCED"
)

// DefaultCurrency is rials (R-05 §۲۲).
const DefaultCurrency = "IRR"

// Sentinel errors.
var (
	ErrVariantNotFound      = errors.New("pricing: variant not found")
	ErrInvalidSupplyType    = errors.New("pricing: invalid supply type")
	ErrSupplierRequired     = errors.New("pricing: supplier_id required for SUPPLIER_SOURCED")
	ErrNoValidCost          = errors.New("pricing: no currently-valid cost record")
	ErrNoApprovedPrice      = errors.New("pricing: no approved price record in validity window")
	ErrPriceAlreadyApproved = errors.New("pricing: price record already approved")
	ErrInvalidTargetRate    = errors.New("pricing: target contribution rate must be in [0,1)")
	ErrNegativeContribution = errors.New("pricing: calculated contribution is non-positive (BR-06)")
)

// CostRecord mirrors product_cost_records. Amounts are rials.
type CostRecord struct {
	ID                       string
	VariantID                string
	SupplyType               string
	SupplierID               string
	PurchaseCost             int64
	InboundFreight           int64
	InspectionCost           int64
	PackagingCost            int64
	FulfillmentCost          int64
	EstimatedPaymentFee      int64
	EstimatedShippingSubsidy int64
	ExpectedReturnCost       int64
	Currency                 string
	ValidFrom                time.Time
	ValidUntil               *time.Time
	CreatedBy                string
}

// PriceRecord mirrors product_price_records. SellingPrice is rials ex-tax.
type PriceRecord struct {
	ID                     string
	VariantID              string
	SellingPrice           int64
	DiscountPolicy         json.RawMessage
	TargetContributionRate *float64
	CalculatedContribution *int64
	Currency               string
	ValidFrom              time.Time
	ValidUntil             *time.Time
	ApprovedBy             string
}

// UnitEconomics is the per-variant computed view (R-05 §۲۱).
type UnitEconomics struct {
	VariantID              string
	Currency               string
	SellingPrice           int64
	Discount               int64
	NetRevenue             int64
	TotalCost              int64
	Contribution           int64
	ContributionRate       float64
	TargetContributionRate *float64
	Warnings               []string `json:"warnings,omitempty"`
}

// Store is the persistence boundary for pricing.
type Store interface {
	CreateCostRecord(ctx context.Context, c CostRecord) (string, error)
	ListCostRecordsByVariant(ctx context.Context, variantID string) ([]CostRecord, error)
	// GetValidCostRecord returns the most recent cost record whose validity window
	// contains `now`, or (nil, nil) if none.
	GetValidCostRecord(ctx context.Context, variantID string, now time.Time) (*CostRecord, error)
	CreatePriceRecord(ctx context.Context, p PriceRecord) (string, error)
	ApprovePriceRecord(ctx context.Context, id, approverID string) error
	// GetValidPriceRecord returns the most recent APPROVED price record whose validity
	// window contains `now`, or (nil, nil) if none.
	GetValidPriceRecord(ctx context.Context, variantID string, now time.Time) (*PriceRecord, error)
	ListPriceRecordsByVariant(ctx context.Context, variantID string) ([]PriceRecord, error)
}

// Service is the pricing module boundary.
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// ── pure arithmetic (R-05 §۱۹ worked example is the verification anchor) ───────

// TotalCost sums all eight landed-cost components. Subsidy is additive: it is a cost
// KhAbavar carries (R-05 §۱۹: 400+30+40+10+50+20 = 550k toman total variable cost).
func TotalCost(c CostRecord) int64 {
	return c.PurchaseCost + c.InboundFreight + c.InspectionCost + c.PackagingCost +
		c.FulfillmentCost + c.EstimatedPaymentFee + c.EstimatedShippingSubsidy +
		c.ExpectedReturnCost
}

// DiscountAmount extracts the rial discount from a discount_policy JSONB. Expected
// shape: {"amount": <int64>, ...}. Returns 0 for empty/invalid policy.
func DiscountAmount(policy json.RawMessage) int64 {
	if len(policy) == 0 {
		return 0
	}
	var p struct {
		Amount int64 `json:"amount"`
	}
	if err := json.Unmarshal(policy, &p); err != nil {
		return 0
	}
	if p.Amount < 0 {
		return 0
	}
	return p.Amount
}

// NetRevenue = selling price ex-tax − discount.
func NetRevenue(sellingPrice, discount int64) int64 {
	net := sellingPrice - discount
	if net < 0 {
		return 0
	}
	return net
}

// Contribution = net revenue − total cost. May be negative (BR-06).
func Contribution(netRevenue, totalCost int64) int64 {
	return netRevenue - totalCost
}

// ContributionRate = contribution / net revenue, or 0 when net revenue is zero.
func ContributionRate(contribution, netRevenue int64) float64 {
	if netRevenue == 0 {
		return 0
	}
	return float64(contribution) / float64(netRevenue)
}

// TargetPrice computes TotalCost / (1 − targetRate) (R-05 §۱۸). targetRate must be in
// [0, 1); returns ErrInvalidTargetRate otherwise.
func TargetPrice(totalCost int64, targetRate float64) (int64, error) {
	if targetRate < 0 || targetRate >= 1 {
		return 0, ErrInvalidTargetRate
	}
	return int64(float64(totalCost) / (1 - targetRate)), nil
}

// ── Service methods ────────────────────────────────────────────────────────────

// CreateCostRecord validates supply type and currency, then persists. For
// SUPPLIER_SOURCED the supplier_id is required.
func (s *Service) CreateCostRecord(ctx context.Context, c CostRecord) (string, error) {
	if c.SupplyType != SupplyOwnedInventory && c.SupplyType != SupplySupplierSourced {
		return "", ErrInvalidSupplyType
	}
	if c.SupplyType == SupplySupplierSourced && c.SupplierID == "" {
		return "", ErrSupplierRequired
	}
	if c.Currency == "" {
		c.Currency = DefaultCurrency
	}
	if c.ValidFrom.IsZero() {
		c.ValidFrom = s.now()
	}
	return s.store.CreateCostRecord(ctx, c)
}

// CreatePriceRecord computes calculated_contribution from the currently-valid cost
// record and the new selling price, then persists the price as PENDING approval. It
// returns the created id and the computed contribution (which may be negative — BR-06).
// A non-positive contribution is NOT blocked (the price is stored pending approval);
// callers surface it as a warning via CreatePriceRecordWithWarning or by inspecting the
// returned contribution. ErrNoValidCost is returned when no cost record covers `now`.
func (s *Service) CreatePriceRecord(ctx context.Context, p PriceRecord) (string, int64, error) {
	if p.Currency == "" {
		p.Currency = DefaultCurrency
	}
	if p.ValidFrom.IsZero() {
		p.ValidFrom = s.now()
	}
	now := s.now()
	cost, err := s.store.GetValidCostRecord(ctx, p.VariantID, now)
	if err != nil {
		return "", 0, fmt.Errorf("load valid cost: %w", err)
	}
	if cost == nil {
		return "", 0, ErrNoValidCost
	}
	discount := DiscountAmount(p.DiscountPolicy)
	net := NetRevenue(p.SellingPrice, discount)
	contrib := Contribution(net, TotalCost(*cost))
	p.CalculatedContribution = &contrib
	id, err := s.store.CreatePriceRecord(ctx, p)
	if err != nil {
		return "", 0, err
	}
	return id, contrib, nil
}

// PriceCreateResult carries the created id plus any BR-06 warning so callers can
// decide whether to alert without parsing errors.
type PriceCreateResult struct {
	ID             string
	Warnings       []string
	NegativeContri bool
}

// CreatePriceRecordWithWarning wraps CreatePriceRecord and surfaces the BR-06
// negative-contribution signal as a warning rather than an error.
func (s *Service) CreatePriceRecordWithWarning(ctx context.Context, p PriceRecord) (PriceCreateResult, error) {
	id, contrib, err := s.CreatePriceRecord(ctx, p)
	if err != nil {
		return PriceCreateResult{}, err
	}
	res := PriceCreateResult{ID: id}
	if contrib <= 0 {
		res.NegativeContri = true
		res.Warnings = append(res.Warnings, "BR-06: non-positive contribution — review before approval")
	}
	return res, nil
}

// ApprovePriceRecord sets the approver on a pending price record (R-05 §۹.۲). A
// already-approved record returns ErrPriceAlreadyApproved.
func (s *Service) ApprovePriceRecord(ctx context.Context, id, approverID string) error {
	if approverID == "" {
		return errors.New("pricing: approver id required")
	}
	return s.store.ApprovePriceRecord(ctx, id, approverID)
}

// UnitEconomics computes the live per-variant economics from the currently-valid
// approved price and the currently-valid cost. Warnings are collected (not fatal):
// missing/expired cost, no approved price, non-positive contribution (BR-06).
func (s *Service) UnitEconomics(ctx context.Context, variantID string) (*UnitEconomics, error) {
	now := s.now()
	cost, err := s.store.GetValidCostRecord(ctx, variantID, now)
	if err != nil {
		return nil, fmt.Errorf("load cost: %w", err)
	}
	price, err := s.store.GetValidPriceRecord(ctx, variantID, now)
	if err != nil {
		return nil, fmt.Errorf("load price: %w", err)
	}
	ue := &UnitEconomics{VariantID: variantID, Currency: DefaultCurrency}
	if cost != nil {
		ue.TotalCost = TotalCost(*cost)
		ue.Currency = cost.Currency
	} else {
		ue.Warnings = append(ue.Warnings, "no currently-valid cost record")
	}
	if price != nil {
		ue.SellingPrice = price.SellingPrice
		ue.Discount = DiscountAmount(price.DiscountPolicy)
		ue.NetRevenue = NetRevenue(price.SellingPrice, ue.Discount)
		ue.Contribution = Contribution(ue.NetRevenue, ue.TotalCost)
		ue.ContributionRate = ContributionRate(ue.Contribution, ue.NetRevenue)
		ue.TargetContributionRate = price.TargetContributionRate
		ue.Currency = price.Currency
		if ue.Contribution <= 0 {
			ue.Warnings = append(ue.Warnings, "BR-06: non-positive contribution")
		}
	} else {
		ue.Warnings = append(ue.Warnings, "no approved price record in validity window")
	}
	return ue, nil
}

// ListCostHistory returns all cost records for a variant (newest first).
func (s *Service) ListCostHistory(ctx context.Context, variantID string) ([]CostRecord, error) {
	return s.store.ListCostRecordsByVariant(ctx, variantID)
}

// ListPriceHistory returns all price records for a variant (newest first).
func (s *Service) ListPriceHistory(ctx context.Context, variantID string) ([]PriceRecord, error) {
	return s.store.ListPriceRecordsByVariant(ctx, variantID)
}

// StorefrontPrice is the customer-visible price slice: selling price, discount and
// net revenue only. Cost/contribution are confidential (R-05 §۲۴ line 489) and never
// appear here.
type StorefrontPrice struct {
	VariantID    string
	SellingPrice int64
	Discount     int64
	NetRevenue   int64
	Currency     string
}

// StorefrontPrice returns the currently-valid APPROVED price for a variant, or
// ErrNoApprovedPrice when none is live (the storefront then treats it as 404 / "not
// for sale").
func (s *Service) StorefrontPrice(ctx context.Context, variantID string) (*StorefrontPrice, error) {
	now := s.now()
	price, err := s.store.GetValidPriceRecord(ctx, variantID, now)
	if err != nil {
		return nil, fmt.Errorf("load price: %w", err)
	}
	if price == nil {
		return nil, ErrNoApprovedPrice
	}
	discount := DiscountAmount(price.DiscountPolicy)
	return &StorefrontPrice{
		VariantID:    variantID,
		SellingPrice: price.SellingPrice,
		Discount:     discount,
		NetRevenue:   NetRevenue(price.SellingPrice, discount),
		Currency:     price.Currency,
	}, nil
}
