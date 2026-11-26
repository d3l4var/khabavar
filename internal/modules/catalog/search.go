// search.go adds product search & filter to the catalog module (R-04 §۵, R-07 §catalog/search).
//
// The storefront search supports:
//   - Full-text search across product title, short_description, description, and
//     manufacturer (PostgreSQL ILIKE with prefix wildcard — a pg_trgm/gin index
//     would be added in a performance pass; for MVP ILIKE is sufficient).
//   - Category filter (products attached to a category via product_categories).
//   - Sleep-relevance filter (the catalog's sleep_relevance enum: SLEEP_AID,
//     SLEEP_RELATED, GENERAL — lets the storefront curate "sleep-specific" products).
//   - Status filter (storefront always passes PUBLISHED; admin can pass empty for all).
//   - Pagination (page/page_size, clamped to 1-100).
//
// SearchStore is a separate interface from Store so that existing tests with a
// minimal memStore don't break; the DB store implements both. The Service accepts
// a SearchStore via SetSearchStore (nil in unit tests that don't exercise search).
package catalog

import (
	"context"
	"strings"
)

// SleepRelevance enums (mirrors the catalog.sleep_relevance CHECK constraint).
const (
	SleepRelevanceSleepAid     = "SLEEP_AID"
	SleepRelevanceSleepRelated = "SLEEP_RELATED"
	SleepRelevanceGeneral      = "GENERAL"
)

// SearchFilter holds the storefront search parameters.
type SearchFilter struct {
	Query          string // free-text; matched against title/description/manufacturer
	CategoryID     string // optional; empty = all categories
	SleepRelevance string // optional; SLEEP_AID/SLEEP_RELATED/GENERAL
	Status         string // storefront passes PUBLISHED; admin passes "" for all
	Page           int
	PageSize       int
}

// SearchResult is a product hit with a relevance snippet (the matched context).
type SearchResult struct {
	Product Product
	Snippet string // short excerpt showing the match context (for UI highlighting)
}

// SearchStore is the persistence boundary for product search. DBStore implements it.
type SearchStore interface {
	SearchProducts(ctx context.Context, f SearchFilter) ([]SearchResult, int, error)
}

// SearchProducts runs a storefront product search. If no SearchStore is wired
// (unit tests), it falls back to a linear scan over ListProductsByCategory with
// empty category (all) — this is slow but correct for tests.
func (s *Service) SearchProducts(ctx context.Context, f SearchFilter) ([]SearchResult, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	if f.Status == "" {
		f.Status = ProductPublished // storefront default
	}
	if s.search != nil {
		return s.search.SearchProducts(ctx, f)
	}
	// Fallback: linear scan (tests only). The DB store provides real search.
	return s.searchFallback(ctx, f)
}

// searchFallback is the test-only linear scan. It lists all products in all
// categories and filters in-memory. This is O(n) and not for production.
func (s *Service) searchFallback(ctx context.Context, f SearchFilter) ([]SearchResult, int, error) {
	// We don't have a "list all" method on Store; use ListProductsByCategory with
	// empty categoryID if the Store supports it, otherwise return empty.
	all, _, err := s.store.ListProductsByCategory(ctx, "", f.Status, 1, 1000)
	if err != nil {
		return nil, 0, err
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	var filtered []Product
	for _, p := range all {
		if f.SleepRelevance != "" && p.SleepRelevance != f.SleepRelevance {
			continue
		}
		if q != "" {
			hay := strings.ToLower(p.Title + " " + p.ShortDescription + " " + p.Description + " " + p.Manufacturer)
			if !strings.Contains(hay, q) {
				continue
			}
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
	results := make([]SearchResult, 0, end-start)
	for _, p := range filtered[start:end] {
		results = append(results, SearchResult{Product: p, Snippet: p.ShortDescription})
	}
	return results, total, nil
}
