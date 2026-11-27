// admin.go exposes the catalogue write surface (R-07 admin-products, R-04, QC-05).
//
// Every handler here is mounted behind authorization.Require (cmd/khabavar/routes.go):
// catalog.product.edit for create/edit, catalog.product.publish for the publish gate
// and status transitions, catalog.attribute.manage for taxonomy. Publication and
// status changes are audited (IAM §۷: order status / catalogue publication are
// sensitive actions).
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// productRequest is the admin create payload. Publish-gate fields are optional at
// creation (the gate is enforced on publish, not on draft).
type productRequest struct {
	Title              string     `json:"title"`
	Slug               string     `json:"slug"`
	BrandID            string     `json:"brand_id"`
	ShortDescription   string     `json:"short_description"`
	Description        string     `json:"description"`
	Manufacturer       string     `json:"manufacturer"`
	CountryOfOrigin    string     `json:"country_of_origin"`
	SleepRelevance     string     `json:"sleep_relevance"`
	SpecSource         string     `json:"spec_source"`
	SpecLastReviewedAt *time.Time `json:"spec_last_reviewed_at"`
	CategoryIDs        []string   `json:"category_ids"`
	Status             string     `json:"status"`
}

// pathID reads the path parameter whatever the route table calls it. The catalog routes declare
// {productId} and {variantId}; the handlers read PathValue("id") and therefore always got "".
// Every one of these endpoints answered 404 on a real id — the same defect the refund path had.
func pathID(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := r.PathValue(n); v != "" {
			return v
		}
	}
	return ""
}

// CreateProductHandler serves POST /api/v1/admin/products.
func CreateProductHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req productRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid JSON body", "code": "invalid_body",
			})
			return
		}
		// A product is a draft until the publish gate passes. The column has DEFAULT 'DRAFT',
		// but the INSERT names status explicitly, so an empty string from a client that does not
		// send the field overrode that default and violated the status CHECK — every create from
		// the panel answered 500.
		status := req.Status
		if status == "" {
			status = ProductDraft
		}
		id, err := svc.CreateProduct(r.Context(), Product{
			Status:             status,
			Title:              req.Title,
			Slug:               req.Slug,
			BrandID:            req.BrandID,
			ShortDescription:   req.ShortDescription,
			Description:        req.Description,
			Manufacturer:       req.Manufacturer,
			CountryOfOrigin:    req.CountryOfOrigin,
			SleepRelevance:     req.SleepRelevance,
			SpecSource:         req.SpecSource,
			SpecLastReviewedAt: req.SpecLastReviewedAt,
			CategoryIDs:        req.CategoryIDs,
		})
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// AdminProductDetailHandler serves GET /api/v1/admin/products/{id} — the panel view,
// including DRAFT/PAUSED products the storefront must never see.
func AdminProductDetailHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, err := svc.GetProductDetail(r.Context(), pathID(r, "productId", "variantId", "id"))
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

// PublishProductHandler serves POST /api/v1/admin/products/{id}/publish. The MA-1/MA-6
// gate is enforced by the Service, so an incomplete product returns 422 with the
// missing-field reason rather than a raw CHECK violation.
func PublishProductHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r, "productId", "variantId", "id")
		if err := svc.PublishProduct(r.Context(), id); err != nil {
			if errors.Is(err, ErrPublishGateMissing) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": err.Error(), "code": "publish_gate_missing",
				})
				return
			}
			writeCatalogErr(w, err)
			return
		}
		auditErr := svc.audit(r, "catalog.product.published", id, nil, map[string]any{"status": ProductPublished})
		body := map[string]string{"status": ProductPublished}
		if auditErr != nil {
			body["audit_warning"] = "the audit trail could not be written"
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// ProductStatusHandler serves POST /api/v1/admin/products/{id}/status with
// {"status":"PAUSED"}. Only transitions the Service allows are accepted; anything else
// is a 400 rather than a silent write.
func ProductStatusHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid JSON body", "code": "invalid_body",
			})
			return
		}
		id := pathID(r, "productId", "variantId", "id")
		var err error
		switch req.Status {
		case ProductPaused:
			err = svc.PauseProduct(r.Context(), id)
		default:
			// Publishing has its own route and its own gate; anything else is refused
			// so the panel cannot bypass MA-1/MA-6 with a status write.
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "unsupported status transition", "code": "invalid_status",
				"allowed": ProductPaused,
			})
			return
		}
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		auditErr := svc.audit(r, "catalog.product.status_changed", id, nil, map[string]any{"status": req.Status})
		body := map[string]string{"status": req.Status}
		if auditErr != nil {
			body["audit_warning"] = "the audit trail could not be written"
		}
		writeJSON(w, http.StatusOK, body)
	}
}

