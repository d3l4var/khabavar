package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestPG_AdminUserLifecycle covers staff account management: creation with roles resolved
// by code, the duplicate-email guard, the unknown-role guard, and a role replacement that
// leaves a name-only PATCH untouched.
func TestPG_AdminUserLifecycle(t *testing.T) {
	pool, _ := setupPG(t)
	store := NewAdminStore(pool)
	ctx := context.Background()

	// testpg gives a clean schema without the operator seed, so the roles under test are
	// created here; the property being checked is resolution BY CODE, not the seed content.
	for _, code := range []string{"STORE_MANAGER", "FINANCE"} {
		if _, err := pool.Exec(ctx, `INSERT INTO roles (code, name) VALUES ($1,$2) ON CONFLICT DO NOTHING`, code, code); err != nil {
			t.Fatalf("seed role %s: %v", code, err)
		}
	}

	email := "staff-" + uuid.NewString()[:8] + "@khabavar.ir"
	id, err := store.CreateAdminUser(ctx, email, "کارمند تازه", "$2a$12$abcdefghijklmnopqrstuv", []string{"STORE_MANAGER"})
	if err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}

	// The same email twice is a conflict, not a second account.
	if _, err := store.CreateAdminUser(ctx, email, "دیگری", "$2a$12$abcdefghijklmnopqrstuv", nil); !errors.Is(err, ErrAdminEmailTaken) {
		t.Fatalf("duplicate email = %v, want ErrAdminEmailTaken", err)
	}
	// An unknown role code must fail the whole insert, not create a roleless account.
	if _, err := store.CreateAdminUser(ctx, "bad-"+email, "x", "$2a$12$abcdefghijklmnopqrstuv", []string{"NO_SUCH_ROLE"}); !errors.Is(err, ErrAdminRoleUnknown) {
		t.Fatalf("unknown role = %v, want ErrAdminRoleUnknown", err)
	}
	var orphans int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM admin_users WHERE email = $1`, "bad-"+email).Scan(&orphans); err != nil {
		t.Fatalf("count: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("a failed role assignment left %d account(s) behind", orphans)
	}

	// A name-only update keeps the roles.
	if err := store.UpdateAdminUser(ctx, id, "نام تازه", "", nil); err != nil {
		t.Fatalf("UpdateAdminUser (name): %v", err)
	}
	users, _, err := store.ListAdminUsers(ctx, 1, 200)
	if err != nil {
		t.Fatalf("ListAdminUsers: %v", err)
	}
	var got AdminUser
	for _, u := range users {
		if u.ID == id {
			got = u
		}
	}
	if got.FullName != "نام تازه" || len(got.Roles) != 1 || got.Roles[0] != "STORE_MANAGER" {
		t.Fatalf("after name update = %+v, want the new name and the same role", got)
	}

	// Replacing the roles works, and suspending is recorded.
	roles := []string{"FINANCE"}
	if err := store.UpdateAdminUser(ctx, id, "", "SUSPENDED", &roles); err != nil {
		t.Fatalf("UpdateAdminUser (roles+status): %v", err)
	}
	users, _, _ = store.ListAdminUsers(ctx, 1, 200)
	for _, u := range users {
		if u.ID == id {
			got = u
		}
	}
	if got.Status != "SUSPENDED" || len(got.Roles) != 1 || got.Roles[0] != "FINANCE" {
		t.Fatalf("after role replacement = %+v", got)
	}

	if err := store.UpdateAdminUser(ctx, uuid.NewString(), "", "ACTIVE", nil); !errors.Is(err, ErrAdminUserNotFound) {
		t.Fatalf("unknown id = %v, want ErrAdminUserNotFound", err)
	}
}
