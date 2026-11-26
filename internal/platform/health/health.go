// Package health serves /healthz (liveness) and /readyz (readiness + DB) per DEP §۳.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Checker is the readiness probe (DB + dependencies).
type Checker interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	ready atomic.Value // bool
	chk   Checker
}

func New(chk Checker) *Handler {
	h := &Handler{chk: chk}
	h.ready.Store(false)
	return h
}

// SetReady toggles readiness; called after successful startup wiring.
func (h *Handler) SetReady(v bool) { h.ready.Store(v) }

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load().(bool) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not ready"})
			return
		}
		if h.chk != nil {
			if err := h.chk.Ping(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "db down", "error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
