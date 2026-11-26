package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBAdminLoginStore reads admin_users for the panel login.
type DBAdminLoginStore struct{ pool *pgxpool.Pool }

// NewDBAdminLoginStore wires the reader.
func NewDBAdminLoginStore(pool *pgxpool.Pool) *DBAdminLoginStore {
	return &DBAdminLoginStore{pool: pool}
}

// FindAdminByEmail returns nil (no error) when the email is unknown, so the caller can
// answer a uniform 401.
func (s *DBAdminLoginStore) FindAdminByEmail(ctx context.Context, email string) (*AdminCredential, error) {
	var c AdminCredential
	err := s.pool.QueryRow(ctx,
		`SELECT id, email, COALESCE(password_hash,''), status
		   FROM admin_users WHERE lower(email) = lower($1)`, email).
		Scan(&c.ID, &c.Email, &c.PasswordHash, &c.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if c.PasswordHash == "" {
		// No password set: treat as credentials failure, never as a bypass.
		return nil, nil
	}
	return &c, nil
}
