// sessioncookie.go gives the browser a way to authenticate /oauth/authorize.
//
// Why a cookie is needed: the authorize endpoint is reached by a top-level navigation
// (the whole point of the redirect flow), and a navigation cannot carry an
// Authorization header. fetch(..., {redirect:'manual'}) is not an alternative — the
// response is opaque, so the page cannot read the Location header either.
//
// CSRF posture (the cookie is only as safe as its guard):
//   - HttpOnly: script cannot read or exfiltrate it.
//   - SameSite=Lax: a cross-site POST never carries it; a top-level GET does, which is
//     exactly the authorize navigation we need.
//   - Path=/oauth: the cookie is not sent to /api/v1/admin/*, so a leaked cookie cannot
//     be replayed against the staff API.
//   - The cookie is accepted ONLY on GET /oauth/authorize, and only when the request
//     carries a PKCE challenge and a state — an attacker who lures a user to authorize
//     still cannot exchange the resulting code without the verifier.
package identity

import (
	"net/http"
	"strings"
	"time"
)

const (
	// SessionCookieName is the browser session cookie.
	SessionCookieName = "kh_session"
	// sessionCookiePath keeps the cookie away from the staff API surface.
	sessionCookiePath = "/oauth"
)

// requestIsSecure reports whether the browser reached us over TLS. Behind the reverse
// proxy r.TLS is nil (nginx terminates TLS), so X-Forwarded-Proto decides — without it
// the cookie would be issued without Secure in production.
func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// SetSessionCookie writes the session cookie (HttpOnly, SameSite=Lax, scoped to /oauth).
func SetSessionCookie(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     sessionCookiePath,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

// ClearSessionCookie expires the session cookie.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     sessionCookiePath,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// bearerFromCookie returns the session token when the request is a plain GET with the
// cookie present. Anything else (a POST, a cookie-less request) yields "".
func bearerFromCookie(r *http.Request) string {
	if r.Method != http.MethodGet {
		return ""
	}
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return ""
	}
	return strings.TrimSpace(c.Value)
}
