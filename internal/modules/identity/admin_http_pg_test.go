package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// TestPG_StaffLoginAndAdminReads covers the panel's auth surface: staff login (success,
// wrong password, unknown email, suspended account) and the three read endpoints the panel
// uses to render the RBAC matrix.
func TestPG_StaffLoginAndAdminReads(t *testing.T) {
	pool, _ := setupPG(t)
	ctx := context.Background()
	store := NewDBStore(pool)
	svc := NewService(store, "staff-signing-key-0123456789", 5*time.Minute, time.Hour, 5,
		ratelimit.New(100, time.Hour))
	adminStore := NewAdminStore(pool)
	loginStore := NewDBAdminLoginStore(pool)

	// A staff account with a real bcrypt hash.
	hash, err := bcrypt.GenerateFromPassword([]byte("Khabavar!Dev2026"), 12)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	email := "staff-" + uuid.NewString()[:8] + "@khabavar.ir"
	if _, err := pool.Exec(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status) VALUES ($1,'کارمند',$2,'ACTIVE')`,
		email, string(hash)); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	suspended := "susp-" + uuid.NewString()[:8] + "@khabavar.ir"
	if _, err := pool.Exec(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status) VALUES ($1,'معلق',$2,'SUSPENDED')`,
		suspended, string(hash)); err != nil {
		t.Fatalf("seed suspended: %v", err)
	}

	login := AdminLoginHandler(svc, loginStore)
	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/admin/login", strings.NewReader(body))
		rec := httptest.NewRecorder()
		login(rec, r)
		return rec
	}

	// Validation and credential failures.
	if rec := post(`{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := post(`{"email":"` + email + `"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password = %d, want 400", rec.Code)
	}
	if rec := post(`{"email":"` + email + `","password":"wrong-password"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want 401", rec.Code)
	}
	// An unknown email must look exactly like a wrong password (no user enumeration).
	if rec := post(`{"email":"nobody-` + uuid.NewString()[:8] + `@khabavar.ir","password":"whatever"}`); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown email = %d, want 401", rec.Code)
	}
	// A suspended account is refused even with the right password.
	if rec := post(`{"email":"` + suspended + `","password":"Khabavar!Dev2026"}`); rec.Code < 400 {
		t.Fatalf("suspended account = %d, want a 4xx", rec.Code)
	}

	// The happy path issues a token that resolves.
	rec := post(`{"email":"` + email + `","password":"Khabavar!Dev2026"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.AccessToken == "" {
		t.Fatalf("login response = %s", rec.Body.String())
	}
	resolver := DBSubjectResolver(pool, "staff-signing-key-0123456789")
	if subj, ok := resolver(ctx, out.AccessToken); !ok || subj.ID == "" {
		t.Fatalf("staff token did not resolve: %+v/%v", subj, ok)
	}

	// The panel's three read endpoints.
	get := func(h http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/x?page=1&page_size=50", nil)
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}
	rec = get(AdminUsersHandler(adminStore))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), email) {
		t.Fatalf("admin users = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get(RolesHandler(adminStore)); rec.Code != http.StatusOK {
		t.Fatalf("roles = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get(PermissionsHandler(adminStore)); rec.Code != http.StatusOK {
		t.Fatalf("permissions = %d: %s", rec.Code, rec.Body.String())
	}
}
