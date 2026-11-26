package pricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestPG_PricingHTTPHandlers drives the whole pricing panel surface through its handlers:
// cost record, price record, approval, the two lists and unit economics. The approval path
// is the money-critical one: it must take the approver from the session, not the body.
func TestPG_PricingHTTPHandlers(t *testing.T) {
	pool, svc, _ := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PRH-1")

	var approverID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'قیمت‌گذار','ACTIVE') RETURNING id`,
		"pricer-"+variantID[:8]+"@khabavar.ir").Scan(&approverID); err != nil {
		t.Fatalf("seed approver: %v", err)
	}

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		r = r.WithContext(shared.WithActorID(r.Context(), approverID))
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Cost record: 60,000 purchase + 5,000 freight + 5,000 fulfillment.
	rec := call(CreateCostRecordHandler(svc), http.MethodPost,
		`{"supply_type":"OWNED_INVENTORY","purchase_cost":60000,"inbound_freight":5000,`+
			`"fulfillment_cost":5000,"currency":"IRR"}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cost = %d: %s", rec.Code, rec.Body.String())
	}
	var cost struct {
		ID        string `json:"id"`
		TotalCost int64  `json:"total_cost"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &cost)
	if cost.TotalCost != 70000 {
		t.Fatalf("total_cost = %d, want 70000", cost.TotalCost)
	}

	// A price below cost is ALLOWED but must carry a BR-06 warning: the spec says the
	// contribution may be zero or negative and the review is the human approval step
	// (R-14 line 492). Refusing it here would contradict the contract.
	rec = call(CreatePriceRecordHandler(svc), http.MethodPost,
		`{"selling_price":10000,"currency":"IRR"}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("below-cost price = %d: %s", rec.Code, rec.Body.String())
	}
	var belowCost struct {
		Warnings []string `json:"warnings"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &belowCost)
	if len(belowCost.Warnings) == 0 {
		t.Fatal("a below-cost price must carry a BR-06 warning")
	}

	// A healthy price is created PENDING_APPROVAL.
	rec = call(CreatePriceRecordHandler(svc), http.MethodPost,
		`{"selling_price":100000,"currency":"IRR"}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create price = %d: %s", rec.Code, rec.Body.String())
	}
	var price struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &price)
	if price.Status != "PENDING_APPROVAL" {
		t.Fatalf("status = %q, want PENDING_APPROVAL", price.Status)
	}

	// Approve: the approver comes from the session.
	rec = call(ApprovePriceRecordHandler(svc), http.MethodPost, "", map[string]string{"id": price.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("approve = %d: %s", rec.Code, rec.Body.String())
	}
	var approved struct {
		Status     string `json:"status"`
		ApprovedBy string `json:"approved_by"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &approved)
	if approved.Status != "APPROVED" || approved.ApprovedBy != approverID {
		t.Fatalf("approval = %+v, want APPROVED by the session actor", approved)
	}

	// Approving twice must not silently succeed.
	if rec = call(ApprovePriceRecordHandler(svc), http.MethodPost, "", map[string]string{"id": price.ID}); rec.Code < 400 {
		t.Fatalf("double approval = %d, want a 4xx", rec.Code)
	}

	// The lists and unit economics must answer.
	for _, tc := range []struct {
		name string
		h    http.HandlerFunc
	}{
		{"cost records", ListCostRecordsHandler(svc)},
		{"price records", ListPriceRecordsHandler(svc)},
		{"unit economics", UnitEconomicsHandler(svc)},
	} {
		if rec := call(tc.h, http.MethodGet, "", map[string]string{"id": variantID}); rec.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}
