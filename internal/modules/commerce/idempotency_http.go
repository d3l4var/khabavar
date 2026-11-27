package commerce

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// IdempotencyTTL is how long a stored response is replayable (contract: at least 24h).
const IdempotencyTTL = 24 * time.Hour

// recorder captures the handler's status and body so they can be stored for replay.
type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Idempotent wraps a money-touching handler with ADR-ARCH-008 semantics:
//   - the header is required (the contract marks it required),
//   - the first request with a key runs and its response is stored,
//   - a retry with the same key and body replays that response verbatim,
//   - a retry with the same key but a different body is a 409,
//   - a retry while the first is still running is a 409 (not a second order).
//
// maxIdempotencyKeyLen matches idempotency_keys.key varchar(100).
const maxIdempotencyKeyLen = 100

func Idempotent(store IdempotencyStore, scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": ErrIdempotencyKeyMissing.Error(), "code": "idempotency_key_required",
			})
			return
		}
		// The key column is varchar(100). Without this check an over-long key reached the
		// database and came back as a 500 "internal error" for what is plainly a bad request.
		if len(key) > maxIdempotencyKeyLen {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": ErrIdempotencyKeyTooLong.Error(), "code": "idempotency_key_too_long",
			})
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body", "code": "invalid_body"})
			return
		}
		hash := HashRequest(body)

		owned, stored, err := store.Claim(r.Context(), key, scope, hash, IdempotencyTTL)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal error", "code": "internal_error"})
			return
		}
		if !owned {
			switch {
			case stored.RequestHash != hash:
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": ErrIdempotencyConflict.Error(), "code": "idempotency_conflict",
				})
				return
			case stored.StatusCode == 0:
				writeJSON(w, http.StatusConflict, map[string]any{
					"error": ErrIdempotencyInFlight.Error(), "code": "idempotency_in_flight",
				})
				return
			default:
				// Replay the first response exactly.
				w.Header().Set("Idempotent-Replay", "true")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(stored.StatusCode)
				_, _ = w.Write(stored.Body)
				return
			}
		}

		// We own the key: run the handler and store what it produced.
		r.Body = io.NopCloser(bytes.NewReader(body))
		rec := &recorder{ResponseWriter: w}
		next(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		// Only successful results are replayable; a failure must be retryable.
		if rec.status < 400 {
			if err := store.Complete(r.Context(), key, rec.status, rec.body.Bytes()); err != nil {
				// The response is already written; log-worthy but not client-facing.
				_ = err
			}
		} else if rel, ok := store.(interface {
			Release(context.Context, string) error
		}); ok {
			// A failure releases the key: the client retries with the same key on purpose.
			_ = rel.Release(r.Context(), key)
		} else {
			_ = store.Complete(r.Context(), key, 0, nil)
		}
	}
}

// jsonBody is a helper for tests and handlers that need the raw body.
func jsonBody(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

var _ = errors.Is