// productPatchRequest uses pointers so "absent" and "empty" are distinguishable: a
// PATCH must not blank a field the caller did not mention.
type productPatchRequest struct {
	Title              *string    `json:"title"`
	Slug               *string    `json:"slug"`
	BrandID            *string    `json:"brand_id"`
	ShortDescription   *string    `json:"short_description"`
	Description        *string    `json:"description"`
	Manufacturer       *string    `json:"manufacturer"`
	CountryOfOrigin    *string    `json:"country_of_origin"`
	SleepRelevance     *string    `json:"sleep_relevance"`
	SpecSource         *string    `json:"spec_source"`
	SpecLastReviewedAt *time.Time `json:"spec_last_reviewed_at"`
	CategoryIDs        *[]string  `json:"category_ids"`
}

// UpdateProductHandler serves PATCH /api/v1/admin/products/{id}. Status is not
// editable here: publishing and pausing have their own routes and gates.
func UpdateProductHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req productPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid JSON body", "code": "invalid_body",
			})
			return
		}
		patch := ProductPatch{
			Title: req.Title, Slug: req.Slug, BrandID: req.BrandID,
			ShortDescription: req.ShortDescription, Description: req.Description,
			Manufacturer: req.Manufacturer, CountryOfOrigin: req.CountryOfOrigin,
			SleepRelevance: req.SleepRelevance, SpecSource: req.SpecSource,
			SpecLastReviewedAt: req.SpecLastReviewedAt,
		}
		if req.CategoryIDs != nil {
			patch.CategoryIDs = *req.CategoryIDs
		}
		id := pathID(r, "productId", "variantId", "id")
		updated, err := svc.UpdateProduct(r.Context(), id, patch)
		if err != nil {
			if errors.Is(err, ErrPublishGateMissing) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": err.Error(), "code": "publish_gate_missing",
				})
				return
			}
			writeCatalogErr(w, err)
			return
		}
		auditErr := svc.audit(r, "catalog.product.updated", id, nil, map[string]any{
			"slug": updated.Slug, "status": updated.Status,
			"sleep_relevance": updated.SleepRelevance, "spec_source": updated.SpecSource,
		})
		if auditErr != nil {
			// The update is written; report the missing trail in a header rather than
			// changing the body shape of a resource response.
			w.Header().Set("X-Audit-Warning", "the audit trail could not be written")
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// CreateCategoryHandler serves POST /api/v1/admin/categories.
func CreateCategoryHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ParentID    string `json:"parent_id"`
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
			SortOrder   int    `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid JSON body", "code": "invalid_body",
			})
			return
		}
		id, err := svc.CreateCategory(r.Context(), Category{
			ParentID: req.ParentID, Name: req.Name, Slug: req.Slug,
			Description: req.Description, SortOrder: req.SortOrder,
		})
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// CreateVariantHandler serves POST /api/v1/admin/variants — the sellable unit. The SKU
// uniqueness rule (§۱۵) is checked by the Service and surfaces as 409.
func CreateVariantHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProductID   string          `json:"product_id"`
			SKU         string          `json:"sku"`
			Barcode     string          `json:"barcode"`
			Attributes  json.RawMessage `json:"attributes"`
			WeightGrams *int            `json:"weight_grams"`
			LengthMm    *int            `json:"length_mm"`
			WidthMm     *int            `json:"width_mm"`
			HeightMm    *int            `json:"height_mm"`
			Status      string          `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid JSON body", "code": "invalid_body",
			})
			return
		}
		id, err := svc.CreateVariant(r.Context(), Variant{
			ProductID: req.ProductID, SKU: req.SKU, Barcode: req.Barcode,
			Attributes: req.Attributes, WeightGrams: req.WeightGrams,
			LengthMm: req.LengthMm, WidthMm: req.WidthMm, HeightMm: req.HeightMm,
			Status: req.Status,
		})
		if err != nil {
			writeCatalogErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// audit records a sensitive catalogue action (IAM §۷). A nil recorder is a no-op. The error
// is returned rather than swallowed: a catalogue change with no trail must be visible.
func (s *Service) audit(r *http.Request, action, entityID string, oldV, newV any) error {
	if s.auditor == nil {
		return nil
	}
	return s.auditor.Record(r.Context(), "ADMIN", shared.ActorID(r.Context()), action, "product", entityID,
		oldV, newV, "")
}

// AdminListStore reads products for the panel, including DRAFT/PAUSED rows the
// storefront must never see. Separate from SearchStore so the storefront query stays
// untouched.
type AdminListStore struct {
	pool *pgxpool.Pool
}

// NewAdminListStore wires the reader.
func NewAdminListStore(pool *pgxpool.Pool) *AdminListStore { return &AdminListStore{pool: pool} }

// AdminProductRow is one row of the panel list.
type AdminProductRow struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Status         string `json:"status"`
	SleepRelevance string `json:"sleep_relevance,omitempty"`
	SpecSource     string `json:"spec_source,omitempty"`
	HasGate        bool   `json:"publish_gate_satisfied"`
	UpdatedAt      string `json:"updated_at"`
}

