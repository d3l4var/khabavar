// suppliers_pg_test.go exercises the suppliers DBStore against the real schema: the
// eight-state lifecycle, the multi-source variant offers, the append-only price history
// (§7.3 "every price change must be recorded") and the weighted evaluations.
//
// Skipped unless KH_TEST_PG_DSN is set.
package suppliers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupSuppliers(t *testing.T) (*pgxpool.Pool, *Service, *DBStore) {
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

// seedProductVariant inserts a product + variant and returns both ids.
func seedProductVariant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sku string) (productID, variantID string) {
	t.Helper()
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
	return productID, variantID
}

// TestPG_SupplierLifecycle walks the approved state machine (IAM §۴ note, R-05 §۲).
func TestPG_SupplierLifecycle(t *testing.T) {
	pool, svc, store := setupSuppliers(t)
	ctx := context.Background()

	id, err := svc.CreateSupplier(ctx, Supplier{Name: "تأمین‌کننده الف", SupplierType: "MANUFACTURER"})
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	got, err := store.GetSupplier(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("GetSupplier: %v", err)
	}
	if got.Status != SupplierActive {
		t.Fatalf("status = %s, want ACTIVE", got.Status)
	}

	// ACTIVE → PROBATION → SUSPENDED → RETIRED is allowed.
	for _, next := range []string{SupplierProbation, SupplierSuspended, SupplierRetired} {
		if err := svc.UpdateSupplierStatus(ctx, id, next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	// RETIRED → ACTIVE must go through PROBATION review.
	if err := svc.UpdateSupplierStatus(ctx, id, SupplierActive); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("RETIRED→ACTIVE: want ErrInvalidStatus, got %v", err)
	}
	if err := svc.UpdateSupplierStatus(ctx, id, SupplierProbation); err != nil {
		t.Fatalf("RETIRED→PROBATION must be allowed: %v", err)
	}
	// An unknown status is refused before touching the row.
	if err := svc.UpdateSupplierStatus(ctx, id, "NOPE"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("unknown status: want ErrInvalidStatus, got %v", err)
	}
	if err := svc.UpdateSupplierStatus(ctx, "00000000-0000-0000-0000-000000000000", SupplierActive); !errors.Is(err, ErrSupplierNotFound) {
		t.Fatalf("missing supplier: want ErrSupplierNotFound, got %v", err)
	}

	// The DB CHECK must agree with the service vocabulary.
	if _, err := pool.Exec(ctx, `UPDATE suppliers SET status='NOPE' WHERE id=$1`, id); err == nil {
		t.Fatal("chk_suppliers_status must reject an unknown status")
	}
}

// TestPG_SupplierVariantAndPriceHistory is the §7.3 rule: every price change appends a
// history row and closes the previous one.
func TestPG_SupplierVariantAndPriceHistory(t *testing.T) {
	pool, svc, store := setupSuppliers(t)
	ctx := context.Background()
	productID, variantID := seedProductVariant(t, ctx, pool, "SKU-SUP-1")

	supplierID, err := svc.CreateSupplier(ctx, Supplier{Name: "تأمین‌کننده ب", SupplierType: "DISTRIBUTOR"})
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	spID, err := svc.CreateSupplierProduct(ctx, SupplierProduct{
		SupplierID: supplierID, ProductID: productID, SupplierProductCode: "SP-1",
	})
	if err != nil {
		t.Fatalf("CreateSupplierProduct: %v", err)
	}
	// The (supplier, product) pair is unique.
	if _, err := svc.CreateSupplierProduct(ctx, SupplierProduct{SupplierID: supplierID, ProductID: productID}); err == nil {
		t.Fatal("duplicate supplier-product pair must fail")
	}
	byPair, err := store.GetSupplierProductByPair(ctx, supplierID, productID)
	if err != nil || byPair == nil || byPair.ID != spID {
		t.Fatalf("GetSupplierProductByPair = %+v, err %v", byPair, err)
	}

	price := int64(400000)
	validUntil := time.Now().Add(720 * time.Hour)
	svID, err := svc.CreateSupplierVariant(ctx, SupplierVariant{
		SupplierProductID: spID, VariantID: variantID, SupplierSKU: "SUP-SKU-1",
		PurchasePrice: &price, AvailableQuantity: 50, PriceValidUntil: &validUntil,
	})
	if err != nil {
		t.Fatalf("CreateSupplierVariant: %v", err)
	}
	offers, err := svc.ListSupplierVariantsForVariant(ctx, variantID)
	if err != nil {
		t.Fatalf("ListSupplierVariantsForVariant: %v", err)
	}
	if len(offers) != 1 || offers[0].ID != svID {
		t.Fatalf("offers = %+v", offers)
	}

	// An already-expired validity date is refused.
	past := time.Now().Add(-time.Hour)
	if _, err := svc.CreateSupplierVariant(ctx, SupplierVariant{
		SupplierProductID: spID, VariantID: variantID, PurchasePrice: &price, PriceValidUntil: &past,
	}); !errors.Is(err, ErrPriceExpired) {
		t.Fatalf("expired price validity: want ErrPriceExpired, got %v", err)
	}

	// Change the price twice: three history rows in total (initial + two changes).
	if err := svc.UpdateSupplierVariantPrice(ctx, svID, 420000, &validUntil); err != nil {
		t.Fatalf("UpdateSupplierVariantPrice: %v", err)
	}
	if err := svc.UpdateSupplierVariantPrice(ctx, svID, 435000, &validUntil); err != nil {
		t.Fatalf("UpdateSupplierVariantPrice 2: %v", err)
	}
	history, err := svc.ListPriceHistory(ctx, svID)
	if err != nil {
		t.Fatalf("ListPriceHistory: %v", err)
	}
	if len(history) != 3 {
		t.Fatalf("price history = %d rows, want 3 (initial + two changes)", len(history))
	}
	// Exactly one row may stay open (valid_until IS NULL) — the current price.
	var open int
	for _, h := range history {
		if h.ValidUntil == nil {
			open++
		}
	}
	if open != 1 {
		t.Fatalf("open history rows = %d, want exactly 1", open)
	}
	// The live price on the offer must match the newest history entry.
	offer, err := store.GetSupplierVariant(ctx, svID)
	if err != nil || offer == nil {
		t.Fatalf("GetSupplierVariant: %v", err)
	}
	if offer.PurchasePrice == nil || *offer.PurchasePrice != 435000 {
		t.Fatalf("purchase_price = %v, want 435000", offer.PurchasePrice)
	}
}

// TestPG_SupplierEvaluation: the eight weighted scores persist and read back.
func TestPG_SupplierEvaluation(t *testing.T) {
	pool, svc, _ := setupSuppliers(t)
	ctx := context.Background()

	supplierID, err := svc.CreateSupplier(ctx, Supplier{Name: "تأمین‌کننده ج", SupplierType: "IMPORTER"})
	if err != nil {
		t.Fatalf("CreateSupplier: %v", err)
	}
	id, err := svc.CreateEvaluation(ctx, Evaluation{
		SupplierID:   supplierID,
		QualityScore: 90, PricingScore: 70, AvailabilityScore: 80, WarrantyScore: 60,
		DocumentationScore: 75, FlexibilityScore: 65, LogisticsScore: 85,
		CommunicationScore: 95, TotalScore: 78.5,
		EvaluationDate: time.Now(), WeightsVersion: "v1",
	})
	if err != nil {
		t.Fatalf("CreateEvaluation: %v", err)
	}
	if id == "" {
		t.Fatal("empty evaluation id")
	}
	evals, err := svc.ListEvaluationsBySupplier(ctx, supplierID)
	if err != nil {
		t.Fatalf("ListEvaluationsBySupplier: %v", err)
	}
	if len(evals) != 1 {
		t.Fatalf("evaluations = %d, want 1", len(evals))
	}
	if evals[0].QualityScore != 90 || evals[0].CommunicationScore != 95 {
		t.Fatalf("evaluation = %+v", evals[0])
	}

	// A score outside 0..100 is refused by the DB CHECK (defence in depth).
	if _, err := pool.Exec(ctx,
		`INSERT INTO supplier_evaluations (supplier_id, quality_score, pricing_score, availability_score,
		    warranty_score, documentation_score, flexibility_score, logistics_score, communication_score,
		    total_score, evaluation_date)
		 VALUES ($1, 101, 50, 50, 50, 50, 50, 50, 50, 50, now())`, supplierID); err == nil {
		t.Fatal("quality_score > 100 must be rejected")
	}
}
