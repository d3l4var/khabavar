// Package idempotency caches responses of idempotent financial operations by key
// (ADR-ARCH-008). v1 is in-process; production must back this with Redis or a DB
// table so it survives restarts and is shared across api units.
package idempotency

import (
	"sync"
	"time"
)

// Result is the cached response snapshot of an idempotent operation.
type Result struct {
	Status   int
	Body     []byte
	StoredAt time.Time
}

type Store struct {
	mu  sync.Mutex
	m   map[string]Result
	ttl time.Duration
}

func New(ttl time.Duration) *Store {
	return &Store{m: make(map[string]Result), ttl: ttl}
}

// Get returns the cached result for key if present and not expired.
func (s *Store) Get(key string, now time.Time) (Result, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.m[key]
	if !ok {
		return Result{}, false
	}
	if now.Sub(r.StoredAt) > s.ttl {
		delete(s.m, key)
		return Result{}, false
	}
	return r, true
}

// Put stores the result for key, stamping StoredAt = now.
func (s *Store) Put(key string, r Result, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.StoredAt = now
	s.m[key] = r
}
