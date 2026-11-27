package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/shared"
)

type failAuditor struct{ err error }

func (f *failAuditor) Record(_ context.Context, _, _, _, _, _ string, _, _ any, _ string) error {
	return f.err
}

// TestAdminHandlersRejectBadJSON pins the fix for a client mistake answered with a 500:
// every catalogue admin write used to route a decode error through writeCatalogErr, whose
// default branch is a 500.
func TestAdminHandlersRejectBadJSON(t *testing.T) {
	svc := NewService(newMemStore())
	handlers := map[string]http.HandlerFunc{
		"create product":  CreateProductHandler(svc),
		"update product":  UpdateProductHandler(svc),
		"create category": CreateCategoryHandler(svc),
		"create variant":  CreateVariantHandler(svc),
		"product status":  ProductStatusHandler(svc),
	}
	for name, h := range handlers {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{`))
		r.SetPathValue("id", "p-1")
		rec := httptest.NewRecorder()
		h(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s with a broken body = %d, want 400: %s", name, rec.Code, rec.Body.String())
		}
	}
}

// TestAuditFailureIsVisible: the write is already done, so the response stays 2xx, but a
// failed audit must leave a visible marker instead of disappearing.
func TestAuditFailureIsVisible(t *testing.T) {
	store := newMemStore()
	svc := NewService(store)
	ctx := context.Background()
	id, err := svc.CreateProduct(ctx, Product{Title: "محصول", Slug: "audit-1", Status: ProductDraft})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	svc.SetAuditor(&failAuditor{err: errors.New("audit down")})

	// The property is "the audit error is not swallowed": call the helper the handlers use.
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r = r.WithContext(shared.WithActorID(r.Context(), "admin-1"))
	if err := svc.audit(r, "catalog.product.updated", id, nil, map[string]any{"status": "DRAFT"}); err == nil {
		t.Fatal("a failed audit must be returned, not swallowed")
	}
	// And a nil recorder is a no-op, not an error.
	svc.SetAuditor(nil)
	if err := svc.audit(r, "catalog.product.updated", id, nil, nil); err != nil {
		t.Fatalf("nil auditor = %v, want nil", err)
	}
}
