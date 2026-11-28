// Package media — HTTP handlers for media upload/retrieve/delete (R-07 §media).
// Upload and Delete are admin endpoints (behind authorization.Require in main.go);
// GetAsset (metadata) is public so the storefront can resolve image URLs.
package media

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
)

// UploadHandler serves POST /api/v1/admin/media — multipart/form-data upload.
// Form fields: media_type (IMAGE|VIDEO|DOCUMENT), alt_text (optional), file (the binary).
// The storage_key is generated as <media_type>/<uuid>/<filename> (media.go).
func UploadHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit upload size to 25 MiB (configurable in a later pass; R-04 §۸ limits).
		r.Body = http.MaxBytesReader(w, r.Body, 25<<20)
		if err := r.ParseMultipartForm(25 << 20); err != nil {
			writeMediaErr(w, http.StatusBadRequest, "multipart parse failed (max 25 MiB)")
			return
		}
		mediaType := r.FormValue("media_type")
		altText := r.FormValue("alt_text")
		file, header, err := r.FormFile("file")
		if err != nil {
			writeMediaErr(w, http.StatusBadRequest, "file field required")
			return
		}
		defer file.Close()
		filename := sanitizeFilename(header.Filename)
		if filename == "" {
			writeMediaErr(w, http.StatusBadRequest, "filename required")
			return
		}
		asset, err := svc.Upload(r.Context(), mediaType, filename, altText, file)
		if err != nil {
			writeMediaSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": asset})
	}
}

// GetAssetHandler serves GET /api/v1/media/{id} — the asset metadata (storage_key,
// media_type, alt_text). The storefront uses this to build the image URL; the
// binary itself is served via CDN/presigned URL, not proxied through the API.
func GetAssetHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := svc.GetAsset(r.Context(), id)
		if err != nil {
			writeMediaSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": a})
	}
}

// GetContentHandler serves GET /api/v1/media/{id}/content — streams the binary
// (for admin download or server-side processing). The storefront should use CDN.
func GetContentHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		rc, asset, err := svc.GetContent(r.Context(), id)
		if err != nil {
			writeMediaSvcErr(w, err)
			return
		}
		defer rc.Close()
		w.Header().Set("Content-Type", contentTypeFor(asset.MediaType, asset.StorageKey))
		w.Header().Set("Content-Disposition", `inline; filename="`+filepath.Base(asset.StorageKey)+`"`)
		_, _ = io.Copy(w, rc)
	}
}

// DeleteHandler serves DELETE /api/v1/admin/media/{id} — removes the DB row and
// the backend object (media.go Delete ordering: backend first, then DB row).
func DeleteHandler(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := svc.Delete(r.Context(), id); err != nil {
			writeMediaSvcErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// sanitizeFilename strips path components — the backend generates the full
// content-addressed key, but we still guard against the uploaded filename
// carrying a path separator.
func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "." || name == "/" || name == "" {
		return ""
	}
	return name
}

// contentTypeFor picks the Content-Type for inline serving.
//
// It used to answer "image/*" for every image. A wildcard is not a media type a browser can
// render: the asset downloaded instead of displaying, and <img> showed a broken icon. The
// extension on the storage key is the real answer; media_type is only the fallback.
func contentTypeFor(mediaType, storageKey string) string {
	switch strings.ToLower(path.Ext(storageKey)) {
	case ".webp":
		return "image/webp"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".avif":
		return "image/avif"
	case ".svg":
		return "image/svg+xml"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".pdf":
		return "application/pdf"
	}
	switch mediaType {
	case MediaTypeImage:
		return "image/jpeg"
	case MediaTypeVideo:
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

func writeMediaSvcErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAssetNotFound):
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "asset not found"})
	case errors.Is(err, ErrInvalidMediaType):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid media_type", "code": "invalid_media_type"})
	case errors.Is(err, ErrFilenameReq):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "filename required"})
	case errors.Is(err, ErrUploadFailed):
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "storage upload failed", "code": "upload_failed"})
	case errors.Is(err, ErrDeleteFailed):
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "storage delete failed", "code": "delete_failed"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server error"})
	}
}

func writeMediaErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
