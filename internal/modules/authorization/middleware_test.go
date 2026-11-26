package authorization_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// fakeChecker is a PermissionChecker with a fixed answer.
type fakeChecker struct {
	allow bool
	err   error
}

func (f fakeChecker) HasPermission(context.Context, string, string) (bool, error) {
	return f.allow, f.err
}

// fakeAuditor captures denial records.
type fakeAuditor struct {
	calls   int
	action  string
	actorID string
	details map[string]string
}

func (f *fakeAuditor) Record(_ context.Context, actorType, actorID, action, entityType, entityID string, oldValues, newValues any, ip string) error {
	f.calls++
	f.action = action
	f.actorID = actorID
	if m, ok := newValues.(map[string]string); ok {
		f.details = m
	}
	return nil
}

// TestAuthenticateRejectsMissingToken — no Authorization header means no panel access.
func TestAuthenticateRejectsMissingToken(t *testing.T) {
	called := false
	h := authorization.Authenticate(
		func(context.Context, string) (string, string, bool) { return "admin", "u1", true },
		okHandler(&called))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without token, got %d", rec.Code)
	}
	if called {
		t.Error("handler must not run without a token")
	}
}

func TestAuthenticateRejectsCustomerToken(t *testing.T) {
	called := false
	// A shop session must never open the panel (IAM §۶).
	h := authorization.Authenticate(
		func(context.Context, string) (string, string, bool) { return "customer", "c1", true },
		okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req.Header.Set("Authorization", "Bearer shop-session-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("customer token must be rejected with 401, got %d", rec.Code)
	}
	if called {
		t.Error("handler must not run for a customer token")
	}
}

func TestAuthenticateRejectsUnknownToken(t *testing.T) {
	called := false
	h := authorization.Authenticate(
		func(context.Context, string) (string, string, bool) { return "", "", false },
		okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req.Header.Set("Authorization", "Bearer revoked")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for unresolvable token, got %d", rec.Code)
	}
}

func TestAuthenticateInjectsAdminID(t *testing.T) {
	var gotID string
	h := authorization.Authenticate(
		func(context.Context, string) (string, string, bool) { return "admin", "admin-42", true },
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotID = authorization.AdminUserID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
	req.Header.Set("Authorization", "Bearer good")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if gotID != "admin-42" {
		t.Fatalf("want admin id in ctx, got %q", gotID)
	}
}

func TestRequireAuditedDeniesAndAudits(t *testing.T) {
	called := false
	aud := &fakeAuditor{}
	h := authorization.RequireAudited(fakeChecker{allow: false}, aud, "pricing.price.approve", okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/variants/v1/price-records", nil)
	req = req.WithContext(authorization.WithAdminUserID(req.Context(), "admin-7"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	if called {
		t.Error("handler must not run when permission is missing")
	}
	if aud.calls != 1 {
		t.Fatalf("denial must be audited once, got %d records", aud.calls)
	}
	if aud.action != "authorization.denied" {
		t.Errorf("audit action = %q", aud.action)
	}
	if aud.actorID != "admin-7" {
		t.Errorf("audit actor = %q", aud.actorID)
	}
	if aud.details["permission"] != "pricing.price.approve" {
		t.Errorf("audit must name the permission, got %v", aud.details)
	}
}

func TestRequireAuditedAllowsWhenPermitted(t *testing.T) {
	called := false
	aud := &fakeAuditor{}
	h := authorization.RequireAudited(fakeChecker{allow: true}, aud, "order.manage", okHandler(&called))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/fulfillment-orders", nil)
	req = req.WithContext(authorization.WithAdminUserID(req.Context(), "admin-7"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("want 200 and handler called, got %d called=%v", rec.Code, called)
	}
	if aud.calls != 0 {
		t.Errorf("allowed requests must not be audited as denials, got %d", aud.calls)
	}
}

func TestRequireAuditedCheckerErrorIsForbidden(t *testing.T) {
	called := false
	aud := &fakeAuditor{}
	h := authorization.RequireAudited(fakeChecker{err: errors.New("db down")}, aud, "order.view", okHandler(&called))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/1/refunds", nil)
	req = req.WithContext(authorization.WithAdminUserID(req.Context(), "admin-7"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// Fail closed: a checker error must never grant access.
	if rec.Code != http.StatusForbidden {
		t.Fatalf("checker error must fail closed with 403, got %d", rec.Code)
	}
	if called {
		t.Error("handler must not run when the checker errors")
	}
}

func TestRequireWithoutIdentityIsUnauthorized(t *testing.T) {
	called := false
	h := authorization.Require(fakeChecker{allow: true}, "order.view", okHandler(&called))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without identity, got %d", rec.Code)
	}
	if called {
		t.Error("handler must not run without identity")
	}
}

func TestBearerFromHeader(t *testing.T) {
	cases := []struct{ header, want string }{
		{"", ""},
		{"Bearer abc123", "abc123"},
		{"bearer abc123", "abc123"},
		{"BEARER  abc123 ", "abc123"},
		{"Basic abc123", ""},
		{"Bearer", ""},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if c.header != "" {
			req.Header.Set("Authorization", c.header)
		}
		if got := authorization.BearerFromHeader(req); got != c.want {
			t.Errorf("BearerFromHeader(%q) = %q, want %q", c.header, got, c.want)
		}
	}
}
