package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestAdminPermissionsAreSeedCodes is the RBAC gate (IAM §۱, TS §۴): every permission
// referenced by an admin route must exist in the seeded 24-permission catalogue, so a
// typo cannot silently lock a role out of an endpoint.
func TestAdminPermissionsAreSeedCodes(t *testing.T) {
	known := make(map[string]bool, len(seedPermissionCodes))
	for _, c := range seedPermissionCodes {
		known[c] = true
	}
	if len(known) != 24 {
		t.Fatalf("seed catalogue must hold 24 permissions (IAM §۳), got %d", len(known))
	}
	for pattern, perm := range adminPermissions {
		if !strings.Contains(pattern, "/admin/") {
			t.Errorf("%q is not an admin route but is in adminPermissions", pattern)
		}
		if !known[perm] {
			t.Errorf("%q requires %q, which is not a seeded permission", pattern, perm)
		}
	}
}

// TestEveryAdminRouteIsGuarded proves the invariant end to end: mounting every mapped
// admin route and calling it without a token yields 401, never 200.
func TestEveryAdminRouteIsGuarded(t *testing.T) {
	if len(adminPermissions) == 0 {
		t.Fatal("no admin routes mapped")
	}
	mux := http.NewServeMux()
	guard := newAdminGuard(
		func(context.Context, string) (string, string, bool) { return "", "", false },
		denyAllChecker{},
		nil,
	)
	api := newAPIMux(mux, guard)
	for pattern := range adminPermissions {
		api.admin(pattern, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
	}
	for pattern := range adminPermissions {
		method, path := splitPattern(pattern)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, concretePath(path), nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: want 401, got %d", pattern, rec.Code)
		}
	}
}

// TestAdminMountPanicsWithoutPermission: an unmapped admin route must fail loudly at
// startup rather than ship unguarded.
func TestAdminMountPanicsWithoutPermission(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("mounting an unmapped admin route must panic")
		}
	}()
	api := newAPIMux(http.NewServeMux(), newAdminGuard(nil, denyAllChecker{}, nil))
	api.admin("POST /api/v1/admin/not-mapped", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
}

// TestPublicRoutesNeedNoToken: storefront reads stay open.
func TestPublicRoutesNeedNoToken(t *testing.T) {
	mux := http.NewServeMux()
	api := newAPIMux(mux, newAdminGuard(nil, denyAllChecker{}, nil))
	api.public("GET /api/v1/products/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/products/p1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public route must not require a token, got %d", rec.Code)
	}
}

// denyAllChecker is a PermissionChecker that never grants.
type denyAllChecker struct{}

func (denyAllChecker) HasPermission(context.Context, string, string) (bool, error) {
	return false, nil
}

// splitPattern turns "POST /api/v1/x/{id}" into ("POST", "/api/v1/x/{id}").
func splitPattern(pattern string) (string, string) {
	if i := strings.IndexByte(pattern, ' '); i > 0 {
		return pattern[:i], pattern[i+1:]
	}
	return http.MethodGet, pattern
}

// concretePath replaces {wildcards} with a placeholder value so ServeMux matches.
func concretePath(path string) string {
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '{' {
			for i < len(path) && path[i] != '}' {
				i++
			}
			b.WriteString("x")
			continue
		}
		b.WriteByte(path[i])
	}
	return b.String()
}

var _ authorization.PermissionChecker = denyAllChecker{}
