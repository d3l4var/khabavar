// http.go exposes the ecosystem module over HTTP (R-19 P7).
//
// GET /api/v1/search      — unified product + article search
// GET /api/v1/navigation  — the shared cross-domain navigation used by all four apps
package ecosystem

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// SearchHandler serves the unified search. Query parameters: q (required),
// page, page_size. The response carries both per-source totals so the UI can show
// "۱۲ محصول و ۳ مقاله" without a second round-trip.
func SearchHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		page := atoiDefault(r.URL.Query().Get("page"), 1)
		pageSize := atoiDefault(r.URL.Query().Get("page_size"), 10)
		results, productTotal, articleTotal, err := svc.UnifiedSearch(r.Context(), SearchQuery{
			Query: q, Page: page, PageSize: pageSize,
		})
		if err != nil {
			status := http.StatusInternalServerError
			code := "internal_error"
			if errors.Is(err, ErrInvalidQuery) {
				status = http.StatusBadRequest
				code = "invalid_query"
			}
			writeJSON(w, status, map[string]string{"error": code})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items":         results,
			"product_total": productTotal,
			"article_total": articleTotal,
			"page":          page,
			"page_size":     pageSize,
		})
	}
}

// NavigationHandler serves the shared navigation structure (P7 shared navigation).
func NavigationHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": Navigation()})
	}
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
