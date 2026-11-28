package content

import (
	"context"
	"encoding/json"
	"net/http"
)

// RelatedProduct is the storefront-facing slice of a product↔article relation. It carries just
// enough to link from a magazine article to the product it is about.
type RelatedProduct struct {
	ProductID  string `json:"product_id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	SalesPitch string `json:"sales_pitch,omitempty"`
	Relation   string `json:"relation_type"`
	Reason     string `json:"relation_reason,omitempty"`
}

// ListArticleProductsHandler serves GET /api/v1/magazine/articles/{slug}/products.
//
// Why it exists: product_article_relations has been written to since the content module landed,
// and nothing ever read it over HTTP. The magazine could not send a reader to the product an
// article is about — which is the point of having a magazine beside a shop at all.
func ListArticleProductsHandler(svc *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if slug == "" {
			writeContentErr(w, http.StatusNotFound, "article not found")
			return
		}
		a, err := svc.GetPublishedArticleBySlug(r.Context(), slug)
		if err != nil || a == nil {
			writeContentErr(w, http.StatusNotFound, "article not found")
			return
		}
		related, err := svc.RelatedProductsForArticle(r.Context(), a.ID)
		if err != nil {
			writeContentErr(w, http.StatusInternalServerError, "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": related})
	})
}

// relatedStore is the optional capability. A minimal test store does not implement it, and the
// handler then answers an empty list rather than failing.
type relatedStore interface {
	ListRelatedProducts(ctx context.Context, articleID string) ([]RelatedProduct, error)
}

// RelatedProductsForArticle returns the published products an article points at. A relation to
// an unpublished product is dropped by the query: the magazine must not reveal a draft.
func (s *Service) RelatedProductsForArticle(ctx context.Context, articleID string) ([]RelatedProduct, error) {
	rs, ok := s.store.(relatedStore)
	if !ok {
		return []RelatedProduct{}, nil
	}
	return rs.ListRelatedProducts(ctx, articleID)
}
