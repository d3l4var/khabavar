package media

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/khabavar/khabavar/internal/platform/testpg"
	"github.com/khabavar/khabavar/migrations"
)

// TestPG_MediaHTTPHandlers drives the media surface through its handlers: upload, metadata,
// content download and delete. http.go had no tests, and an upload path that silently
// accepts a bad type or an empty file is exactly the kind of thing only a handler test sees.
func TestPG_MediaHTTPHandlers(t *testing.T) {
	pool, cleanup := testpg.Setup(t)
	t.Cleanup(cleanup)
	// testpg gives a bare schema; the module's own migrations are applied here.
	if err := migrations.ApplyAll(context.Background(), pool, nil); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	svc := NewService(NewDBStore(pool), NewLocalStorage(t.TempDir()))
	ctx := context.Background()

	upload := func(fields map[string]string, filename string, content []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		if filename != "" {
			fw, _ := mw.CreateFormFile("file", filename)
			_, _ = fw.Write(content)
		}
		_ = mw.Close()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media", &buf)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		rec := httptest.NewRecorder()
		UploadHandler(svc)(rec, r)
		return rec
	}

	// A missing file field is a 400.
	if rec := upload(map[string]string{"media_type": "IMAGE"}, "", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("no file = %d, want 400", rec.Code)
	}
	// An unsupported media type is refused before anything is written.
	if rec := upload(map[string]string{"media_type": "NONSENSE"}, "a.png", []byte("x")); rec.Code < 400 {
		t.Fatalf("bad media type = %d, want a 4xx", rec.Code)
	}

	// A real upload.
	rec := upload(map[string]string{"media_type": "IMAGE", "alt_text": "بالش"}, "pillow.png", []byte("PNGDATA"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Data struct {
			ID         string `json:"id"`
			StorageKey string `json:"storage_key"`
			MediaType  string `json:"media_type"`
			AltText    string `json:"alt_text"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Data.ID == "" || out.Data.StorageKey == "" || out.Data.MediaType != "IMAGE" {
		t.Fatalf("upload response = %s", rec.Body.String())
	}

	// Metadata by id, and a 404 for an unknown one.
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("id", out.Data.ID)
	recMeta := httptest.NewRecorder()
	GetAssetHandler(svc)(recMeta, r)
	if recMeta.Code != http.StatusOK {
		t.Fatalf("asset metadata = %d: %s", recMeta.Code, recMeta.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("id", "00000000-0000-0000-0000-000000000000")
	recMissing := httptest.NewRecorder()
	GetAssetHandler(svc)(recMissing, r)
	if recMissing.Code != http.StatusNotFound {
		t.Fatalf("unknown asset = %d, want 404", recMissing.Code)
	}

	// The bytes come back through the content route.
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.SetPathValue("id", out.Data.ID)
	recContent := httptest.NewRecorder()
	GetContentHandler(svc)(recContent, r)
	if recContent.Code != http.StatusOK || recContent.Body.String() != "PNGDATA" {
		t.Fatalf("content = %d %q", recContent.Code, recContent.Body.String())
	}

	// Delete removes it; a second delete is a 4xx.
	r = httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.SetPathValue("id", out.Data.ID)
	recDel := httptest.NewRecorder()
	DeleteHandler(svc)(recDel, r)
	if recDel.Code != http.StatusNoContent && recDel.Code != http.StatusOK {
		t.Fatalf("delete = %d: %s", recDel.Code, recDel.Body.String())
	}
	r = httptest.NewRequest(http.MethodDelete, "/x", nil)
	r.SetPathValue("id", out.Data.ID)
	recDel2 := httptest.NewRecorder()
	DeleteHandler(svc)(recDel2, r)
	if recDel2.Code < 400 {
		t.Fatalf("second delete = %d, want a 4xx", recDel2.Code)
	}
	_ = ctx
}
