// Package pricing — HTTP handlers. The storefront price endpoint is public and
// exposes only customer-visible fields (R-05 §۲۴: confidential economics stay hidden).
// The full unit-economics endpoint is admin-only and is intended to sit behind the
// authorization.Require middleware (RBAC wiring lands in P5b-catalog admin pass).
package pricing

import (
	"encoding/json"
	"errors"
	"net/http"
)

// StorefrontPriceHandler serves GET /api/v1/variants/{id}/price — the live approved
// selling price for a variant. Returns 404 when no approved price is currently valid
// (the product is not sellable at that moment).
func StorefrontPriceHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writePricingErr(w, ErrNoApprovedPrice)
			return
		}
		sp, err := svc.StorefrontPrice(r.Context(), id)
		if err != nil {
			writePricingErr(w, err)
			return
		}
		writeJSONP(w, http.StatusOK, map[string]any{"data": sp})
	}
}

// EconomicsHandler serves GET /api/v1/admin/variants/{id}/economics — the full
// per-variant unit economics (cost, contribution, rate, warnings). CONFIDENTIAL:
// must be mounted behind authorization.Require (role with pricing:read).
func EconomicsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writePricingErr(w, ErrVariantNotFound)
			return
		}
		ue, err := svc.UnitEconomics(r.Context(), id)
		if err != nil {
			writePricingErr(w, err)
			return
		}
		writeJSONP(w, http.StatusOK, map[string]any{"data": ue})
	}
}

func writePricingErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoApprovedPrice), errors.Is(err, ErrVariantNotFound):
		writeJSONP(w, http.StatusNotFound, map[string]any{"error": "not found"})
	case errors.Is(err, ErrNoValidCost):
		writeJSONP(w, http.StatusNotFound, map[string]any{"error": "no valid cost record", "code": "no_valid_cost"})
	case errors.Is(err, ErrInvalidSupplyType):
		writeJSONP(w, http.StatusBadRequest, map[string]any{"error": "invalid supply type"})
	case errors.Is(err, ErrSupplierRequired):
		writeJSONP(w, http.StatusBadRequest, map[string]any{"error": "supplier_id required"})
	case errors.Is(err, ErrPriceAlreadyApproved):
		writeJSONP(w, http.StatusConflict, map[string]any{"error": "price already approved", "code": "already_approved"})
	case errors.Is(err, ErrInvalidTargetRate):
		writeJSONP(w, http.StatusBadRequest, map[string]any{"error": "target rate must be in [0,1)"})
	default:
		writeJSONP(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeJSONP(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
