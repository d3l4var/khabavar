package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// TestPG_OTPLoginPage covers the HTML login page: the continuation is mandatory and it is
// rendered as data, not markup. otplogin.go was at 0%.
func TestPG_OTPLoginPage(t *testing.T) {
	h := OTPLoginPageHandler()

	// Without a continuation the page refuses rather than rendering a dead form.
	r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no continuation = %d, want 400", rec.Code)
	}

	// With one, the page renders and carries the token.
	r = httptest.NewRequest(http.MethodGet, "/auth/login?continuation=abc123", nil)
	rec = httptest.NewRecorder()
	h(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("page = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", cc)
	}
	if !strings.Contains(rec.Body.String(), `"abc123"`) {
		t.Fatalf("the continuation is not in the page: %s", rec.Body.String())
	}

	// A token carrying markup must be escaped as a JS string, not injected as HTML.
	r = httptest.NewRequest(http.MethodGet, "/auth/login?continuation="+`"><script>alert(1)</script>`, nil)
	rec = httptest.NewRecorder()
	h(rec, r)
	body := rec.Body.String()
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatal("the continuation was injected as markup")
	}
	if !strings.Contains(body, `\u003cscript`) && !strings.Contains(body, `\u003Cscript`) {
		t.Fatalf("the continuation was not JSON-escaped: %s", body)
	}
}

// TestPG_OTPLoginSubmit covers the submit path's validation and its failure codes.
func TestPG_OTPLoginSubmit(t *testing.T) {
	pool, oidcStore := setupPG(t)
	_ = pool
	svc := NewService(NewDBStore(pool), "staff-signing-key-0123456789", 5*time.Minute,
		time.Hour, 5, ratelimit.New(100, time.Hour))
	oidc := NewOIDCService(oidcStore, nil, "server-key-0123456789", "https://khabavar.ir",
		5*time.Minute, time.Hour, 24*time.Hour)
	h := OTPLoginSubmitHandler(svc, oidc)

	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	if rec := post(`{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	// Each of the three fields is mandatory.
	for _, body := range []string{
		`{"phone":"09120000001","code":"123456"}`,
		`{"continuation":"c","code":"123456"}`,
		`{"continuation":"c","phone":"09120000001"}`,
	} {
		if rec := post(body); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing field %s = %d, want 400", body, rec.Code)
		}
	}
	// A wrong code is a 4xx, never a 500, and it does not leak whether the phone exists.
	rec := post(`{"continuation":"c","phone":"09120000009","code":"000000"}`)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("wrong code = %d, want a 4xx: %s", rec.Code, rec.Body.String())
	}
	_ = context.Background()
}
