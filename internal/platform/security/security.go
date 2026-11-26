// Package security provides HTTP middleware for security headers, CORS, and
// rate-limit hardening (R-19 P9). These wrap the mux in main.go.
package security

import (
	"net/http"
	"strings"
)

// SecurityHeaders is middleware that adds the standard security response headers:
//   - X-Content-Type-Options: nosniff (prevents MIME sniffing)
//   - X-Frame-Options: DENY (prevents clickjacking — the API is never framed)
//   - X-XSS-Protection: 0 (disabled in favor of CSP; modern browsers ignore this)
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains (HSTS —
//     forces HTTPS for one year, including subdomains)
//   - Referrer-Policy: strict-origin-when-cross-origin (limits referrer leakage)
//   - Content-Security-Policy: default-src 'none' (API returns JSON, no inline
//     content — the strictest possible CSP for an API)
//   - Cache-Control: no-store (API responses are never cached — prevents stale
//     auth/order data)
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-XSS-Protection", "0")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// CORSConfig holds the CORS policy.
type CORSConfig struct {
	// AllowedOrigins is the list of origins permitted to make cross-origin
	// requests. For khabavar: https://khabavar.ir, https://shop.khabavar.ir,
	// https://magazine.khabavar.ir, https://admin.khabavar.ir.
	AllowedOrigins []string
	// AllowedMethods defaults to GET, POST, PATCH, DELETE, OPTIONS.
	AllowedMethods []string
	// AllowedHeaders defaults to Content-Type, Authorization, X-Idempotency-Key.
	AllowedHeaders []string
}

// DefaultCORSConfig returns the production CORS policy for khabavar's subdomains.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{
			"https://khabavar.ir",
			"https://shop.khabavar.ir",
			"https://magazine.khabavar.ir",
			"https://admin.khabavar.ir",
		},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Idempotency-Key"},
	}
}

// CORS is middleware that enforces the CORS policy. It checks the Origin header
// against the allowed list, sets the appropriate CORS response headers, and
// short-circuits OPTIONS preflight requests with 204 No Content.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedSet := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowedSet[o] = true
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowedSet[origin] {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", methods)
				h.Set("Access-Control-Allow-Headers", headers)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Max-Age", "600")
			}
			// Preflight short-circuit.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain composes multiple middlewares: Chain(h, m1, m2, m3) runs m1 → m2 → m3 → h.
func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
