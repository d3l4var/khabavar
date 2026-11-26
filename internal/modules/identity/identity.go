package identity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// Sentinel errors for the OTP flow. HTTP handlers map these to status codes.
var (
	ErrRateLimited      = errors.New("identity: otp rate limited")
	ErrChallengeNone    = errors.New("identity: no active otp challenge")
	ErrChallengeExpired = errors.New("identity: otp challenge expired")
	ErrMaxAttempts      = errors.New("identity: max otp attempts exceeded")
	ErrAlreadyConsumed  = errors.New("identity: otp already consumed")
	ErrInvalidCode      = errors.New("identity: invalid otp code")
)

// Purpose values (otp_challenges.purpose CHECK constraint).
const (
	PurposeLogin       = "LOGIN"
	PurposeVerifyPhone = "VERIFY_PHONE"
	PurposeAdminLogin  = "ADMIN_LOGIN"
)

// OTPChallenge mirrors the otp_challenges row relevant to verification.
type OTPChallenge struct {
	ID          string
	Phone       string
	Purpose     string
	CodeHash    string
	Attempts    int
	MaxAttempts int
	ExpiresAt   time.Time
	ConsumedAt  *time.Time
}

// Session mirrors identity_sessions.
type Session struct {
	ID          string
	CustomerID  string
	AdminUserID string
	ServiceCode string
	TokenHash   string
	ExpiresAt   time.Time
	IPAddress   string
	UserAgent   string
}

// Store is the persistence boundary for identity. The DB implementation uses pgx;
// unit tests use an in-memory implementation (identity_test.go).
type Store interface {
	CreateOTPChallenge(ctx context.Context, phone, purpose, codeHash string, maxAttempts int, expiresAt time.Time, ip string) (string, error)
	ActiveOTPChallenge(ctx context.Context, phone, purpose string) (*OTPChallenge, error)
	IncrementOTPAttempts(ctx context.Context, id string) (int, error)
	ConsumeOTPChallenge(ctx context.Context, id string) error
	UpsertCustomerByPhone(ctx context.Context, phone string) (string, error)
	CreateSession(ctx context.Context, sess Session) error
}

// Service is the identity module boundary (SAD §۴.۱).
type Service struct {
	store       Store
	key         string
	ttl         time.Duration
	accessTTL   time.Duration
	maxAttempts int
	rl          *ratelimit.Limiter
	now         func() time.Time
}

