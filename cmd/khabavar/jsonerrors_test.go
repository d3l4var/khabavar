package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestJSONErrors covers the mux's plain-text 404/405: both must answer the same JSON envelope
// every other response uses, and a normal response must pass through untouched.
func TestJSONErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/thing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h := jsonErrors(mux)

	check := func(method, path string, wantCode int, wantName string) {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != wantCode {
			t.Fatalf("%s %s = %d, want %d", method, path, rec.Code, wantCode)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Fatalf("%s %s content-type = %q", method, path, ct)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s body is not JSON: %q", method, path, rec.Body.String())
		}
		if body["code"] != wantName {
			t.Fatalf("%s %s code = %q, want %q", method, path, body["code"], wantName)
		}
	}

	// Wrong method on a known path.
	check(http.MethodGet, "/api/v1/thing", http.StatusMethodNotAllowed, "method_not_allowed")
	// Unknown path.
	check(http.MethodGet, "/api/v1/nope", http.StatusNotFound, "not_found")

	// A normal response passes through unchanged.
	r := httptest.NewRequest(http.MethodPost, "/api/v1/thing", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusCreated || rec.Body.String() != `{"ok":true}` {
		t.Fatalf("pass-through = %d %q", rec.Code, rec.Body.String())
	}
}
