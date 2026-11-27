package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// brokenStore fails every call, so the handlers' error branches can be exercised without a
// database. These branches are where a store failure must become a 5xx with a stable body,
// not a panic or a 200 with empty data.
type brokenStore struct{ err error }

func (b brokenStore) CreateCostRecord(context.Context, CostRecord) (string, error) { return "", b.err }
func (b brokenStore) ListCostRecordsByVariant(context.Context, string) ([]CostRecord, error) {
	return nil, b.err
}
func (b brokenStore) GetValidCostRecord(context.Context, string, time.Time) (*CostRecord, error) {
	return nil, b.err
}
func (b brokenStore) CreatePriceRecord(context.Context, PriceRecord) (string, error) {
	return "", b.err
}
func (b brokenStore) ApprovePriceRecord(context.Context, string, string) error { return b.err }
func (b brokenStore) GetValidPriceRecord(context.Context, string, time.Time) (*PriceRecord, error) {
	return nil, b.err
}
func (b brokenStore) ListPriceRecordsByVariant(context.Context, string) ([]PriceRecord, error) {
	return nil, b.err
}

// TestPricingStoreFailuresAreServerErrors: a store failure is a 5xx everywhere, never a
// silent 200 with an empty list.
func TestPricingStoreFailuresAreServerErrors(t *testing.T) {
	svc := NewService(brokenStore{err: errors.New("db down")})
	id := map[string]string{"id": "v-1"}

	cases := map[string]http.HandlerFunc{
		"cost records":   ListCostRecordsHandler(svc),
		"price records":  ListPriceRecordsHandler(svc),
		"unit economics": UnitEconomicsHandler(svc),
		"storefront":     StorefrontPriceHandler(svc),
		"economics":      EconomicsHandler(svc),
	}
	for name, h := range cases {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		for k, v := range id {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		if rec.Code < 500 {
			t.Fatalf("%s with a failing store = %d, want 5xx: %s", name, rec.Code, rec.Body.String())
		}
	}
}

// TestDiscountAmount pins the discount helper: a policy with an amount is read, an empty
// or malformed policy is zero, and a negative amount never becomes a negative discount.
func TestDiscountAmount(t *testing.T) {
	if got := DiscountAmount(json.RawMessage(`{"amount":100}`)); got != 100 {
		t.Fatalf("DiscountAmount(100) = %d, want 100", got)
	}
	if got := DiscountAmount(nil); got != 0 {
		t.Fatalf("DiscountAmount(nil) = %d, want 0", got)
	}
	if got := DiscountAmount(json.RawMessage(`{not json`)); got != 0 {
		t.Fatalf("DiscountAmount(malformed) = %d, want 0", got)
	}
	if got := DiscountAmount(json.RawMessage(`{"amount":-500}`)); got != 0 {
		t.Fatalf("DiscountAmount(negative) = %d, want 0", got)
	}
}