// NewService wires the store, server HMAC key, OTP TTL, access-token TTL, max OTP
// attempts and the per-phone rate limiter (CIAM §۴: ۵/ساعت).
func NewService(store Store, serverKey string, otpTTL, accessTTL time.Duration, maxAttempts int, rl *ratelimit.Limiter) *Service {
	return &Service{
		store:       store,
		key:         serverKey,
		ttl:         otpTTL,
		accessTTL:   accessTTL,
		maxAttempts: maxAttempts,
		rl:          rl,
		now:         time.Now,
	}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// OTPStart creates a new OTP challenge for phone after the rate-limit check (F1).
// It returns the generated code so the SmsAdapter can deliver it; the code is
// never persisted in cleartext (CIAM §۳-۴).
func (s *Service) OTPStart(ctx context.Context, phone, purpose, ip string) (string, error) {
	if s.rl != nil && !s.rl.Allow("otp:"+phone, s.now()) {
		return "", ErrRateLimited
	}
	if purpose == "" {
		purpose = PurposeLogin
	}
	code, err := GenerateOTP()
	if err != nil {
		return "", err
	}
	hash := HashOTP(code, s.key)
	expires := s.now().Add(s.ttl)
	if _, err := s.store.CreateOTPChallenge(ctx, phone, purpose, hash, s.maxAttempts, expires, ip); err != nil {
		return "", fmt.Errorf("create otp challenge: %w", err)
	}
	return code, nil
}

// OTPVerify validates the submitted code against the active challenge, consumes it
// on success, and (for LOGIN/VERIFY_PHONE) upserts the customer by phone (F1).
func (s *Service) OTPVerify(ctx context.Context, phone, purpose, submittedCode string) (string, error) {
	// OTPStart defaults an empty purpose to LOGIN; verify must do the same, otherwise a
	// client that omits purpose on the second call looks up purpose='' , finds no
	// challenge, and is told "invalid code" — which reads as a wrong code, not a
	// missing field.
	if purpose == "" {
		purpose = PurposeLogin
	}
	ch, err := s.store.ActiveOTPChallenge(ctx, phone, purpose)
	if err != nil {
		return "", fmt.Errorf("load challenge: %w", err)
	}
	if ch == nil {
		return "", ErrChallengeNone
	}
	if ch.ConsumedAt != nil {
		return "", ErrAlreadyConsumed
	}
	if s.now().After(ch.ExpiresAt) {
		return "", ErrChallengeExpired
	}
	if ch.Attempts >= ch.MaxAttempts {
		return "", ErrMaxAttempts
	}
	if !VerifyOTP(submittedCode, ch.CodeHash, s.key) {
		if _, ierr := s.store.IncrementOTPAttempts(ctx, ch.ID); ierr != nil {
			return "", fmt.Errorf("increment attempts: %w", ierr)
		}
		return "", ErrInvalidCode
	}
	if err := s.store.ConsumeOTPChallenge(ctx, ch.ID); err != nil {
		return "", fmt.Errorf("consume challenge: %w", err)
	}
	if purpose == PurposeLogin || purpose == PurposeVerifyPhone {
		cid, err := s.store.UpsertCustomerByPhone(ctx, phone)
		if err != nil {
			return "", fmt.Errorf("upsert customer: %w", err)
		}
		return cid, nil
	}
	return "", nil
}

// IssueSession creates a per-service session for the given customer and returns
// the raw access token (returned once; only token_hash is stored — ADR-IAM-00۴).
// IssueAdminSession issues a session bound to admin_user_id (panel login).
func (s *Service) IssueAdminSession(ctx context.Context, adminUserID, serviceCode, ip, ua string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	sess := Session{
		ID:          uuid.NewString(),
		AdminUserID: adminUserID,
		ServiceCode: serviceCode,
		TokenHash:   HashOTP(token, s.key),
		ExpiresAt:   s.now().Add(s.accessTTL),
		IPAddress:   ip,
		UserAgent:   ua,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) IssueSession(ctx context.Context, customerID, serviceCode, ip, ua string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	sess := Session{
		ID:          uuid.NewString(),
		CustomerID:  customerID,
		ServiceCode: serviceCode,
		TokenHash:   HashOTP(token, s.key),
		ExpiresAt:   s.now().Add(s.accessTTL),
		IPAddress:   ip,
		UserAgent:   ua,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

// ── DBStore: pgx implementation of Store ──────────────────────────────────────

type DBStore struct {
	pool *pgxpool.Pool
}

// NewDBStore builds a Store backed by a pgxpool (connected as app_runtime).
func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateOTPChallenge(ctx context.Context, phone, purpose, codeHash string, maxAttempts int, expiresAt time.Time, ip string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO otp_challenges (phone, purpose, code_hash, max_attempts, expires_at, ip_address)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		phone, purpose, codeHash, maxAttempts, expiresAt, nullableStr(ip)).Scan(&id)
	return id, err
}

func (s *DBStore) ActiveOTPChallenge(ctx context.Context, phone, purpose string) (*OTPChallenge, error) {
	var ch OTPChallenge
	var consumed pgtype.Timestamptz
	err := s.pool.QueryRow(ctx,
		`SELECT id, phone, purpose, code_hash, attempts, max_attempts, expires_at, consumed_at
		 FROM otp_challenges
		 WHERE phone=$1 AND purpose=$2
		 ORDER BY created_at DESC LIMIT 1`,
		phone, purpose).Scan(&ch.ID, &ch.Phone, &ch.Purpose, &ch.CodeHash, &ch.Attempts, &ch.MaxAttempts, &ch.ExpiresAt, &consumed)
	if err == pgx.ErrNoRows {
		return nil, nil // no active challenge → Service returns ErrChallengeNone
	}
	if err != nil {
		return nil, err
	}
	if consumed.Valid {
		t := consumed.Time
		ch.ConsumedAt = &t
	}
	return &ch, nil
}

func (s *DBStore) IncrementOTPAttempts(ctx context.Context, id string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`UPDATE otp_challenges SET attempts = attempts + 1 WHERE id = $1 RETURNING attempts`, id).Scan(&n)
	return n, err
}

func (s *DBStore) ConsumeOTPChallenge(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE otp_challenges SET consumed_at = now() WHERE id = $1`, id)
	return err
}

func (s *DBStore) UpsertCustomerByPhone(ctx context.Context, phone string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		// The unique index on phone is PARTIAL (uq_customers_phone ... WHERE phone IS
		// NOT NULL), so the conflict target must repeat that predicate — without it
		// Postgres cannot infer the index and every login fails with 42P10.
		`INSERT INTO customers (phone, phone_verified_at)
		 VALUES ($1, now())
		 ON CONFLICT (phone) WHERE phone IS NOT NULL DO UPDATE
		   SET phone_verified_at = COALESCE(customers.phone_verified_at, now()),
		       updated_at = now()
		 RETURNING id`, phone).Scan(&id)
	return id, err
}

func (s *DBStore) CreateSession(ctx context.Context, sess Session) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO identity_sessions (id, customer_id, admin_user_id, service_code, token_hash, ip_address, user_agent, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		sess.ID, nullableStr(sess.CustomerID), nullableStr(sess.AdminUserID), sess.ServiceCode,
		sess.TokenHash, nullableStr(sess.IPAddress), sess.UserAgent, sess.ExpiresAt)
	return err
}

func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
