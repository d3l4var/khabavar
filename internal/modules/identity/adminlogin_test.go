package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// fakeStore implements the identity Store for these tests and records the session.
type fakeStore struct {
	sessions []Session
}

func (f *fakeStore) CreateOTPChallenge(context.Context, string, string, string, int, time.Time, string) (string, error) {
	return "", nil
}
func (f *fakeStore) ActiveOTPChallenge(context.Context, string, string) (*OTPChallenge, error) {
	return nil, nil
}
func (f *fakeStore) IncrementOTPAttempts(context.Context, string) (int, error) { return 0, nil }
func (f *fakeStore) ConsumeOTPChallenge(context.Context, string) error         { return nil }
func (f *fakeStore) UpsertCustomerByPhone(context.Context, string) (string, error) {
	return "", nil
}
func (f *fakeStore) CreateSession(_ context.Context, sess Session) error {
	f.sessions = append(f.sessions, sess)
	return nil
}

func (f *fakeStore) lastSession() (Session, error) {
	if len(f.sessions) == 0 {
		return Session{}, errors.New("no session recorded")
	}
	return f.sessions[len(f.sessions)-1], nil
}

type fakeAdminStore struct {
	cred *AdminCredential
	err  error
}

func (f *fakeAdminStore) FindAdminByEmail(_ context.Context, email string) (*AdminCredential, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.cred == nil || f.cred.Email != email {
		return nil, nil
	}
	return f.cred, nil
}

func adminFixture(t *testing.T, password, status string) (*Service, *fakeAdminStore) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	store := &fakeAdminStore{cred: &AdminCredential{
		ID: "adm-1", Email: "root@khabavar.ir", PasswordHash: string(hash), Status: status,
	}}
	svc := NewService(&fakeStore{}, "test-key", time.Minute, time.Hour, 5, nil)
	return svc, store
}

// The happy path must issue a session bound to admin_user_id, not to a customer.
func TestAdminLogin_IssuesAdminSession(t *testing.T) {
	svc, store := adminFixture(t, "Khabavar!Dev2026", "ACTIVE")
	token, err := svc.AdminLogin(context.Background(), store, "ROOT@khabavar.ir ", "Khabavar!Dev2026", "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("AdminLogin: %v", err)
	}
	if token == "" {
		t.Fatal("expected a session token")
	}
	sess, err := svc.store.(*fakeStore).lastSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if sess.AdminUserID != "adm-1" {
		t.Fatalf("admin_user_id = %q, want adm-1", sess.AdminUserID)
	}
	if sess.CustomerID != "" {
		t.Fatalf("customer_id = %q, a staff session must not be a customer session", sess.CustomerID)
	}
}

// A wrong password and an unknown email must be indistinguishable to the caller.
func TestAdminLogin_RejectsWrongPasswordAndUnknownEmail(t *testing.T) {
	svc, store := adminFixture(t, "Khabavar!Dev2026", "ACTIVE")
	ctx := context.Background()

	if _, err := svc.AdminLogin(ctx, store, "root@khabavar.ir", "wrong", "", ""); !errors.Is(err, ErrAdminCredentials) {
		t.Fatalf("wrong password: %v, want ErrAdminCredentials", err)
	}
	if _, err := svc.AdminLogin(ctx, store, "nobody@khabavar.ir", "Khabavar!Dev2026", "", ""); !errors.Is(err, ErrAdminCredentials) {
		t.Fatalf("unknown email: %v, want ErrAdminCredentials", err)
	}
}

// A suspended account must not receive a session even with the right password.
func TestAdminLogin_RejectsInactiveAccount(t *testing.T) {
	svc, store := adminFixture(t, "Khabavar!Dev2026", "SUSPENDED")
	_, err := svc.AdminLogin(context.Background(), store, "root@khabavar.ir", "Khabavar!Dev2026", "", "")
	if !errors.Is(err, ErrAdminInactive) {
		t.Fatalf("inactive account: %v, want ErrAdminInactive", err)
	}
}
