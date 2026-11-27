package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeOIDCStore serves one staff client and records issued codes.
type fakeOIDCStore struct {
	client *OIDCClient
	codes  []AuthorizationCode
}

func (f *fakeOIDCStore) GetClient(_ context.Context, id string) (*OIDCClient, error) {
	if f.client != nil && f.client.ClientID == id {
		return f.client, nil
	}
	return nil, nil
}

func (f *fakeOIDCStore) CreateAuthorizationCode(_ context.Context, c AuthorizationCode) error {
	f.codes = append(f.codes, c)
	return nil
}

func (f *fakeOIDCStore) ConsumeAuthorizationCode(_ context.Context, _ string) (*AuthorizationCode, error) {
	return nil, ErrOIDCInvalidCode
}

func (f *fakeOIDCStore) CreateSession(context.Context, string, string, string, string, string, string, string, time.Time) error {
	return nil
}

func (f *fakeOIDCStore) CreateRefreshToken(context.Context, RefreshToken) error { return nil }

func (f *fakeOIDCStore) ConsumeRefreshToken(context.Context, string) (*RefreshToken, error) {
	return nil, ErrOIDCInvalidRefresh
}

func newTestOIDC(t *testing.T, staff bool) (*OIDCService, *fakeOIDCStore) {
	t.Helper()
	scopes := []string{"openid", "customer.commerce"}
	code := "shop"
	if staff {
		scopes = []string{"openid", "staff"}
		code = "admin"
	}
	st := &fakeOIDCStore{client: &OIDCClient{
		ClientID: "c1", ServiceCode: code, Name: "t", IsActive: true,
		RedirectURIs: []string{"http://127.0.0.1:5180/auth/callback"}, Scopes: scopes,
	}}
	// A session resolves to a subject; an empty bearer does not.
	resolve := func(_ context.Context, bearer string) (Subject, bool) {
		if bearer == "" {
			return Subject{}, false
		}
		return Subject{ID: "admin-1", Kind: "admin"}, true
	}
	svc := NewOIDCService(st, resolve, "test-key-0123456789", "http://localhost:8099",
		5*time.Minute, time.Hour, 24*time.Hour)
	return svc, st
}

func testAuthorizeRequest() AuthorizeRequest {
	return AuthorizeRequest{
		ClientID:            "c1",
		RedirectURI:         "http://127.0.0.1:5180/auth/callback",
		State:               "st1",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
		ResponseType:        "code",
		Scope:               "openid",
	}
}

// TestContinuationRoundTrip covers the token that carries an authorize request across the login
// page: mint → resume, and that a resumed flow with a session issues a code.
func TestContinuationRoundTrip(t *testing.T) {
	svc, st := newTestOIDC(t, true)
	scopes, err := normalizeScopes("openid", st.client.Scopes)
	if err != nil {
		t.Fatalf("scopes: %v", err)
	}
	tok, err := svc.mintContinuation(testAuthorizeRequest(), scopes)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if !strings.Contains(tok, ".") {
		t.Fatalf("token is not payload.signature: %q", tok)
	}

	// Without a session it still requires login, and the redirect is the staff page.
	res, err := svc.ResumeAuthorize(context.Background(), tok, "")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !res.RequiresLogin {
		t.Fatal("resume without a session must require login")
	}
	if !strings.HasPrefix(res.RedirectURL, "/api/v1/auth/admin/login?") {
		t.Fatalf("staff client resumed to %q", res.RedirectURL)
	}

	// With a session it issues a code and redirects to the client.
	res, err = svc.ResumeAuthorize(context.Background(), tok, "session-token")
	if err != nil {
		t.Fatalf("resume with session: %v", err)
	}
	if res.RequiresLogin || res.RedirectURL == "" {
		t.Fatalf("resume with session = %+v", res)
	}
	if !strings.Contains(res.RedirectURL, "code=") || !strings.Contains(res.RedirectURL, "state=st1") {
		t.Fatalf("redirect = %q", res.RedirectURL)
	}
	if len(st.codes) != 1 {
		t.Fatalf("issued %d codes, want 1", len(st.codes))
	}
}

