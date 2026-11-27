package authorization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWriteAuthErrIsJSON pins the envelope: a 401/403 used to be text/plain, so a client that
// always parses JSON got a parse error on exactly the responses it needs to branch on.
func TestWriteAuthErrIsJSON(t *testing.T) {
	for _, c := range []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
	} {
		rec := httptest.NewRecorder()
		writeAuthErr(rec, c.status, c.code)
		if rec.Code != c.status {
			t.Fatalf("status = %d, want %d", rec.Code, c.status)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Fatalf("content-type = %q", ct)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Fatal("WWW-Authenticate header is missing on a 401")
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("body is not JSON: %q", rec.Body.String())
		}
		if body["code"] != c.code {
			t.Fatalf("code = %q, want %q", body["code"], c.code)
		}
	}
}

// TestRequireAnswersJSON covers the guard itself: no identity is a JSON 401, a denied
// permission a JSON 403.
func TestRequireAnswersJSON(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	// No admin identity in the context.
	rec := httptest.NewRecorder()
	Require(denyChecker{}, "order.view", next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no identity = %d, want 401", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("401 body is not JSON: %q", rec.Body.String())
	}

	// An identity without the permission.
	ctx := WithAdminUserID(context.Background(), "admin-1")
	rec = httptest.NewRecorder()
	Require(denyChecker{}, "order.view", next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil).WithContext(ctx))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("denied = %d, want 403", rec.Code)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["code"] != "forbidden" {
		t.Fatalf("403 body = %q", rec.Body.String())
	}
}

type denyChecker struct{}

func (denyChecker) HasPermission(context.Context, string, string) (bool, error) { return false, nil }

// TestOptionalCustomer covers the public-with-identity routes: a valid customer bearer is
// attached, and anything else (no token, a staff token, a bad token) continues anonymously
// instead of failing — the cart and checkout routes must work for guests.
func TestOptionalCustomer(t *testing.T) {
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = CustomerID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	resolve := func(_ context.Context, bearer string) (string, string, bool) {
		switch bearer {
		case "customer-token":
			return "customer", "cust-1", true
		case "staff-token":
			return "admin", "admin-1", true
		}
		return "", "", false
	}
	h := OptionalCustomer(resolve, next)

	cases := []struct {
		name   string
		bearer string
		want   string
	}{
		{"no token", "", ""},
		{"customer", "customer-token", "cust-1"},
		{"staff token is not a customer", "staff-token", ""},
		{"bad token", "garbage", ""},
	}
	for _, c := range cases {
		seen = "unset"
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		if c.bearer != "" {
			r.Header.Set("Authorization", "Bearer "+c.bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (never a 401 on an optional route)", c.name, rec.Code)
		}
		if seen != c.want {
			t.Fatalf("%s: customer id = %q, want %q", c.name, seen, c.want)
		}
	}
}
