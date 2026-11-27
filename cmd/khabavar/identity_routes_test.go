package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/identity"
)

// TestIdentityRoutesRegister is the test that would have caught the identity unit failing to
// boot: a duplicate pattern panics inside ServeMux.HandleFunc, and KH_ROLE=identity died at
// startup because POST /api/v1/auth/admin/login was registered twice.
func TestIdentityRoutesRegister(t *testing.T) {
	mux := http.NewServeMux()
	svc := identity.NewService(nil, "test-key-0123456789", time.Minute, time.Hour, 5, nil)
	oidc := identity.NewOIDCService(nil, nil, "test-key-0123456789", "http://localhost", time.Minute, time.Hour, time.Hour)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("identity routes panic on registration: %v", r)
		}
	}()
	mountIdentityRoutes(mux, svc, oidc, nil, identity.NewAdminLoginGuard(10, 5, time.Minute), nil, identity.MockSMS{}, nil)
}
