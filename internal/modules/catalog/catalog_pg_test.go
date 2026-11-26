// catalog_pg_test.go exercises the catalog DBStore against the real schema. The unit
// tests use an in-memory fake, so a column rename or a missing CHECK would only surface
// at runtime — exactly how the refunds bug reached the api unit. Skipped unless
// KH_TEST_PG_DSN is set.
package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func setupPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	if err := migrations.ApplyAll(context.Background(), pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

// TestPG_CatalogStoreLifecycle walks the store the way the panel and the storefront do:
// create a category, a product and a variant, publish, then read each back through the
// storefront queries.
func TestPG_CatalogStoreLifecycle(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store)
	svc.SetSearchStore(store)
	svc.SetUpdateStore(store)

	catID, err := svc.CreateCategory(ctx, Category{Name: "بالش", Slug: "pillows"})
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	cats, err := store.ListCategories(ctx)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 1 || cats[0].Slug != "pillows" {
		t.Fatalf("categories = %+v", cats)
	}

	reviewed := time.Now().UTC().Truncate(time.Second)
	prodID, err := svc.CreateProduct(ctx, Product{
		Title: "بالش طبی", Slug: "memory-pillow",
		SleepRelevance: "SLEEP_AID", SpecSource: "سازنده",
		SpecLastReviewedAt: &reviewed,
		CategoryIDs:        []string{catID},
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	// A draft must not be visible to the storefront, but the panel must still read it.
	if _, err := svc.GetStorefrontProduct(ctx, prodID); err == nil {
		t.Error("a DRAFT product must not be readable through the storefront detail")
	}
	if _, err := svc.GetProductDetail(ctx, prodID); err != nil {
		t.Fatalf("the panel must still read a DRAFT product: %v", err)
	}

	if err := svc.PublishProduct(ctx, prodID); err != nil {
		t.Fatalf("PublishProduct: %v", err)
	}
	detail, err := svc.GetStorefrontProduct(ctx, prodID)
	if err != nil {
		t.Fatalf("GetProductDetail after publish: %v", err)
	}
	if detail.Product.Status != ProductPublished {
		t.Fatalf("status = %s, want PUBLISHED", detail.Product.Status)
	}

	variantID, err := svc.CreateVariant(ctx, Variant{ProductID: prodID, SKU: "SKU-PG-1", Status: VariantActive})
	if err != nil {
		t.Fatalf("CreateVariant: %v", err)
	}
	variants, err := store.ListVariantsByProduct(ctx, prodID)
	if err != nil {
		t.Fatalf("ListVariantsByProduct: %v", err)
	}
	if len(variants) != 1 || variants[0].ID != variantID {
		t.Fatalf("variants = %+v", variants)
	}
	bySKU, err := store.GetVariantBySKU(ctx, "SKU-PG-1")
	if err != nil || bySKU == nil || bySKU.ID != variantID {
		t.Fatalf("GetVariantBySKU = %+v, err %v", bySKU, err)
	}

	// Duplicate SKU is a domain error, not a raw unique violation.
	if _, err := svc.CreateVariant(ctx, Variant{ProductID: prodID, SKU: "SKU-PG-1"}); err == nil {
		t.Error("duplicate SKU must fail")
	}

	// Storefront listing by category.
	prods, total, err := svc.ListPublishedByCategory(ctx, catID, 1, 20)
	if err != nil {
		t.Fatalf("ListPublishedByCategory: %v", err)
	}
	if total != 1 || len(prods) != 1 {
		t.Fatalf("category listing: total=%d len=%d", total, len(prods))
	}

	// Search (ILIKE path in the DB store, not the fallback).
	hits, total, err := svc.SearchProducts(ctx, SearchFilter{Query: "بالش", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	if total != 1 || len(hits) != 1 || hits[0].Product.ID != prodID {
		t.Fatalf("search: total=%d hits=%+v", total, hits)
	}

	// Pause removes it from the storefront without deleting anything.
	if err := svc.PauseProduct(ctx, prodID); err != nil {
		t.Fatalf("PauseProduct: %v", err)
	}
	if _, _, err := svc.SearchProducts(ctx, SearchFilter{Query: "بالش", Page: 1, PageSize: 20}); err != nil {
		t.Fatalf("search after pause: %v", err)
	}
	prods, total, err = svc.ListPublishedByCategory(ctx, catID, 1, 20)
	if err != nil {
		t.Fatalf("listing after pause: %v", err)
	}
	if total != 0 || len(prods) != 0 {
		t.Fatalf("a PAUSED product must leave the storefront: total=%d", total)
	}
}

// TestPG_CatalogPublishGateIsEnforcedByTheDatabase: the CHECK constraint must reject a
// PUBLISHED row without the gate fields even if the service is bypassed.
func TestPG_CatalogPublishGateIsEnforcedByTheDatabase(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('x','gate-bypass','DRAFT') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE products SET status='PUBLISHED' WHERE id=$1`, id); err == nil {
		t.Fatal("chk_products_publish_gates must reject a PUBLISHED product without gate fields")
	}
}
