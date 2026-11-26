package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// IdempotencyStore implements ADR-ARCH-008 at the request level: the first request with
// a key wins, a retry with the same key and body replays the stored response, and a
// retry with a different body is a conflict.
type IdempotencyStore interface {
	// Claim inserts the key. It returns (true, nil) when this call owns the key, and
	// (false, stored) when the key already existed.
	Claim(ctx context.Context, key, scope, requestHash string, ttl time.Duration) (bool, *IdempotentResult, error)
	// Complete stores the response so a retry can replay it.
	Complete(ctx context.Context, key string, statusCode int, body []byte) error
}

// IdempotentResult is a stored response.
type IdempotentResult struct {
	RequestHash string
	StatusCode  int
	Body        []byte
}

// HashRequest canonicalises a request body for comparison: JSON is re-encoded so key
// order and whitespace do not look like a different request.
func HashRequest(body []byte) string {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		sum := sha256.Sum256(body)
		return hex.EncodeToString(sum[:])
	}
	canon, _ := json.Marshal(v)
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:])
}

// Claim implements IdempotencyStore.
func (s *DBStore) Claim(ctx context.Context, key, scope, requestHash string, ttl time.Duration) (bool, *IdempotentResult, error) {
	var inserted string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO idempotency_keys (key, scope, request_hash, expires_at)
		 VALUES ($1,$2,$3, now() + $4::interval)
		 ON CONFLICT (key) DO NOTHING
		 RETURNING key`, key, scope, requestHash, fmt.Sprintf("%d seconds", int(ttl.Seconds()))).Scan(&inserted)
	if err == nil {
		return true, nil, nil // we own the key
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, nil, err
	}

	// The key exists: replay if the body matches, conflict if it does not.
	var res IdempotentResult
	var body []byte
	var status *int
	if err := s.pool.QueryRow(ctx,
		`SELECT request_hash, status_code, response_body FROM idempotency_keys WHERE key = $1`, key).
		Scan(&res.RequestHash, &status, &body); err != nil {
		return false, nil, err
	}
	if status != nil {
		res.StatusCode = *status
	}
	res.Body = body
	return false, &res, nil
}

// Complete stores the response for replay.
func (s *DBStore) Complete(ctx context.Context, key string, statusCode int, body []byte) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE idempotency_keys SET status_code = $2, response_body = $3 WHERE key = $1`,
		key, statusCode, body)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errors.New("commerce: idempotency key vanished before completion")
	}
	return nil
}
