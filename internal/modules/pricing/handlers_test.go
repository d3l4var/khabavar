package pricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

// TestStorefrontPriceHandler_UnknownVariant: the storefront asks for a price that does not exist.
// It must answer 404, not 500 — a customer opening a stale link is not a server fault.
func TestStorefrontPriceHandler_UnknownVariant(t *testing.T) {
	svc := NewService(newMemStore())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/variants/{id}/price", StorefrontPriceHandler(svc))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/variants/does-not-exist/price", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestEconomicsHandler_UnknownVariant covers the confidential economics read for a variant that
// has no cost record: the panel must get a clear answer, not an internal error.
func TestEconomicsHandler_UnknownVariant(t *testing.T) {
	svc := NewService(newMemStore())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/admin/variants/{variantId}/economics", EconomicsHandler(svc))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/variants/nope/economics", nil))
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("answered 500 for a missing variant; body=%s", rec.Body.String())
	}
}

// TestApprovePriceRecordHandler_RequiresActor: the route sits behind Authenticate, so an empty
// actor means the guard is misconfigured. It must fail closed rather than write a nameless
// approval — an approval nobody signed is worse than a rejected request.
func TestApprovePriceRecordHandler_RequiresActor(t *testing.T) {
	svc := NewService(newMemStore())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/admin/price-records/{id}/approve", ApprovePriceRecordHandler(svc))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/price-records/pr-1/approve", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestApprovePriceRecordHandler_UnknownRecord: with a real actor but no such record, the answer is
// a client error, not 500.
func TestApprovePriceRecordHandler_UnknownRecord(t *testing.T) {
	svc := NewService(newMemStore())
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/admin/price-records/{id}/approve", ApprovePriceRecordHandler(svc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/price-records/missing/approve", nil)
	req = req.WithContext(shared.WithActorID(context.Background(), "staff-1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("answered 500 for a missing price record; body=%s", rec.Body.String())
	}
}
