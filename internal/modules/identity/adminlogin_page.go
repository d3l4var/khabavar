package identity

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strings"
)

// The authorize endpoint sent every client to the customer OTP page. For the staff client that
// page cannot authenticate anyone: a staff member has an email and a password, not a phone.
// The panel's OIDC login therefore ended on a form that could never succeed.
//
// This adds the staff half: a page, and a submit that verifies the password, issues an admin
// session and resumes the authorize request the continuation token carries.

const adminLoginPage = `<!doctype html><html lang="fa" dir="rtl"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ورود کارکنان — خبرور</title>
<style>
body{font-family:system-ui,sans-serif;background:#0f1115;color:#e7e9ee;display:grid;place-items:center;min-height:100vh;margin:0}
form{background:#171a21;padding:2rem;border-radius:12px;width:min(360px,90vw);display:grid;gap:.75rem}
h1{font-size:1.1rem;margin:0 0 .5rem}
label{font-size:.85rem;color:#9aa3b2}
input{padding:.7rem;border-radius:8px;border:1px solid #2b303b;background:#0f1115;color:#e7e9ee;font:inherit}
button{padding:.75rem;border-radius:8px;border:0;background:#3b82f6;color:#fff;font:inherit;cursor:pointer}
p{margin:0;font-size:.85rem;color:#f87171;min-height:1.2em}
</style></head><body>
<form id="f"><h1>ورود کارکنان</h1>
<label for="email">ایمیل</label><input id="email" name="email" type="email" autocomplete="username" required>
<label for="password">گذرواژه</label><input id="password" name="password" type="password" autocomplete="current-password" required>
<button type="submit">ورود</button><p id="err"></p></form>
<script>
const CONT = {{.Continuation}};
document.getElementById('f').addEventListener('submit', async (e) => {
  e.preventDefault();
  const err = document.getElementById('err');
  err.textContent = '';
  const res = await fetch('/api/v1/auth/admin/login', {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      email: document.getElementById('email').value,
      password: document.getElementById('password').value,
      continuation: CONT,
    }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) { err.textContent = body.error || 'ورود ناموفق بود.'; return; }
  if (body.redirect_url) { window.location.href = body.redirect_url; return; }
  err.textContent = 'پاسخ سرور ناقص بود.';
});
</script></body></html>`

// AdminLoginPageHandler serves GET /api/v1/auth/admin/login — the staff form the authorize
// redirect points at.
func AdminLoginPageHandler() http.HandlerFunc {
	tmpl := template.Must(template.New("adminlogin").Parse(adminLoginPage))
	return func(w http.ResponseWriter, r *http.Request) {
		cont := r.URL.Query().Get("continuation")
		if cont == "" {
			writeErr(w, http.StatusBadRequest, "continuation is required")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = tmpl.Execute(w, map[string]any{
			// The token is data, not markup: template.JS would allow injection.
			"Continuation": template.JS(strconvQuote(cont)),
		})
	}
}

type adminLoginPageRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Continuation string `json:"continuation"`
}

// AdminLoginSubmitHandler serves POST /api/v1/auth/admin/login when the body carries a
// continuation token — that is the OIDC page flow, which must answer with a redirect_url
// instead of a bare token. Without a continuation it falls through to the JSON login.
func AdminLoginSubmitHandler(svc *Service, oidc *OIDCService, store AdminLoginStore, opts ...AdminLoginOption) http.HandlerFunc {
	plain := AdminLoginHandler(svc, store, opts...)
	return func(w http.ResponseWriter, r *http.Request) {
		var peek adminLoginPageRequest
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := json.Unmarshal(body, &peek); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if peek.Continuation == "" {
			// Not the OIDC flow: hand the buffered body back to the JSON handler.
			r.Body = io.NopCloser(bytes.NewReader(body))
			plain(w, r)
			return
		}
		if peek.Email == "" || peek.Password == "" {
			writeErr(w, http.StatusBadRequest, "email and password required")
			return
		}
		token, err := svc.AdminLogin(r.Context(), store, peek.Email, peek.Password, clientIP(r), r.UserAgent())
		switch {
		case err != nil:
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		res, err := oidc.ResumeAuthorize(r.Context(), peek.Continuation, token)
		if err != nil {
			writeOIDCErr(w, err)
			return
		}
		SetSessionCookie(w, r, token, svc.accessTTL)
		writeJSON(w, http.StatusOK, map[string]any{"redirect_url": res.RedirectURL})
	}
}

// isStaffClient reports whether a client authenticates staff (email + password) rather than
// customers (phone + OTP).
func isStaffClient(scopes []string, serviceCode string) bool {
	if strings.EqualFold(serviceCode, "admin") {
		return true
	}
	for _, s := range scopes {
		if strings.EqualFold(s, "staff") {
			return true
		}
	}
	return false
}
