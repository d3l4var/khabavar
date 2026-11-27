// Package suppliers — HTTP handlers for supplier admin + storefront read endpoints (R-07 §suppliers).
// Admin write endpoints (create supplier/product/variant, price update, evaluation) are
// mounted behind authorization.Require in main.go; storefront reads (variant supply
// options, price history) are public.
package suppliers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// CreateSupplierHandler serves POST /api/v1/admin/suppliers — create a new supplier (R-05 §۳).
// pathID reads the path parameter whatever the route table calls it. The evaluation routes
// declare {supplierId} and the cost/price routes {variantId}, but these handlers read
// PathValue("id") and got "" — every one answered 404 on a real id.
func pathID(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := r.PathValue(n); v != "" {
			return v
		}
	}
	return ""
}

func CreateSupplierHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name               string `json:"name"`
			SupplierType       string `json:"supplier_type"`
			RegistrationNumber string `json:"registration_number"`
			ContactName        string `json:"contact_name"`
			Phone              string `json:"phone"`
			Email              string `json:"email"`
			Address            string `json:"address"`
			Website            string `json:"website"`
			Status             string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSupErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if req.Name == "" {
			writeSupErr(w, http.StatusBadRequest, "name required")
			return
		}
		if req.Status == "" {
			req.Status = "ACTIVE"
		}
		id, err := svc.CreateSupplier(r.Context(), Supplier{
			Name: req.Name, SupplierType: req.SupplierType,
			RegistrationNumber: req.RegistrationNumber, ContactName: req.ContactName,
			Phone: req.Phone, Email: req.Email, Address: req.Address,
			Website: req.Website, Status: req.Status,
		})
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// UpdateSupplierStatusHandler serves PATCH /api/v1/admin/suppliers/{id}/status.
func UpdateSupplierStatusHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r, "supplierId", "variantId", "id")
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSupErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := svc.UpdateSupplierStatus(r.Context(), id, req.Status); err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// ListSupplierVariantsHandler serves GET /api/v1/variants/{id}/suppliers — the
// storefront reads which suppliers can fulfil a variant (R-05 §۲ multi-source).
func ListSupplierVariantsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		variantID := pathID(r, "supplierId", "variantId", "id")
		if variantID == "" {
			writeSupErr(w, http.StatusBadRequest, "variant id required")
			return
		}
		svs, err := svc.ListSupplierVariantsForVariant(r.Context(), variantID)
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": svs})
	}
}

// ListPriceHistoryHandler serves GET /api/v1/admin/supplier-variants/{id}/price-history.
func ListPriceHistoryHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svID := pathID(r, "supplierId", "variantId", "id")
		if svID == "" {
			writeSupErr(w, http.StatusBadRequest, "supplier-variant id required")
			return
		}
		entries, err := svc.ListPriceHistory(r.Context(), svID)
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": entries})
	}
}

// CreateEvaluationHandler serves POST /api/v1/admin/suppliers/{id}/evaluations —
// record a weighted supplier evaluation (R-05 §۳). Scores are 0-100 per dimension.
func CreateEvaluationHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		supplierID := pathID(r, "supplierId", "variantId", "id")
		var req struct {
			QualityScore       int `json:"quality_score"`
			PricingScore       int `json:"pricing_score"`
			AvailabilityScore  int `json:"availability_score"`
			WarrantyScore      int `json:"warranty_score"`
			DocumentationScore int `json:"documentation_score"`
			FlexibilityScore   int `json:"flexibility_score"`
			LogisticsScore     int `json:"logistics_score"`
			CommunicationScore int `json:"communication_score"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSupErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		ev := Evaluation{
			SupplierID:         supplierID,
			QualityScore:       req.QualityScore,
			PricingScore:       req.PricingScore,
			AvailabilityScore:  req.AvailabilityScore,
			WarrantyScore:      req.WarrantyScore,
			DocumentationScore: req.DocumentationScore,
			FlexibilityScore:   req.FlexibilityScore,
			LogisticsScore:     req.LogisticsScore,
			CommunicationScore: req.CommunicationScore,
			EvaluationDate:     time.Now(),
		}
		id, err := svc.CreateEvaluation(r.Context(), ev)
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// ListEvaluationsHandler serves GET /api/v1/admin/suppliers/{id}/evaluations.
func ListEvaluationsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		supplierID := pathID(r, "supplierId", "variantId", "id")
		if supplierID == "" {
			writeSupErr(w, http.StatusBadRequest, "supplier id required")
			return
		}
		evs, err := svc.ListEvaluationsBySupplier(r.Context(), supplierID)
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": evs})
	}
}

func writeSupSvcErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSupplierNotFound), errors.Is(err, ErrSupplierProductNotFound), errors.Is(err, ErrSupplierVariantNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrUnknownStatus):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown supplier status", "code": "unknown_status"})
	case errors.Is(err, ErrInvalidStatus):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "invalid status transition", "code": "invalid_status"})
	case errors.Is(err, ErrDuplicateSupplierProduct):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "supplier+product link already exists", "code": "duplicate_link"})
	case errors.Is(err, ErrNameRequired):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
	case errors.Is(err, ErrScoreOutOfRange):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "score must be 0-100", "code": "score_out_of_range"})
	case errors.Is(err, ErrPriceNegative):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "purchase price cannot be negative"})
	case errors.Is(err, ErrPriceExpired):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "supplier price expired", "code": "price_expired"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeSupErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// UpdateSupplierHandler serves PATCH /api/v1/admin/suppliers/{id}. Status is not editable
// here: it has its own guarded transition.
func UpdateSupplierHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name        *string `json:"name"`
			ContactName *string `json:"contact_name"`
			Phone       *string `json:"phone"`
			Email       *string `json:"email"`
			Address     *string `json:"address"`
			Website     *string `json:"website"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeSupErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		sup, err := svc.UpdateSupplier(r.Context(), pathID(r, "supplierId", "variantId", "id"), SupplierPatch{
			Name: req.Name, ContactName: req.ContactName, Phone: req.Phone,
			Email: req.Email, Address: req.Address, Website: req.Website,
		})
		if err != nil {
			writeSupSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sup)
	}
}
