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

// TestRequireAuditedRecordsSuccess pins the central trail: a successful mutation is audited,
// a read is not, and a failed mutation is not recorded as if it happened. Before this only
// denials were audited, so you could answer "who was refused" but not "who changed this".
func TestRequireAuditedRecordsSuccess(t *testing.T) {
	type entry struct {
		action, entityType string
		newValues          map[string]string
	}
	var got []entry
	aud := recorderAuditor{fn: func(action, entityType string, nv map[string]string) {
		got = append(got, entry{action, entityType, nv})
	}}
	allow := allowChecker{}
	ctx := WithAdminUserID(context.Background(), "admin-1")

	run := func(method string, status int) {
		got = nil
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) })
		r := httptest.NewRequest(method, "/api/v1/admin/products", nil).WithContext(ctx)
		RequireAudited(allow, aud, "catalog.manage", next).ServeHTTP(httptest.NewRecorder(), r)
	}

	// A successful POST is recorded, with the permission and the path.
	run(http.MethodPost, http.StatusCreated)
	if len(got) != 1 {
		t.Fatalf("POST 201 wrote %d audit entries, want 1", len(got))
	}
	if got[0].action != "admin.post" || got[0].newValues["permission"] != "catalog.manage" ||
		got[0].newValues["status"] != "201" || got[0].newValues["path"] != "/api/v1/admin/products" {
		t.Fatalf("unexpected entry: %+v", got[0])
	}

	// A failed mutation is not recorded as a success.
	run(http.MethodPost, http.StatusBadRequest)
	if len(got) != 0 {
		t.Fatalf("POST 400 wrote %d audit entries, want 0", len(got))
	}

	// A read is not a mutation.
	run(http.MethodGet, http.StatusOK)
	if len(got) != 0 {
		t.Fatalf("GET wrote %d audit entries, want 0", len(got))
	}

	// Every mutating method is covered.
	for _, m := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete} {
		run(m, http.StatusOK)
		if len(got) != 1 {
			t.Fatalf("%s wrote %d audit entries, want 1", m, len(got))
		}
	}
}

type allowChecker struct{}

func (allowChecker) HasPermission(context.Context, string, string) (bool, error) { return true, nil }

type recorderAuditor struct {
	fn func(action, entityType string, newValues map[string]string)
}

func (r recorderAuditor) Record(_ context.Context, _, _, action, entityType, _ string,
	_, newValues any, _ string) error {
	nv, _ := newValues.(map[string]string)
	r.fn(action, entityType, nv)
	return nil
}
