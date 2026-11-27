package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPG_AdminProductList covers the panel list: the status filter, the title/slug search,
// the publish-gate flag per row, the page cap and the paging metadata. NewAdminListStore,
// List and ListProductsHandler were all at 0%.
func TestPG_AdminProductList(t *testing.T) {
	pool := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store)
	svc.SetUpdateStore(store)

	// Two products: one complete (gate satisfied), one bare.
	fullID, err := svc.CreateProduct(ctx, Product{
		Title: "بالش کامل", Slug: "pillow-full",
		SleepRelevance: "HIGH", SpecSource: "سازنده",
	})
	if err != nil {
		t.Fatalf("create full: %v", err)
	}
	bareID, err := svc.CreateProduct(ctx, Product{Title: "بالش ناقص", Slug: "pillow-bare"})
	if err != nil {
		t.Fatalf("create bare: %v", err)
	}
	// Give the full product its review date so the gate flag is unambiguous.
	if _, err := pool.Exec(ctx, `UPDATE products SET spec_last_reviewed_at = now() WHERE id = $1`, fullID); err != nil {
		t.Fatalf("set review date: %v", err)
	}
	_ = bareID

	list := NewAdminListStore(pool)
	get := func(q string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?"+q, nil)
		rec := httptest.NewRecorder()
		ListProductsHandler(list)(rec, r)
		return rec
	}

	// Unfiltered: both products, with paging metadata.
	rec := get("")
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data     []AdminProductRow `json:"data"`
		Total    int               `json:"total"`
		Page     int               `json:"page"`
		PageSize int               `json:"page_size"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Total < 2 || len(out.Data) < 2 || out.Page != 1 || out.PageSize != 20 {
		t.Fatalf("list body = %s", rec.Body.String())
	}

	// The gate flag is per row: the complete product says true, the bare one false.
	gate := map[string]bool{}
	for _, r := range out.Data {
		gate[r.Slug] = r.HasGate
	}
	if !gate["pillow-full"] || gate["pillow-bare"] {
		t.Fatalf("gate flags = %+v", gate)
	}

	// The q filter matches title and slug.
	rec = get("q=ناقص")
	var filtered struct {
		Data  []AdminProductRow `json:"data"`
		Total int               `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &filtered)
	if filtered.Total != 1 || len(filtered.Data) != 1 || filtered.Data[0].Slug != "pillow-bare" {
		t.Fatalf("q filter = %s", rec.Body.String())
	}
	if rec := get("q=pillow-full"); rec.Code != http.StatusOK {
		t.Fatalf("slug filter = %d", rec.Code)
	}

	// The status filter: DRAFT matches both, PUBLISHED matches none yet.
	rec = get("status=PUBLISHED")
	var published struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &published)
	if published.Total != 0 {
		t.Fatalf("published filter = %s", rec.Body.String())
	}
	if rec := get("status=DRAFT"); rec.Code != http.StatusOK {
		t.Fatalf("draft filter = %d", rec.Code)
	}

	// page_size is capped at 100 and nonsense paging falls back to the defaults.
	rec = get("page_size=5000")
	var capped struct {
		PageSize int `json:"page_size"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &capped)
	if capped.PageSize != 100 {
		t.Fatalf("page_size cap = %d, want 100", capped.PageSize)
	}
	rec = get("page=abc&page_size=xyz")
	var fallback struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &fallback)
	if fallback.Page != 1 || fallback.PageSize != 20 {
		t.Fatalf("paging fallback = %+v", fallback)
	}
}
