// catalog_pg_test.go exercises the catalog DBStore against the real schema. The unit
// tests use an in-memory fake, so a column rename or a missing CHECK would only surface
// at runtime — exactly how the refunds bug reached the api unit. Skipped unless
// KH_TEST_PG_DSN is set.
package catalog

import (
	"context"
	"errors"
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

// TestPG_StatusUpdateRejectsUnknownProduct guards the same silent-write class: a
// publish/pause of a product that does not exist must fail, not report success.
func TestPG_StatusUpdateRejectsUnknownProduct(t *testing.T) {
	store := NewDBStore(setupPG(t))
	ctx := context.Background()
	missing := "00000000-0000-0000-0000-000000000000"
	if err := store.UpdateProductStatus(ctx, missing, ProductPublished, nil); !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("UpdateProductStatus on a missing product: %v, want ErrProductNotFound", err)
	}
}

// TestPG_ListProductsByCategory: the storefront category page depends on this query
// and it had no test on the real schema.
func TestPG_ListProductsByCategory(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)

	var catID, productID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO categories (name, slug) VALUES ('بالش','pillows-list') RETURNING id`).Scan(&catID); err != nil {
		t.Fatalf("seed category: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO products (title, slug, status, sleep_relevance, spec_source, spec_last_reviewed_at)
		 VALUES ('بالش طبی','pillow-list-1','PUBLISHED','SLEEP_RELATED','برگه فنی', now()) RETURNING id`).Scan(&productID); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO product_categories (product_id, category_id) VALUES ($1,$2)`, productID, catID); err != nil {
		t.Fatalf("link category: %v", err)
	}

	items, total, err := store.ListProductsByCategory(ctx, catID, ProductPublished, 1, 10)
	if err != nil {
		t.Fatalf("ListProductsByCategory: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != productID {
		t.Fatalf("items=%d total=%d, want the one published product", len(items), total)
	}

	// A draft in the same category must not appear in the published listing.
	if _, err := pool.Exec(ctx,
		`INSERT INTO products (title, slug, status) VALUES ('پیش‌نویس','pillow-list-2','DRAFT')`); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	items, total, err = store.ListProductsByCategory(ctx, catID, ProductPublished, 1, 10)
	if err != nil {
		t.Fatalf("ListProductsByCategory (2): %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("after adding a draft: items=%d total=%d, want 1", len(items), total)
	}
}
