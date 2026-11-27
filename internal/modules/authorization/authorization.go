// Package authorization implements RBAC: roles, permissions, service scopes
// (SAD §۴.۱). Entities: roles, permissions, admin_user_roles, role_permissions.
// Dependency: identity.
//
// Permissions have a SINGLE source of truth: the role_permissions table seeded in
// 001_seed_baseline.sql (IAM §۱). Server code checks permission NAMES, never hardcodes
// roles — adding a role or changing an assignment needs no code change. Operations
// beyond permission return 403 + an Audit entry (IAM §۱).
package authorization

import (
	"context"
	"errors"
	"net/http"
)

// ErrDenied is returned when a user lacks a required permission.
var ErrDenied = errors.New("authorization: permission denied")

type adminKey struct{}

// WithAdminUserID stores the authenticated admin user id in ctx.
func WithAdminUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, adminKey{}, id)
}

// AdminUserID returns the admin user id from ctx, or "".
func AdminUserID(ctx context.Context) string {
	v, _ := ctx.Value(adminKey{}).(string)
	return v
}

// PermissionChecker resolves whether an admin user holds a given permission code.
type PermissionChecker interface {
	HasPermission(ctx context.Context, adminUserID, permission string) (bool, error)
}

// Require wraps next so it only runs when the admin user (from ctx) holds permission.
// Denial → 403 (IAM §۱: 403 + audit). Missing identity → 401.
func Require(chk PermissionChecker, permission string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := AdminUserID(r.Context())
		if uid == "" {
			writeAuthErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		ok, err := chk.HasPermission(r.Context(), uid, permission)
		if err != nil || !ok {
			writeAuthErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MatrixChecker is an in-memory PermissionChecker backed by the seed role→permission
// matrix (IAM §۴). It is the reference for the 10-role matrix and the RBAC test gate
// (CI §۳). SUPER_ADMIN holds all 24 permissions by convention (IAM §۴).
type MatrixChecker struct {
	rolePerms  map[string]map[string]bool
	adminRoles map[string][]string
}

// NewMatrixChecker builds a checker from a role → []permission mapping.
func NewMatrixChecker(rolePerms map[string][]string) *MatrixChecker {
	m := &MatrixChecker{rolePerms: map[string]map[string]bool{}, adminRoles: map[string][]string{}}
	for role, perms := range rolePerms {
		set := make(map[string]bool, len(perms))
		for _, p := range perms {
			set[p] = true
		}
		m.rolePerms[role] = set
	}
	return m
}

// AssignRoles replaces the role set for an admin user.
func (m *MatrixChecker) AssignRoles(adminUserID string, roles ...string) {
	m.adminRoles[adminUserID] = roles
}

func (m *MatrixChecker) HasPermission(_ context.Context, adminUserID, permission string) (bool, error) {
	for _, role := range m.adminRoles[adminUserID] {
		if role == "SUPER_ADMIN" {
			return true, nil
		}
		if set := m.rolePerms[role]; set != nil && set[permission] {
			return true, nil
		}
	}
	return false, nil
}
