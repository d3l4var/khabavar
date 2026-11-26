// DB-backed permission checking (IAM §۱).
//
// The seeded role_permissions matrix is the single source of truth for who may do
// what. MatrixChecker is the in-memory reference used by the RBAC test gate; DBChecker
// is the production implementation that reads the same matrix from Postgres, so adding
// a role or moving a permission needs no code change and no redeploy.
package authorization

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Querier is the minimal database surface DBChecker needs. *pgxpool.Pool and
// *pgx.Conn both satisfy it.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBChecker resolves permissions from admin_users ⋈ admin_user_roles ⋈
// role_permissions ⋈ permissions. Suspended or inactive staff hold no permissions.
type DBChecker struct {
	q Querier
}

// NewDBChecker wires a checker to the given pool.
func NewDBChecker(q Querier) *DBChecker { return &DBChecker{q: q} }

// hasPermissionSQL joins the four RBAC tables. SUPER_ADMIN needs no special case here:
// the seed grants it every permission row, so the join already returns true (IAM §۴).
const hasPermissionSQL = `
SELECT EXISTS (
    SELECT 1
      FROM admin_users au
      JOIN admin_user_roles aur ON aur.admin_user_id = au.id
      JOIN role_permissions rp  ON rp.role_id = aur.role_id
      JOIN permissions p        ON p.id = rp.permission_id
     WHERE au.id = $1::uuid
       AND au.status = 'ACTIVE'
       AND p.code = $2
)`

// HasPermission reports whether the admin user currently holds the permission code.
// A malformed user id is treated as "no permission" rather than an error so a bad
// token cannot turn into a 500.
func (c *DBChecker) HasPermission(ctx context.Context, adminUserID, permission string) (bool, error) {
	if _, err := uuid.Parse(adminUserID); err != nil {
		return false, nil
	}
	var ok bool
	if err := c.q.QueryRow(ctx, hasPermissionSQL, adminUserID, permission).Scan(&ok); err != nil {
		return false, fmt.Errorf("authorization: permission lookup: %w", err)
	}
	return ok, nil
}

// Compile-time assertion that DBChecker is a PermissionChecker.
var _ PermissionChecker = (*DBChecker)(nil)
