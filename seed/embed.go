// Package seed embeds the baseline seed so tests can apply it without reading the
// filesystem (mirrors migrations/embed.go).
//
// The seed is the single source of truth for the RBAC matrix (IAM §۱/§۴): roles,
// permission codes and role→permission assignments live here and nowhere else. The
// seed_test.go gate asserts the matrix below so a seed edit cannot silently widen
// access.
package seed

import (
	"embed"
	"io/fs"
)

//go:embed *.sql
var fsys embed.FS

// BaselineSQL returns the baseline seed script.
func BaselineSQL() (string, error) {
	b, err := fs.ReadFile(fsys, "001_seed_baseline.sql")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Files lists the embedded seed files (for diagnostics).
func Files() ([]string, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}