// TestContinuationRejectsTampering covers the three ways a continuation must fail closed:
// a bad shape, a flipped signature, and a payload edited after signing.
func TestContinuationRejectsTampering(t *testing.T) {
	svc, st := newTestOIDC(t, true)
	scopes, _ := normalizeScopes("openid", st.client.Scopes)
	tok, err := svc.mintContinuation(testAuthorizeRequest(), scopes)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	parts := strings.Split(tok, ".")

	cases := map[string]string{
		"empty":          "",
		"no dot":         "abc",
		"too many parts": tok + ".extra",
		"bad base64":     "!!!.!!!",
		"flipped sig":    parts[0] + "." + strings.Repeat("A", len(parts[1])),
		"swapped body":   parts[1] + "." + parts[0],
	}
	for name, bad := range cases {
		if _, err := svc.ResumeAuthorize(context.Background(), bad, "session"); !errors.Is(err, ErrContinuationInvalid) {
			t.Fatalf("%s: err = %v, want ErrContinuationInvalid", name, err)
		}
	}

	// A token signed with a different key must not verify.
	other := NewOIDCService(st, nil, "another-key-9876543210", "http://localhost:8099",
		5*time.Minute, time.Hour, 24*time.Hour)
	foreign, err := other.mintContinuation(testAuthorizeRequest(), scopes)
	if err != nil {
		t.Fatalf("mint foreign: %v", err)
	}
	if _, err := svc.ResumeAuthorize(context.Background(), foreign, "session"); !errors.Is(err, ErrContinuationInvalid) {
		t.Fatalf("foreign key: err = %v, want ErrContinuationInvalid", err)
	}
}

// TestContinuationExpires covers the TTL: a continuation is a short-lived handoff, not a
// permanent credential.
func TestContinuationExpires(t *testing.T) {
	svc, st := newTestOIDC(t, true)
	scopes, _ := normalizeScopes("openid", st.client.Scopes)
	tok, err := svc.mintContinuation(testAuthorizeRequest(), scopes)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// Six minutes later, past the five-minute window.
	svc.SetNow(func() time.Time { return time.Now().Add(6 * time.Minute) })
	if _, err := svc.ResumeAuthorize(context.Background(), tok, "session"); !errors.Is(err, ErrContinuationInvalid) {
		t.Fatalf("expired: err = %v, want ErrContinuationInvalid", err)
	}
}

// TestAdminLoginSubmitHandler covers the two shapes the endpoint serves: a plain JSON login
// (no continuation) and the OIDC page flow (continuation → redirect_url). The page flow is what
// the panel uses, and it was 0% covered.
func TestAdminLoginSubmitHandler(t *testing.T) {
	oidcSvc, st := newTestOIDC(t, true)
	scopes, _ := normalizeScopes("openid", st.client.Scopes)
	cont, err := oidcSvc.mintContinuation(testAuthorizeRequest(), scopes)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	store := noAccountsStore{}
	svc := NewService(nil, "test-key-0123456789", time.Minute, time.Hour, 5, nil)
	h := AdminLoginSubmitHandler(svc, oidcSvc, store)

	post := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/admin/login", strings.NewReader(body))
		r.RemoteAddr = "7.7.7.7:1234"
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Malformed body is a 400 with a code.
	rec := post("not-json")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code"`) {
		t.Fatalf("malformed = %d: %s", rec.Code, rec.Body.String())
	}
	// A continuation with no credentials is a 400, not a 500.
	rec = post(`{"continuation":"` + cont + `"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("continuation without credentials = %d", rec.Code)
	}
	// Unknown account: a 401, and the continuation is not consumed.
	rec = post(`{"email":"nobody@x.ir","password":"p","continuation":"` + cont + `"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown account = %d, want 401", rec.Code)
	}
	// Without a continuation the plain JSON handler answers — same 401 here.
	rec = post(`{"email":"nobody@x.ir","password":"p"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("plain login = %d, want 401", rec.Code)
	}
}
