package main

import (
	"log/slog"
	"net/http"

	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/modules/identity"
)

// mountIdentityRoutes is the KH_ROLE=identity route table. It lives in its own function so a
// test can prove it registers without a panic: a duplicate pattern panics at startup, and that
// is how the identity unit silently stopped booting (two registrations of
// POST /api/v1/auth/admin/login, one from before the OIDC page flow existed).
func mountIdentityRoutes(mux *http.ServeMux, svc *identity.Service, oidc *identity.OIDCService,
	store identity.AdminLoginStore, guard *identity.AdminLoginGuard, aud authorization.Auditor,
	logger *slog.Logger) {

	// One table, so the patterns and the registrations cannot drift.
	routes := []struct {
		pattern string
		h       http.HandlerFunc
	}{
		// Customer OTP login (SAD §۷; CIAM §۳-۵).
		{"POST /api/v1/auth/otp/start", identity.OTPStartHandler(svc, mockSmsDeliver(logger))},
		{"POST /api/v1/auth/otp/request", identity.OTPStartHandler(svc, mockSmsDeliver(logger))},
		{"POST /api/v1/auth/otp/verify", identity.OTPVerifyHandler(svc)},
		{"GET /api/v1/auth/otp/login", identity.OTPLoginPageHandler()},
		{"POST /api/v1/auth/otp/login", identity.OTPLoginSubmitHandler(svc, oidc)},
		// Staff login. One POST registration, not two: the submit handler serves both the
		// plain JSON login and the OIDC page flow.
		{"GET /api/v1/auth/admin/login", identity.AdminLoginPageHandler()},
		{"POST /api/v1/auth/admin/login", identity.AdminLoginSubmitHandler(svc, oidc, store,
			identity.WithAdminLoginGuard(guard, aud))},
		// OIDC Authorization Code + PKCE.
		{"GET /oauth/authorize", identity.AuthorizeHandler(oidc)},
		{"POST /oauth/token", identity.TokenHandler(oidc)},
	}
	for _, r := range routes {
		mux.HandleFunc(r.pattern, r.h)
	}
}
