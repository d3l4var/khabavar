package authorization_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// seedMatrix mirrors 001_seed_baseline.sql role_permissions (IAM §۴ — ۱۰ roles).
func seedMatrix() map[string][]string {
	return map[string][]string{
		"STORE_MANAGER":     {"catalog.product.view", "catalog.product.edit", "catalog.product.publish", "catalog.attribute.manage", "pricing.cost.manage", "pricing.price.approve", "order.view", "reports.view"},
		"CONTENT_MANAGER":   {"content.article.edit", "content.article.review", "content.article.publish", "content.relation.manage", "catalog.product.view", "reports.view"},
		"EDITOR":            {"content.article.edit", "catalog.product.view"},
		"CONTENT_REVIEWER":  {"content.article.review", "content.article.edit", "catalog.product.view"},
		"ORDER_OPERATOR":    {"order.view", "order.manage", "order.cancel.approve", "inventory.manage", "identity.customer.view", "reports.view"},
		"INVENTORY_MANAGER": {"inventory.manage", "inventory.adjust", "catalog.product.view", "reports.view"},
		"SUPPORT_OPERATOR":  {"order.view", "identity.customer.view"},
		"SUPPLIER_OPERATOR": {"supplier.manage", "supplier.evaluate"},
		"FINANCE":           {"order.refund.approve", "pricing.cost.manage", "pricing.price.approve", "reports.financial.view", "audit.read", "reports.view"},
	}
}

func TestRBACRefundIsFinanceExclusive(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	nonFinance := []string{
		"STORE_MANAGER", "CONTENT_MANAGER", "EDITOR", "CONTENT_REVIEWER",
		"ORDER_OPERATOR", "INVENTORY_MANAGER", "SUPPORT_OPERATOR", "SUPPLIER_OPERATOR",
	}
	for _, role := range nonFinance {
		chk.AssignRoles(uid, role)
		got, err := chk.HasPermission(context.Background(), uid, "order.refund.approve")
		if err != nil {
			t.Fatalf("%s: unexpected error %v", role, err)
		}
		if got {
			t.Errorf("%s must NOT have order.refund.approve — refund is FINANCE-exclusive", role)
		}
	}
	chk.AssignRoles(uid, "FINANCE")
	got, _ := chk.HasPermission(context.Background(), uid, "order.refund.approve")
	if !got {
		t.Error("FINANCE must have order.refund.approve")
	}
}

func TestRBACSupportOperatorReadOnly(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "SUPPORT_OPERATOR")
	if ok, _ := chk.HasPermission(context.Background(), uid, "order.view"); !ok {
		t.Error("SUPPORT_OPERATOR must view orders")
	}
	if ok, _ := chk.HasPermission(context.Background(), uid, "order.cancel.approve"); ok {
		t.Error("SUPPORT_OPERATOR must NOT approve cancel — that is ORDER_OPERATOR")
	}
}

func TestRBACPublishSplit(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "EDITOR")
	if ok, _ := chk.HasPermission(context.Background(), uid, "content.article.publish"); ok {
		t.Error("EDITOR must NOT publish — only CONTENT_MANAGER publishes")
	}
	if ok, _ := chk.HasPermission(context.Background(), uid, "content.article.edit"); !ok {
		t.Error("EDITOR must edit articles")
	}
	chk.AssignRoles(uid, "CONTENT_MANAGER")
	if ok, _ := chk.HasPermission(context.Background(), uid, "content.article.publish"); !ok {
		t.Error("CONTENT_MANAGER must publish articles")
	}
}

func TestRBACSupplierOperator(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "SUPPLIER_OPERATOR")
	if ok, _ := chk.HasPermission(context.Background(), uid, "supplier.manage"); !ok {
		t.Error("SUPPLIER_OPERATOR must manage suppliers")
	}
	if ok, _ := chk.HasPermission(context.Background(), uid, "supplier.evaluate"); !ok {
		t.Error("SUPPLIER_OPERATOR must evaluate suppliers")
	}
	if ok, _ := chk.HasPermission(context.Background(), uid, "order.view"); ok {
		t.Error("SUPPLIER_OPERATOR must NOT view orders (least privilege — IAM §۱)")
	}
}

func TestRBACSuperAdminAll(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "SUPER_ADMIN")
	for _, perm := range []string{"audit.read", "order.refund.approve", "content.article.publish", "settings.manage"} {
		if ok, _ := chk.HasPermission(context.Background(), uid, perm); !ok {
			t.Errorf("SUPER_ADMIN must have %s (all 24 permissions — IAM §۴)", perm)
		}
	}
}

func TestRequireDeniesMissingIdentity(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	called := false
	h := authorization.Require(chk, "order.view", okHandler(&called))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("missing identity: want 401, got %d", rr.Code)
	}
	if called {
		t.Error("next handler must NOT run without identity")
	}
}

func TestRequireDeniesWithoutPermission(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "SUPPORT_OPERATOR") // has order.view but not order.manage
	called := false
	h := authorization.Require(chk, "order.manage", okHandler(&called))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/orders/42/manage", nil)
	req = req.WithContext(authorization.WithAdminUserID(req.Context(), uid))
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("SUPPORT_OPERATOR without order.manage: want 403, got %d", rr.Code)
	}
	if called {
		t.Error("next handler must NOT run when permission is missing")
	}
}

func TestRequireAllowsWithPermission(t *testing.T) {
	chk := authorization.NewMatrixChecker(seedMatrix())
	uid := uuid.NewString()
	chk.AssignRoles(uid, "ORDER_OPERATOR") // has order.manage
	called := false
	h := authorization.Require(chk, "order.manage", okHandler(&called))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders/42/fulfill", nil)
	req = req.WithContext(authorization.WithAdminUserID(req.Context(), uid))
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("ORDER_OPERATOR with order.manage: want 200, got %d", rr.Code)
	}
	if !called {
		t.Error("next handler must run when permission is present")
	}
}

func okHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}
