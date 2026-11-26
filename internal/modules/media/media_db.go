// media_db.go provides the pgx implementation of media.Store, plus the
// LocalStorage adapter (filesystem-backed StorageAdapter for dev/test).
package media

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DBStore is the pgx-backed media.Store.
type DBStore struct {
	pool *pgxpool.Pool
}

func NewDBStore(pool *pgxpool.Pool) *DBStore { return &DBStore{pool: pool} }

func (s *DBStore) CreateAsset(ctx context.Context, a Asset) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO media_assets (media_type, storage_key, alt_text)
		 VALUES ($1,$2,$3) RETURNING id`,
		a.MediaType, a.StorageKey, nullableStr(a.AltText)).Scan(&id)
	return id, err
}

func (s *DBStore) GetAsset(ctx context.Context, id string) (*Asset, error) {
	var a Asset
	var alt pgtype.Text
	err := s.pool.QueryRow(ctx,
		`SELECT id, media_type, storage_key, alt_text, created_at
		   FROM media_assets WHERE id = $1`, id).
		Scan(&a.ID, &a.MediaType, &a.StorageKey, &alt, &a.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.AltText = textOr(alt)
	return &a, nil
}

func (s *DBStore) DeleteAsset(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM media_assets WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrAssetNotFound
	}
	return nil
}

// LocalStorage is a filesystem-backed StorageAdapter for dev/test. The rootDir
// is created on first Put. Storage keys are mapped to paths under rootDir:
//
//	"IMAGE/<uuid>/file.jpg" → rootDir/IMAGE/<uuid>/file.jpg
//
// In production this would be replaced by S3Storage (wrapping aws-sdk-go-v2).
type LocalStorage struct {
	rootDir string
}

// NewLocalStorage creates a LocalStorage rooted at rootDir. The directory is
// created lazily on the first Put.
func NewLocalStorage(rootDir string) *LocalStorage {
	return &LocalStorage{rootDir: rootDir}
}

func (ls *LocalStorage) Put(_ context.Context, key string, r io.Reader) error {
	path := filepath.Join(ls.rootDir, filepath.Clean("/"+key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
		}
		if err != nil {
			break
		}
	}
	return nil
}

func (ls *LocalStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(ls.rootDir, filepath.Clean("/"+key))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, err
	}
	return f, nil
}

func (ls *LocalStorage) Delete(_ context.Context, key string) error {
	path := filepath.Join(ls.rootDir, filepath.Clean("/"+key))
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (ls *LocalStorage) Exists(_ context.Context, key string) (bool, error) {
	path := filepath.Join(ls.rootDir, filepath.Clean("/"+key))
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func textOr(t pgtype.Text) string {
	if t.Valid {
		return t.String
	}
	return ""
}

func nullableStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// Compile-time assertions.
var _ Store = (*DBStore)(nil)
var _ StorageAdapter = (*LocalStorage)(nil)
