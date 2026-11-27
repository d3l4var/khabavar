package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBuildLoginRedirectPicksTheRightPage pins the split: the authorize endpoint sent every
// client to the customer OTP page, which a staff member can never satisfy.
func TestBuildLoginRedirectPicksTheRightPage(t *testing.T) {
	staff := &OIDCClient{ServiceCode: "admin", Scopes: []string{"openid", "staff"}}
	customer := &OIDCClient{ServiceCode: "shop", Scopes: []string{"openid", "customer.commerce"}}

	if got := buildLoginRedirect(staff, "http://x/cb", "s", "c"); !strings.HasPrefix(got, "/api/v1/auth/admin/login?") {
		t.Fatalf("staff client redirected to %q, want the staff page", got)
	}
	if got := buildLoginRedirect(customer, "http://x/cb", "s", "c"); !strings.HasPrefix(got, "/api/v1/auth/otp/login?") {
		t.Fatalf("customer client redirected to %q, want the OTP page", got)
	}
	// A staff scope alone is enough, even without service_code=admin.
	if got := buildLoginRedirect(&OIDCClient{ServiceCode: "x", Scopes: []string{"staff"}}, "u", "s", "c"); !strings.HasPrefix(got, "/api/v1/auth/admin/login?") {
		t.Fatalf("staff scope redirected to %q", got)
	}
}

// TestAdminLoginPageRequiresContinuation covers the page's guard and that it renders the token
// as data, not markup.
func TestAdminLoginPageRequiresContinuation(t *testing.T) {
	h := AdminLoginPageHandler()

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/admin/login", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("no continuation = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/admin/login?continuation=abc", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("with continuation = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"abc"`) {
		t.Fatalf("continuation not rendered as a JS string literal")
	}
	// A token carrying markup must not become markup.
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, `/api/v1/auth/admin/login?continuation=<script>alert(1)</script>`, nil))
	if strings.Contains(rec.Body.String(), "<script>alert(1)</script>") {
		t.Fatal("continuation was rendered as markup: XSS")
	}
}
