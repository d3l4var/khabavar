// admin.go exposes the pricing write surface (R-07 admin-pricing, R-05).
//
// Money is rials (int64) throughout; the panel converts to toman for display only
// (R-05 §۲۲). Every handler is mounted behind authorization.Require:
// pricing.cost.manage for cost records, pricing.price.approve for selling prices and
// approvals, reports.financial.view for unit economics.
//
// The approver is taken from the authenticated session (shared.ActorID), never from the
// request body — R-05 §۹.۲ requires approved_by to be the authorised person who acted.
package pricing

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// costRequest is the landed-cost payload: eight additive components (R-05 §۱۹).
type costRequest struct {
	SupplyType               string     `json:"supply_type"`
	SupplierID               string     `json:"supplier_id"`
	PurchaseCost             int64      `json:"purchase_cost"`
	InboundFreight           int64      `json:"inbound_freight"`
	InspectionCost           int64      `json:"inspection_cost"`
	PackagingCost            int64      `json:"packaging_cost"`
	FulfillmentCost          int64      `json:"fulfillment_cost"`
	EstimatedPaymentFee      int64      `json:"estimated_payment_fee"`
	EstimatedShippingSubsidy int64      `json:"estimated_shipping_subsidy"`
	ExpectedReturnCost       int64      `json:"expected_return_cost"`
	Currency                 string     `json:"currency"`
	ValidFrom                *time.Time `json:"valid_from"`
	ValidUntil               *time.Time `json:"valid_until"`
}

// CreateCostRecordHandler serves POST /api/v1/admin/variants/{id}/cost-records.
func CreateCostRecordHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		variantID := r.PathValue("id")
		var req costRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writePricingErr(w, errors.New("pricing: invalid JSON body"))
			return
		}
		c := CostRecord{
			VariantID:                variantID,
			SupplyType:               req.SupplyType,
			SupplierID:               req.SupplierID,
			PurchaseCost:             req.PurchaseCost,
			InboundFreight:           req.InboundFreight,
			InspectionCost:           req.InspectionCost,
			PackagingCost:            req.PackagingCost,
			FulfillmentCost:          req.FulfillmentCost,
			EstimatedPaymentFee:      req.EstimatedPaymentFee,
			EstimatedShippingSubsidy: req.EstimatedShippingSubsidy,
			ExpectedReturnCost:       req.ExpectedReturnCost,
			Currency:                 req.Currency,
			ValidUntil:               req.ValidUntil,
			CreatedBy:                shared.ActorID(r.Context()),
		}
		if req.ValidFrom != nil {
			c.ValidFrom = *req.ValidFrom
		}
		if c.Currency == "" {
			c.Currency = DefaultCurrency
		}
		id, err := svc.CreateCostRecord(r.Context(), c)
		if err != nil {
			writePricingErr(w, err)
			return
		}
		svc.audit(r, "pricing.cost.recorded", variantID, nil, map[string]any{
			"cost_record_id": id,
			"total_cost":     TotalCost(c),
			"currency":       c.Currency,
		})
		writeJSONP(w, http.StatusCreated, map[string]any{
			"id": id, "total_cost": TotalCost(c), "currency": c.Currency,
		})
	}
}

// ListCostRecordsHandler serves GET /api/v1/admin/variants/{id}/cost-records.
func ListCostRecordsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := svc.ListCostHistory(r.Context(), r.PathValue("id"))
		if err != nil {
			writePricingErr(w, err)
			return
		}
		writeJSONP(w, http.StatusOK, map[string]any{"data": records})
	}
}

// priceRequest is the selling-price payload. DiscountPolicy is the raw JSONB policy
// (R-05 §۹) used to compute the discount for contribution.
type priceRequest struct {
	SellingPrice           int64           `json:"selling_price"`
	DiscountPolicy         json.RawMessage `json:"discount_policy"`
	TargetContributionRate *float64        `json:"target_contribution_rate"`
	Currency               string          `json:"currency"`
	ValidFrom              *time.Time      `json:"valid_from"`
	ValidUntil             *time.Time      `json:"valid_until"`
}

// CreatePriceRecordHandler serves POST /api/v1/admin/variants/{id}/price-records.
// The record is created PENDING approval; a non-positive contribution (BR-06) is
// returned as a warning, not a rejection.
func CreatePriceRecordHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		variantID := r.PathValue("id")
		var req priceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writePricingErr(w, errors.New("pricing: invalid JSON body"))
			return
		}
		p := PriceRecord{
			VariantID:              variantID,
			SellingPrice:           req.SellingPrice,
			DiscountPolicy:         req.DiscountPolicy,
			TargetContributionRate: req.TargetContributionRate,
			Currency:               req.Currency,
			ValidUntil:             req.ValidUntil,
		}
		if req.ValidFrom != nil {
			p.ValidFrom = *req.ValidFrom
		}
		res, err := svc.CreatePriceRecordWithWarning(r.Context(), p)
		if err != nil {
			if errors.Is(err, ErrNoValidCost) {
				writeJSONP(w, http.StatusConflict, map[string]any{
					"error": err.Error(), "code": "no_valid_cost",
				})
				return
			}
			writePricingErr(w, err)
			return
		}
		svc.audit(r, "pricing.price.recorded", variantID, nil, map[string]any{
			"price_record_id": res.ID,
			"selling_price":   req.SellingPrice,
			"currency":        p.Currency,
		})
		body := map[string]any{"id": res.ID, "status": "PENDING_APPROVAL"}
		if len(res.Warnings) > 0 {
			body["warnings"] = res.Warnings
		}
		writeJSONP(w, http.StatusCreated, body)
	}
}

// ListPriceRecordsHandler serves GET /api/v1/admin/variants/{id}/price-records.
func ListPriceRecordsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := svc.ListPriceHistory(r.Context(), r.PathValue("id"))
		if err != nil {
			writePricingErr(w, err)
			return
		}
		writeJSONP(w, http.StatusOK, map[string]any{"data": records})
	}
}

// ApprovePriceRecordHandler serves POST /api/v1/admin/price-records/{id}/approve.
// The approver is the authenticated admin (R-05 §۹.۲), not a body field.
func ApprovePriceRecordHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		approver := shared.ActorID(r.Context())
		if approver == "" {
			// Defence in depth: the route is behind Authenticate, so this cannot happen
			// unless the guard is misconfigured — fail closed rather than write a
			// nameless approval.
			writeJSONP(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized", "code": "unauthorized",
			})
			return
		}
		if err := svc.ApprovePriceRecord(r.Context(), id, approver); err != nil {
			writePricingErr(w, err)
			return
		}
		svc.audit(r, "pricing.price.approved", id, nil, map[string]any{"approved_by": approver})
		writeJSONP(w, http.StatusOK, map[string]string{"status": "APPROVED", "approved_by": approver})
	}
}

// UnitEconomicsHandler serves GET /api/v1/admin/variants/{id}/economics.
func UnitEconomicsHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ue, err := svc.UnitEconomics(r.Context(), r.PathValue("id"))
		if err != nil {
			writePricingErr(w, err)
			return
		}
		writeJSONP(w, http.StatusOK, ue)
	}
}

// audit records a sensitive pricing action (IAM §۷). A nil recorder is a no-op.
func (s *Service) audit(r *http.Request, action, entityID string, oldV, newV any) {
	if s.auditor == nil {
		return
	}
	_ = s.auditor.Record(r.Context(), "ADMIN", shared.ActorID(r.Context()), action, "variant", entityID,
		oldV, newV, "")
}
