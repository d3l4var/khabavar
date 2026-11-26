package ecosystem

import (
	"context"
	"errors"
	"testing"
)

// stubProductSearcher is a test ProductSearcher.
type stubProductSearcher struct {
	hits  []ProductHit
	total int
	err   error
}

func (s stubProductSearcher) SearchProducts(_ context.Context, q string, _, _ int) ([]ProductHit, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.hits, s.total, nil
}

// stubArticleSearcher is a test ArticleSearcher.
type stubArticleSearcher struct {
	hits  []ArticleHit
	total int
	err   error
}

func (s stubArticleSearcher) SearchArticles(_ context.Context, q string, _, _ int) ([]ArticleHit, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	return s.hits, s.total, nil
}

func TestUnifiedSearch_MergesResults(t *testing.T) {
	svc := NewService(
		stubProductSearcher{hits: []ProductHit{
			{ID: "p1", Title: "بالش مموریفوم", Slug: "memory-pillow", Snippet: "بالش طبی"},
		}, total: 1},
		stubArticleSearcher{hits: []ArticleHit{
			{ID: "a1", Title: "تاثیر خواب بر سلامت", Slug: "sleep-health", Snippet: "مقاله"},
		}, total: 1},
	)
	results, prodTotal, artTotal, err := svc.UnifiedSearch(context.Background(), SearchQuery{
		Query: "خواب", Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results=%d want 2", len(results))
	}
	if prodTotal != 1 || artTotal != 1 {
		t.Fatalf("totals: prod=%d art=%d want 1/1", prodTotal, artTotal)
	}
	// Verify type discrimination.
	if results[0].Type != ResultTypeProduct {
		t.Fatalf("first result type=%s want product", results[0].Type)
	}
	if results[1].Type != ResultTypeArticle {
		t.Fatalf("second result type=%s want article", results[1].Type)
	}
}

func TestUnifiedSearch_EmptyQueryRejected(t *testing.T) {
	svc := NewService(stubProductSearcher{}, stubArticleSearcher{})
	_, _, _, err := svc.UnifiedSearch(context.Background(), SearchQuery{Query: ""})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("expected ErrInvalidQuery, got %v", err)
	}
}

func TestUnifiedSearch_PageSizeClamped(t *testing.T) {
	svc := NewService(
		stubProductSearcher{total: 0},
		stubArticleSearcher{total: 0},
	)
	// PageSize 0 → clamped to 10 (no panic, returns empty).
	results, _, _, err := svc.UnifiedSearch(context.Background(), SearchQuery{
		Query: "x", PageSize: 0,
	})
	if err != nil {
		t.Fatalf("clamp: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results=%d want 0", len(results))
	}
}

func TestNavigation_ReturnsSharedNav(t *testing.T) {
	nav := Navigation()
	if len(nav) < 4 {
		t.Fatalf("nav items=%d want ≥4", len(nav))
	}
	// Verify all expected domains present.
	types := map[string]bool{}
	for _, item := range nav {
		types[item.Type] = true
	}
	for _, want := range []string{"store", "magazine", "account", "admin"} {
		if !types[want] {
			t.Fatalf("missing nav type %s", want)
		}
	}
}
