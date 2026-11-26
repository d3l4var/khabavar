// oidc.go implements the OIDC Authorization Code + PKCE flow (CIAM §۳, SAD §۴.۱).
// Endpoints: GET /oauth/authorize, POST /oauth/token (R-07 §oauth).
//
// Design notes:
//   - Authorization codes and refresh tokens are random opaque values; only their
//     HMAC-SHA256 hash is persisted (oauth_authorization_codes.code_hash,
//     oauth_refresh_tokens.token_hash) — raw secrets never leave the server.
//   - PKCE S256 is mandatory: code_challenge == base64url(sha256(code_verifier)).
//   - Refresh tokens rotate on every use (CIAM §۵): the old token is consumed and a
//     fresh one returned in the same response.
//   - id_token is a compact JWS (HS256) issued only when the requested scope set
//     contains "openid". It carries iss/sub/aud/iat/exp/scope.
//   - The subject (authenticated customer/admin) is resolved by an injected
//     SubjectResolver so OIDC stays decoupled from the OTP session store and is
//     unit-testable without a database.
package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors for the OIDC flow. HTTP handlers map these to OAuth error codes.
var (
	ErrOIDCInvalidClient    = errors.New("oidc: invalid client")
	ErrOIDCInvalidRedirect  = errors.New("oidc: redirect_uri not allowed")
	ErrOIDCInvalidRequest   = errors.New("oidc: invalid request")
	ErrOIDCUnsupportedGrant = errors.New("oidc: unsupported grant_type")
	ErrOIDCInvalidCode      = errors.New("oidc: invalid authorization code")
	ErrOIDCInvalidPKCE      = errors.New("oidc: pkce verification failed")
	ErrOIDCInvalidScope     = errors.New("oidc: requested scope not permitted for client")
	ErrOIDCInvalidRefresh   = errors.New("oidc: invalid or expired refresh token")
	ErrOIDCSubjectRequired  = errors.New("oidc: authenticated subject required")
)

// Supported scopes (R-07 §securitySchemes). Staff scope gates admin clients.
var supportedScopes = map[string]bool{
	"openid":            true,
	"profile":           true,
	"customer.basic":    true,
	"customer.commerce": true,
	"customer.content":  true,
	"staff":             true,
}

// Subject is the authenticated principal driving an authorize request.
type Subject struct {
	Kind string // "customer" | "admin"
	ID   string
}

// SubjectResolver maps a bearer access token to a Subject. In production it queries
// identity_sessions by token_hash; tests inject a fake.
type SubjectResolver func(ctx context.Context, bearerToken string) (Subject, bool)

// OIDCClient mirrors oauth_clients.
type OIDCClient struct {
	ClientID     string
	ServiceCode  string
	Name         string
	RedirectURIs []string
	Scopes       []string
	IsActive     bool
}

// AuthorizationCode mirrors oauth_authorization_codes (the persisted view — code is hashed).
type AuthorizationCode struct {
	CodeHash            string
	ClientID            string
	CustomerID          string
	AdminUserID         string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Scopes              []string
	ExpiresAt           time.Time
	ConsumedAt          *time.Time
}

// RefreshToken mirrors oauth_refresh_tokens.
type RefreshToken struct {
	TokenHash   string
	ClientID    string
	SessionID   string
	CustomerID  string
	AdminUserID string
	Scopes      []string
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
	RevokedAt   *time.Time
}

// TokenResult is the /oauth/token response body (R-07 §TokenResponse).
type TokenResult struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OIDCStore is the persistence boundary for the OIDC flow. The DB implementation
// uses pgx; unit tests use an in-memory implementation (oidc_test.go).
type OIDCStore interface {
	GetClient(ctx context.Context, clientID string) (*OIDCClient, error)
	CreateAuthorizationCode(ctx context.Context, code AuthorizationCode) error
	// ConsumeAuthorizationCode atomically loads an unconsumed, unexpired code by hash
	// and marks it consumed. Returns ErrOIDCInvalidCode (wrapped) when not found.
	ConsumeAuthorizationCode(ctx context.Context, codeHash string) (*AuthorizationCode, error)
	CreateSession(ctx context.Context, sessID, customerID, adminUserID, serviceCode, tokenHash, ip, ua string, expiresAt time.Time) error
	CreateRefreshToken(ctx context.Context, rt RefreshToken) error
	// ConsumeRefreshToken atomically loads an unconsumed, unrevoked, unexpired refresh
	// token by hash and marks it consumed (rotation). Returns ErrOIDCInvalidRefresh
	// (wrapped) when not found.
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
}

