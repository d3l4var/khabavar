package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPG_ProductDetailBySlugOrID pins the storefront contract: the web app links products by
// SLUG while the panel uses the uuid, so both must resolve. Before this the slug path was a
// driver error on a uuid column and every product page answered 500.
func TestPG_ProductDetailBySlugOrID(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store)

	id, err := svc.CreateProduct(ctx, Product{
		Title: "بالش اسلاگ", Slug: "pillow-slug-lookup",
		SleepRelevance: "HIGH", SpecSource: "سازنده",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE products SET status = 'PUBLISHED', published_at = now(), spec_last_reviewed_at = now() WHERE id = $1`, id); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// By uuid.
	byID, err := svc.GetStorefrontProduct(ctx, id)
	if err != nil {
		t.Fatalf("by uuid: %v", err)
	}
	// By slug — the storefront's actual path.
	bySlug, err := svc.GetStorefrontProduct(ctx, "pillow-slug-lookup")
	if err != nil {
		t.Fatalf("by slug: %v", err)
	}
	if bySlug.Product.ID != byID.Product.ID {
		t.Fatalf("slug resolved to %s, uuid to %s", bySlug.Product.ID, byID.Product.ID)
	}

	// The handler answers 200 on both shapes.
	for _, key := range []string{id, "pillow-slug-lookup"} {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.SetPathValue("id", key)
		rec := httptest.NewRecorder()
		ProductDetailHandler(svc)(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("detail(%s) = %d: %s", key, rec.Code, rec.Body.String())
		}
	}
	// An unknown slug is a 404, not a 500.
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("id", "no-such-slug")
	rec := httptest.NewRecorder()
	ProductDetailHandler(svc)(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown slug = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}
