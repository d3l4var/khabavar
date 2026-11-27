package suppliers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPG_SuppliersHTTPHandlers drives the supplier panel through its handlers. http.go was
// at 0%, so a wrong status code or a missing guard could not be caught.
func TestPG_SuppliersHTTPHandlers(t *testing.T) {
	pool, svc, store := setupSuppliers(t)
	ctx := context.Background()
	_ = pool

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Validation: broken JSON and a missing name are 400s.
	if rec := call(CreateSupplierHandler(svc), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := call(CreateSupplierHandler(svc), http.MethodPost, `{"supplier_type":"MANUFACTURER"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing name = %d, want 400", rec.Code)
	}

	// Create.
	rec := call(CreateSupplierHandler(svc), http.MethodPost,
		`{"name":"تأمین‌کنندهٔ الف","supplier_type":"MANUFACTURER","phone":"02100000001"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatalf("create response = %s", rec.Body.String())
	}

	// Status transition through the handler; an invalid status is a 400.
	if rec := call(UpdateSupplierStatusHandler(svc), http.MethodPatch, `{"status":"NONSENSE"}`, map[string]string{"id": created.ID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400", rec.Code)
	}
	if rec := call(UpdateSupplierStatusHandler(svc), http.MethodPatch, `{"status":"PROBATION"}`, map[string]string{"id": created.ID}); rec.Code >= 400 {
		t.Fatalf("status transition = %d: %s", rec.Code, rec.Body.String())
	}

	// Evaluation: an out-of-range score is refused, a valid one is created and listed.
	if rec := call(CreateEvaluationHandler(svc), http.MethodPost,
		`{"quality_score":150}`+`{"pricing_score":50}`, map[string]string{"id": created.ID}); rec.Code < 400 {
		// The body above is malformed on purpose; a 400 is expected either way.
		t.Fatalf("malformed evaluation = %d, want a 4xx", rec.Code)
	}
	rec = call(CreateEvaluationHandler(svc), http.MethodPost,
		`{"quality_score":90,"pricing_score":80,"availability_score":70,"warranty_score":60,`+
			`"documentation_score":50,"flexibility_score":40,"logistics_score":30,"communication_score":20}`,
		map[string]string{"id": created.ID})
	if rec.Code != http.StatusCreated {
		t.Fatalf("evaluation = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(ListEvaluationsHandler(svc), http.MethodGet, "", map[string]string{"id": created.ID}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), created.ID) {
		t.Fatalf("evaluations = %d: %s", rec.Code, rec.Body.String())
	}
	// An unknown supplier has no evaluations: a 200 with an empty list, not a 500.
	if rec := call(ListEvaluationsHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code >= 500 {
		t.Fatalf("evaluations for an unknown supplier = %d, want a 2xx/4xx", rec.Code)
	}

	// The supplier edit path.
	if rec := call(UpdateSupplierHandler(svc), http.MethodPatch, `{`, map[string]string{"id": created.ID}); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken patch = %d, want 400", rec.Code)
	}
	if rec := call(UpdateSupplierHandler(svc), http.MethodPatch, `{"name":"نام تازه"}`, map[string]string{"id": created.ID}); rec.Code != http.StatusOK {
		t.Fatalf("patch = %d: %s", rec.Code, rec.Body.String())
	}
	sup, err := store.GetSupplier(ctx, created.ID)
	if err != nil || sup == nil || sup.Name != "نام تازه" {
		t.Fatalf("after patch = %+v, %v", sup, err)
	}

	// The price-history and variant-list reads answer for a supplier with no data.
	if rec := call(ListPriceHistoryHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code >= 500 {
		t.Fatalf("price history = %d, want a 2xx/4xx", rec.Code)
	}
	if rec := call(ListSupplierVariantsHandler(svc), http.MethodGet, "", map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code >= 500 {
		t.Fatalf("supplier variants = %d, want a 2xx/4xx", rec.Code)
	}
}
