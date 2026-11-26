package identity_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/khabavar/khabavar/internal/modules/identity"
	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// memStore is an in-memory identity.Store for unit tests (no Postgres needed).
type memStore struct {
	mu         sync.Mutex
	challenges []*identity.OTPChallenge
	customers  map[string]string
	sessions   []identity.Session
}

func newMemStore() *memStore { return &memStore{customers: map[string]string{}} }

func (m *memStore) CreateOTPChallenge(_ context.Context, phone, purpose, codeHash string, maxAttempts int, expiresAt time.Time, _ string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.NewString()
	m.challenges = append(m.challenges, &identity.OTPChallenge{
		ID: id, Phone: phone, Purpose: purpose, CodeHash: codeHash,
		MaxAttempts: maxAttempts, ExpiresAt: expiresAt,
	})
	return id, nil
}

func (m *memStore) ActiveOTPChallenge(_ context.Context, phone, purpose string) (*identity.OTPChallenge, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last *identity.OTPChallenge
	for _, c := range m.challenges {
		if c.Phone == phone && c.Purpose == purpose {
			last = c // latest wins; Service checks ConsumedAt (replay → ErrAlreadyConsumed)
		}
	}
	return last, nil // nil → Service returns ErrChallengeNone
}

func (m *memStore) IncrementOTPAttempts(_ context.Context, id string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.challenges {
		if c.ID == id {
			c.Attempts++
			return c.Attempts, nil
		}
	}
	return 0, nil
}

func (m *memStore) ConsumeOTPChallenge(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := time.Now()
	for _, c := range m.challenges {
		if c.ID == id {
			c.ConsumedAt = &t
		}
	}
	return nil
}

func (m *memStore) UpsertCustomerByPhone(_ context.Context, phone string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.customers[phone]; ok {
		return id, nil
	}
	id := uuid.NewString()
	m.customers[phone] = id
	return id, nil
}

func (m *memStore) CreateSession(_ context.Context, sess identity.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = append(m.sessions, sess)
	return nil
}

func newSvc(store identity.Store, now time.Time) *identity.Service {
	rl := ratelimit.New(5, time.Hour)
	svc := identity.NewService(store, "test-server-key",
		120*time.Second, 900*time.Second, 5, rl)
	svc.SetNow(func() time.Time { return now })
	return svc
}

func TestOTPVerifyHappy(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	svc := newSvc(store, now)
	code, err := svc.OTPStart(context.Background(), "+98912000001", identity.PurposeLogin, "1.2.3.4")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	cid, err := svc.OTPVerify(context.Background(), "+98912000001", identity.PurposeLogin, code)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if cid == "" {
		t.Fatal("expected customer id after successful verify")
	}
}

func TestOTPVerifyInvalidCode(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	svc := newSvc(store, now)
	if _, err := svc.OTPStart(context.Background(), "+98912000002", identity.PurposeLogin, ""); err != nil {
		t.Fatal(err)
	}
	_, err := svc.OTPVerify(context.Background(), "+98912000002", identity.PurposeLogin, "000000")
	if err != identity.ErrInvalidCode {
		t.Fatalf("want ErrInvalidCode, got %v", err)
	}
}

func TestOTPVerifyExpired(t *testing.T) {
	store := newMemStore()
	t0 := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	rl := ratelimit.New(5, time.Hour)
	svc := identity.NewService(store, "test-server-key", 120*time.Second, 900*time.Second, 5, rl)
	clock := t0
	svc.SetNow(func() time.Time { return clock })
	code, _ := svc.OTPStart(context.Background(), "+98912000003", identity.PurposeLogin, "")
	clock = t0.Add(3 * time.Minute) // past 120s TTL
	_, err := svc.OTPVerify(context.Background(), "+98912000003", identity.PurposeLogin, code)
	if err != identity.ErrChallengeExpired {
		t.Fatalf("want ErrChallengeExpired, got %v", err)
	}
}

func TestOTPStartRateLimited(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	svc := newSvc(store, now)
	for i := 0; i < 5; i++ {
		if _, err := svc.OTPStart(context.Background(), "+98912000004", identity.PurposeLogin, ""); err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	_, err := svc.OTPStart(context.Background(), "+98912000004", identity.PurposeLogin, "")
	if err != identity.ErrRateLimited {
		t.Fatalf("6th start want ErrRateLimited (CIAM §۴: ۵/ساعت), got %v", err)
	}
}

func TestOTPVerifyAlreadyConsumed(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	svc := newSvc(store, now)
	code, _ := svc.OTPStart(context.Background(), "+98912000005", identity.PurposeLogin, "")
	if _, err := svc.OTPVerify(context.Background(), "+98912000005", identity.PurposeLogin, code); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	_, err := svc.OTPVerify(context.Background(), "+98912000005", identity.PurposeLogin, code)
	if err != identity.ErrAlreadyConsumed {
		t.Fatalf("second verify want ErrAlreadyConsumed, got %v", err)
	}
}

func TestIssueSession(t *testing.T) {
	store := newMemStore()
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	svc := newSvc(store, now)
	token, err := svc.IssueSession(context.Background(), uuid.NewString(), "shop", "1.2.3.4", "test")
	if err != nil {
		t.Fatalf("issue session: %v", err)
	}
	if len(token) != 64 { // 32 bytes hex
		t.Fatalf("token length = %d, want 64", len(token))
	}
	if len(store.sessions) != 1 {
		t.Fatalf("expected 1 session stored, got %d", len(store.sessions))
	}
	if store.sessions[0].TokenHash == token {
		t.Fatal("stored hash must differ from raw token (ADR-IAM-00۴)")
	}
}
