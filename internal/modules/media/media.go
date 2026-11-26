// Package media implements the media module (SAD §۴.۲, DB v2 §۶ media_assets,
// R-04 §۸): media assets (images, videos, documents) and the Object Storage
// adapter abstraction.
//
// The signature design decisions:
//   - media_assets stores only the storage_key (the object's path in the storage
//     backend), NOT the binary blob. The binary lives in an Object Storage
//     backend (S3/MinIO/local filesystem). This keeps the DB small and lets the
//     storage backend handle large files, CDN, and lifecycle.
//   - The StorageAdapter interface abstracts the backend. NewDBStore uses pgx
//     for the asset metadata; the adapter is injected separately. This means
//     the media module can switch backends (local dev filesystem → production
//     S3) without touching the asset-table code.
//   - Upload generates a content-addressed storage_key: <media_type>/<uuid>/<filename>.
//     The UUID prevents collisions; the media_type prefix enables backend
//     lifecycle policies (e.g. expire old documents, keep images forever).
//   - Delete removes both the DB row and the backend object. If the backend
//     delete fails, the DB row is NOT removed (we keep the orphan row so the
//     asset is still discoverable for a retry). The caller decides whether to
//     retry or leave it; the media module never silently loses a reference.
package media

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

// MediaType enums (mirrors CHECK constraint in 000002_catalog.up.sql).
const (
	MediaTypeImage    = "IMAGE"
	MediaTypeVideo    = "VIDEO"
	MediaTypeDocument = "DOCUMENT"
)

// Sentinel errors. HTTP handlers map these to status codes.
var (
	ErrAssetNotFound    = errors.New("media: asset not found")
	ErrStorageKeyReq    = errors.New("media: storage_key required")
	ErrInvalidMediaType = errors.New("media: invalid media_type (must be IMAGE/VIDEO/DOCUMENT)")
	ErrFilenameReq      = errors.New("media: filename required")
	ErrUploadFailed     = errors.New("media: storage upload failed")
	ErrDeleteFailed     = errors.New("media: storage delete failed")
)

// Asset mirrors media_assets (DB v2 §۶). StorageKey is the object path in the
// storage backend; the binary itself is never stored in the DB.
type Asset struct {
	ID         string
	MediaType  string
	StorageKey string
	AltText    string
	CreatedAt  time.Time
}

// StorageAdapter abstracts the Object Storage backend. Implementations:
//   - LocalStorage (local filesystem — dev/test)
//   - S3Storage (production — not implemented here, would wrap aws-sdk-go-v2)
//   - memStorage (in-memory — unit tests)
//
// The adapter is responsible for: Put (write bytes), Get (read bytes), Delete
// (remove object), and Exists (check). The media.Service uses it for the binary
// side; the Store (DBStore/memStore) handles the media_assets metadata row.
type StorageAdapter interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// Store is the persistence boundary for media_assets metadata. The DB
// implementation uses pgx; unit tests use an in-memory implementation.
type Store interface {
	CreateAsset(ctx context.Context, a Asset) (string, error)
	GetAsset(ctx context.Context, id string) (*Asset, error)
	DeleteAsset(ctx context.Context, id string) error
}

// Service is the media module boundary (SAD §۴.۲). It composes a Store (for
// the media_assets row) and a StorageAdapter (for the binary object).
type Service struct {
	store   Store
	storage StorageAdapter
	now     func() time.Time
}

// NewService wires the store and storage adapter. now is injected for tests.
func NewService(store Store, storage StorageAdapter) *Service {
	return &Service{store: store, storage: storage, now: time.Now}
}

// SetNow injects a clock (tests only).
func (s *Service) SetNow(f func() time.Time) {
	if f != nil {
		s.now = f
	}
}

// validMediaType checks the media_type enum.
func validMediaType(t string) bool {
	return t == MediaTypeImage || t == MediaTypeVideo || t == MediaTypeDocument
}

// Upload registers a new media asset and stores its binary in the backend.
// The storage_key is generated as <media_type>/<uuid>/<filename>. If the
// backend Put fails, NO DB row is created (no orphan metadata). If the DB
// insert fails after a successful Put, the caller gets an error and the
// orphan object remains in the backend (a cleanup worker would sweep it;
// the media module prefers an orphan object over an orphan DB reference,
// because a DB reference to a missing object breaks the storefront).
func (s *Service) Upload(ctx context.Context, mediaType, filename, altText string, content io.Reader) (*Asset, error) {
	if !validMediaType(mediaType) {
		return nil, ErrInvalidMediaType
	}
	if filename == "" {
		return nil, ErrFilenameReq
	}
	uuid, err := randomUUID()
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}
	key := fmt.Sprintf("%s/%s/%s", mediaType, uuid, filename)
	// 1. Store the binary first (if this fails, no DB row is created).
	if err := s.storage.Put(ctx, key, content); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, err)
	}
	// 2. Insert the metadata row.
	id, err := s.store.CreateAsset(ctx, Asset{
		MediaType:  mediaType,
		StorageKey: key,
		AltText:    altText,
	})
	if err != nil {
		// DB insert failed after a successful Put → orphan object in backend.
		// Best-effort cleanup (ignore error; a sweep worker will catch it).
		_ = s.storage.Delete(ctx, key)
		return nil, fmt.Errorf("create asset row: %w", err)
	}
	return &Asset{
		ID: id, MediaType: mediaType, StorageKey: key, AltText: altText,
		CreatedAt: s.now(),
	}, nil
}

// GetAsset returns the metadata for an asset (the storefront uses this to
// build the image URL from storage_key).
func (s *Service) GetAsset(ctx context.Context, id string) (*Asset, error) {
	if id == "" {
		return nil, ErrAssetNotFound
	}
	a, err := s.store.GetAsset(ctx, id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, ErrAssetNotFound
	}
	return a, nil
}

// GetContent returns the binary content of an asset (for admin download or
// server-side processing). The storefront should use a CDN/presigned URL
// instead of proxying through the API.
func (s *Service) GetContent(ctx context.Context, id string) (io.ReadCloser, *Asset, error) {
	a, err := s.GetAsset(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.storage.Get(ctx, a.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("storage get: %w", err)
	}
	return rc, a, nil
}

// Delete removes both the DB row and the backend object. If the backend Delete
// fails, the DB row is NOT removed (we keep the orphan row so the asset is
// still discoverable for a retry). The caller can retry Delete later.
func (s *Service) Delete(ctx context.Context, id string) error {
	a, err := s.GetAsset(ctx, id)
	if err != nil {
		return err
	}
	// 1. Delete the backend object first.
	if err := s.storage.Delete(ctx, a.StorageKey); err != nil {
		return fmt.Errorf("%w: %v", ErrDeleteFailed, err)
	}
	// 2. Delete the DB row.
	if err := s.store.DeleteAsset(ctx, id); err != nil {
		return fmt.Errorf("delete asset row: %w", err)
	}
	return nil
}

// randomUUID generates a 16-byte hex UUID (no external dependency). This is a
// v4-style random UUID without the version bits set — sufficient for storage_key
// uniqueness (collisions are cryptographically negligible).
func randomUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
