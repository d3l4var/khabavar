package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_ContentHTTPEdges covers the remaining HTTP handlers: sources, topics, product
// relations, the update path, the review decision switch and the product-facing read.
func TestPG_ContentHTTPEdges(t *testing.T) {
	pool, svc, store := setupContent(t)
	ctx := context.Background()
	_ = pool
	svc.SetSearchStore(store)

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

	topicID, err := svc.CreateTopic(ctx, Topic{Name: "خواب", Slug: "sleep-http-edges"})
	if err != nil {
		t.Fatalf("topic: %v", err)
	}
	id, err := svc.CreateArticle(ctx, Article{Title: "خواب", Slug: "edges", Body: "متن"})
	if err != nil {
		t.Fatalf("article: %v", err)
	}
	pv := map[string]string{"id": id}

	// --- sources ---
	if rec := call(AddArticleSourceHandler(svc), http.MethodPost, `{`, pv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken source = %d, want 400", rec.Code)
	}
	rec := call(AddArticleSourceHandler(svc), http.MethodPost,
		`{"source_type":"JOURNAL","citation":"مجلهٔ خواب","url":"https://example.com/x"}`, pv, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("add source = %d: %s", rec.Code, rec.Body.String())
	}
	rec = call(ListArticleSourcesHandler(svc), http.MethodGet, "", pv, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "مجلهٔ خواب") {
		t.Fatalf("list sources = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown article has no sources: an empty list, not a 500.
	if rec := call(ListArticleSourcesHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}, ""); rec.Code >= 500 {
		t.Fatalf("sources for an unknown article = %d", rec.Code)
	}

	// --- topics ---
	if rec := call(LinkArticleTopicHandler(svc), http.MethodPost, `{`, pv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken topic link = %d, want 400", rec.Code)
	}
	if rec := call(LinkArticleTopicHandler(svc), http.MethodPost, `{"topic_id":"`+topicID+`"}`, pv, ""); rec.Code != http.StatusOK {
		t.Fatalf("link topic = %d: %s", rec.Code, rec.Body.String())
	}
	// The article's topics are read through the service; the HTTP surface only links them.
	topics, err := svc.ListTopicsForArticle(ctx, id)
	if err != nil || len(topics) == 0 || topics[0].ID != topicID {
		t.Fatalf("topics for article = %+v, %v", topics, err)
	}

	// --- product relations: the reason is mandatory at the HTTP layer too ---
	if rec := call(AddProductRelationHandler(svc), http.MethodPost, `{`, pv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken relation = %d, want 400", rec.Code)
	}
	if rec := call(AddProductRelationHandler(svc), http.MethodPost,
		`{"product_id":"00000000-0000-0000-0000-000000000001","relation_type":"RELATED"}`,
		pv, ""); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("relation without a reason = %d, want a 4xx", rec.Code)
	}
	if rec := call(ListRelationsForArticleHandler(svc), http.MethodGet, "", pv, ""); rec.Code != http.StatusOK {
		t.Fatalf("relations for article = %d: %s", rec.Code, rec.Body.String())
	}

	// --- update ---
	if rec := call(UpdateArticleHandler(svc), http.MethodPatch, `{`, pv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken update = %d, want 400", rec.Code)
	}
	if rec := call(UpdateArticleHandler(svc), http.MethodPatch, `{"title":"عنوان تازه"}`, pv, ""); rec.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", rec.Code, rec.Body.String())
	}
	a, err := svc.GetArticle(ctx, id)
	if err != nil || a.Title != "عنوان تازه" {
		t.Fatalf("after update = %+v, %v", a, err)
	}
	// A partial update must not clobber the body.
	if a.Body != "متن" {
		t.Fatalf("partial update clobbered the body: %q", a.Body)
	}

	// --- the review decision switch ---
	if rec := call(ReviewArticleHandler(svc), http.MethodPost, `{`, pv, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken review = %d, want 400", rec.Code)
	}
	if rec := call(ReviewArticleHandler(svc), http.MethodPost, `{"decision":"NONSENSE"}`, pv, ""); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("nonsense decision = %d, want a 4xx", rec.Code)
	}
	if rec := call(ReviewArticleHandler(svc), http.MethodPost, `{"decision":"submit"}`, pv, ""); rec.Code >= 400 {
		t.Fatalf("submit decision = %d: %s", rec.Code, rec.Body.String())
	}
	// reject sends it back to DRAFT.
	if rec := call(RejectToDraftHandler(svc), http.MethodPost, "", pv, ""); rec.Code >= 400 {
		t.Fatalf("reject = %d: %s", rec.Code, rec.Body.String())
	}
	a, _ = svc.GetArticle(ctx, id)
	if a.Status != StatusDraft {
		t.Fatalf("status after reject = %s", a.Status)
	}

	// --- the product-facing read ---
	if rec := call(PublishedArticlesForProductHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000001"}, ""); rec.Code != http.StatusOK {
		t.Fatalf("articles for product = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data []Article `json:"data"`
	}
	_ = json.Unmarshal(call(PublishedArticlesForProductHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000001"}, "").Body.Bytes(), &out)
}
