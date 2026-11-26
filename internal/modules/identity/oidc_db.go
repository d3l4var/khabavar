// oidc_db.go provides the pgx implementation of OIDCStore and a SubjectResolver that
// validates bearer tokens against identity_sessions. It is the production wiring for
// the OIDC endpoints mounted by cmd/khabavar (KH_ROLE=identity).
package identity

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBOIDCStore is the pgx implementation of OIDCStore.
type DBOIDCStore struct {
	pool *pgxpool.Pool
}

func NewDBOIDCStore(pool *pgxpool.Pool) *DBOIDCStore { return &DBOIDCStore{pool: pool} }

func (s *DBOIDCStore) GetClient(ctx context.Context, clientID string) (*OIDCClient, error) {
	var c OIDCClient
	var redirectURIs, scopes []string
	err := s.pool.QueryRow(ctx,
		`SELECT client_id, service_code, name, redirect_uris, scopes, is_active
		   FROM oauth_clients WHERE client_id = $1`, clientID).
		Scan(&c.ClientID, &c.ServiceCode, &c.Name, &redirectURIs, &scopes, &c.IsActive)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.RedirectURIs = redirectURIs
	c.Scopes = scopes
	return &c, nil
}

func (s *DBOIDCStore) CreateAuthorizationCode(ctx context.Context, code AuthorizationCode) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_authorization_codes
		   (client_id, code_hash, customer_id, admin_user_id, redirect_uri,
		    code_challenge, code_challenge_method, scopes, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		code.ClientID, code.CodeHash,
		nullableStr(code.CustomerID), nullableStr(code.AdminUserID),
		code.RedirectURI, code.CodeChallenge, code.CodeChallengeMethod,
		code.Scopes, code.ExpiresAt)
	return err
}

// ConsumeAuthorizationCode atomically loads an unconsumed, unexpired code by hash and
// marks it consumed. Returns ErrOIDCInvalidCode when not found/expired/consumed.
func (s *DBOIDCStore) ConsumeAuthorizationCode(ctx context.Context, codeHash string) (*AuthorizationCode, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var c AuthorizationCode
	var custID, admID pgtype.Text
	err = tx.QueryRow(ctx,
		`SELECT client_id, customer_id, admin_user_id, redirect_uri,
		        code_challenge, code_challenge_method, scopes, expires_at, consumed_at
		   FROM oauth_authorization_codes
		  WHERE code_hash = $1 AND consumed_at IS NULL AND expires_at > now()
		  FOR UPDATE`, codeHash).
		Scan(&c.ClientID, &custID, &admID, &c.RedirectURI,
			&c.CodeChallenge, &c.CodeChallengeMethod, &c.Scopes, &c.ExpiresAt, &c.ConsumedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrOIDCInvalidCode
	}
	if err != nil {
		return nil, err
	}
	c.CodeHash = codeHash
	if custID.Valid {
		c.CustomerID = custID.String
	}
	if admID.Valid {
		c.AdminUserID = admID.String
	}
	if _, err := tx.Exec(ctx,
		`UPDATE oauth_authorization_codes SET consumed_at = now() WHERE code_hash = $1`, codeHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *DBOIDCStore) CreateSession(ctx context.Context, sessID, customerID, adminUserID, serviceCode, tokenHash, ip, ua string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO identity_sessions
		   (id, customer_id, admin_user_id, service_code, token_hash, ip_address, user_agent, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		sessID, nullableStr(customerID), nullableStr(adminUserID),
		serviceCode, tokenHash, nullableStr(ip), nullableStr(ua), expiresAt)
	return err
}

func (s *DBOIDCStore) CreateRefreshToken(ctx context.Context, rt RefreshToken) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO oauth_refresh_tokens
		   (token_hash, client_id, session_id, customer_id, admin_user_id, scopes, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		rt.TokenHash, rt.ClientID, nullableStr(rt.SessionID),
		nullableStr(rt.CustomerID), nullableStr(rt.AdminUserID),
		rt.Scopes, rt.ExpiresAt)
	return err
}

// ConsumeRefreshToken atomically loads an unconsumed, unrevoked, unexpired refresh
// token by hash and marks it consumed (rotation). Returns ErrOIDCInvalidRefresh when
// not found/expired/consumed/revoked.
func (s *DBOIDCStore) ConsumeRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var rt RefreshToken
	var sessID, custID, admID pgtype.Text
	err = tx.QueryRow(ctx,
		`SELECT client_id, session_id, customer_id, admin_user_id, scopes, expires_at
		   FROM oauth_refresh_tokens
		  WHERE token_hash = $1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now()
		  FOR UPDATE`, tokenHash).
		Scan(&rt.ClientID, &sessID, &custID, &admID, &rt.Scopes, &rt.ExpiresAt)
	if err == pgx.ErrNoRows {
		return nil, ErrOIDCInvalidRefresh
	}
	if err != nil {
		return nil, err
	}
	rt.TokenHash = tokenHash
	if sessID.Valid {
		rt.SessionID = sessID.String
	}
	if custID.Valid {
		rt.CustomerID = custID.String
	}
	if admID.Valid {
		rt.AdminUserID = admID.String
	}
	if _, err := tx.Exec(ctx,
		`UPDATE oauth_refresh_tokens SET consumed_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &rt, nil
}

// DBSubjectResolver returns a SubjectResolver that validates a bearer access token by
// looking up identity_sessions by token_hash (HMAC of the raw token with serverKey)
// and returns the bound customer or admin subject. Expired or revoked sessions fail.
func DBSubjectResolver(pool *pgxpool.Pool, serverKey string) SubjectResolver {
	return func(ctx context.Context, bearerToken string) (Subject, bool) {
		if bearerToken == "" {
			return Subject{}, false
		}
		var custID, admID pgtype.Text
		err := pool.QueryRow(ctx,
			`SELECT customer_id, admin_user_id
			   FROM identity_sessions
			  WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
			HashOTP(bearerToken, serverKey)).Scan(&custID, &admID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Subject{}, false
			}
			return Subject{}, false
		}
		if custID.Valid {
			return Subject{Kind: "customer", ID: custID.String}, true
		}
		if admID.Valid {
			return Subject{Kind: "admin", ID: admID.String}, true
		}
		return Subject{}, false
	}
}

// Compile-time assertions that DBOIDCStore satisfies OIDCStore.
var _ OIDCStore = (*DBOIDCStore)(nil)