// OIDCService implements the OIDC endpoints (SAD §۴.۱, CIAM §۳-۵).
type OIDCService struct {
	store      OIDCStore
	resolve    SubjectResolver
	key        string // server HMAC key — hashes codes/tokens and signs id_token
	issuer     string // id_token.iss (e.g. https://id.khabavar.ir)
	codeTTL    time.Duration
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewOIDCService wires the store, subject resolver, server key, issuer and TTLs.
// codeTTL ~10m, accessTTL ~15m, refreshTTL ~30d (DEP v1.1 §۴; tune via config).
func NewOIDCService(store OIDCStore, resolve SubjectResolver, serverKey, issuer string, codeTTL, accessTTL, refreshTTL time.Duration) *OIDCService {
	if resolve == nil {
		resolve = func(context.Context, string) (Subject, bool) { return Subject{}, false }
	}
	return &OIDCService{
		store:      store,
		resolve:    resolve,
		key:        serverKey,
		issuer:     issuer,
		codeTTL:    codeTTL,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		now:        time.Now,
	}
}

// SetNow injects a clock (tests only).
func (s *OIDCService) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// ── PKCE (RFC 7636, method S256) ───────────────────────────────────────────────

// VerifyPKCE returns true when challenge == base64url(sha256(verifier)) (no padding).
func VerifyPKCE(verifier, challenge, method string) bool {
	if method != "S256" || verifier == "" || challenge == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtleEqual(want, challenge)
}

func subtleEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var d byte
	for i := 0; i < len(a); i++ {
		d |= a[i] ^ b[i]
	}
	return d == 0
}

// ── id_token (compact JWS HS256) ───────────────────────────────────────────────

