package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_AddsAllHeaders(t *testing.T) {
	h := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	cases := map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Cache-Control":             "no-store",
	}
	for header, want := range cases {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("%s=%q want %q", header, got, want)
		}
	}
}

func TestCORS_AllowsAllowedOrigin(t *testing.T) {
	cfg := DefaultCORSConfig()
	h := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://shop.khabavar.ir")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://shop.khabavar.ir" {
		t.Fatalf("ACAO=%q want https://shop.khabavar.ir", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("ACAC=%q want true", got)
	}
}

func TestCORS_RejectsUnknownOrigin(t *testing.T) {
	cfg := DefaultCORSConfig()
	h := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("ACAO=%q want empty (origin not allowed)", got)
	}
}

func TestCORS_PreflightShortCircuits(t *testing.T) {
	cfg := DefaultCORSConfig()
	called := false
	h := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "https://shop.khabavar.ir")
	h.ServeHTTP(rec, req)
	if called {
		t.Fatal("preflight should short-circuit, not call next handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d want 204", rec.Code)
	}
}

func TestChain_ComposesMiddlewares(t *testing.T) {
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-MW", "1")
			next.ServeHTTP(w, r)
		})
	}
	mw2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-MW", "2")
			next.ServeHTTP(w, r)
		})
	}
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), SecurityHeaders, mw1, mw2)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	// Both middlewares should have run (X-MW set twice).
	if len(rec.Header().Values("X-MW")) != 2 {
		t.Fatalf("X-MW count=%d want 2", len(rec.Header().Values("X-MW")))
	}
	// Security headers should also be present.
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing after Chain")
	}
}
