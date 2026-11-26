// admin.go exposes the read-only staff endpoints that the contract declares under
// /admin (R-07): audit log, staff users, roles and permissions. They are separate
// from the identity Store interface so the existing unit-test fakes stay minimal.
//
// Every handler here is mounted behind authorization.Require (see
// cmd/khabavar/routes.go): audit.read for the log, identity.user.manage for the IAM
// views (IAM §۷).
package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AdminStore reads the staff/IAM tables for the panel.
type AdminStore struct {
	pool *pgxpool.Pool
}

// NewAdminStore wires the reader to the pool.
func NewAdminStore(pool *pgxpool.Pool) *AdminStore { return &AdminStore{pool: pool} }

// AdminUser is a staff account as the panel lists it.
type AdminUser struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	FullName string   `json:"full_name"`
	Status   string   `json:"status"`
	Roles    []string `json:"roles"`
}

// Role is a staff role with its permission codes.
type Role struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// Permission is a single fine-grained permission code (IAM §۳).
type Permission struct {
	Code        string `json:"code"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

// ListAdminUsers returns staff accounts with their role codes.
func (s *AdminStore) ListAdminUsers(ctx context.Context, page, pageSize int) ([]AdminUser, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM admin_users`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT au.id, au.email, au.full_name, au.status,
		        COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}')
		   FROM admin_users au
		   LEFT JOIN admin_user_roles aur ON aur.admin_user_id = au.id
		   LEFT JOIN roles r ON r.id = aur.role_id
		  GROUP BY au.id, au.email, au.full_name, au.status
		  ORDER BY au.email
		  LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AdminUser
	for rows.Next() {
		var u AdminUser
		if err := rows.Scan(&u.ID, &u.Email, &u.FullName, &u.Status, &u.Roles); err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

// ListRoles returns the seeded roles with their permission codes (IAM §۴).
func (s *AdminStore) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT r.code, r.name, COALESCE(r.description,''),
		        COALESCE(array_agg(p.code ORDER BY p.code) FILTER (WHERE p.code IS NOT NULL), '{}')
		   FROM roles r
		   LEFT JOIN role_permissions rp ON rp.role_id = r.id
		   LEFT JOIN permissions p ON p.id = rp.permission_id
		  GROUP BY r.code, r.name, r.description
		  ORDER BY r.code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		var desc pgtype.Text
		if err := rows.Scan(&r.Code, &r.Name, &desc, &r.Permissions); err != nil {
			return nil, err
		}
		r.Description = textOr(desc)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListPermissions returns the 24 seeded permission codes.
func (s *AdminStore) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT code, resource, action, COALESCE(description,'')
		   FROM permissions ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Code, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AdminUsersHandler serves GET /api/v1/admin/users.
func AdminUsersHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, size := pageParams(r)
		users, total, err := s.ListAdminUsers(r.Context(), page, size)
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"data": users, "total": total, "page": page, "page_size": size})
	}
}

// RolesHandler serves GET /api/v1/admin/roles.
func RolesHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		roles, err := s.ListRoles(r.Context())
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"data": roles})
	}
}

// PermissionsHandler serves GET /api/v1/admin/permissions.
func PermissionsHandler(s *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		perms, err := s.ListPermissions(r.Context())
		if err != nil {
			writeAdminErr(w, err)
			return
		}
		writeAdminJSON(w, http.StatusOK, map[string]any{"data": perms})
	}
}

// textOr unwraps a nullable text column.
func textOr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

// pageParams clamps page/page_size to the contract defaults (page_size 20, max 100).
func pageParams(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

// writeAdminErr never leaks driver detail to a client.
func writeAdminErr(w http.ResponseWriter, err error) {
	writeAdminJSON(w, http.StatusInternalServerError, map[string]string{
		"error": "internal error", "code": "internal_error",
	})
}

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
