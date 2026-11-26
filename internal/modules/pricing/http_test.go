package pricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedPricedVariant sets up a service with a valid cost record and an APPROVED price
// for variant "v1", anchored on the R-05 §۱۹ example.
func seedPricedVariant(t *testing.T) *Service {
	t.Helper()
	svc, _, _, _ := newTestService(t)
	policy := mustJSON(t, map[string]any{"amount": 500_000})
	pid, _, _ := svc.CreatePriceRecord(context.Background(), PriceRecord{
		VariantID: "v1", SellingPrice: 10_000_000, DiscountPolicy: policy,
	})
	if err := svc.ApprovePriceRecord(context.Background(), pid, "admin-1"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return svc
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStorefrontPriceHandler_ApprovedVariant(t *testing.T) {
	svc := seedPricedVariant(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/variants/v1/price", nil)
	req.SetPathValue("id", "v1")
	rec := httptest.NewRecorder()
	StorefrontPriceHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Customer-visible fields present.
	for _, want := range []string{`"SellingPrice":10000000`, `"Discount":500000`, `"NetRevenue":9500000`, `"Currency":"IRR"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	// Confidential fields MUST NOT leak to the storefront (R-05 §۲۴).
	for _, secret := range []string{"TotalCost", "Contribution", "ContributionRate", "Warnings"} {
		if strings.Contains(body, secret) {
			t.Fatalf("storefront price leaked confidential field %q: %s", secret, body)
		}
	}
}

func TestStorefrontPriceHandler_PendingReturns404(t *testing.T) {
	// Variant with a price that is NOT yet approved → 404.
	svc, _, _, _ := newTestService(t)
	svc.CreatePriceRecord(context.Background(), PriceRecord{VariantID: "v1", SellingPrice: 10_000_000})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/variants/v1/price", nil)
	req.SetPathValue("id", "v1")
	rec := httptest.NewRecorder()
	StorefrontPriceHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("pending price must be 404 for storefront, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestStorefrontPriceHandler_NoPriceReturns404(t *testing.T) {
	svc := seedPricedVariant(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/variants/no-such/price", nil)
	req.SetPathValue("id", "no-such")
	rec := httptest.NewRecorder()
	StorefrontPriceHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestEconomicsHandler_FullEconomics(t *testing.T) {
	svc := seedPricedVariant(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/variants/v1/economics", nil)
	req.SetPathValue("id", "v1")
	rec := httptest.NewRecorder()
	EconomicsHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Admin endpoint exposes the confidential fields.
	for _, want := range []string{`"total_cost":5500000`, `"contribution":4000000`, `"net_revenue":9500000`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestEconomicsHandler_NoCostWarns(t *testing.T) {
	svc := seedPricedVariant(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/variants/empty/economics", nil)
	req.SetPathValue("id", "empty")
	rec := httptest.NewRecorder()
	EconomicsHandler(svc).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no currently-valid cost record") {
		t.Fatalf("expected no-cost warning: %s", rec.Body.String())
	}
}
