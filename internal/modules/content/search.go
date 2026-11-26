// search.go adds article search to the content module (R-19 P7 unified search).
//
// The ecosystem module needs to search articles the same way the storefront searches
// products, so the magazine and the shop share one search box. Like the catalog's
// search, the persistence boundary is a separate optional interface: the DB store
// implements it with ILIKE over title/summary/body, and unit tests without a database
// fall back to a bounded in-memory scan.
package content

import (
	"context"
	"strings"
)

// ArticleSearchHit is a published-article hit with a UI snippet.
type ArticleSearchHit struct {
	Article Article
	Snippet string
}

// ArticleSearchStore is the persistence boundary for article search. DBStore implements it.
type ArticleSearchStore interface {
	SearchArticles(ctx context.Context, q string, page, pageSize int) ([]ArticleSearchHit, int, error)
}

// SetSearchStore wires the article search backend (DBStore in production).
func (s *Service) SetSearchStore(ss ArticleSearchStore) { s.search = ss }

// SearchArticles runs a published-article search. Only PUBLISHED articles are
// searchable: the magazine must never leak drafts through the shared search box.
func (s *Service) SearchArticles(ctx context.Context, q string, page, pageSize int) ([]ArticleSearchHit, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if s.search != nil {
		return s.search.SearchArticles(ctx, q, page, pageSize)
	}
	return s.searchFallback(ctx, q, page, pageSize)
}

// searchFallback is the test-only in-memory scan (bounded to 1000 published articles).
func (s *Service) searchFallback(ctx context.Context, q string, page, pageSize int) ([]ArticleSearchHit, int, error) {
	all, _, err := s.store.ListPublishedArticles(ctx, 1, 1000)
	if err != nil {
		return nil, 0, err
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	var hits []ArticleSearchHit
	for _, a := range all {
		if needle != "" {
			hay := strings.ToLower(a.Title + " " + a.Summary + " " + a.Body)
			if !strings.Contains(hay, needle) {
				continue
			}
		}
		hits = append(hits, ArticleSearchHit{Article: a, Snippet: a.Summary})
	}
	total := len(hits)
	start := (page - 1) * pageSize
	if start >= total {
		return nil, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return hits[start:end], total, nil
}

// SearchArticles is the DBStore implementation: ILIKE over title/summary/body of
// PUBLISHED articles only. A pg_trgm/GIN index is a later performance pass; for MVP
// the published-article set is small enough for a sequential scan.
func (s *DBStore) SearchArticles(ctx context.Context, q string, page, pageSize int) ([]ArticleSearchHit, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	pattern := "%" + strings.TrimSpace(q) + "%"
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM articles
		  WHERE status = 'PUBLISHED'
		    AND (title ILIKE $1 OR summary ILIKE $1 OR body ILIKE $1)`, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, slug, summary, body, language, source_type, source_url,
		         parent_article_id, author_id, editor_id, status, published_at,
		         created_at, updated_at
		   FROM articles
		  WHERE status = 'PUBLISHED'
		    AND (title ILIKE $1 OR summary ILIKE $1 OR body ILIKE $1)
		  ORDER BY published_at DESC LIMIT $2 OFFSET $3`,
		pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	articles, _, err := scanArticles(rows, total)
	if err != nil {
		return nil, 0, err
	}
	hits := make([]ArticleSearchHit, 0, len(articles))
	for _, a := range articles {
		hits = append(hits, ArticleSearchHit{Article: a, Snippet: a.Summary})
	}
	return hits, total, nil
}

// Compile-time assertion that DBStore provides article search.
var _ ArticleSearchStore = (*DBStore)(nil)
