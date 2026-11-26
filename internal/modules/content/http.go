// Package content — HTTP handlers for magazine storefront + admin endpoints (R-07 §content).
// Storefront reads (published articles, topics, relations) are public; admin writes
// (create, editorial transitions, sources, relations) are behind authorization.Require.
package content

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// ListTopicsHandler serves GET /api/v1/topics — the topic tree (storefront).
func ListTopicsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		topics, err := svc.ListTopics(r.Context())
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": topics})
	}
}

// CreateTopicHandler serves POST /api/v1/admin/topics.
func CreateTopicHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			Slug      string `json:"slug"`
			ParentID  string `json:"parent_id"`
			SortOrder int    `json:"sort_order"`
			Status    string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeContentErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.CreateTopic(r.Context(), Topic{
			Name: req.Name, Slug: req.Slug, ParentID: req.ParentID,
			SortOrder: req.SortOrder, Status: req.Status,
		})
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ListPublishedArticlesHandler serves GET /api/v1/articles — paginated published articles.
func ListPublishedArticlesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), 20)
		arts, total, err := svc.ListPublishedArticles(r.Context(), page, pageSize)
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": arts, "page": page, "page_size": pageSize, "total": total,
		})
	}
}

// GetArticleBySlugHandler serves GET /api/v1/articles/{slug} — storefront article view.
func GetArticleBySlugHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		a, err := svc.GetPublishedArticleBySlug(r.Context(), slug)
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		// Attach sources and topics for the storefront view.
		sources, _ := svc.ListArticleSources(r.Context(), a.ID)
		topics, _ := svc.ListTopicsForArticle(r.Context(), a.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"data": a, "sources": sources, "topics": topics,
		})
	}
}

// CreateArticleHandler serves POST /api/v1/admin/articles.
func CreateArticleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Article
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeContentErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.CreateArticle(r.Context(), req)
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ArticleTransitionHandler is the shared builder for editorial workflow transitions.
func ArticleTransitionHandler(svc *Service, fn func(context.Context, string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := fn(r.Context(), id); err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// SubmitForReviewHandler serves POST /api/v1/admin/articles/{id}/submit.
func SubmitForReviewHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.SubmitForReview)
}

// ApproveArticleHandler serves POST /api/v1/admin/articles/{id}/approve.
func ApproveArticleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req struct {
			EditorID string `json:"editor_id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		if err := svc.Approve(r.Context(), id, req.EditorID); err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ScheduleArticleHandler serves POST /api/v1/admin/articles/{id}/schedule.
func ScheduleArticleHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.Schedule)
}

// PublishArticleHandler serves POST /api/v1/admin/articles/{id}/publish.
func PublishArticleHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.Publish)
}

// UnpublishArticleHandler serves POST /api/v1/admin/articles/{id}/unpublish.
func UnpublishArticleHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.Unpublish)
}

// ArchiveArticleHandler serves POST /api/v1/admin/articles/{id}/archive.
func ArchiveArticleHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.Archive)
}

// RejectToDraftHandler serves POST /api/v1/admin/articles/{id}/reject.
func RejectToDraftHandler(svc *Service) http.HandlerFunc {
	return ArticleTransitionHandler(svc, svc.RejectToDraft)
}

// AddArticleSourceHandler serves POST /api/v1/admin/articles/{id}/sources —
// adds a citation source (the publish gate, ADR-CONTENT-001).
func AddArticleSourceHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artID := r.PathValue("id")
		var req struct {
			SourceType string `json:"source_type"`
			Citation   string `json:"citation"`
			URL        string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeContentErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.AddArticleSource(r.Context(), ArticleSource{
			ArticleID: artID, SourceType: req.SourceType, Citation: req.Citation, URL: req.URL,
		})
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ListArticleSourcesHandler serves GET /api/v1/articles/{id}/sources (public).
func ListArticleSourcesHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artID := r.PathValue("id")
		sources, err := svc.ListArticleSources(r.Context(), artID)
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": sources})
	}
}

// LinkArticleTopicHandler serves POST /api/v1/admin/articles/{id}/topics.
func LinkArticleTopicHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artID := r.PathValue("id")
		var req struct {
			TopicID string `json:"topic_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeContentErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := svc.LinkTopic(r.Context(), artID, req.TopicID); err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// AddProductRelationHandler serves POST /api/v1/admin/articles/{id}/products —
// links a product to an article with a mandatory relation_reason (MA-5).
func AddProductRelationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artID := r.PathValue("id")
		var req struct {
			ProductID      string `json:"product_id"`
			RelationType   string `json:"relation_type"`
			RelationReason string `json:"relation_reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeContentErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		id, err := svc.AddProductRelation(r.Context(), ProductArticleRelation{
			ArticleID: artID, ProductID: req.ProductID,
			RelationType: req.RelationType, RelationReason: req.RelationReason,
		})
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ListRelationsForArticleHandler serves GET /api/v1/articles/{id}/products —
// the "related products" section under an article (storefront, public).
func ListRelationsForArticleHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		artID := r.PathValue("id")
		rels, err := svc.ListRelationsForArticle(r.Context(), artID)
		if err != nil {
			writeContentSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rels})
	}
}

func writeContentSvcErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrArticleNotFound), errors.Is(err, ErrTopicNotFound),
		errors.Is(err, ErrSourceNotFound), errors.Is(err, ErrRelationNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrSlugRequired), errors.Is(err, ErrTitleRequired),
		errors.Is(err, ErrBodyRequired), errors.Is(err, ErrRelationReasonReq):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	case errors.Is(err, ErrDuplicateSlug):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "slug already exists", "code": "duplicate_slug"})
	case errors.Is(err, ErrInvalidStatus):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid status transition", "code": "invalid_status"})
	case errors.Is(err, ErrPublishGateNoSource):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "cannot publish without a source", "code": "publish_gate_no_source"})
	case errors.Is(err, ErrInvalidRelationType), errors.Is(err, ErrInvalidCitationType),
		errors.Is(err, ErrInvalidArticleSource):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeContentErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
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
