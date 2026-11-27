package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAdminLoginGuardCapsBothDimensions pins the two ceilings: an IP cap alone lets a botnet
// spread attempts, an email cap alone lets an attacker lock a colleague out.
func TestAdminLoginGuardCapsBothDimensions(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := NewAdminLoginGuard(3, 2, 15*time.Minute)
	g.now = func() time.Time { return now }

	// Per-email ceiling: a third attempt for the same account is refused even from a new IP.
	if !g.Allow("1.1.1.1", "a@x.ir") || !g.Allow("2.2.2.2", "a@x.ir") {
		t.Fatal("first two attempts for one account must pass")
	}
	if g.Allow("3.3.3.3", "a@x.ir") {
		t.Fatal("third attempt for the same account must be refused")
	}
	// Per-IP ceiling: a different account from the same IP still counts against the IP.
	if !g.Allow("1.1.1.1", "b@x.ir") {
		t.Fatal("second account from the same IP must pass")
	}
	// Third attempt from 1.1.1.1 is still within the per-IP budget of 3.
	if !g.Allow("1.1.1.1", "c@x.ir") {
		t.Fatal("third attempt from one IP must pass")
	}
	if g.Allow("1.1.1.1", "e@x.ir") {
		t.Fatal("fourth attempt from one IP must be refused")
	}
	// A different IP and account is unaffected.
	if !g.Allow("9.9.9.9", "d@x.ir") {
		t.Fatal("a fresh IP and account must pass")
	}
	// The window is a window, not a lifetime.
	now = now.Add(16 * time.Minute)
	if !g.Allow("1.1.1.1", "a@x.ir") {
		t.Fatal("after the window the budget must reset")
	}
}

// TestAdminLoginThrottleAnswers429 covers the handler: a throttled attempt is a 429 with a JSON
// body, and it is audited — a lockout without a trail cannot be investigated.
func TestAdminLoginThrottleAnswers429(t *testing.T) {
	var audited []string
	aud := recorderLoginAuditor{fn: func(action string, nv map[string]string) {
		audited = append(audited, action+"|"+nv["reason"])
	}}
	g := NewAdminLoginGuard(1, 1, time.Minute)
	// A real service with a store that knows no accounts: the first attempt reaches
	// authentication and fails with ErrAdminCredentials, the second never gets that far.
	svc := NewService(nil, "test-key-0123456789", time.Minute, time.Hour, 5, nil)
	h := AdminLoginHandler(svc, noAccountsStore{}, WithAdminLoginGuard(g, aud))

	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/admin/login",
			strings.NewReader(`{"email":"a@x.ir","password":"wrong"}`))
		r.RemoteAddr = "5.5.5.5:1234"
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// The first attempt is allowed through to authentication, which fails on nil store.
	post()
	// The second is throttled before any password work.
	rec := post()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second attempt = %d, want 429", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 body is not JSON: %q", rec.Body.String())
	}
	if !strings.Contains(strings.Join(audited, ","), "throttled") {
		t.Fatalf("throttled attempt was not audited: %v", audited)
	}
}

// noAccountsStore answers "no such admin" for every email.
type noAccountsStore struct{}

func (noAccountsStore) FindAdminByEmail(context.Context, string) (*AdminCredential, error) {
	return nil, nil
}

type recorderLoginAuditor struct {
	fn func(action string, newValues map[string]string)
}

func (r recorderLoginAuditor) Record(_ context.Context, _, _, action, _, _ string, _, newValues any, _ string) error {
	nv, _ := newValues.(map[string]string)
	r.fn(action, nv)
	return nil
}
