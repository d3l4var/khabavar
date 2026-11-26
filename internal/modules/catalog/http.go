// Package catalog — HTTP handlers for the storefront read endpoints (R-07 §catalog).
// Admin write endpoints (create/publish/pause) are wired with RBAC in a later pass.
package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// CategoryTreeHandler serves GET /api/v1/categories — the full category forest.
func CategoryTreeHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tree, err := svc.CategoryTree(r.Context())
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": tree})
	}
}

// ProductDetailHandler serves GET /api/v1/products/{id}. The storefront only sees
// PUBLISHED products; a draft/paused/discontinued product returns 404 (R-07).
func ProductDetailHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeCatalogErr(w, ErrProductNotFound)
			return
		}
		detail, err := svc.GetStorefrontProduct(r.Context(), id)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		if detail.Status != ProductPublished {
			// Storefront must not leak non-published products.
			writeCatalogErr(w, ErrProductNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": detail})
	}
}

// ListProductsByCategoryHandler serves GET /api/v1/categories/{id}/products —
// paginated PUBLISHED products in a category (storefront).
func ListProductsByCategoryHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		categoryID := r.PathValue("id")
		if categoryID == "" {
			writeCatalogErr(w, ErrCategoryNotFound)
			return
		}
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), 20)
		prods, total, err := svc.ListPublishedByCategory(r.Context(), categoryID, page, pageSize)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data":      prods,
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		})
	}
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func writeCatalogErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrProductNotFound), errors.Is(err, ErrCategoryNotFound), errors.Is(err, ErrVariantNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrPublishGateMissing):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "publish gate not satisfied", "code": "publish_gate_missing"})
	case errors.Is(err, ErrDuplicateSKU):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "sku already exists", "code": "duplicate_sku"})
	case errors.Is(err, ErrInvalidStatus):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid status transition", "code": "invalid_status"})
	case errors.Is(err, ErrSlugRequired):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "slug required"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// clientIP is shared with the identity package's helper conceptually; kept local to
// avoid an inter-module dependency (SAD §۴.۲: modules must not import each other).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

var _ = clientIP // reserved for audit wiring in the admin write endpoints (P5b)

// SearchProductsHandler serves GET /api/v1/products/search — storefront product
// search with full-text query + category/sleep-relevance filters (R-07 §catalog/search).
func SearchProductsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		f := SearchFilter{
			Query:          q.Get("q"),
			CategoryID:     q.Get("category_id"),
			SleepRelevance: q.Get("sleep_relevance"),
			Status:         ProductPublished, // storefront always searches published
			Page:           parsePositiveInt(q.Get("page"), 1),
			PageSize:       parsePositiveInt(q.Get("page_size"), 20),
		}
		results, total, err := svc.SearchProducts(r.Context(), f)
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": results, "page": f.Page, "page_size": f.PageSize, "total": total,
			"query": f.Query,
		})
	}
}
