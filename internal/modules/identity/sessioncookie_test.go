package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestSessionCookieFlags covers the cookie contract: HttpOnly always, Secure when the
// browser reached us over TLS (including behind a proxy), a path that keeps it away from
// the staff API, and a logout that clears it. sessioncookie.go was at 64%.
func TestSessionCookieFlags(t *testing.T) {
	// A plain-HTTP request: no Secure, but still HttpOnly.
	r := httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)
	rec := httptest.NewRecorder()
	SetSessionCookie(rec, r, "tok-1", time.Hour)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName || c.Value != "tok-1" {
		t.Fatalf("cookie = %s=%s", c.Name, c.Value)
	}
	if !c.HttpOnly {
		t.Fatal("the session cookie is not HttpOnly")
	}
	if c.Secure {
		t.Fatal("the cookie is Secure on a plain-HTTP request")
	}
	if c.Path != sessionCookiePath {
		t.Fatalf("path = %q, want %q", c.Path, sessionCookiePath)
	}
	if c.MaxAge != int(time.Hour.Seconds()) {
		t.Fatalf("max-age = %d", c.MaxAge)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("samesite = %v", c.SameSite)
	}

	// Behind a TLS-terminating proxy r.TLS is nil, so X-Forwarded-Proto must set Secure.
	r2 := httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)
	r2.Header.Set("X-Forwarded-Proto", "HTTPS")
	rec2 := httptest.NewRecorder()
	SetSessionCookie(rec2, r2, "tok-2", time.Hour)
	if !rec2.Result().Cookies()[0].Secure {
		t.Fatal("the cookie is not Secure behind an https proxy")
	}

	// A real TLS connection is Secure too.
	r3 := httptest.NewRequest(http.MethodGet, "https://khabavar.ir/oauth/authorize", nil)
	rec3 := httptest.NewRecorder()
	SetSessionCookie(rec3, r3, "tok-3", time.Hour)
	if !rec3.Result().Cookies()[0].Secure {
		t.Fatal("the cookie is not Secure over TLS")
	}

	// Logout clears it with the same path and an expiry in the past.
	rec4 := httptest.NewRecorder()
	ClearSessionCookie(rec4, r)
	cleared := rec4.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Value != "" || cleared[0].MaxAge >= 0 {
		t.Fatalf("cleared cookie = %+v", cleared)
	}
	if cleared[0].Path != sessionCookiePath {
		t.Fatalf("cleared path = %q", cleared[0].Path)
	}

	// The bearer is read back from the cookie, and an absent cookie is empty.
	if got := bearerFromCookie(r); got != "" {
		t.Fatalf("bearer without a cookie = %q", got)
	}
	withCookie := httptest.NewRequest(http.MethodGet, "/oauth/authorize", nil)
	withCookie.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "tok-9"})
	if got := bearerFromCookie(withCookie); got != "tok-9" {
		t.Fatalf("bearer = %q", got)
	}

	// Logout answers 204 and clears the cookie.
	rec5 := httptest.NewRecorder()
	LogoutHandler()(rec5, withCookie)
	if rec5.Code != http.StatusNoContent {
		t.Fatalf("logout = %d, want 204", rec5.Code)
	}
	if len(rec5.Result().Cookies()) != 1 {
		t.Fatal("logout did not clear the cookie")
	}
}
