package ecosystem

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeProducts struct {
	hits []ProductHit
	err  error
	gotQ string
}

func (f *fakeProducts) SearchProducts(_ context.Context, q string, _, _ int) ([]ProductHit, int, error) {
	f.gotQ = q
	return f.hits, len(f.hits), f.err
}

type fakeArticles struct {
	hits []ArticleHit
	err  error
	gotQ string
}

func (f *fakeArticles) SearchArticles(_ context.Context, q string, _, _ int) ([]ArticleHit, int, error) {
	f.gotQ = q
	return f.hits, len(f.hits), f.err
}

// TestSearchHandler covers the unified search endpoint: the query is forwarded, both
// sources contribute, an empty query is a 400 (not a full dump of the catalogue), and a
// source failure is a 500 rather than a half-empty 200.
func TestSearchHandler(t *testing.T) {
	products := &fakeProducts{hits: []ProductHit{{ID: "p1", Title: "بالش طبی"}}}
	articles := &fakeArticles{hits: []ArticleHit{{ID: "a1", Title: "راهنمای خواب"}}}
	svc := NewService(products, articles)
	h := SearchHandler(svc)

	get := func(target string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// An empty query must not return everything.
	if rec := get("/api/v1/search"); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty query = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	// A real query reaches both sources.
	rec := get("/api/v1/search?q=خواب&page=1&page_size=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("search = %d: %s", rec.Code, rec.Body.String())
	}
	if products.gotQ != "خواب" || articles.gotQ != "خواب" {
		t.Fatalf("query not forwarded: products=%q articles=%q", products.gotQ, articles.gotQ)
	}
	var out struct {
		Items        []map[string]any `json:"items"`
		ProductTotal int              `json:"product_total"`
		ArticleTotal int              `json:"article_total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Items) != 2 || out.ProductTotal != 1 || out.ArticleTotal != 1 {
		t.Fatalf("search body = %s", rec.Body.String())
	}

	// Nonsense paging falls back to the defaults instead of erroring.
	if rec := get("/api/v1/search?q=خواب&page=abc&page_size=xyz"); rec.Code != http.StatusOK {
		t.Fatalf("bad paging = %d: %s", rec.Code, rec.Body.String())
	}

	// A source failure is a 500, not a partial 200.
	broken := NewService(&fakeProducts{err: errors.New("db down")}, articles)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=خواب", nil)
	recBroken := httptest.NewRecorder()
	SearchHandler(broken)(recBroken, r)
	if recBroken.Code != http.StatusInternalServerError {
		t.Fatalf("source failure = %d, want 500", recBroken.Code)
	}
	if !strings.Contains(recBroken.Body.String(), "internal_error") {
		t.Fatalf("failure body = %s", recBroken.Body.String())
	}
}

// TestNavigationHandler pins the shared navigation payload.
func TestNavigationHandler(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/navigation", nil)
	rec := httptest.NewRecorder()
	NavigationHandler()(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("navigation = %d", rec.Code)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Items) == 0 {
		t.Fatalf("navigation is empty: %s", rec.Body.String())
	}
}
