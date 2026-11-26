// pricing_pg_test.go exercises the pricing DBStore against the real schema: the eight
// cost components, the approval gate on selling prices, the validity windows and the
// unit-economics arithmetic. Pricing is a financial module, so R-CI §۳ requires ≥90%
// coverage here and a wrong column name is a money bug, not a cosmetic one.
//
// Skipped unless KH_TEST_PG_DSN is set.
package pricing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// seedVariant inserts a product + variant and returns the variant id.
func seedVariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sku string) string {
	t.Helper()
	var productID, variantID string
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
	return variantID
}

// setupPricing applies migrations and returns a service wired to the real store.
func setupPricing(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
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

// TestPG_PricingCostAndPriceLifecycle walks the R-05 chain: cost → price → approval →
// live price → unit economics.
func TestPG_PricingCostAndPriceLifecycle(t *testing.T) {
	pool, svc, store := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PRICE-1")

	costID, err := svc.CreateCostRecord(ctx, CostRecord{
		VariantID:                variantID,
		SupplyType:               SupplyOwnedInventory,
		PurchaseCost:             400000,
		InboundFreight:           20000,
		InspectionCost:           5000,
		PackagingCost:            15000,
		FulfillmentCost:          30000,
		EstimatedPaymentFee:      12000,
		EstimatedShippingSubsidy: 25000,
		ExpectedReturnCost:       8000,
	})
	if err != nil {
		t.Fatalf("CreateCostRecord: %v", err)
	}
	if costID == "" {
		t.Fatal("empty cost record id")
	}

	// The live cost must be readable and sum all eight components.
	cost, err := store.GetValidCostRecord(ctx, variantID, time.Now())
	if err != nil {
		t.Fatalf("GetValidCostRecord: %v", err)
	}
	if cost == nil {
		t.Fatal("no valid cost record after creation")
	}
	if got := TotalCost(*cost); got != 515000 {
		t.Fatalf("TotalCost = %d, want 515000 (400000+20000+5000+15000+30000+12000+25000+8000)", got)
	}
	if cost.Currency != DefaultCurrency {
		t.Fatalf("currency = %q, want %q", cost.Currency, DefaultCurrency)
	}

	history, err := store.ListCostRecordsByVariant(ctx, variantID)
	if err != nil {
		t.Fatalf("ListCostRecordsByVariant: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("cost history = %d rows, want 1", len(history))
	}

	// A selling price needs an approved record before it is live (R-05 §۹.۲).
	priceID, contribution, err := svc.CreatePriceRecord(ctx, PriceRecord{
		VariantID:    variantID,
		SellingPrice: 900000,
	})
	if err != nil {
		t.Fatalf("CreatePriceRecord: %v", err)
	}
	if contribution != 385000 {
		t.Fatalf("contribution = %d, want 385000 (900000 − 515000)", contribution)
	}
	if live, err := store.GetValidPriceRecord(ctx, variantID, time.Now()); err != nil {
		t.Fatalf("GetValidPriceRecord: %v", err)
	} else if live != nil {
		t.Fatal("an unapproved price must not be live")
	}

	approverID := seedAdmin(t, ctx, pool)
	if err := svc.ApprovePriceRecord(ctx, priceID, approverID); err != nil {
		t.Fatalf("ApprovePriceRecord: %v", err)
	}
	live, err := store.GetValidPriceRecord(ctx, variantID, time.Now())
	if err != nil {
		t.Fatalf("GetValidPriceRecord after approve: %v", err)
	}
	if live == nil {
		t.Fatal("an approved price must be live")
	}
	if live.ApprovedBy != approverID {
		t.Fatalf("approved_by = %q, want %q", live.ApprovedBy, approverID)
	}
	if live.CalculatedContribution == nil || *live.CalculatedContribution != 385000 {
		t.Fatalf("persisted contribution = %v, want 385000", live.CalculatedContribution)
	}

	// Approving twice is a domain error, not a silent overwrite.
	if err := svc.ApprovePriceRecord(ctx, priceID, approverID); !errors.Is(err, ErrPriceAlreadyApproved) {
		t.Fatalf("second approve: want ErrPriceAlreadyApproved, got %v", err)
	}

	prices, err := store.ListPriceRecordsByVariant(ctx, variantID)
	if err != nil {
		t.Fatalf("ListPriceRecordsByVariant: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("price history = %d rows, want 1", len(prices))
	}

	// Unit economics from the live price and cost.
	ue, err := svc.UnitEconomics(ctx, variantID)
	if err != nil {
		t.Fatalf("UnitEconomics: %v", err)
	}
	if ue.SellingPrice != 900000 || ue.TotalCost != 515000 || ue.Contribution != 385000 {
		t.Fatalf("unit economics = %+v", ue)
	}
	if ue.ContributionRate < 0.42 || ue.ContributionRate > 0.43 {
		t.Fatalf("contribution rate = %v, want ~0.4278", ue.ContributionRate)
	}
}

// TestPG_PricingExpiredCostIsNotLive pins the validity window rule (R-05 §۲۲): an
// expired record is never used as the live cost.
func TestPG_PricingExpiredCostIsNotLive(t *testing.T) {
	pool, svc, store := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PRICE-2")

	past := time.Now().Add(-48 * time.Hour)
	ended := time.Now().Add(-24 * time.Hour)
	if _, err := svc.CreateCostRecord(ctx, CostRecord{
		VariantID: variantID, SupplyType: SupplyOwnedInventory,
		PurchaseCost: 100000, ValidFrom: past, ValidUntil: &ended,
	}); err != nil {
		t.Fatalf("CreateCostRecord: %v", err)
	}
	cost, err := store.GetValidCostRecord(ctx, variantID, time.Now())
	if err != nil {
		t.Fatalf("GetValidCostRecord: %v", err)
	}
	if cost != nil {
		t.Fatalf("an expired cost record must not be live, got %+v", cost)
	}

	// Pricing without a live cost is refused with the domain sentinel.
	if _, _, err := svc.CreatePriceRecord(ctx, PriceRecord{VariantID: variantID, SellingPrice: 200000}); !errors.Is(err, ErrNoValidCost) {
		t.Fatalf("want ErrNoValidCost, got %v", err)
	}
}

// TestPG_PricingSupplierSourcedNeedsSupplier: the CHECK and the service rule agree that
// a SUPPLIER_SOURCED cost must name its supplier.
func TestPG_PricingSupplierSourcedNeedsSupplier(t *testing.T) {
	pool, svc, _ := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PRICE-3")

	if _, err := svc.CreateCostRecord(ctx, CostRecord{
		VariantID: variantID, SupplyType: SupplySupplierSourced, PurchaseCost: 1000,
	}); !errors.Is(err, ErrSupplierRequired) {
		t.Fatalf("want ErrSupplierRequired, got %v", err)
	}
	if _, err := svc.CreateCostRecord(ctx, CostRecord{
		VariantID: variantID, SupplyType: "NOT_A_TYPE", PurchaseCost: 1000,
	}); !errors.Is(err, ErrInvalidSupplyType) {
		t.Fatalf("want ErrInvalidSupplyType, got %v", err)
	}
}

// TestPG_PricingBR06NegativeContribution: a below-cost price is stored (pending review)
// and surfaced as a warning, not rejected (BR-06).
func TestPG_PricingBR06NegativeContribution(t *testing.T) {
	pool, svc, store := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PRICE-4")

	if _, err := svc.CreateCostRecord(ctx, CostRecord{
		VariantID: variantID, SupplyType: SupplyOwnedInventory, PurchaseCost: 500000,
	}); err != nil {
		t.Fatalf("CreateCostRecord: %v", err)
	}
	res, err := svc.CreatePriceRecordWithWarning(ctx, PriceRecord{VariantID: variantID, SellingPrice: 400000})
	if err != nil {
		t.Fatalf("CreatePriceRecordWithWarning: %v", err)
	}
	if !res.NegativeContri || len(res.Warnings) == 0 {
		t.Fatalf("BR-06 warning missing: %+v", res)
	}
	prices, err := store.ListPriceRecordsByVariant(ctx, variantID)
	if err != nil {
		t.Fatalf("ListPriceRecordsByVariant: %v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("the below-cost price must still be persisted for review, got %d rows", len(prices))
	}
	if prices[0].CalculatedContribution == nil || *prices[0].CalculatedContribution >= 0 {
		t.Fatalf("calculated_contribution = %v, want a negative value", prices[0].CalculatedContribution)
	}
}

// seedAdmin inserts an ACTIVE admin user and returns its id (approved_by FK).
func seedAdmin(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ('pricing@khabavar.ir','مالی','ACTIVE')
		 RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	return id
}
