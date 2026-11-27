package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_ArticleLifecycleAndSearch covers the transition handlers that were at 0%
// (schedule/unpublish/archive), the unlink paths and the search store.
func TestPG_ArticleLifecycleAndSearch(t *testing.T) {
	pool, svc, store := setupContent(t)
	ctx := context.Background()
	_ = pool
	svc.SetSearchStore(store)

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

	topicID, err := svc.CreateTopic(ctx, Topic{Name: "خواب", Slug: "sleep-life"})
	if err != nil {
		t.Fatalf("topic: %v", err)
	}
	id, err := svc.CreateArticle(ctx, Article{
		Title: "خواب بهتر", Slug: "better-sleep", Body: "متن", Summary: "خلاصه",
	})
	if err != nil {
		t.Fatalf("article: %v", err)
	}
	if _, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: id, SourceType: "JOURNAL", Citation: "منبع",
	}); err != nil {
		t.Fatalf("source: %v", err)
	}

	// submit -> approve -> publish, then unpublish and archive.
	// An ordered slice, not a map: the state machine requires this order and Go map
	// iteration is random (the same mistake already cost a debug cycle in fulfillment).
	steps := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"submit", SubmitForReviewHandler(svc)},
		{"approve", ApproveArticleHandler(svc)},
		{"publish", PublishArticleHandler(svc)},
	}
	for _, step := range steps {
		if rec := call(step.h, http.MethodPost, "", map[string]string{"id": id}); rec.Code >= 400 {
			t.Fatalf("%s = %d: %s", step.name, rec.Code, rec.Body.String())
		}
	}
	// Unpublish is a real transition, not a no-op.
	if rec := call(UnpublishArticleHandler(svc), http.MethodPost, "", map[string]string{"id": id}); rec.Code >= 400 {
		t.Fatalf("unpublish = %d: %s", rec.Code, rec.Body.String())
	}
	a, err := svc.GetArticle(ctx, id)
	if err != nil {
		t.Fatalf("get after unpublish: %v", err)
	}
	if a.Status == StatusPublished {
		t.Fatalf("unpublish left the article published")
	}
	// Archive from the current state.
	if rec := call(ArchiveArticleHandler(svc), http.MethodPost, "", map[string]string{"id": id}); rec.Code >= 400 {
		t.Fatalf("archive = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown id on a transition is a 4xx, not a 500.
	if rec := call(ArchiveArticleHandler(svc), http.MethodPost, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("archive unknown = %d, want a 4xx", rec.Code)
	}

	// Schedule on a fresh draft: the handler exists and answers consistently.
	draftID, err := svc.CreateArticle(ctx, Article{Title: "زمان‌بندی", Slug: "scheduled", Body: "b"})
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if rec := call(ScheduleArticleHandler(svc), http.MethodPost, `{"publish_at":"2030-01-01T00:00:00Z"}`, map[string]string{"id": draftID}); rec.Code >= 500 {
		t.Fatalf("schedule = %d: %s", rec.Code, rec.Body.String())
	}

	// Unlink a topic and remove a product relation (both were at 0%).
	if err := svc.LinkTopic(ctx, id, topicID); err != nil {
		t.Fatalf("link topic: %v", err)
	}
	if err := store.UnlinkArticleTopic(ctx, id, topicID); err != nil {
		t.Fatalf("unlink topic: %v", err)
	}
	topics, _ := svc.ListTopicsForArticle(ctx, id)
	for _, tp := range topics {
		if tp.ID == topicID {
			t.Fatal("the unlinked topic is still listed")
		}
	}

	// Search only sees published articles. draftID went to SCHEDULED above, so use a fresh
	// article for this path.
	searchID, err := svc.CreateArticle(ctx, Article{Title: "خواب بهتر", Slug: "search-me", Body: "متن", Summary: "خلاصه"})
	if err != nil {
		t.Fatalf("search article: %v", err)
	}
	if _, err := svc.AddArticleSource(ctx, ArticleSource{
		ArticleID: searchID, SourceType: "JOURNAL", Citation: "منبع ۲",
	}); err != nil {
		t.Fatalf("source 2: %v", err)
	}
	for _, step := range []struct {
		name string
		h    http.HandlerFunc
	}{
		{"submit", SubmitForReviewHandler(svc)},
		{"approve", ApproveArticleHandler(svc)},
		{"publish", PublishArticleHandler(svc)},
	} {
		if rec := call(step.h, http.MethodPost, "", map[string]string{"id": searchID}); rec.Code >= 400 {
			t.Fatalf("publish search article: %s = %d: %s", step.name, rec.Code, rec.Body.String())
		}
	}

	// Search: a hit for the title, nothing for a nonsense term.
	hits, total, err := svc.SearchArticles(ctx, "خواب", 1, 20)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total == 0 || len(hits) == 0 {
		t.Fatalf("search found nothing for a seeded title")
	}
	if _, total, err := svc.SearchArticles(ctx, "zzz-no-such-term-zzz", 1, 20); err != nil || total != 0 {
		t.Fatalf("nonsense search = %d, %v", total, err)
	}
	// The search handler wraps the same path.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/magazine/articles?q=خواب", nil)
	rec := httptest.NewRecorder()
	ListPublishedArticlesHandler(svc)(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("magazine list = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
}
