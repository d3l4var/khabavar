package identity

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// sha256Sum returns the raw SHA-256 of s (test helper for PKCE challenge derivation).
func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// base64RawURL returns the unpadded base64url encoding of b.
func base64RawURL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// memOIDCStore is an in-memory OIDCStore for unit tests. It enforces the same
// "load + consume atomically" semantics as the DB implementation (FOR UPDATE).
type memOIDCStore struct {
	mu       sync.Mutex
	clients  map[string]*OIDCClient
	codes    map[string]*AuthorizationCode // keyed by code_hash
	refresh  map[string]*RefreshToken      // keyed by token_hash
	sessions []memSession
}

type memSession struct {
	id, customerID, adminUserID, serviceCode, tokenHash, ip, ua string
	expiresAt                                                    time.Time
}

func newMemOIDCStore() *memOIDCStore {
	return &memOIDCStore{clients: map[string]*OIDCClient{}, codes: map[string]*AuthorizationCode{}, refresh: map[string]*RefreshToken{}}
}

func (m *memOIDCStore) GetClient(_ context.Context, clientID string) (*OIDCClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.clients[clientID]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (m *memOIDCStore) CreateAuthorizationCode(_ context.Context, code AuthorizationCode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := code
	m.codes[code.CodeHash] = &cp
	return nil
}

func (m *memOIDCStore) ConsumeAuthorizationCode(_ context.Context, codeHash string) (*AuthorizationCode, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.codes[codeHash]
	if !ok || c.ConsumedAt != nil {
		return nil, ErrOIDCInvalidCode
	}
	if !time.Now().Before(c.ExpiresAt) {
		return nil, ErrOIDCInvalidCode
	}
	now := time.Now()
	c.ConsumedAt = &now
	cp := *c
	return &cp, nil
}

func (m *memOIDCStore) CreateSession(_ context.Context, sessID, customerID, adminUserID, serviceCode, tokenHash, ip, ua string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = append(m.sessions, memSession{sessID, customerID, adminUserID, serviceCode, tokenHash, ip, ua, expiresAt})
	return nil
}

func (m *memOIDCStore) CreateRefreshToken(_ context.Context, rt RefreshToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := rt
	m.refresh[rt.TokenHash] = &cp
	return nil
}

func (m *memOIDCStore) ConsumeRefreshToken(_ context.Context, tokenHash string) (*RefreshToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rt, ok := m.refresh[tokenHash]
	if !ok || rt.ConsumedAt != nil || rt.RevokedAt != nil {
		return nil, ErrOIDCInvalidRefresh
	}
	if !time.Now().Before(rt.ExpiresAt) {
		return nil, ErrOIDCInvalidRefresh
	}
	now := time.Now()
	rt.ConsumedAt = &now
	cp := *rt
	return &cp, nil
}

// ── test fixtures ──────────────────────────────────────────────────────────────

func testOIDCService(t *testing.T, resolve SubjectResolver) (*OIDCService, *memOIDCStore) {
	t.Helper()
	store := newMemOIDCStore()
	store.clients["shop"] = &OIDCClient{
		ClientID: "shop", ServiceCode: "shop", Name: "KHABAVAR Store",
		RedirectURIs: []string{"https://shop.khabavar.ir/auth/cb"},
		Scopes:       []string{"openid", "profile", "customer.basic", "customer.commerce"},
		IsActive:     true,
	}
	svc := NewOIDCService(store, resolve, "test-server-key", "https://id.khabavar.ir",
		10*time.Minute, 15*time.Minute, 30*24*time.Hour)
	return svc, store
}

// pkcePair returns a verifier and its S256 challenge.
func pkcePair(t *testing.T) (verifier, challenge string) {
	t.Helper()
	verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	sum := sha256Sum(verifier)
	challenge = base64RawURL(sum)
	return
}

// validAuthorizeReq builds a fully valid /authorize request for the shop client.
func validAuthorizeReq(verifier, challenge string) AuthorizeRequest {
	return AuthorizeRequest{
		ClientID:            "shop",
		RedirectURI:         "https://shop.khabavar.ir/auth/cb",
		ResponseType:        "code",
		Scope:               "openid profile customer.commerce",
		State:               "xyz123",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
	}
}

// ── PKCE ───────────────────────────────────────────────────────────────────────

func TestVerifyPKCE_S256(t *testing.T) {
	v, c := pkcePair(t)
	if !VerifyPKCE(v, c, "S256") {
		t.Fatal("valid PKCE pair rejected")
	}
	if VerifyPKCE("wrong-verifier", c, "S256") {
		t.Fatal("wrong verifier accepted")
	}
	if VerifyPKCE(v, c, "plain") {
		t.Fatal("non-S256 method accepted")
	}
	if VerifyPKCE("", "", "S256") {
		t.Fatal("empty inputs accepted")
	}
}

// ── Authorize ──────────────────────────────────────────────────────────────────

func TestAuthorize_MintsCode_WhenSubjectPresent(t *testing.T) {
	svc, store := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	v, c := pkcePair(t)
	res, err := svc.Authorize(context.Background(), validAuthorizeReq(v, c), "bearer-abc")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if res.RequiresLogin {
		t.Fatal("expected code redirect, got login redirect")
	}
	if !strings.HasPrefix(res.RedirectURL, "https://shop.khabavar.ir/auth/cb") {
		t.Fatalf("unexpected redirect: %s", res.RedirectURL)
	}
	if !strings.Contains(res.RedirectURL, "state=xyz123") {
		t.Fatal("state not echoed back")
	}
	if !strings.Contains(res.RedirectURL, "code=") {
		t.Fatal("code missing from redirect")
	}
	if len(store.codes) != 1 {
		t.Fatalf("expected 1 stored code, got %d", len(store.codes))
	}
}

func TestAuthorize_RequiresLogin_WhenNoSession(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{}, false
	})
	v, c := pkcePair(t)
	res, err := svc.Authorize(context.Background(), validAuthorizeReq(v, c), "")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if !res.RequiresLogin {
		t.Fatal("expected login redirect, got code redirect")
	}
	if res.ContinuationToken == "" {
		t.Fatal("continuation token missing")
	}
	if !strings.HasPrefix(res.RedirectURL, "/api/v1/auth/otp/login") {
		t.Fatalf("expected login redirect, got %s", res.RedirectURL)
	}
}

