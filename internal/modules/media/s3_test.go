package media

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeS3 is a minimal S3-compatible server: path-style PUT/GET/HEAD/DELETE over an in-memory map.
//
// It is NOT a real S3. It exists because the adapter's contract — sign the request, address the
// object path-style, map a missing key to Exists=false — can be verified without a network
// dependency, and because dl.min.io was unreachable from the build machine, so MinIO could not be
// used here. What it proves: the SDK signs every call (Authorization: AWS4-HMAC-SHA256), the
// path is /bucket/key, and the four operations round-trip. What it does not prove: compatibility
// with a specific provider's quirks.
type fakeS3 struct {
	mu       sync.Mutex
	objects  map[string][]byte
	unsigned int
	paths    []string
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	key := strings.TrimPrefix(r.URL.Path, "/")
	f.paths = append(f.paths, r.Method+" /"+key)
	if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
		f.unsigned++
		w.WriteHeader(http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		f.objects[key] = b
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		b, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>NoSuchKey</Code></Error>`))
			return
		}
		_, _ = w.Write(b)
	case http.MethodHead:
		if _, ok := f.objects[key]; !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func newTestS3(t *testing.T) (*S3Storage, *fakeS3) {
	t.Helper()
	fake := newFakeS3()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	st, err := NewS3Storage(S3Config{
		Endpoint:  srv.URL,
		Region:    "us-east-1",
		Bucket:    "media",
		AccessKey: "test-key",
		SecretKey: "test-secret",
		PathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3Storage: %v", err)
	}
	return st, fake
}

// TestS3Storage_RoundTrip covers the four operations the media service depends on.
func TestS3Storage_RoundTrip(t *testing.T) {
	st, fake := newTestS3(t)
	ctx := context.Background()
	key := "products/2026/blanket.webp"
	want := []byte("fake-webp-bytes")

	if err := st.Put(ctx, key, bytes.NewReader(want)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	ok, err := st.Exists(ctx, key)
	if err != nil || !ok {
		t.Fatalf("Exists after Put = (%v,%v), want (true,nil)", ok, err)
	}

	rc, err := st.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, want) {
		t.Fatalf("Get returned %q, want %q", got, want)
	}

	// Path style: the bucket is the first segment, not a virtual host.
	if len(fake.paths) == 0 || !strings.Contains(fake.paths[0], "/media/"+key) {
		t.Fatalf("request path = %v, want /media/%s", fake.paths, key)
	}

	if err := st.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ok, err = st.Exists(ctx, key)
	if err != nil || ok {
		t.Fatalf("Exists after Delete = (%v,%v), want (false,nil)", ok, err)
	}
}

// TestS3Storage_ExistsMissingIsNotAnError pins the mapping the handler relies on: a missing
// object answers false, not an error. Without it every 404 from the store became a 500.
func TestS3Storage_ExistsMissingIsNotAnError(t *testing.T) {
	st, _ := newTestS3(t)
	ok, err := st.Exists(context.Background(), "never/uploaded.webp")
	if err != nil {
		t.Fatalf("Exists on a missing key returned an error: %v", err)
	}
	if ok {
		t.Fatal("Exists on a missing key returned true")
	}
}

// TestS3Storage_EveryCallIsSigned proves the adapter does not send anonymous requests: an
// unsigned PUT would be accepted by a permissive bucket and rejected by a locked-down one.
func TestS3Storage_EveryCallIsSigned(t *testing.T) {
	st, fake := newTestS3(t)
	ctx := context.Background()
	if err := st.Put(ctx, "a/b.bin", strings.NewReader("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, err := st.Exists(ctx, "a/b.bin"); err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if fake.unsigned != 0 {
		t.Fatalf("%d requests arrived unsigned", fake.unsigned)
	}
}

// TestNewS3Storage_RequiresBucket: a missing bucket would fail on every upload instead of at
// startup, which is the difference between a boot error and a broken deploy.
func TestNewS3Storage_RequiresBucket(t *testing.T) {
	if _, err := NewS3Storage(S3Config{Region: "us-east-1"}); err == nil {
		t.Fatal("expected an error when the bucket is empty")
	}
}

// TestContentTypeFor pins the header a browser needs to render an asset inline. "image/*" is not
// a media type: the file downloaded instead of displaying.
func TestContentTypeFor(t *testing.T) {
	cases := []struct{ mediaType, key, want string }{
		{MediaTypeImage, "IMAGE/abc/photo.webp", "image/webp"},
		{MediaTypeImage, "IMAGE/abc/photo.JPG", "image/jpeg"},
		{MediaTypeImage, "IMAGE/abc/photo.png", "image/png"},
		{MediaTypeImage, "IMAGE/abc/photo.avif", "image/avif"},
		{MediaTypeVideo, "VIDEO/abc/clip.mp4", "video/mp4"},
		{MediaTypeDocument, "DOCUMENT/abc/manual.pdf", "application/pdf"},
		// No extension: fall back to the media type rather than a wildcard.
		{MediaTypeImage, "IMAGE/abc/blob", "image/jpeg"},
		{MediaTypeDocument, "DOCUMENT/abc/blob", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := contentTypeFor(c.mediaType, c.key); got != c.want {
			t.Fatalf("contentTypeFor(%s,%s) = %q, want %q", c.mediaType, c.key, got, c.want)
		}
	}
}
