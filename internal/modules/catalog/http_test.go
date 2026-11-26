package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// seedStore builds a store with one category, one published product (with variant)
// and one draft product, for handler tests.
func seedStore(t *testing.T) (*Service, string, string) {
	t.Helper()
	store := newMemStore()
	svc := NewService(store)
	svc.SetNow(func() time.Time { return time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC) })
	catID, _ := svc.CreateCategory(context.Background(), Category{Slug: "bedding", Name: "رختخواب"})

	// Published product (gate satisfied).
	pubID, _ := svc.CreateProduct(context.Background(), Product{
		Slug: "memory-pillow", Title: "بالش مموریفوم", CategoryIDs: []string{catID},
	})
	store.prods[pubID].SleepRelevance = "پشتیبانی گردن در خواب"
	store.prods[pubID].SpecSource = "سازنده"
	reviewed := time.Now()
	store.prods[pubID].SpecLastReviewedAt = &reviewed
	if err := svc.PublishProduct(context.Background(), pubID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	svc.CreateVariant(context.Background(), Variant{ProductID: pubID, SKU: "KHA-PIL-001"})

	// Draft product (must be invisible to storefront).
	svc.CreateProduct(context.Background(), Product{
		Slug: "draft-pillow", Title: "بالش پیش‌نویس", CategoryIDs: []string{catID},
	})
	return svc, catID, pubID
}

func TestCategoryTreeHandler_ReturnsJSON(t *testing.T) {
	svc, _, _ := seedStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	rec := httptest.NewRecorder()
	CategoryTreeHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("data not a slice: %v", body["data"])
	}
	if len(data) != 1 {
		t.Fatalf("roots=%d want 1", len(data))
	}
}

func TestProductDetailHandler_PublishedVisible(t *testing.T) {
	svc, _, pubID := seedStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+pubID, nil)
	req.SetPathValue("id", pubID)
	rec := httptest.NewRecorder()
	ProductDetailHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "memory-pillow") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "KHA-PIL-001") {
		t.Fatalf("variant missing from detail: %s", rec.Body.String())
	}
}

func TestProductDetailHandler_DraftReturns404(t *testing.T) {
	svc, _, _ := seedStore(t)
	// The draft product slug is "draft-pillow".
	draftID := "draft-pillow"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+draftID, nil)
	req.SetPathValue("id", draftID)
	rec := httptest.NewRecorder()
	ProductDetailHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("draft must be 404 for storefront, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestProductDetailHandler_NotFound(t *testing.T) {
	svc, _, _ := seedStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/products/nope", nil)
	req.SetPathValue("id", "nope")
	rec := httptest.NewRecorder()
	ProductDetailHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestListProductsByCategoryHandler_PaginatedJSON(t *testing.T) {
	svc, catID, _ := seedStore(t)
	// Add two more published products to exercise pagination.
	for i := 0; i < 2; i++ {
		pid, _ := svc.CreateProduct(context.Background(), Product{
			Slug: "extra-" + itoa(i), Title: "Extra", CategoryIDs: []string{catID},
		})
		svc.store.(*memStore).prods[pid].SleepRelevance = "x"
		svc.store.(*memStore).prods[pid].SpecSource = "y"
		rv := time.Now()
		svc.store.(*memStore).prods[pid].SpecLastReviewedAt = &rv
		if err := svc.PublishProduct(context.Background(), pid); err != nil {
			t.Fatalf("publish extra: %v", err)
		}
	}
	url := "/api/v1/categories/" + catID + "/products?page=1&page_size=2"
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.SetPathValue("id", catID)
	rec := httptest.NewRecorder()
	ListProductsByCategoryHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["total"].(float64) != 3 {
		t.Fatalf("total=%v want 3", body["total"])
	}
	if body["page"].(float64) != 1 || body["page_size"].(float64) != 2 {
		t.Fatalf("page/page_size=%v/%v", body["page"], body["page_size"])
	}
	if len(body["data"].([]any)) != 2 {
		t.Fatalf("returned=%v want 2", len(body["data"].([]any)))
	}
}

func TestListProductsByCategoryHandler_DefaultsPageParams(t *testing.T) {
	svc, catID, _ := seedStore(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories/"+catID+"/products", nil)
	req.SetPathValue("id", catID)
	rec := httptest.NewRecorder()
	ListProductsByCategoryHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["page"].(float64) != 1 || body["page_size"].(float64) != 20 {
		t.Fatalf("defaults page=%v page_size=%v", body["page"], body["page_size"])
	}
}

func TestParsePositiveInt(t *testing.T) {
	cases := map[string]int{"": 7, "0": 7, "-1": 7, "abc": 7, "3": 3, "100": 100}
	for in, want := range cases {
		if got := parsePositiveInt(in, 7); got != want {
			t.Errorf("parsePositiveInt(%q)=%d want %d", in, got, want)
		}
	}
}
