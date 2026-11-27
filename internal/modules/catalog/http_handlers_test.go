package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCatalogHTTPHandlers exercises the storefront and panel handlers against the in-memory
// store. admin.go was at 27% and http.go at 48%, so the status codes and the storefront
// visibility rule were only partly pinned.
func TestCatalogHTTPHandlers(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	svc.SetUpdateStore(&memUpdateStore{store: store})

	call := func(h http.HandlerFunc, method, body string, pv map[string]string, q string) *httptest.ResponseRecorder {
		target := "/x"
		if q != "" {
			target += "?" + q
		}
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, target, nil)
		} else {
			r = httptest.NewRequest(method, target, strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// --- panel: create a category, then a product in it ---
	if rec := call(CreateCategoryHandler(svc), http.MethodPost, `{`, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken category body = %d, want 400", rec.Code)
	}
	rec := call(CreateCategoryHandler(svc), http.MethodPost, `{"name":"بالش","slug":"pillow"}`, nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create category = %d: %s", rec.Code, rec.Body.String())
	}
	var cat struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &cat)

	if rec := call(CreateProductHandler(svc), http.MethodPost, `{`, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken product body = %d, want 400", rec.Code)
	}
	rec = call(CreateProductHandler(svc), http.MethodPost,
		`{"title":"بالش طبی","slug":"medical-pillow","category_ids":["`+cat.ID+`"]}`, nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create product = %d: %s", rec.Code, rec.Body.String())
	}
	var prod struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &prod)
	if prod.ID == "" {
		t.Fatalf("create product response = %s", rec.Body.String())
	}

	// A DRAFT product must be invisible to the storefront but visible in the panel.
	if rec := call(ProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": prod.ID}, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("draft on the storefront = %d, want 404", rec.Code)
	}
	if rec := call(AdminProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": prod.ID}, ""); rec.Code != http.StatusOK {
		t.Fatalf("draft in the panel = %d: %s", rec.Code, rec.Body.String())
	}

	// The publish gate (MA-1/MA-6) refuses a product without the sleep/spec fields, and it
	// says why: a 422 with a machine-readable code, not a bare 400.
	recGate := call(PublishProductHandler(svc), http.MethodPost, "", map[string]string{"id": prod.ID}, "")
	if recGate.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish without the spec fields = %d, want 422: %s", recGate.Code, recGate.Body.String())
	}
	if !strings.Contains(recGate.Body.String(), "publish_gate_missing") {
		t.Fatalf("publish gate body = %s", recGate.Body.String())
	}

	// A complete product publishes, then the storefront sees it.
	rec = call(CreateProductHandler(svc), http.MethodPost,
		`{"title":"بالش کامل","slug":"full-pillow","category_ids":["`+cat.ID+`"],`+
			`"sleep_relevance":"HIGH","spec_source":"سازنده","spec_last_reviewed_at":"2026-01-01T00:00:00Z"}`, nil, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create complete product = %d: %s", rec.Code, rec.Body.String())
	}
	var full struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &full)
	if rec := call(PublishProductHandler(svc), http.MethodPost, "", map[string]string{"id": full.ID}, ""); rec.Code >= 400 {
		t.Fatalf("publish a complete product = %d: %s", rec.Code, rec.Body.String())
	}
	prod = full
	if rec := call(ProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": prod.ID}, ""); rec.Code != http.StatusOK {
		t.Fatalf("published on the storefront = %d: %s", rec.Code, rec.Body.String())
	}

	// Status change to a nonsense value is a 4xx, not a 500.
	if rec := call(ProductStatusHandler(svc), http.MethodPatch, `{"status":"NONSENSE"}`, map[string]string{"id": prod.ID}, ""); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("nonsense status = %d, want a 4xx", rec.Code)
	}
	// A real status change works.
	if rec := call(ProductStatusHandler(svc), http.MethodPatch, `{"status":"PAUSED"}`, map[string]string{"id": prod.ID}, ""); rec.Code >= 400 {
		t.Fatalf("pause = %d: %s", rec.Code, rec.Body.String())
	}
	// And a paused product is hidden from the storefront again.
	if rec := call(ProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": prod.ID}, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("paused on the storefront = %d, want 404", rec.Code)
	}

	// Unknown ids are 404s, not 500s.
	if rec := call(ProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": "nope"}, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown storefront product = %d, want 404", rec.Code)
	}
	if rec := call(AdminProductDetailHandler(svc), http.MethodGet, "", map[string]string{"id": "nope"}, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown panel product = %d, want 404", rec.Code)
	}

	// --- storefront reads ---
	if rec := call(CategoryTreeHandler(svc), http.MethodGet, "", nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("category tree = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(ListProductsByCategoryHandler(svc), http.MethodGet, "", map[string]string{"id": cat.ID}, ""); rec.Code != http.StatusOK {
		t.Fatalf("products by category = %d: %s", rec.Code, rec.Body.String())
	}
	// Nonsense paging falls back to defaults.
	if rec := call(ListProductsByCategoryHandler(svc), http.MethodGet, "", map[string]string{"id": cat.ID}, "page=abc&page_size=xyz"); rec.Code != http.StatusOK {
		t.Fatalf("bad paging = %d: %s", rec.Code, rec.Body.String())
	}

	// --- product update ---
	if rec := call(UpdateProductHandler(svc), http.MethodPatch, `{`, map[string]string{"id": prod.ID}, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken update = %d, want 400", rec.Code)
	}
	if rec := call(UpdateProductHandler(svc), http.MethodPatch, `{"title":"بالش طبی مموری"}`, map[string]string{"id": prod.ID}, ""); rec.Code >= 400 {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}

	// --- variant creation ---
	if rec := call(CreateVariantHandler(svc), http.MethodPost, `{`, nil, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken variant = %d, want 400", rec.Code)
	}
	if rec := call(CreateVariantHandler(svc), http.MethodPost,
		`{"product_id":"`+prod.ID+`","sku":"SKU-CAT-1","title":"سایز متوسط"}`, nil, ""); rec.Code != http.StatusCreated {
		t.Fatalf("create variant = %d: %s", rec.Code, rec.Body.String())
	}
	// A duplicate SKU is a conflict, not a 500.
	if rec := call(CreateVariantHandler(svc), http.MethodPost,
		`{"product_id":"`+prod.ID+`","sku":"SKU-CAT-1","title":"تکراری"}`, nil, ""); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("duplicate sku = %d, want a 4xx", rec.Code)
	}
}
