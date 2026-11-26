// adminlogin.go implements staff authentication for the panel.
//
// The panel is a first-party client, so it authenticates against the identity unit
// directly (email + password) and receives an opaque session token; the OIDC code
// exchange stays available for third-party clients. Passwords are bcrypt (cost 12) —
// admin_users.password_hash existed for exactly this and was unused.
//
// A failed login answers the same 401 for "unknown email" and "wrong password", and
// always runs a bcrypt comparison (against a dummy hash when the email is unknown) so
// response time does not reveal which emails exist.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrAdminCredentials is returned for any failed staff login.
	ErrAdminCredentials = errors.New("identity: invalid credentials")
	// ErrAdminInactive is returned when the account is not ACTIVE.
	ErrAdminInactive = errors.New("identity: admin account is not active")
)

// AdminCredential is the row needed to authenticate a staff user.
type AdminCredential struct {
	ID           string
	Email        string
	PasswordHash string
	Status       string
}

// AdminLoginStore looks up a staff user by email.
type AdminLoginStore interface {
	FindAdminByEmail(ctx context.Context, email string) (*AdminCredential, error)
}

// dummyHash is compared when the email is unknown, so a missing account costs the same
// time as a wrong password.
var dummyHash = []byte("$2a$12$C6UzMDM.H6dfI/f/IKcEe.7Zl9O0T0v0S9r0T0v0S9r0T0v0S9r0T")

// AdminLogin verifies the password and issues a session bound to admin_user_id.
func (s *Service) AdminLogin(ctx context.Context, store AdminLoginStore, email, password, ip, ua string) (string, error) {
	cred, err := store.FindAdminByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return "", err
	}
	if cred == nil {
		// Constant-ish work for unknown emails (no user enumeration).
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return "", ErrAdminCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)); err != nil {
		return "", ErrAdminCredentials
	}
	if cred.Status != "ACTIVE" {
		return "", ErrAdminInactive
	}
	return s.IssueAdminSession(ctx, cred.ID, "admin", ip, ua)
}

type adminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AdminLoginHandler serves POST /api/v1/auth/admin/login.
func AdminLoginHandler(svc *Service, store AdminLoginStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req adminLoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Email == "" || req.Password == "" {
			writeErr(w, http.StatusBadRequest, "email and password required")
			return
		}
		token, err := svc.AdminLogin(r.Context(), store, req.Email, req.Password, clientIP(r), r.UserAgent())
		switch {
		case errors.Is(err, ErrAdminCredentials):
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		case errors.Is(err, ErrAdminInactive):
			writeErr(w, http.StatusForbidden, "account is not active")
			return
		case err != nil:
			writeErr(w, http.StatusInternalServerError, "login failed")
			return
		}
		// The same session goes into an HttpOnly /oauth cookie so the browser can run the
		// authorize redirect without putting a token in a URL or in script reach.
		SetSessionCookie(w, r, token, svc.accessTTL)
		writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "service": "admin"})
	}
}
