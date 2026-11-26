// Package log provides a structured JSON logger (log/slog) that emits a constant
// trace_id field per request (DEP §۳). Output goes to stdout, collected by journald in prod.
package log

import (
	"context"
	"log/slog"
	"os"
)

type traceKey struct{}

// New returns a *slog.Logger writing JSON at the given level.
func New(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}

// WithTrace stores a trace id in ctx so handlers can attach it to every log entry.
func WithTrace(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceKey{}, traceID)
}

// TraceFrom returns the trace id stored in ctx, or "".
func TraceFrom(ctx context.Context) string {
	v, _ := ctx.Value(traceKey{}).(string)
	return v
}
