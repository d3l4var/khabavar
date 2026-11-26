// seed_test.go is the RBAC matrix gate (IAM §۴, TS §۴). It applies the real migrations
// and the real seed to a throwaway schema and asserts the approved role → permission
// matrix, so a careless seed edit cannot widen access without a red test.
//
// Skipped unless KH_TEST_PG_DSN is set (see platform/testpg).
package seed

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// setup applies migrations + the baseline seed and returns the pool.
func setup(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	ctx := context.Background()
	if err := migrations.ApplyAll(ctx, pool, nil); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	sql, err := BaselineSQL()
	if err != nil {
		t.Fatalf("read baseline seed: %v", err)
	}
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatalf("apply seed: %v", err)
	}
	return pool
}

// permissionsOf returns the permission codes granted to a role.
func permissionsOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, role string) map[string]bool {
	t.Helper()
	rows, err := pool.Query(ctx,
		`SELECT p.code FROM role_permissions rp
		   JOIN roles r ON r.id = rp.role_id
		   JOIN permissions p ON p.id = rp.permission_id
		  WHERE r.code = $1`, role)
	if err != nil {
		t.Fatalf("query permissions of %s: %v", role, err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[code] = true
	}
	return got
}

func TestSeedCatalogueCounts(t *testing.T) {
	pool := setup(t)
	ctx := context.Background()

	var roles, perms, clients, topics int
	for _, q := range []struct {
		sql  string
		dest *int
	}{
		{`SELECT count(*) FROM roles`, &roles},
		{`SELECT count(*) FROM permissions`, &perms},
		{`SELECT count(*) FROM oauth_clients`, &clients},
		{`SELECT count(*) FROM topics`, &topics},
	} {
		if err := pool.QueryRow(ctx, q.sql).Scan(q.dest); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	if roles != 10 {
		t.Errorf("roles = %d, want 10 (IAM §۲)", roles)
	}
	if perms != 24 {
		t.Errorf("permissions = %d, want 24 (IAM §۳)", perms)
	}
	if clients != 4 {
		t.Errorf("oauth_clients = %d, want 4 (IAM §۶)", clients)
	}
	if topics != 11 {
		t.Errorf("topics = %d, want 11 (DB v2 §12.2)", topics)
	}
}

// TestSeedSuperAdminHoldsEveryPermission pins the IAM §۴ convention: SUPER_ADMIN is
// granted all 24 codes by the seed, not by a code path.
func TestSeedSuperAdminHoldsEveryPermission(t *testing.T) {
	pool := setup(t)
	ctx := context.Background()
	got := permissionsOf(t, ctx, pool, "SUPER_ADMIN")
	if len(got) != 24 {
		t.Fatalf("SUPER_ADMIN holds %d permissions, want 24", len(got))
	}
}

// TestSeedRefundIsFinanceExclusive is the rule IAM §۴ calls out explicitly. SUPER_ADMIN
// is the only other holder (it holds everything by convention).
func TestSeedRefundIsFinanceExclusive(t *testing.T) {
	pool := setup(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx,
		`SELECT r.code FROM role_permissions rp
		   JOIN roles r ON r.id = rp.role_id
		   JOIN permissions p ON p.id = rp.permission_id
		  WHERE p.code = 'order.refund.approve' ORDER BY r.code`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	var holders []string
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			t.Fatalf("scan: %v", err)
		}
		holders = append(holders, code)
	}
	want := []string{"FINANCE", "SUPER_ADMIN"}
	if len(holders) != len(want) {
		t.Fatalf("order.refund.approve holders = %v, want %v", holders, want)
	}
	for i := range want {
		if holders[i] != want[i] {
			t.Fatalf("order.refund.approve holders = %v, want %v", holders, want)
		}
	}
}

// TestSeedRoleMatrixSpotChecks verifies the least-privilege decisions the matrix calls
// out: support is read-only, editors cannot publish, suppliers only manage suppliers.
func TestSeedRoleMatrixSpotChecks(t *testing.T) {
	pool := setup(t)
	ctx := context.Background()

	cases := []struct {
		role    string
		must    []string
		mustNot []string
	}{
		{
			role:    "SUPPORT_OPERATOR",
			must:    []string{"order.view", "identity.customer.view"},
			mustNot: []string{"order.manage", "order.cancel.approve", "order.refund.approve"},
		},
		{
			role:    "EDITOR",
			must:    []string{"content.article.edit", "catalog.product.view"},
			mustNot: []string{"content.article.publish", "content.article.review"},
		},
		{
			role:    "CONTENT_REVIEWER",
			must:    []string{"content.article.review", "content.article.edit"},
			mustNot: []string{"content.article.publish"},
		},
		{
			role:    "SUPPLIER_OPERATOR",
			must:    []string{"supplier.manage", "supplier.evaluate"},
			mustNot: []string{"inventory.manage", "pricing.price.approve", "catalog.product.edit"},
		},
		{
			role:    "INVENTORY_MANAGER",
			must:    []string{"inventory.manage", "inventory.adjust", "catalog.product.view"},
			mustNot: []string{"inventory.reserve", "order.manage", "pricing.price.approve"},
		},
		{
			role:    "ORDER_OPERATOR",
			must:    []string{"order.view", "order.manage", "order.cancel.approve", "inventory.manage"},
			mustNot: []string{"order.refund.approve", "pricing.price.approve", "identity.user.manage"},
		},
		{
			role:    "FINANCE",
			must:    []string{"order.refund.approve", "pricing.cost.manage", "pricing.price.approve", "audit.read"},
			mustNot: []string{"order.manage", "catalog.product.edit", "identity.user.manage"},
		},
		{
			role:    "STORE_MANAGER",
			must:    []string{"catalog.product.edit", "catalog.product.publish", "pricing.price.approve"},
			mustNot: []string{"order.refund.approve", "identity.user.manage", "audit.read"},
		},
	}
	for _, c := range cases {
		got := permissionsOf(t, ctx, pool, c.role)
		for _, p := range c.must {
			if !got[p] {
				t.Errorf("%s must hold %s", c.role, p)
			}
		}
		for _, p := range c.mustNot {
			if got[p] {
				t.Errorf("%s must NOT hold %s", c.role, p)
			}
		}
	}
}

// TestSeedOIDCScopeSeparation pins IAM §۶: no customer service may request the staff
// scope, and the admin client never asks for customer scopes.
func TestSeedOIDCScopeSeparation(t *testing.T) {
	pool := setup(t)
	ctx := context.Background()

	rows, err := pool.Query(ctx, `SELECT service_code, scopes FROM oauth_clients`)
	if err != nil {
		t.Fatalf("query clients: %v", err)
	}
	defer rows.Close()
	seen := map[string][]string{}
	for rows.Next() {
		var svc string
		var scopes []string
		if err := rows.Scan(&svc, &scopes); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seen[svc] = scopes
	}
	for _, svc := range []string{"main", "shop", "magazine"} {
		for _, s := range seen[svc] {
			if s == "staff" {
				t.Errorf("customer service %s must not request the staff scope", svc)
			}
		}
	}
	for _, s := range seen["admin"] {
		if s == "customer.basic" || s == "customer.commerce" || s == "customer.content" {
			t.Errorf("admin client must not request customer scope %s", s)
		}
	}
	if len(seen) != 4 {
		t.Errorf("clients = %d, want 4", len(seen))
	}
}
