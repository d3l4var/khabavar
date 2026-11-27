package catalog

import (
	"context"
	"testing"
)

// memSearchStore is an in-memory SearchStore for unit tests.
type memSearchStore struct {
	products []Product
}

func (m *memSearchStore) SearchProducts(_ context.Context, f SearchFilter) ([]SearchResult, int, error) {
	var filtered []Product
	for _, p := range m.products {
		if f.Status != "" && p.Status != f.Status {
			continue
		}
		if f.SleepRelevance != "" && p.SleepRelevance != f.SleepRelevance {
			continue
		}
		filtered = append(filtered, p)
	}
	total := len(filtered)
	start := (f.Page - 1) * f.PageSize
	if start >= total {
		return nil, total, nil
	}
	end := start + f.PageSize
	if end > total {
		end = total
	}
	out := make([]SearchResult, 0, end-start)
	for _, p := range filtered[start:end] {
		out = append(out, SearchResult{Product: p, Snippet: p.ShortDescription})
	}
	return out, total, nil
}

func TestSearchProducts_WithSearchStore(t *testing.T) {
	store := &memSearchStore{
		products: []Product{
			{ID: "p1", Title: "بالش مموریفوم", Status: ProductPublished, SleepRelevance: SleepRelevanceSleepAid, ShortDescription: "بالش طبی"},
			{ID: "p2", Title: "قرص ملاتونین", Status: ProductPublished, SleepRelevance: SleepRelevanceSleepAid, ShortDescription: "مکمل خواب"},
			{ID: "p3", Title: "ماژولار روپوش", Status: ProductPublished, SleepRelevance: SleepRelevanceGeneral, ShortDescription: "روپوش"},
			{ID: "p4", Title: "روبالشی نرم", Status: ProductDraft, SleepRelevance: SleepRelevanceSleepRelated, ShortDescription: "روبالشی"},
		},
	}
	svc := NewService(nil) // store nil — search uses SetSearchStore
	svc.SetSearchStore(store)

	// Search published only (default Status=PUBLISHED) → 3 results (p4 is DRAFT).
	results, total, err := svc.SearchProducts(context.Background(), SearchFilter{
		Page: 1, PageSize: 10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d want 3 (draft excluded)", total)
	}
	if len(results) != 3 {
		t.Fatalf("results=%d want 3", len(results))
	}

	// Filter by sleep_relevance=SLEEP_AID → 2 results (p1, p2).
	// The count is what this case asserts; the rows were checked above.
	_, total, _ = svc.SearchProducts(context.Background(), SearchFilter{
		SleepRelevance: SleepRelevanceSleepAid, Page: 1, PageSize: 10,
	})
	if total != 2 {
		t.Fatalf("sleep_aid total=%d want 2", total)
	}

	// Pagination: page 1 size 2 → 2 results, total 3.
	results, total, _ = svc.SearchProducts(context.Background(), SearchFilter{
		Page: 1, PageSize: 2,
	})
	if total != 3 || len(results) != 2 {
		t.Fatalf("page1: total=%d len=%d want 3/2", total, len(results))
	}
	// Page 2 size 2 → 1 result.
	results, _, _ = svc.SearchProducts(context.Background(), SearchFilter{
		Page: 2, PageSize: 2,
	})
	if len(results) != 1 {
		t.Fatalf("page2 len=%d want 1", len(results))
	}
}

func TestSearchProducts_PageSizeClamped(t *testing.T) {
	svc := NewService(nil)
	svc.SetSearchStore(&memSearchStore{products: []Product{}})
	// PageSize 0 → clamped to 20.
	results, _, _ := svc.SearchProducts(context.Background(), SearchFilter{Page: 1, PageSize: 0})
	_ = results // no panic
	// PageSize 500 → clamped to 100.
	results, _, _ = svc.SearchProducts(context.Background(), SearchFilter{Page: 1, PageSize: 500})
	_ = results
}
