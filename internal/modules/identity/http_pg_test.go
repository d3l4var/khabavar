package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/platform/ratelimit"
)

// TestPG_OTPFlowThroughHandlers drives the real auth surface end to end: the OTP start
// handler issues a code, the verify handler exchanges it for a session token, and the
// token then opens /me. These handlers had no tests at all — only manual curl checks.
func TestPG_OTPFlowThroughHandlers(t *testing.T) {
	pool, _ := setupPG(t)
	store := NewDBStore(pool)
	svc := NewService(store, "handler-signing-key-0123456789", 5*time.Minute, time.Hour, 5,
		ratelimit.New(100, time.Hour))
	ctx := context.Background()

	var delivered struct{ phone, code string }
	start := OTPStartHandler(svc, captureSMS{&delivered})

	post := func(h http.HandlerFunc, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// A broken body and a missing phone are 400.
	if rec := post(start, `{`); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}
	if rec := post(start, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing phone = %d, want 400", rec.Code)
	}

	// A real start delivers a code.
	phone := "09120000123"
	if rec := post(start, `{"phone":"`+phone+`"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body.String())
	}
	if delivered.code == "" || delivered.phone != phone {
		t.Fatalf("delivered = %+v", delivered)
	}

	verify := OTPVerifyHandler(svc)
	// A wrong code is refused.
	if rec := post(verify, `{"phone":"`+phone+`","code":"000000"}`); rec.Code < 400 {
		t.Fatalf("wrong code = %d, want a 4xx", rec.Code)
	}
	// The right code issues a token.
	rec := post(verify, `{"phone":"`+phone+`","code":"`+delivered.code+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Service     string `json:"service"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.AccessToken == "" || out.Service != "shop" {
		t.Fatalf("verify response = %s", rec.Body.String())
	}

	// The issued token resolves to that customer, and /me serves the profile.
	resolver := DBSubjectResolver(pool, "handler-signing-key-0123456789")
	subj, ok := resolver(ctx, out.AccessToken)
	if !ok || subj.ID == "" {
		t.Fatalf("resolver = %+v/%v, want a customer subject", subj, ok)
	}

	me := MeHandler(NewDBCustomerStore(pool))
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r = r.WithContext(authorization.WithCustomerID(r.Context(), subj.ID))
	recMe := httptest.NewRecorder()
	me(recMe, r)
	if recMe.Code != http.StatusOK {
		t.Fatalf("/me = %d: %s", recMe.Code, recMe.Body.String())
	}
	// Without an identity it is a 401, never a 200 with an empty body.
	r = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	recAnon := httptest.NewRecorder()
	me(recAnon, r)
	if recAnon.Code != http.StatusUnauthorized {
		t.Fatalf("/me without identity = %d, want 401", recAnon.Code)
	}
}