func TestAuthorize_RejectsInvalidClient(t *testing.T) {
	svc, _ := testOIDCService(t, nil)
	v, c := pkcePair(t)
	req := validAuthorizeReq(v, c)
	req.ClientID = "unknown"
	_, err := svc.Authorize(context.Background(), req, "bearer")
	if !errors.Is(err, ErrOIDCInvalidClient) {
		t.Fatalf("expected ErrOIDCInvalidClient, got %v", err)
	}
}

func TestAuthorize_RejectsRedirectNotInWhitelist(t *testing.T) {
	svc, _ := testOIDCService(t, nil)
	v, c := pkcePair(t)
	req := validAuthorizeReq(v, c)
	req.RedirectURI = "https://evil.example/cb"
	_, err := svc.Authorize(context.Background(), req, "bearer")
	if !errors.Is(err, ErrOIDCInvalidRedirect) {
		t.Fatalf("expected ErrOIDCInvalidRedirect, got %v", err)
	}
}

func TestAuthorize_RejectsMissingPKCE(t *testing.T) {
	svc, _ := testOIDCService(t, nil)
	req := validAuthorizeReq("", "")
	req.CodeChallengeMethod = ""
	_, err := svc.Authorize(context.Background(), req, "bearer")
	if !errors.Is(err, ErrOIDCInvalidRequest) {
		t.Fatalf("expected ErrOIDCInvalidRequest, got %v", err)
	}
}

func TestAuthorize_RejectsScopeNotPermittedForClient(t *testing.T) {
	svc, _ := testOIDCService(t, nil)
	v, c := pkcePair(t)
	req := validAuthorizeReq(v, c)
	req.Scope = "openid staff" // staff not permitted for the shop client
	_, err := svc.Authorize(context.Background(), req, "bearer")
	if !errors.Is(err, ErrOIDCInvalidScope) {
		t.Fatalf("expected ErrOIDCInvalidScope, got %v", err)
	}
}

