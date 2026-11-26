// otplogin.go serves the browser login page that the authorize redirect points at.
//
// The OIDC flow needs a human: /oauth/authorize without a session answers
// 302 → /api/v1/auth/otp/login?continuation=…&redirect_uri=…&state=… . Before this
// handler existed that URL was a dead end, so no third-party client could complete the
// flow in a browser.
//
// The page is server-rendered and talks only to the identity unit, so the session
// token never appears in a URL: the POST verifies the OTP, resumes the authorize
// request with the fresh session, and returns the redirect to send the browser to.
package identity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"time"
)

var (
	// ErrContinuationInvalid covers a malformed, tampered or expired continuation.
	ErrContinuationInvalid = errors.New("oidc: invalid continuation")
)

// ResumeAuthorize decodes a continuation token (HMAC-verified, unexpired) and re-runs
// Authorize with the caller's session, so the flow continues where it stopped.
func (s *OIDCService) ResumeAuthorize(ctx context.Context, continuation, bearer string) (AuthorizeResult, error) {
	req, err := s.parseContinuation(continuation)
	if err != nil {
		return AuthorizeResult{}, err
	}
	return s.Authorize(ctx, req, bearer)
}

func (s *OIDCService) parseContinuation(token string) (AuthorizeRequest, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	if !hmacEqual(sig, signContinuation(parts[0], s.key)) {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	var payload struct {
		AuthorizeRequest
		Scopes []string  `json:"scopes"`
		Exp    time.Time `json:"exp"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	if s.now().After(payload.Exp) {
		return AuthorizeRequest{}, ErrContinuationInvalid
	}
	return payload.AuthorizeRequest, nil
}

// otpLoginPage is the whole UI: one form, two steps, no framework.
const otpLoginPage = `<!doctype html>
<html lang="fa-IR" dir="rtl"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ورود — خواب‌آور</title>
<style>
 body{font-family:system-ui,sans-serif;background:#0f1115;color:#e8eaed;display:grid;place-items:center;min-height:100vh;margin:0}
 form{background:#171a21;padding:2rem;border-radius:12px;width:min(92vw,380px);display:grid;gap:.75rem}
 h1{font-size:1.15rem;margin:0 0 .5rem}
 input,button{font:inherit;padding:.7rem;border-radius:8px;border:1px solid #2a2f3a;background:#0f1115;color:inherit}
 button{background:#4f7cff;border:0;cursor:pointer}
 .err{color:#ff8a80;font-size:.85rem;min-height:1.2em}
 .hint{color:#9aa0a6;font-size:.8rem}
</style></head><body>
<form id="f">
  <h1>ورود به خواب‌آور</h1>
  <div class="hint">کد یک‌بارمصرف به شمارهٔ همراه ارسال می‌شود.</div>
  <input id="phone" name="phone" placeholder="۰۹xxxxxxxxx" inputmode="numeric" autocomplete="tel" required>
  <button type="button" id="send">ارسال کد</button>
  <input id="code" name="code" placeholder="کد ۶ رقمی" inputmode="numeric" autocomplete="one-time-code">
  <button type="submit">ورود</button>
  <div class="err" id="err"></div>
</form>
<script>
const continuation = {{.Continuation}};
const err = document.getElementById('err');
document.getElementById('send').onclick = async () => {
  err.textContent = '';
  const phone = document.getElementById('phone').value.trim();
  if (!phone) { err.textContent = 'شمارهٔ همراه لازم است.'; return; }
  const r = await fetch('/api/v1/auth/otp/start', {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({phone})
  });
  err.textContent = r.ok ? 'کد ارسال شد.' : 'ارسال کد ناموفق بود.';
};
document.getElementById('f').onsubmit = async (e) => {
  e.preventDefault();
  err.textContent = '';
  const r = await fetch('/api/v1/auth/otp/login', {
    method: 'POST', headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      continuation,
      phone: document.getElementById('phone').value.trim(),
      code: document.getElementById('code').value.trim()
    })
  });
  const body = await r.json().catch(() => ({}));
  if (r.ok && body.redirect_url) { window.location.href = body.redirect_url; return; }
  err.textContent = body.error || 'ورود ناموفق بود.';
};
</script></body></html>`

// OTPLoginPageHandler serves GET /api/v1/auth/otp/login.
func OTPLoginPageHandler() http.HandlerFunc {
	tmpl := template.Must(template.New("login").Parse(otpLoginPage))
	return func(w http.ResponseWriter, r *http.Request) {
		cont := r.URL.Query().Get("continuation")
		if cont == "" {
			http.Error(w, "continuation is required", http.StatusBadRequest)
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

// strconvQuote renders the token as a safe JS string literal (JSON encoding).
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type otpLoginRequest struct {
	Continuation string `json:"continuation"`
	Phone        string `json:"phone"`
	Code         string `json:"code"`
}

// OTPLoginSubmitHandler serves POST /api/v1/auth/otp/login: verify the code, issue a
// session, resume the authorize request and return the redirect for the browser.
func OTPLoginSubmitHandler(svc *Service, oidc *OIDCService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req otpLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Continuation == "" || req.Phone == "" || req.Code == "" {
			writeErr(w, http.StatusBadRequest, "continuation, phone and code are required")
			return
		}
		customerID, err := svc.OTPVerify(r.Context(), req.Phone, PurposeLogin, req.Code)
		if err != nil {
			writeVerifyErr(w, err)
			return
		}
		if customerID == "" {
			writeErr(w, http.StatusUnauthorized, "no customer")
			return
		}
		token, err := svc.IssueSession(r.Context(), customerID, "shop", clientIP(r), r.UserAgent())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "session failed")
			return
		}
		res, err := oidc.ResumeAuthorize(r.Context(), req.Continuation, token)
		if err != nil {
			writeOIDCErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"redirect_url": res.RedirectURL})
	}
}
