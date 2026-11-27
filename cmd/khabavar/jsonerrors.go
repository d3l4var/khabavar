package main

import (
	"encoding/json"
	"net/http"
)

// jsonErrors converts the ServeMux's plain-text 404/405 into the JSON envelope every other
// response uses. A client that always parses JSON otherwise gets a parse error on exactly the
// responses it most needs to branch on (wrong method, unknown path).
func jsonErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&jsonErrorWriter{ResponseWriter: w}, r)
	})
}

type jsonErrorWriter struct {
	http.ResponseWriter
	replaced bool
}

func (w *jsonErrorWriter) WriteHeader(code int) {
	if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
		// The mux has not written a body yet; take over the response.
		w.replaced = true
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Del("X-Content-Type-Options")
		w.ResponseWriter.WriteHeader(code)
		msg, name := "not found", "not_found"
		if code == http.StatusMethodNotAllowed {
			msg, name = "method not allowed", "method_not_allowed"
		}
		_ = json.NewEncoder(w.ResponseWriter).Encode(map[string]string{"error": msg, "code": name})
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *jsonErrorWriter) Write(b []byte) (int, error) {
	if w.replaced {
		// Swallow the mux's plain-text body; the JSON envelope is already written.
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
