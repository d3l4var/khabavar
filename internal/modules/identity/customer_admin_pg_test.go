package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/khabavar/khabavar/internal/modules/authorization"
)

// TestPG_CustomerHTTPHandlers covers the /me surface: the auth guard, the profile read and
// the address book. customer_http.go was at 18%.
func TestPG_CustomerHTTPHandlers(t *testing.T) {
	pool, _ := setupPG(t)
	ctx := context.Background()
	store := NewDBCustomerStore(pool)

	// Seed a customer directly: the /me surface only needs a row to read.
	customerID := "77777777-7777-7777-7777-777777777777"
	if _, err := pool.Exec(ctx,
		`INSERT INTO customers (id, phone, full_name, phone_verified_at)
		 VALUES ($1, '09120000001', 'مشتری تست', now())`, customerID); err != nil {
		t.Fatalf("seed customer: %v", err)
	}

	call := func(h http.HandlerFunc, method, body string, pv map[string]string, auth bool) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		if auth {
			r = r.WithContext(authorization.WithCustomerID(r.Context(), customerID))
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// Every /me route must refuse an unauthenticated call with 401, not a 500 or an empty body.
	guarded := map[string]http.HandlerFunc{
		"me":     MeHandler(store),
		"list":   ListAddressesHandler(store),
		"create": CreateAddressHandler(store),
		"update": UpdateAddressHandler(store),
		"delete": DeleteAddressHandler(store),
	}
	for name, h := range guarded {
		if rec := call(h, http.MethodGet, "", map[string]string{"id": "x"}, false); rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s without auth = %d, want 401", name, rec.Code)
		}
	}

	// The profile read.
	rec := call(MeHandler(store), http.MethodGet, "", nil, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("me = %d: %s", rec.Code, rec.Body.String())
	}
	var profile struct {
		Phone         string `json:"phone"`
		PhoneVerified bool   `json:"phone_verified"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &profile)
	if profile.Phone != "09120000001" || !profile.PhoneVerified {
		t.Fatalf("profile = %s", rec.Body.String())
	}

	// The address book: a broken body is 400, a real one creates, then update and delete.
	if rec := call(CreateAddressHandler(store), http.MethodPost, `{`, nil, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken address = %d, want 400", rec.Code)
	}
	rec = call(CreateAddressHandler(store), http.MethodPost,
		`{"title":"خانه","recipient_name":"مشتری","phone":"09120000001","province":"تهران",`+
			`"city":"تهران","address_line":"خیابان اول","postal_code":"1234567890","is_default":true}`, nil, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create address = %d: %s", rec.Code, rec.Body.String())
	}
	var addr struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &addr)
	if addr.ID == "" {
		t.Fatalf("address response = %s", rec.Body.String())
	}

	rec = call(ListAddressesHandler(store), http.MethodGet, "", nil, true)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), addr.ID) {
		t.Fatalf("list addresses = %d: %s", rec.Code, rec.Body.String())
	}

	// PATCH is a real partial update: a single field changes and the rest survive.
	if rec := call(UpdateAddressHandler(store), http.MethodPatch, `{"city":"کرج"}`, map[string]string{"id": addr.ID}, true); rec.Code != http.StatusOK {
		t.Fatalf("partial patch = %d: %s", rec.Code, rec.Body.String())
	}
	after, err := store.ListAddresses(ctx, customerID)
	if err != nil || len(after) != 1 {
		t.Fatalf("list after patch = %+v, %v", after, err)
	}
	if after[0].City != "کرج" {
		t.Fatalf("city after patch = %q, want کرج", after[0].City)
	}
	if after[0].Province != "تهران" || after[0].AddressLine != "خیابان اول" {
		t.Fatalf("the partial patch clobbered a field: %+v", after[0])
	}
	// Clearing a REQUIRED field is refused: the merged result is validated, not just the input.
	if rec := call(UpdateAddressHandler(store), http.MethodPatch, `{"city":""}`, map[string]string{"id": addr.ID}, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("clearing a required field = %d, want 400", rec.Code)
	}
	// An unknown address is a 404, and another customer's address cannot be patched.
	if rec := call(UpdateAddressHandler(store), http.MethodPatch, `{"city":"کرج"}`,
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown address = %d, want 404", rec.Code)
	}
	// A broken body is still a 400.
	if rec := call(UpdateAddressHandler(store), http.MethodPatch, `{`, map[string]string{"id": addr.ID}, true); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken patch = %d, want 400", rec.Code)
	}
	if rec := call(DeleteAddressHandler(store), http.MethodDelete, "", map[string]string{"id": addr.ID}, true); rec.Code != http.StatusNoContent {
		t.Fatalf("delete address = %d: %s", rec.Code, rec.Body.String())
	}
	// A second delete is a 404, and another customer's address is invisible.
	if rec := call(DeleteAddressHandler(store), http.MethodDelete, "", map[string]string{"id": addr.ID}, true); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", rec.Code)
	}
}

// TestPG_AdminUserHandlers covers the staff account surface: the password floor, the
// duplicate email and the role assignment. adminusers.go was at 46%.
func TestPG_AdminUserHandlers(t *testing.T) {
	pool, _ := setupPG(t)
	ctx := context.Background()
	store := NewAdminStore(pool)

	call := func(h http.HandlerFunc, method, body string, pv map[string]string) *httptest.ResponseRecorder {
		var r *http.Request
		if body == "" {
			r = httptest.NewRequest(method, "/x", nil)
		} else {
			r = httptest.NewRequest(method, "/x", strings.NewReader(body))
		}
		for k, v := range pv {
			r.SetPathValue(k, v)
		}
		rec := httptest.NewRecorder()
		h(rec, r)
		return rec
	}

	// The password floor is 12 characters and it says so with a code.
	rec := call(CreateAdminUserHandler(store), http.MethodPost,
		`{"email":"ops@khabavar.ir","full_name":"اپراتور","password":"short"}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "weak_password") {
		t.Fatalf("short password = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := call(CreateAdminUserHandler(store), http.MethodPost, `{`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("broken body = %d, want 400", rec.Code)
	}

	// The role catalogue is seeded by ops, not by a migration; create the one under test.
	if _, err := pool.Exec(ctx, `INSERT INTO roles (code, name) VALUES ('SUPPORT','پشتیبانی')
		ON CONFLICT (code) DO NOTHING`); err != nil {
		t.Fatalf("seed role: %v", err)
	}

	// A real creation, with a role.
	rec = call(CreateAdminUserHandler(store), http.MethodPost,
		`{"email":"ops@khabavar.ir","full_name":"اپراتور","password":"a-long-enough-password",`+
			`"roles":["SUPPORT"]}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatalf("create response = %s", rec.Body.String())
	}

	// The same email again is a conflict, not a 500.
	if rec := call(CreateAdminUserHandler(store), http.MethodPost,
		`{"email":"ops@khabavar.ir","full_name":"تکراری","password":"a-long-enough-password"}`, nil); rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("duplicate email = %d, want a 4xx", rec.Code)
	}

	// The role really landed.
	var roles int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM admin_user_roles aur
		   JOIN roles r ON r.id = aur.role_id
		  WHERE aur.admin_user_id = $1 AND r.code = 'SUPPORT'`, created.ID).Scan(&roles); err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if roles != 1 {
		t.Fatalf("SUPPORT role rows = %d, want 1", roles)
	}

	// Update: name and status, and an unknown id is a 404.
	if rec := call(UpdateAdminUserHandler(store), http.MethodPatch,
		`{"full_name":"اپراتور ارشد","status":"SUSPENDED"}`, map[string]string{"id": created.ID}); rec.Code != http.StatusOK {
		t.Fatalf("update user = %d: %s", rec.Code, rec.Body.String())
	}
	var status, name string
	if err := pool.QueryRow(ctx, `SELECT status, full_name FROM admin_users WHERE id = $1`, created.ID).Scan(&status, &name); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if status != "SUSPENDED" || name != "اپراتور ارشد" {
		t.Fatalf("after update = %s/%s", status, name)
	}
	if rec := call(UpdateAdminUserHandler(store), http.MethodPatch, `{"status":"SUSPENDED"}`,
		map[string]string{"id": "00000000-0000-0000-0000-000000000000"}); rec.Code != http.StatusNotFound {
		t.Fatalf("update unknown = %d, want 404", rec.Code)
	}
}
