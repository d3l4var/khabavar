// adapters.go bridges module boundaries that deliberately use different types.
//
// The ecosystem module declares its own ProductHit/ArticleHit so it never imports the
// catalog or content packages (SAD §۴.۲). These adapters translate the concrete module
// services into those interfaces at the composition root, which is the only place that
// is allowed to know both sides.
package main

import (
	"context"

	"github.com/khabavar/khabavar/internal/modules/catalog"
	"github.com/khabavar/khabavar/internal/modules/content"
	"github.com/khabavar/khabavar/internal/modules/ecosystem"
)

// catalogSearch adapts catalog.Service to ecosystem.ProductSearcher. Only PUBLISHED
// products are searchable, so the storefront never surfaces drafts.
type catalogSearch struct{ svc *catalog.Service }

func (a catalogSearch) SearchProducts(ctx context.Context, q string, page, pageSize int) ([]ecosystem.ProductHit, int, error) {
	res, total, err := a.svc.SearchProducts(ctx, catalog.SearchFilter{
		Query:    q,
		Status:   catalog.ProductPublished,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	hits := make([]ecosystem.ProductHit, 0, len(res))
	for _, r := range res {
		hits = append(hits, ecosystem.ProductHit{
			ID:      r.Product.ID,
			Title:   r.Product.Title,
			Slug:    r.Product.Slug,
			Snippet: r.Snippet,
		})
	}
	return hits, total, nil
}

// articleSearch adapts content.Service to ecosystem.ArticleSearcher.
type articleSearch struct{ svc *content.Service }

func (a articleSearch) SearchArticles(ctx context.Context, q string, page, pageSize int) ([]ecosystem.ArticleHit, int, error) {
	res, total, err := a.svc.SearchArticles(ctx, q, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	hits := make([]ecosystem.ArticleHit, 0, len(res))
	for _, r := range res {
		hits = append(hits, ecosystem.ArticleHit{
			ID:      r.Article.ID,
			Title:   r.Article.Title,
			Slug:    r.Article.Slug,
			Snippet: r.Snippet,
		})
	}
	return hits, total, nil
}
