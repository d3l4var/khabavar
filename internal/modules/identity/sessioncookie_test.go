package identity

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSessionCookieFlags pins the CSRF/transport posture: HttpOnly, SameSite=Lax, path
// scoped to /oauth, and Secure whenever the browser used TLS — including when TLS was
// terminated by the reverse proxy (X-Forwarded-Proto), which is the production case.
func TestSessionCookieFlags(t *testing.T) {
	cases := []struct {
		name   string
		header string
		secure bool
	}{
		{"direct TLS", "", true},
		{"behind nginx https", "https", true},
		{"plain http", "http", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/auth/admin/login", nil)
			if tc.name == "direct TLS" {
				r.TLS = &tls.ConnectionState{}
			}
			if tc.header != "" {
				r.Header.Set("X-Forwarded-Proto", tc.header)
			}
			w := httptest.NewRecorder()
			SetSessionCookie(w, r, "tok", time.Hour)

			c := w.Result().Cookies()[0]
			if c.Name != SessionCookieName {
				t.Fatalf("name = %q", c.Name)
			}
			if !c.HttpOnly {
				t.Fatal("the session cookie must be HttpOnly")
			}
			if c.SameSite != 2 { // http.SameSiteLaxMode
				t.Fatalf("SameSite = %v, want Lax", c.SameSite)
			}
			if c.Path != sessionCookiePath {
				t.Fatalf("path = %q, want %q (kept away from /api/v1/admin)", c.Path, sessionCookiePath)
			}
			if c.Secure != tc.secure {
				t.Fatalf("secure = %v, want %v", c.Secure, tc.secure)
			}
		})
	}
}

// TestBearerFromCookieIgnoresNonGET: a cookie must never authenticate a state-changing
// method, so a cross-site POST cannot ride on it.
func TestBearerFromCookieIgnoresNonGET(t *testing.T) {
	r := httptest.NewRequest("POST", "/oauth/authorize", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tok"})
	if got := bearerFromCookie(r); got != "" {
		t.Fatalf("bearerFromCookie on POST = %q, want empty", got)
	}
	r = httptest.NewRequest("GET", "/oauth/authorize", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tok"})
	if got := bearerFromCookie(r); got != "tok" {
		t.Fatalf("bearerFromCookie on GET = %q, want tok", got)
	}
}
