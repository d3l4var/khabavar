// Package ratelimit implements a per-key fixed-window limiter for OTP/login/API
// (CIAM §۴: OTP ۵/ساعت؛ SAD §۱۰). This is the software layer; nginx auth_zone is
// the proxy layer (DEP §۴ — two-layer defense).
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	hits   map[string][]time.Time
}

// New builds a limiter allowing `limit` hits per `window` per key.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{window: window, limit: limit, hits: make(map[string][]time.Time)}
}

// Allow returns true if key is under its limit within the rolling window, and
// records the hit. Returns false (and does not record) when over limit.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	hits := l.hits[key]
	keep := make([]time.Time, 0, len(hits))
	for _, t := range hits {
		if t.After(cutoff) {
			keep = append(keep, t)
		}
	}
	if len(keep) >= l.limit {
		l.hits[key] = keep
		return false
	}
	keep = append(keep, now)
	l.hits[key] = keep
	return true
}
