package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Staff account statuses (mirrors the admin_users.status CHECK).
const (
	AdminUserActive    = "ACTIVE"
	AdminUserSuspended = "SUSPENDED"
)

var (
	ErrAdminEmailRequired    = errors.New("identity: email is required")
	ErrAdminEmailTaken       = errors.New("identity: this email already has an account")
	ErrAdminPasswordTooShort = errors.New("identity: password must be at least 12 characters")
	ErrAdminRoleUnknown      = errors.New("identity: unknown role code")
	ErrAdminUserNotFound     = errors.New("identity: admin user not found")
)

// CreateAdminUser inserts a staff account and its roles in one transaction. Roles are
// resolved by CODE from the seeded table — never by a hardcoded id — so the seed stays
// the single source of truth (IAM §۴).
func (s *AdminStore) CreateAdminUser(ctx context.Context, email, fullName, passwordHash string, roleCodes []string) (string, error) {
	if email == "" {
		return "", ErrAdminEmailRequired
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO admin_users (email, full_name, password_hash, status)
		 VALUES ($1,$2,$3,'ACTIVE') RETURNING id`,
		strings.ToLower(strings.TrimSpace(email)), fullName, passwordHash).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrAdminEmailTaken
		}
		return "", err
	}
	if err := assignRoles(ctx, tx, id, roleCodes); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

// UpdateAdminUser edits the name and status, and replaces the role set when roleCodes is
// non-nil. A nil roleCodes means "leave the roles alone" so a name-only PATCH cannot
// silently strip permissions.
func (s *AdminStore) UpdateAdminUser(ctx context.Context, id, fullName, status string, roleCodes *[]string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	ct, err := tx.Exec(ctx,
		`UPDATE admin_users
		    SET full_name = COALESCE(NULLIF($2,''), full_name),
		        status    = COALESCE(NULLIF($3,''), status),
		        updated_at = now()
		  WHERE id = $1`, id, fullName, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrAdminUserNotFound
	}
	if roleCodes != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM admin_user_roles WHERE admin_user_id = $1`, id); err != nil {
			return err
		}
		if err := assignRoles(ctx, tx, id, *roleCodes); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// assignRoles resolves role codes to ids and links them. An unknown code fails the whole
// transaction, so a typo cannot leave an account with a partial role set.
func assignRoles(ctx context.Context, tx pgx.Tx, adminUserID string, roleCodes []string) error {
	for _, code := range roleCodes {
		var roleID string
		err := tx.QueryRow(ctx, `SELECT id FROM roles WHERE code = $1`, code).Scan(&roleID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAdminRoleUnknown
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO admin_user_roles (admin_user_id, role_id) VALUES ($1,$2)
			 ON CONFLICT DO NOTHING`, adminUserID, roleID); err != nil {
			return err
		}
	}
	return nil
}

// CreateAdminUserHandler serves POST /api/v1/admin/users.
func CreateAdminUserHandler(store *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string   `json:"email"`
			FullName string   `json:"full_name"`
			Password string   `json:"password"`
			Roles    []string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		if len(req.Password) < 12 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": ErrAdminPasswordTooShort.Error(), "code": "weak_password"})
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error", "code": "internal_error"})
			return
		}
		id, err := store.CreateAdminUser(r.Context(), req.Email, req.FullName, string(hash), req.Roles)
		if err != nil {
			writeAdminUserErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	}
}

// UpdateAdminUserHandler serves PATCH /api/v1/admin/users/{id}. The password is not
// editable here: a credential change needs its own audited flow.
func UpdateAdminUserHandler(store *AdminStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			FullName string    `json:"full_name"`
			Status   string    `json:"status"`
			Roles    *[]string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json", "code": "invalid_body"})
			return
		}
		if req.Status != "" && req.Status != AdminUserActive && req.Status != AdminUserSuspended {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "status must be ACTIVE or SUSPENDED", "code": "invalid_status"})
			return
		}
		if err := store.UpdateAdminUser(r.Context(), r.PathValue("id"), req.FullName, req.Status, req.Roles); err != nil {
			writeAdminUserErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "updated"})
	}
}

// isUniqueViolation reports a Postgres unique-constraint error (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func writeAdminUserErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAdminEmailTaken):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "code": "email_taken"})
	case errors.Is(err, ErrAdminUserNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error(), "code": "not_found"})
	case errors.Is(err, ErrAdminEmailRequired), errors.Is(err, ErrAdminRoleUnknown):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "invalid_request"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error", "code": "internal_error"})
	}
}