// ── Token (authorization_code grant) ───────────────────────────────────────────

// runAuthorize extracts the raw code AND the PKCE verifier the service minted, by
// replaying the authorize call with a subject resolver and reading the `code` query
// param off the redirect. The returned verifier matches the code's challenge, so
// callers can pass it straight to Token as code_verifier.
func runAuthorize(t *testing.T, svc *OIDCService) (rawCode, verifier string) {
	t.Helper()
	v, c := pkcePair(t)
	res, err := svc.Authorize(context.Background(), validAuthorizeReq(v, c), "bearer-abc")
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	u, err := url.Parse(res.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	return u.Query().Get("code"), v
}

func TestToken_AuthorizationCodeGrant_IssuesTokensAndIDToken(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	rawCode, v := runAuthorize(t, svc)

	out, err := svc.Token(context.Background(), TokenRequest{
		GrantType: "authorization_code", ClientID: "shop",
		Code: rawCode, RedirectURI: "https://shop.khabavar.ir/auth/cb",
		CodeVerifier: v,
	})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if out.TokenType != "Bearer" {
		t.Fatalf("token_type=%s", out.TokenType)
	}
	if out.AccessToken == "" || out.RefreshToken == "" {
		t.Fatal("missing access/refresh token")
	}
	if out.IDToken == "" {
		t.Fatal("id_token missing despite openid scope")
	}
	if !strings.Contains(out.Scope, "openid") {
		t.Fatalf("scope=%s", out.Scope)
	}
	if out.ExpiresIn != 900 {
		t.Fatalf("expires_in=%d want 900", out.ExpiresIn)
	}
	// id_token must be a 3-part compact JWS.
	if strings.Count(out.IDToken, ".") != 2 {
		t.Fatalf("id_token not a compact JWS: %s", out.IDToken)
	}
}

func TestToken_AuthorizationCodeGrant_RejectsWrongPKCE(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	rawCode, _ := runAuthorize(t, svc)
	_, err := svc.Token(context.Background(), TokenRequest{
		GrantType: "authorization_code", ClientID: "shop",
		Code: rawCode, RedirectURI: "https://shop.khabavar.ir/auth/cb",
		CodeVerifier: "wrong-verifier",
	})
	if !errors.Is(err, ErrOIDCInvalidPKCE) {
		t.Fatalf("expected ErrOIDCInvalidPKCE, got %v", err)
	}
}

func TestToken_AuthorizationCodeGrant_CodeIsOneTimeUse(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	rawCode, v := runAuthorize(t, svc)

	req := TokenRequest{
		GrantType: "authorization_code", ClientID: "shop",
		Code: rawCode, RedirectURI: "https://shop.khabavar.ir/auth/cb",
		CodeVerifier: v,
	}
	if _, err := svc.Token(context.Background(), req); err != nil {
		t.Fatalf("first exchange: %v", err)
	}
	// Note: PKCE check happens after code consumption, so a wrong verifier on reuse
	// surfaces as invalid_grant (code already consumed) — the code must NOT yield a
	// second token set regardless.
	_, err := svc.Token(context.Background(), req)
	if err == nil {
		t.Fatal("code reuse must fail")
	}
}

func TestToken_RejectsUnsupportedGrant(t *testing.T) {
	svc, _ := testOIDCService(t, nil)
	_, err := svc.Token(context.Background(), TokenRequest{GrantType: "password"})
	if !errors.Is(err, ErrOIDCUnsupportedGrant) {
		t.Fatalf("expected ErrOIDCUnsupportedGrant, got %v", err)
	}
}

// ── Token (refresh_token grant — rotation) ─────────────────────────────────────

func TestToken_RefreshToken_RotatesAndInvalidatesOld(t *testing.T) {
	svc, store := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	rawCode, v := runAuthorize(t, svc)

	first, err := svc.Token(context.Background(), TokenRequest{
		GrantType: "authorization_code", ClientID: "shop",
		Code: rawCode, RedirectURI: "https://shop.khabavar.ir/auth/cb",
		CodeVerifier: v,
	})
	if err != nil {
		t.Fatalf("first token: %v", err)
	}

	// Refresh rotation: a new access + refresh token, distinct from the first.
	second, err := svc.Token(context.Background(), TokenRequest{
		GrantType: "refresh_token", ClientID: "shop",
		RefreshToken: first.RefreshToken,
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if second.AccessToken == first.AccessToken {
		t.Fatal("access token did not rotate")
	}
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	if second.IDToken == "" {
		t.Fatal("id_token missing on refresh despite openid scope")
	}

	// The old refresh token must now be consumed (single-use).
	_, err = svc.Token(context.Background(), TokenRequest{
		GrantType: "refresh_token", ClientID: "shop",
		RefreshToken: first.RefreshToken,
	})
	if !errors.Is(err, ErrOIDCInvalidRefresh) {
		t.Fatalf("old refresh token must be invalid after rotation, got %v", err)
	}

	// The new refresh token must still be usable once.
	_, err = svc.Token(context.Background(), TokenRequest{
		GrantType: "refresh_token", ClientID: "shop",
		RefreshToken: second.RefreshToken,
	})
	if err != nil {
		t.Fatalf("new refresh token should work once: %v", err)
	}
	// Sanity: the store recorded two consumed refresh tokens.
	consumed := 0
	for _, rt := range store.refresh {
		if rt.ConsumedAt != nil {
			consumed++
		}
	}
	if consumed != 2 {
		t.Fatalf("expected 2 consumed refresh tokens, got %d", consumed)
	}
}

func TestToken_RefreshToken_RejectsWrongClient(t *testing.T) {
	svc, store := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	// Seed a refresh token bound to client "shop".
	rt := RefreshToken{
		TokenHash: HashOTP("rt-raw", "test-server-key"),
		ClientID:  "shop", CustomerID: "cust-1",
		Scopes:    []string{"openid"},
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.CreateRefreshToken(context.Background(), rt); err != nil {
		t.Fatal(err)
	}
	// Register a second client so GetClient("admin") succeeds (otherwise the client
	// check fails before the token-client mismatch is reached).
	store.clients["admin"] = &OIDCClient{
		ClientID: "admin", ServiceCode: "admin", Name: "Admin",
		RedirectURIs: []string{"https://admin.khabavar.ir/cb"},
		Scopes:       []string{"openid", "staff"}, IsActive: true,
	}
	_, err := svc.Token(context.Background(), TokenRequest{
		GrantType: "refresh_token", ClientID: "admin",
		RefreshToken: "rt-raw",
	})
	if !errors.Is(err, ErrOIDCInvalidRefresh) {
		t.Fatalf("expected ErrOIDCInvalidRefresh on client mismatch, got %v", err)
	}
}

// ── HTTP handlers ──────────────────────────────────────────────────────────────

func TestAuthorizeHandler_Redirects302(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	_, c := pkcePair(t)
	q := url.Values{}
	q.Set("client_id", "shop")
	q.Set("redirect_uri", "https://shop.khabavar.ir/auth/cb")
	q.Set("response_type", "code")
	q.Set("scope", "openid profile")
	q.Set("state", "s1")
	q.Set("code_challenge", c)
	q.Set("code_challenge_method", "S256")

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer abc")
	rec := httptest.NewRecorder()
	AuthorizeHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status=%d want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "code=") || !strings.Contains(loc, "state=s1") {
		t.Fatalf("Location=%s", loc)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing Cache-Control: no-store")
	}
}

func TestTokenHandler_ReturnsJSON(t *testing.T) {
	svc, _ := testOIDCService(t, func(_ context.Context, _ string) (Subject, bool) {
		return Subject{Kind: "customer", ID: "cust-1"}, true
	})
	rawCode, v := runAuthorize(t, svc)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", "shop")
	form.Set("code", rawCode)
	form.Set("redirect_uri", "https://shop.khabavar.ir/auth/cb")
	form.Set("code_verifier", v)

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	TokenHandler(svc).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("missing no-store")
	}
}
