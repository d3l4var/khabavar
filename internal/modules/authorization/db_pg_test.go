// db_pg_test.go exercises DBChecker against a real Postgres. Skipped unless
// KH_TEST_PG_DSN is set (see platform/testpg).
package authorization_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/khabavar/khabavar/internal/modules/authorization"
	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

func TestDBCheckerReadsSeededMatrix(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var storeRoleID, financeRoleID, viewPermID, refundPermID string
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	must(pool.QueryRow(ctx, `INSERT INTO roles (code, name) VALUES ('STORE_MANAGER','مدیر فروشگاه') RETURNING id`).Scan(&storeRoleID), "insert store role")
	must(pool.QueryRow(ctx, `INSERT INTO roles (code, name) VALUES ('FINANCE','مالی') RETURNING id`).Scan(&financeRoleID), "insert finance role")
	must(pool.QueryRow(ctx, `INSERT INTO permissions (code, resource, action) VALUES ('order.view','commerce','view') RETURNING id`).Scan(&viewPermID), "insert view permission")
	must(pool.QueryRow(ctx, `INSERT INTO permissions (code, resource, action) VALUES ('order.refund.approve','commerce','refund') RETURNING id`).Scan(&refundPermID), "insert refund permission")

	// STORE_MANAGER may view orders but never approve refunds (IAM §۴).
	_, err := pool.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1,$2)`, storeRoleID, viewPermID)
	must(err, "grant view to store manager")
	_, err = pool.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1,$2)`, financeRoleID, refundPermID)
	must(err, "grant refund to finance")

	activeID := uuid.NewString()
	suspendedID := uuid.NewString()
	_, err = pool.Exec(ctx, `INSERT INTO admin_users (id, email, full_name, status) VALUES ($1,'a@x.ir','Active','ACTIVE')`, activeID)
	must(err, "insert active admin")
	_, err = pool.Exec(ctx, `INSERT INTO admin_users (id, email, full_name, status) VALUES ($1,'b@x.ir','Suspended','SUSPENDED')`, suspendedID)
	must(err, "insert suspended admin")
	_, err = pool.Exec(ctx, `INSERT INTO admin_user_roles (admin_user_id, role_id) VALUES ($1,$2)`, activeID, storeRoleID)
	must(err, "assign store role")
	_, err = pool.Exec(ctx, `INSERT INTO admin_user_roles (admin_user_id, role_id) VALUES ($1,$2)`, suspendedID, financeRoleID)
	must(err, "assign finance role")

	chk := authorization.NewDBChecker(pool)

	ok, err := chk.HasPermission(ctx, activeID, "order.view")
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if !ok {
		t.Error("STORE_MANAGER must hold order.view")
	}

	ok, err = chk.HasPermission(ctx, activeID, "order.refund.approve")
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if ok {
		t.Error("STORE_MANAGER must NOT hold order.refund.approve — refund is FINANCE-exclusive")
	}

	// A suspended staff member holds nothing, even though the role grants the code.
	ok, err = chk.HasPermission(ctx, suspendedID, "order.refund.approve")
	if err != nil {
		t.Fatalf("HasPermission: %v", err)
	}
	if ok {
		t.Error("a SUSPENDED admin must hold no permissions")
	}

	// Malformed ids are "no permission", never an error (no 500 from a bad token).
	ok, err = chk.HasPermission(ctx, "not-a-uuid", "order.view")
	if err != nil || ok {
		t.Errorf("malformed admin id: want (false,nil), got (%v,%v)", ok, err)
	}

	// Unknown permission code is false, not an error.
	ok, err = chk.HasPermission(ctx, activeID, "settings.manage")
	if err != nil || ok {
		t.Errorf("ungranted permission: want (false,nil), got (%v,%v)", ok, err)
	}
}
