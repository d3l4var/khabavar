package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_ContentHTTPHandlers drives the magazine's HTTP surface: the editorial write path
// through its handlers and the storefront reads. http.go had no tests at all, so a wrong
// status code or a missing guard could not be caught.
func TestPG_ContentHTTPHandlers(t *testing.T) {
	pool, svc, _ := setupContent(t)
	ctx := context.Background()

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// A topic, then an article in it.
	rec := call(CreateTopicHandler(svc), http.MethodPost, `{"name":"خواب","slug":"sleep-http"}`, nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create topic = %d: %s", rec.Code, rec.Body.String())
	}
	var topic struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &topic)
	if topic.ID == "" {
		t.Fatalf("topic response = %s", rec.Body.String())
	}

	// Broken JSON on the write handlers is a 400, not a 500.
	if rec := call(CreateArticleHandler(svc), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("create article with a broken body = %d, want 400", rec.Code)
	}
	if rec := call(CreateTopicHandler(svc), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("create topic with a broken body = %d, want 400", rec.Code)
	}

	// Create an article and walk the workflow through the handlers.
	rec = call(CreateArticleHandler(svc), http.MethodPost,
		`{"title":"خواب عمیق","slug":"deep-sleep-http","summary":"خلاصه","body":"متن","language":"fa","source_type":"ORIGINAL"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create article = %d: %s", rec.Code, rec.Body.String())
	}
	var art struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &art)
	if art.ID == "" {
		t.Fatalf("article response = %s", rec.Body.String())
	}
	// A duplicate slug is a 409.
	if rec := call(CreateArticleHandler(svc), http.MethodPost,
		`{"title":"تکراری","slug":"deep-sleep-http","body":"متن","language":"fa","source_type":"ORIGINAL"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate slug = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// Publishing without a source is refused by the gate (ADR-CONTENT-001).
	if rec := call(SubmitForReviewHandler(svc), http.MethodPost, "", map[string]string{"id": art.ID}); rec.Code != http.StatusOK {
		t.Fatalf("submit = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(ApproveArticleHandler(svc), http.MethodPost, "", map[string]string{"id": art.ID}); rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(PublishArticleHandler(svc), http.MethodPost, "", map[string]string{"id": art.ID})
	if rec.Code < 400 {
		t.Fatalf("publish without a source = %d, want a 4xx (the gate must hold)", rec.Code)
	}
	// With a source it publishes.
	if _, err := svc.AddArticleSource(ctx, ArticleSource{ArticleID: art.ID, SourceType: CitationGuideline, Citation: "AASM"}); err != nil {
		t.Fatalf("AddArticleSource: %v", err)
	}
	if rec := call(PublishArticleHandler(svc), http.MethodPost, "", map[string]string{"id": art.ID}); rec.Code != http.StatusOK {
		t.Fatalf("publish with a source = %d: %s", rec.Code, rec.Body.String())
	}

	// Storefront reads.
	if rec := call(ListTopicsHandler(svc), http.MethodGet, "", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "sleep-http") {
		t.Fatalf("topics = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(ListPublishedArticlesHandler(svc), http.MethodGet, "", nil); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "deep-sleep-http") {
		t.Fatalf("published articles = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(GetArticleBySlugHandler(svc), http.MethodGet, "", map[string]string{"slug": "deep-sleep-http"}); rec.Code != http.StatusOK {
		t.Fatalf("article by slug = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown slug is a 404, and a draft is never served by slug.
	if rec := call(GetArticleBySlugHandler(svc), http.MethodGet, "", map[string]string{"slug": "no-such-article"}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown slug = %d, want 404", rec.Code)
	}
	_ = pool
}
