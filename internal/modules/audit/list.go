// list.go exposes the append-only audit log to the panel (IAM §۷: audit.read, only
// SUPER_ADMIN and FINANCE). Records are read-only; there is no write path here.
package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Entry is one audit_logs row as the panel reads it.
type Entry struct {
	ID         string          `json:"id"`
	ActorType  string          `json:"actor_type"`
	ActorID    string          `json:"actor_id"`
	Action     string          `json:"action"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	OldValues  json.RawMessage `json:"old_values,omitempty"`
	NewValues  json.RawMessage `json:"new_values,omitempty"`
	IPAddress  string          `json:"ip_address,omitempty"`
	CreatedAt  string          `json:"created_at"`
}

// ListStore reads audit_logs.
type ListStore struct {
	pool *pgxpool.Pool
}

// NewListStore wires the reader.
func NewListStore(pool *pgxpool.Pool) *ListStore { return &ListStore{pool: pool} }

// List returns a page of audit entries, newest first, optionally filtered by action
// and actor.
func (s *ListStore) List(ctx context.Context, action, actorID string, page, pageSize int) ([]Entry, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_logs
		  WHERE ($1 = '' OR action = $1)
		    AND ($2 = '' OR actor_id::text = $2)`, action, actorID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, actor_type, COALESCE(actor_id::text,''), action, entity_type,
		        COALESCE(entity_id::text,''), old_values, new_values,
		        COALESCE(host(ip_address),''), created_at
		   FROM audit_logs
		  WHERE ($1 = '' OR action = $1)
		    AND ($2 = '' OR actor_id::text = $2)
		  ORDER BY created_at DESC
		  LIMIT $3 OFFSET $4`, action, actorID, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var oldV, newV []byte
		var created pgtype.Timestamptz
		if err := rows.Scan(&e.ID, &e.ActorType, &e.ActorID, &e.Action, &e.EntityType,
			&e.EntityID, &oldV, &newV, &e.IPAddress, &created); err != nil {
			return nil, 0, err
		}
		e.OldValues = oldV
		e.NewValues = newV
		if created.Valid {
			e.CreatedAt = created.Time.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ListHandler serves GET /api/v1/admin/audit-logs (audit.read).
func ListHandler(s *ListStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if size < 1 || size > 100 {
			size = 20
		}
		entries, total, err := s.List(r.Context(),
			r.URL.Query().Get("action"), r.URL.Query().Get("actor_id"), page, size)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error", "code": "internal_error"})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": entries, "total": total, "page": page, "page_size": size,
		})
	}
}
