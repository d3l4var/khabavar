// Package ecosystem implements the cross-domain integration layer (R-19 P7):
// unified search across products AND articles, and the shared navigation service.
//
// The ecosystem module is a thin aggregator — it does NOT own data. It calls the
// catalog and content module boundaries (SAD §۴.۲: no cross-table joins across
// modules). The unified search runs both searches in sequence (a future
// optimization could parallelize with errgroup) and merges results with a
// unified type discriminator ("product" vs "article").
package ecosystem

import (
	"context"
	"errors"
)

// ResultType discriminates unified search hits.
const (
	ResultTypeProduct = "product"
	ResultTypeArticle = "article"
)

// SearchResult is a unified hit — either a product or an article.
type SearchResult struct {
	Type    string `json:"type"`     // "product" | "article"
	ID      string `json:"id"`
	Title   string `json:"title"`
	Slug    string `json:"slug"`
	Snippet string `json:"snippet"` // short excerpt for UI
}

// SearchQuery holds the unified search parameters.
type SearchQuery struct {
	Query    string
	Page     int
	PageSize int
}

// ErrInvalidQuery is returned when the query is empty.
var ErrInvalidQuery = errors.New("ecosystem: query is required")

// ProductSearcher is the catalog module's search boundary (subset of
// catalog.Service that the ecosystem needs). This avoids importing catalog
// directly (SAD §۴.۲: modules communicate via interfaces, not concrete types).
type ProductSearcher interface {
	SearchProducts(ctx context.Context, q string, page, pageSize int) ([]ProductHit, int, error)
}

// ProductHit is what the catalog module returns to ecosystem.
type ProductHit struct {
	ID          string
	Title       string
	Slug        string
	Snippet     string
}

// ArticleSearcher is the content module's search boundary.
type ArticleSearcher interface {
	SearchArticles(ctx context.Context, q string, page, pageSize int) ([]ArticleHit, int, error)
}

// ArticleHit is what the content module returns to ecosystem.
type ArticleHit struct {
	ID      string
	Title   string
	Slug    string
	Snippet string
}

// Service is the ecosystem module boundary.
type Service struct {
	products ProductSearcher
	articles ArticleSearcher
}

// NewService wires the product and article searchers.
func NewService(products ProductSearcher, articles ArticleSearcher) *Service {
	return &Service{products: products, articles: articles}
}

// UnifiedSearch runs a search across products and articles, returning merged
// results. The page/page_size apply to EACH source (so page_size=10 returns up
// to 20 results — 10 products + 10 articles). A future refinement could
// interleave by relevance score.
func (s *Service) UnifiedSearch(ctx context.Context, q SearchQuery) ([]SearchResult, int, int, error) {
	if q.Query == "" {
		return nil, 0, 0, ErrInvalidQuery
	}
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 50 {
		q.PageSize = 10
	}
	prods, prodTotal, err := s.products.SearchProducts(ctx, q.Query, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, 0, err
	}
	arts, artTotal, err := s.articles.SearchArticles(ctx, q.Query, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, 0, err
	}
	out := make([]SearchResult, 0, len(prods)+len(arts))
	for _, p := range prods {
		out = append(out, SearchResult{
			Type: ResultTypeProduct, ID: p.ID, Title: p.Title, Slug: p.Slug, Snippet: p.Snippet,
		})
	}
	for _, a := range arts {
		out = append(out, SearchResult{
			Type: ResultTypeArticle, ID: a.ID, Title: a.Title, Slug: a.Slug, Snippet: a.Snippet,
		})
	}
	return out, prodTotal, artTotal, nil
}

// NavigationItem is a single nav entry for the shared cross-domain header.
type NavigationItem struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Type  string `json:"type"` // "store" | "magazine" | "account" | "admin"
}

// Navigation returns the shared navigation structure used by all four web apps
// (main, shop, magazine, admin). This is the P7 "shared navigation" requirement:
// the same nav data drives the header on every subdomain, so the user sees a
// consistent navigation when moving between store and magazine.
func Navigation() []NavigationItem {
	return []NavigationItem{
		{Label: "خانه", Href: "https://khabavar.ir", Type: "store"},
		{Label: "فروشگاه", Href: "https://shop.khabavar.ir", Type: "store"},
		{Label: "مجله", Href: "https://magazine.khabavar.ir", Type: "magazine"},
		{Label: "حساب من", Href: "https://khabavar.ir/account", Type: "account"},
		{Label: "مدیریت", Href: "https://admin.khabavar.ir", Type: "admin"},
	}
}
