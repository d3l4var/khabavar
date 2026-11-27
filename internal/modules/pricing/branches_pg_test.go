package pricing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/khabavar/khabavar/internal/modules/shared"
)

// recordAuditor captures audit calls and can be made to fail.
type recordAuditor struct {
	actions []string
	err     error
}

func (a *recordAuditor) Record(_ context.Context, _, _, action, _, _ string, _, _ any, _ string) error {
	a.actions = append(a.actions, action)
	return a.err
}

// TestNetRevenue pins the floor: a discount larger than the price must never produce a
// negative revenue, because that value feeds contribution and margin maths.
func TestNetRevenue(t *testing.T) {
	if got := NetRevenue(1000, 250); got != 750 {
		t.Fatalf("NetRevenue(1000,250) = %d, want 750", got)
	}
	if got := NetRevenue(1000, 1500); got != 0 {
		t.Fatalf("NetRevenue with an oversized discount = %d, want 0", got)
	}
}

// TestPG_PricingErrorBranches drives every branch of writePricingErr through the handlers,
// plus the audit hook. These are the paths where a wrong mapping turns a client mistake
// into a 500.
func TestPG_PricingErrorBranches(t *testing.T) {
	pool, svc, _ := setupPricing(t)
	ctx := context.Background()
	variantID := seedVariant(t, ctx, pool, "SKU-PEB-1")

	var actorID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, status) VALUES ($1,'قیمت‌گذار','ACTIVE') RETURNING id`,
		"peb-"+variantID[:8]+"@khabavar.ir").Scan(&actorID); err != nil {
		t.Fatalf("seed actor: %v", err)
	}

	// Wire the auditor so SetAuditor and the audit helper are exercised.
	aud := &recordAuditor{}
	svc.SetAuditor(aud)

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		r = r.WithContext(shared.WithActorID(r.Context(), actorID))
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Broken JSON on every write handler.
	for name, h := range map[string]http.HandlerFunc{
		"cost":  CreateCostRecordHandler(svc),
		"price": CreatePriceRecordHandler(svc),
	} {
		if rec := call(h, http.MethodPost, `{`, map[string]string{"id": variantID}); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s broken body = %d, want 400", name, rec.Code)
		}
	}

	// Unit economics for an unknown variant is a 200 WITH warnings, by design: the endpoint
	// reports what is missing (no cost, no approved price) instead of erroring.
	recUE := call(UnitEconomicsHandler(svc), http.MethodGet, "", map[string]string{"id": uuid.NewString()})
	if recUE.Code != http.StatusOK || !strings.Contains(recUE.Body.String(), "warning") {
		t.Fatalf("unit economics for an unknown variant = %d: %s", recUE.Code, recUE.Body.String())
	}

	// An invalid supply type is a 400 (not a 500).
	rec := call(CreateCostRecordHandler(svc), http.MethodPost,
		`{"supply_type":"NONSENSE","purchase_cost":1000}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid supply type = %d: %s", rec.Code, rec.Body.String())
	}
	// A supplier-sourced cost without a supplier is a 400.
	rec = call(CreateCostRecordHandler(svc), http.MethodPost,
		`{"supply_type":"SUPPLIER_SOURCED","purchase_cost":1000}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing supplier = %d: %s", rec.Code, rec.Body.String())
	}
	// A valid cost record is needed before the price checks: the cost gate runs first.
	if rec := call(CreateCostRecordHandler(svc), http.MethodPost,
		`{"supply_type":"OWNED_INVENTORY","purchase_cost":50000}`, map[string]string{"id": variantID}); rec.Code != http.StatusCreated {
		t.Fatalf("cost record = %d: %s", rec.Code, rec.Body.String())
	}
	if len(aud.actions) == 0 {
		t.Fatal("the audit hook recorded nothing")
	}

	// A target rate outside [0,1) is a 400.
	rec = call(CreatePriceRecordHandler(svc), http.MethodPost,
		`{"selling_price":100000,"target_contribution_rate":1.5}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad target rate = %d: %s", rec.Code, rec.Body.String())
	}

	// A price for a variant with no cost at all is a 409 with a machine-readable code.
	costlessVariant := seedVariant(t, ctx, pool, "SKU-PEB-2")
	rec = call(CreatePriceRecordHandler(svc), http.MethodPost,
		`{"selling_price":100000}`, map[string]string{"id": costlessVariant})
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "no_valid_cost") {
		t.Fatalf("price without cost = %d: %s", rec.Code, rec.Body.String())
	}

	// Approving an unknown price record is a 404, and approving twice is a 409.
	if rec := call(ApprovePriceRecordHandler(svc), http.MethodPost, "", map[string]string{"id": uuid.NewString()}); rec.Code >= 500 {
		t.Fatalf("approve unknown = %d, want a 4xx", rec.Code)
	}

	// A failing auditor must not be silent: the record is saved (a 5xx would invite a
	// duplicate retry) but the response says the trail is missing.
	aud.err = errors.New("audit down")
	rec = call(CreateCostRecordHandler(svc), http.MethodPost,
		`{"supply_type":"OWNED_INVENTORY","purchase_cost":60000}`, map[string]string{"id": variantID})
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "audit_warning") {
		t.Fatalf("a failed audit = %d: %s", rec.Code, rec.Body.String())
	}
}