// List returns a page of products, optionally filtered by status and a title/slug term.
func (s *AdminListStore) List(ctx context.Context, status, q string, page, pageSize int) ([]AdminProductRow, int, error) {
	const where = `
	  WHERE ($1 = '' OR status = $1)
	    AND ($2 = '' OR title ILIKE '%' || $2 || '%' OR slug ILIKE '%' || $2 || '%')`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM products`+where, status, q).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, title, slug, status, COALESCE(sleep_relevance,''), COALESCE(spec_source,''),
		        (sleep_relevance IS NOT NULL AND sleep_relevance <> ''
		         AND spec_source IS NOT NULL AND spec_source <> ''
		         AND spec_last_reviewed_at IS NOT NULL) AS gate_ok,
		        updated_at
		   FROM products`+where+`
		  ORDER BY updated_at DESC
		  LIMIT $3 OFFSET $4`, status, q, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]AdminProductRow, 0, pageSize)
	for rows.Next() {
		var r AdminProductRow
		var updated pgtype.Timestamptz
		if err := rows.Scan(&r.ID, &r.Title, &r.Slug, &r.Status, &r.SleepRelevance,
			&r.SpecSource, &r.HasGate, &updated); err != nil {
			return nil, 0, err
		}
		if updated.Valid {
			r.UpdatedAt = updated.Time.UTC().Format(time.RFC3339)
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// ListProductsHandler serves GET /api/v1/admin/products (catalog.product.view).
func ListProductsHandler(s *AdminListStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		size := parsePositiveInt(r.URL.Query().Get("page_size"), 20)
		if size > 100 {
			size = 100
		}
		rows, total, err := s.List(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("q"), page, size)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error", "code": "internal_error"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": rows, "total": total, "page": page, "page_size": size,
		})
	}
}
