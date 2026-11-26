// oidc_pg_test.go exercises DBOIDCStore and DBSubjectResolver against a real
// Postgres instance. Skipped unless KH_TEST_PG_DSN is set, so the default test
// suite runs without a database. When enabled, the harness applies all up
// migrations, seeds the test fixtures, runs the assertions, then drops the
// public schema on cleanup.
package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// setupPG is the per-test entry point. It opens a pool, applies migrations, and
// seeds the standard OIDC client fixture. Tests receive the pool and a
// DBOIDCStore; t.Cleanup drops the schema and closes the pool.
func setupPG(t *testing.T) (*pgxpool.Pool, *DBOIDCStore) {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := testpg.SeedOIDCClient(ctx, pool, "shop", "shop", "KHABAVAR Store",
		[]string{"https://shop.khabavar.ir/auth/cb"},
		[]string{"openid", "profile", "customer.basic", "customer.commerce"}); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	return pool, NewDBOIDCStore(pool)
}

// pkceS256Challenge derives the S256 challenge from a verifier (mirror of the
// production helper, kept local so the test stays self-contained).
func pkceS256Challenge(verifier string) string {
	return base64RawURL(sha256Sum(verifier))
}

// mintTestCode generates a random opaque code and returns (raw, hash, err).
// The raw value is what /oauth/authorize redirects with; the hash is what is
// persisted in oauth_authorization_codes.code_hash.
func mintTestCode() (string, string, error) {
	raw, err := randomOpaque(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashServer(raw, "test-server-key"), nil
}

// mintTestRefresh is the refresh-token equivalent of mintTestCode.
func mintTestRefresh() (string, string, error) {
	raw, err := randomOpaque(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashServer(raw, "test-server-key"), nil
}

// randomOpaque and hashServer are thin wrappers around the package-internal
// randomToken/HashOTP — exposed here only to keep the test readable.
func randomOpaque(n int) (string, error) { return randomToken() }

// hashServer hashes raw with serverKey using the OIDC code/refresh hash function.
// We reuse HashOTP because OIDC codes/refresh tokens use the same HMAC-SHA256.
func hashServer(raw, serverKey string) string { return HashOTP(raw, serverKey) }

// ── GetClient ──────────────────────────────────────────────────────────────────

func TestPG_OIDCStore_GetClient_ReturnsActiveClient(t *testing.T) {
	ctx := context.Background()
	_, store := setupPG(t)

	c, err := store.GetClient(ctx, "shop")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if c == nil {
		t.Fatal("expected client, got nil")
	}
	if c.ClientID != "shop" || c.ServiceCode != "shop" || !c.IsActive {
		t.Fatalf("client = %+v", c)
	}
	if len(c.RedirectURIs) != 1 || c.RedirectURIs[0] != "https://shop.khabavar.ir/auth/cb" {
		t.Fatalf("redirect_uris = %v", c.RedirectURIs)
	}
}

func TestPG_OIDCStore_GetClient_ReturnsNilForUnknown(t *testing.T) {
	ctx := context.Background()
	_, store := setupPG(t)

	c, err := store.GetClient(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil, got %+v", c)
	}
}

// ── AuthorizationCode lifecycle ────────────────────────────────────────────────

func TestPG_OIDCStore_AuthorizationCode_CreateConsumeRotate(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000001")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := pkceS256Challenge(verifier)
	_, hash, err := mintTestCode()
	if err != nil {
		t.Fatalf("mint code: %v", err)
	}

	if err := store.CreateAuthorizationCode(ctx, AuthorizationCode{
		ClientID:            "shop",
		CodeHash:            hash,
		CustomerID:          custID,
		RedirectURI:         "https://shop.khabavar.ir/auth/cb",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		Scopes:              []string{"openid", "profile"},
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}); err != nil {
		t.Fatalf("create code: %v", err)
	}

	// First consume succeeds.
	c, err := store.ConsumeAuthorizationCode(ctx, hash)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if c.ClientID != "shop" || c.CustomerID != custID {
		t.Fatalf("consumed code = %+v", c)
	}
	if c.CodeChallenge != challenge {
		t.Fatalf("challenge mismatch: got %s want %s", c.CodeChallenge, challenge)
	}

	// Second consume fails — code is one-time-use.
	_, err = store.ConsumeAuthorizationCode(ctx, hash)
	if !errors.Is(err, ErrOIDCInvalidCode) {
		t.Fatalf("reuse must fail with ErrOIDCInvalidCode, got %v", err)
	}
}

func TestPG_OIDCStore_AuthorizationCode_RejectsExpired(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000002")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	_, hash, err := mintTestCode()
	if err != nil {
		t.Fatalf("mint code: %v", err)
	}
	if err := store.CreateAuthorizationCode(ctx, AuthorizationCode{
		ClientID: "shop", CodeHash: hash, CustomerID: custID,
		RedirectURI:         "https://shop.khabavar.ir/auth/cb",
		CodeChallenge:       pkceS256Challenge("verifier-x"),
		CodeChallengeMethod: "S256",
		Scopes:              []string{"openid"},
		ExpiresAt:           time.Now().Add(-1 * time.Minute), // already expired
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = store.ConsumeAuthorizationCode(ctx, hash)
	if !errors.Is(err, ErrOIDCInvalidCode) {
		t.Fatalf("expired code must fail with ErrOIDCInvalidCode, got %v", err)
	}
}

// ── RefreshToken rotation ──────────────────────────────────────────────────────

func TestPG_OIDCStore_RefreshToken_CreateConsumeRotate(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000003")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	_, hash, err := mintTestRefresh()
	if err != nil {
		t.Fatalf("mint refresh: %v", err)
	}
	if err := store.CreateRefreshToken(ctx, RefreshToken{
		TokenHash:  hash,
		ClientID:   "shop",
		CustomerID: custID,
		Scopes:     []string{"openid", "profile"},
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("create refresh: %v", err)
	}

	// First consume succeeds.
	rt, err := store.ConsumeRefreshToken(ctx, hash)
	if err != nil {
		t.Fatalf("consume refresh: %v", err)
	}
	if rt.ClientID != "shop" || rt.CustomerID != custID {
		t.Fatalf("consumed refresh = %+v", rt)
	}

	// Rotation: the old hash must now be consumed.
	_, err = store.ConsumeRefreshToken(ctx, hash)
	if !errors.Is(err, ErrOIDCInvalidRefresh) {
		t.Fatalf("reused refresh must fail with ErrOIDCInvalidRefresh, got %v", err)
	}
}

func TestPG_OIDCStore_RefreshToken_RejectsExpired(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000004")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	_, hash, err := mintTestRefresh()
	if err != nil {
		t.Fatalf("mint refresh: %v", err)
	}
	if err := store.CreateRefreshToken(ctx, RefreshToken{
		TokenHash:  hash,
		ClientID:   "shop",
		CustomerID: custID,
		Scopes:     []string{"openid"},
		ExpiresAt:  time.Now().Add(-time.Hour), // expired
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = store.ConsumeRefreshToken(ctx, hash)
	if !errors.Is(err, ErrOIDCInvalidRefresh) {
		t.Fatalf("expired refresh must fail, got %v", err)
	}
}

// ── DBSubjectResolver ──────────────────────────────────────────────────────────

func TestPG_SubjectResolver_ResolvesValidSession(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000005")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	rawToken := "raw-bearer-token-xyz"
	serverKey := "test-server-key"
	tokenHash := HashOTP(rawToken, serverKey)

	// Issue a session bound to the customer.
	if err := store.CreateSession(ctx,
		uuid.NewString(), custID, "", "shop", tokenHash, "127.0.0.1", "test", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}

	resolve := DBSubjectResolver(pool, serverKey)
	sub, ok := resolve(ctx, rawToken)
	if !ok {
		t.Fatal("expected subject, got none")
	}
	if sub.Kind != "customer" || sub.ID != custID {
		t.Fatalf("subject = %+v", sub)
	}
}

func TestPG_SubjectResolver_RejectsUnknownToken(t *testing.T) {
	ctx := context.Background()
	pool, _ := setupPG(t)
	resolve := DBSubjectResolver(pool, "test-server-key")
	if _, ok := resolve(ctx, "never-issued"); ok {
		t.Fatal("expected no subject for unknown token")
	}
}

func TestPG_SubjectResolver_RejectsExpiredSession(t *testing.T) {
	ctx := context.Background()
	pool, store := setupPG(t)
	custID, err := testpg.SeedCustomer(ctx, pool, "09120000006")
	if err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	rawToken := "raw-expired"
	serverKey := "test-server-key"
	if err := store.CreateSession(ctx,
		uuid.NewString(), custID, "", "shop", HashOTP(rawToken, serverKey), "127.0.0.1", "test", time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	resolve := DBSubjectResolver(pool, serverKey)
	if _, ok := resolve(ctx, rawToken); ok {
		t.Fatal("expired session must not resolve")
	}
}