type idTokenClaims struct {
	Iss   string `json:"iss"`
	Sub   string `json:"sub"`
	Aud   string `json:"aud"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Scope string `json:"scope"`
}

// mintIDToken builds a compact JWS (header.payload.signature, base64url no padding)
// signed with HMAC-SHA256 over "header.payload". It is intentionally minimal — no
// external JWT dependency — yet structurally a valid JWS (CIAM §۵: id_token with
// openid scope). Issued at/expire use the service clock.
func (s *OIDCService) mintIDToken(clientID string, subj Subject, scopes []string) (string, error) {
	now := s.now()
	claims := idTokenClaims{
		Iss:   s.issuer,
		Sub:   subj.ID,
		Aud:   clientID,
		Iat:   now.Unix(),
		Exp:   now.Add(s.accessTTL).Unix(),
		Scope: strings.Join(scopes, " "),
	}
	header := `{"alg":"HS256","typ":"JWT"}`
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("id_token marshal: %w", err)
	}
	hb := base64.RawURLEncoding.EncodeToString([]byte(header))
	pb := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(s.key))
	mac.Write([]byte(hb + "." + pb))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hb + "." + pb + "." + sig, nil
}

// ── random secrets ─────────────────────────────────────────────────────────────

// randomCode returns a 32-byte hex value used as the raw authorization code (the
// client never sees it in cleartext from storage — only via the redirect).
func randomCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth code rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ── Authorize (GET /oauth/authorize) ───────────────────────────────────────────

// AuthorizeRequest is the validated /oauth/authorize input.
type AuthorizeRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	ContinuationToken   string
}

// AuthorizeResult is the outcome: either an immediate redirect with a fresh code,
// or a redirect to the OTP login page carrying a continuation token.
type AuthorizeResult struct {
	RedirectURL       string // full URL to send the user to (302 Location)
	RequiresLogin     bool   // true → user has no session; redirect to OTP login
	ContinuationToken string // set when RequiresLogin
}

// Authorize validates the request, resolves the subject (from the bearer token) and
// either mints an authorization code (302 → redirect_uri?code=&state=) or signals
// that the user must complete OTP login first (302 → login page with continuation).
func (s *OIDCService) Authorize(ctx context.Context, req AuthorizeRequest, bearer string) (AuthorizeResult, error) {
	if req.ResponseType != "code" {
		return AuthorizeResult{}, ErrOIDCInvalidRequest
	}
	if req.CodeChallenge == "" || req.CodeChallengeMethod != "S256" {
		return AuthorizeResult{}, ErrOIDCInvalidRequest
	}
	client, err := s.store.GetClient(ctx, req.ClientID)
	if err != nil {
		return AuthorizeResult{}, ErrOIDCInvalidClient
	}
	if client == nil || !client.IsActive {
		return AuthorizeResult{}, ErrOIDCInvalidClient
	}
	if !redirectAllowed(client.RedirectURIs, req.RedirectURI) {
		return AuthorizeResult{}, ErrOIDCInvalidRedirect
	}
	scopes, err := normalizeScopes(req.Scope, client.Scopes)
	if err != nil {
		return AuthorizeResult{}, err
	}

	subj, ok := s.resolve(ctx, bearer)
	if !ok || subj.ID == "" {
		ct, err := s.mintContinuation(req, scopes)
		if err != nil {
			return AuthorizeResult{}, err
		}
		return AuthorizeResult{
			RedirectURL:       buildLoginRedirect(req.RedirectURI, req.State, ct),
			RequiresLogin:     true,
			ContinuationToken: ct,
		}, nil
	}

	raw, err := randomCode()
	if err != nil {
		return AuthorizeResult{}, err
	}
	code := AuthorizationCode{
		CodeHash:            HashOTP(raw, s.key),
		ClientID:            req.ClientID,
		CustomerID:          ifThen(subj.Kind == "customer", subj.ID),
		AdminUserID:         ifThen(subj.Kind == "admin", subj.ID),
		RedirectURI:         req.RedirectURI,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		Scopes:              scopes,
		ExpiresAt:           s.now().Add(s.codeTTL),
	}
	if err := s.store.CreateAuthorizationCode(ctx, code); err != nil {
		return AuthorizeResult{}, fmt.Errorf("create auth code: %w", err)
	}
	return AuthorizeResult{
		RedirectURL: appendQuery(req.RedirectURI, "code", raw, "state", req.State),
	}, nil
}

// mintContinuation signs a compact token encoding the authorize params + chosen
// scopes + expiry, so the OTP verify endpoint can reconstruct and resume the flow.
// Format: base64url(payload).base64url(hmac) where payload is the JSON request.
func (s *OIDCService) mintContinuation(req AuthorizeRequest, scopes []string) (string, error) {
	body, err := json.Marshal(struct {
		AuthorizeRequest
		Scopes []string  `json:"scopes"`
		Exp    time.Time `json:"exp"`
	}{AuthorizeRequest: req, Scopes: scopes, Exp: s.now().Add(5 * time.Minute)})
	if err != nil {
		return "", fmt.Errorf("continuation marshal: %w", err)
	}
	bb := base64.RawURLEncoding.EncodeToString(body)
	return bb + "." + base64.RawURLEncoding.EncodeToString(signContinuation(bb, s.key)), nil
}

// signContinuation returns the HMAC over the encoded payload. Mint and verify share
// it so the two cannot drift.
func signContinuation(encoded, key string) []byte {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(encoded))
	return mac.Sum(nil)
}

// hmacEqual is a constant-time comparison for the continuation signature.
func hmacEqual(a, b []byte) bool {
	return hmac.Equal(a, b)
}

// ── Token (POST /oauth/token) ──────────────────────────────────────────────────

// TokenRequest is the /oauth/token input (R-07 §TokenRequest).
type TokenRequest struct {
	GrantType    string
	ClientID     string
	Code         string // authorization_code grant
	RedirectURI  string
	CodeVerifier string // PKCE
	RefreshToken string // refresh_token grant
}

// Token exchanges an authorization code (or rotates a refresh token) for tokens.
func (s *OIDCService) Token(ctx context.Context, req TokenRequest) (TokenResult, error) {
	switch req.GrantType {
	case "authorization_code":
		return s.tokenFromCode(ctx, req)
	case "refresh_token":
		return s.tokenFromRefresh(ctx, req)
	default:
		return TokenResult{}, ErrOIDCUnsupportedGrant
	}
}

func (s *OIDCService) tokenFromCode(ctx context.Context, req TokenRequest) (TokenResult, error) {
	if req.Code == "" || req.CodeVerifier == "" {
		return TokenResult{}, ErrOIDCInvalidRequest
	}
	client, err := s.store.GetClient(ctx, req.ClientID)
	if err != nil || client == nil || !client.IsActive {
		return TokenResult{}, ErrOIDCInvalidClient
	}
	code, err := s.store.ConsumeAuthorizationCode(ctx, HashOTP(req.Code, s.key))
	if err != nil {
		return TokenResult{}, ErrOIDCInvalidCode
	}
	if code.ClientID != req.ClientID {
		return TokenResult{}, ErrOIDCInvalidCode
	}
	if code.RedirectURI != req.RedirectURI {
		return TokenResult{}, ErrOIDCInvalidRequest
	}
	if !VerifyPKCE(req.CodeVerifier, code.CodeChallenge, code.CodeChallengeMethod) {
		return TokenResult{}, ErrOIDCInvalidPKCE
	}
	return s.issueTokens(ctx, client, code.CustomerID, code.AdminUserID, code.Scopes)
}

func (s *OIDCService) tokenFromRefresh(ctx context.Context, req TokenRequest) (TokenResult, error) {
	if req.RefreshToken == "" {
		return TokenResult{}, ErrOIDCInvalidRequest
	}
	client, err := s.store.GetClient(ctx, req.ClientID)
	if err != nil || client == nil || !client.IsActive {
		return TokenResult{}, ErrOIDCInvalidClient
	}
	rt, err := s.store.ConsumeRefreshToken(ctx, HashOTP(req.RefreshToken, s.key))
	if err != nil {
		return TokenResult{}, ErrOIDCInvalidRefresh
	}
	if rt.ClientID != req.ClientID {
		return TokenResult{}, ErrOIDCInvalidRefresh
	}
	return s.issueTokens(ctx, client, rt.CustomerID, rt.AdminUserID, rt.Scopes)
}

// issueTokens mints an access token (opaque session token), a rotating refresh token,
// and an id_token when "openid" is in scopes. The access token's hash is persisted in
// identity_sessions; the refresh token's hash in oauth_refresh_tokens.
func (s *OIDCService) issueTokens(ctx context.Context, client *OIDCClient, customerID, adminUserID string, scopes []string) (TokenResult, error) {
	access, err := randomToken()
	if err != nil {
		return TokenResult{}, err
	}
	sessID := uuid.NewString()
	expires := s.now().Add(s.accessTTL)
	if err := s.store.CreateSession(ctx, sessID, customerID, adminUserID, client.ServiceCode,
		HashOTP(access, s.key), "", "", expires); err != nil {
		return TokenResult{}, fmt.Errorf("create session: %w", err)
	}
	refresh, err := randomToken()
	if err != nil {
		return TokenResult{}, err
	}
	rt := RefreshToken{
		TokenHash:   HashOTP(refresh, s.key),
		ClientID:    client.ClientID,
		SessionID:   sessID,
		CustomerID:  customerID,
		AdminUserID: adminUserID,
		Scopes:      scopes,
		ExpiresAt:   s.now().Add(s.refreshTTL),
	}
	if err := s.store.CreateRefreshToken(ctx, rt); err != nil {
		return TokenResult{}, fmt.Errorf("create refresh token: %w", err)
	}
	out := TokenResult{
		AccessToken:  access,
		TokenType:    "Bearer",
		ExpiresIn:    int(s.accessTTL / time.Second),
		RefreshToken: refresh,
		Scope:        strings.Join(scopes, " "),
	}
	if contains(scopes, "openid") {
		subj := Subject{Kind: subjectKind(customerID, adminUserID), ID: orElse(customerID, adminUserID)}
		idt, err := s.mintIDToken(client.ClientID, subj, scopes)
		if err != nil {
			return TokenResult{}, err
		}
		out.IDToken = idt
	}
	return out, nil
}

// ── HTTP handlers ──────────────────────────────────────────────────────────────

// AuthorizeHandler serves GET /oauth/authorize. On validation failure it writes 400
// with an OAuth error body; on success it writes 302 with the Location header.
func AuthorizeHandler(svc *OIDCService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := AuthorizeRequest{
			ClientID:            r.URL.Query().Get("client_id"),
			RedirectURI:         r.URL.Query().Get("redirect_uri"),
			ResponseType:        r.URL.Query().Get("response_type"),
			Scope:               r.URL.Query().Get("scope"),
			State:               r.URL.Query().Get("state"),
			CodeChallenge:       r.URL.Query().Get("code_challenge"),
			CodeChallengeMethod: r.URL.Query().Get("code_challenge_method"),
			ContinuationToken:   r.URL.Query().Get("continuation_token"),
		}
		bearer := bearerFromHeader(r)
		res, err := svc.Authorize(r.Context(), req, bearer)
		if err != nil {
			writeOIDCErr(w, err)
			return
		}
		w.Header().Set("Location", res.RedirectURL)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusFound)
	}
}

// TokenHandler serves POST /oauth/token (application/x-www-form-urlencoded per
// RFC 6749 §۴.1.3). Writes a JSON TokenResponse on success.
func TokenHandler(svc *OIDCService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeOIDCErr(w, ErrOIDCInvalidRequest)
			return
		}
		req := TokenRequest{
			GrantType:    r.PostForm.Get("grant_type"),
			ClientID:     r.PostForm.Get("client_id"),
			Code:         r.PostForm.Get("code"),
			RedirectURI:  r.PostForm.Get("redirect_uri"),
			CodeVerifier: r.PostForm.Get("code_verifier"),
			RefreshToken: r.PostForm.Get("refresh_token"),
		}
		out, err := svc.Token(r.Context(), req)
		if err != nil {
			writeOIDCErr(w, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		writeJSON(w, http.StatusOK, out)
	}
}

func writeOIDCErr(w http.ResponseWriter, err error) {
	code, desc := http.StatusBadRequest, "invalid_request"
	switch {
	case errors.Is(err, ErrOIDCInvalidClient), errors.Is(err, ErrOIDCInvalidRedirect):
		code, desc = http.StatusBadRequest, "invalid_client"
	case errors.Is(err, ErrOIDCUnsupportedGrant):
		code, desc = http.StatusBadRequest, "unsupported_grant_type"
	case errors.Is(err, ErrOIDCInvalidCode):
		code, desc = http.StatusBadRequest, "invalid_grant"
	case errors.Is(err, ErrOIDCInvalidPKCE):
		code, desc = http.StatusBadRequest, "invalid_grant"
	case errors.Is(err, ErrOIDCInvalidRefresh):
		code, desc = http.StatusBadRequest, "invalid_grant"
	case errors.Is(err, ErrOIDCInvalidScope):
		code, desc = http.StatusBadRequest, "invalid_scope"
	default:
		code, desc = http.StatusInternalServerError, "server_error"
	}
	writeJSON(w, code, map[string]any{"error": desc})
}

// ── helpers ────────────────────────────────────────────────────────────────────

func redirectAllowed(whitelist []string, uri string) bool {
	for _, w := range whitelist {
		if w == uri {
			return true
		}
	}
	return false
}

func normalizeScopes(scope string, clientScopes []string) ([]string, error) {
	allowed := make(map[string]bool, len(clientScopes))
	for _, s := range clientScopes {
		allowed[s] = true
	}
	allowed["openid"] = true
	var out []string
	seen := map[string]bool{}
	for _, s := range strings.Fields(scope) {
		if !supportedScopes[s] {
			return nil, ErrOIDCInvalidScope
		}
		if !allowed[s] {
			return nil, ErrOIDCInvalidScope
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, ErrOIDCInvalidScope
	}
	return out, nil
}

func appendQuery(base, k1, v1, k2, v2 string) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + k1 + "=" + v1 + "&" + k2 + "=" + v2
}

func buildLoginRedirect(redirectURI, state, continuation string) string {
	return "/api/v1/auth/otp/login?continuation=" + continuation + "&redirect_uri=" + redirectURI + "&state=" + state
}

func bearerFromHeader(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if v == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return strings.TrimSpace(v[7:])
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func ifThen(cond bool, v string) string {
	if cond {
		return v
	}
	return ""
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func subjectKind(customerID, adminUserID string) string {
	if adminUserID != "" {
		return "admin"
	}
	return "customer"
}
