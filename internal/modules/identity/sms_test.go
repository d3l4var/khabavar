package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/identity"
	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// failingSMS stands in for a provider whose gateway is down.
type failingSMS struct{ calls int }

func (f *failingSMS) Name() string { return "failing" }
func (f *failingSMS) Send(context.Context, string, string) error {
	f.calls++
	return errSMSDown
}

var errSMSDown = &smsError{"gateway unreachable"}

type smsError struct{ msg string }

func (e *smsError) Error() string { return e.msg }

// TestOTPStart_ReportsDeliveryFailure pins the behaviour a customer depends on: a code that was
// never delivered must not be reported as "sent". The handler used to answer 202 regardless, so
// the user waited for an SMS that was never coming and had no way to retry.
func TestOTPStart_ReportsDeliveryFailure(t *testing.T) {
	svc := newTestService(t)
	sms := &failingSMS{}
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"phone":"09120000123","purpose":"login"}`)
	identity.OTPStartHandler(svc, sms)(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/start", body))

	if sms.calls != 1 {
		t.Fatalf("provider called %d times, want 1", sms.calls)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503; body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["error"] != "sms_unavailable" {
		t.Fatalf("error=%q want sms_unavailable", got["error"])
	}
}

// TestOTPStart_MockSMSAccepts covers the default adapter: it must never fail a login.
func TestOTPStart_MockSMSAccepts(t *testing.T) {
	svc := newTestService(t)
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"phone":"09120000124","purpose":"login"}`)
	identity.OTPStartHandler(svc, identity.MockSMS{})(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/start", body))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d want 202; body=%s", rec.Code, rec.Body.String())
	}
}

// newTestService builds a Service with a memory store, enough for the handler-level tests.
func newTestService(t *testing.T) *identity.Service {
	t.Helper()
	return identity.NewService(newMemStore(), "sms-test-signing-key-0123456789", 5*time.Minute, time.Hour, 5,
		ratelimit.New(1000, time.Hour))
}

// TestNormalizePurpose covers the closed set the otp_challenges CHECK constraint enforces.
// "login" — the obvious client spelling — used to reach the INSERT and answer 500.
func TestNormalizePurpose(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", identity.PurposeLogin, true},
		{"login", identity.PurposeLogin, true},
		{"LOGIN", identity.PurposeLogin, true},
		{" Login ", identity.PurposeLogin, true},
		{"verify_phone", identity.PurposeVerifyPhone, true},
		{"ADMIN_LOGIN", identity.PurposeAdminLogin, true},
		{"signup", "", false},
		{"'; DROP TABLE", "", false},
	}
	for _, c := range cases {
		got, ok := identity.NormalizePurpose(c.in)
		if ok != c.ok || got != c.want {
			t.Fatalf("normalizePurpose(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
