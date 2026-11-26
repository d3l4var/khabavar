// Package migrations embeds the SQL migration files so they ship inside the
// binary and are available to integration tests without an external tool.
//
// Files follow the convention NNNNNN_name.{up,down}.sql. Only *.up.sql files
// are applied by ApplyAll (down migrations are not used by the application at
// runtime — they exist for local development rollback only).
package migrations

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *.sql
var fsys embed.FS

// Migration is a single ordered up-migration.
type Migration struct {
	Seq  string // "000001"
	Name string // "identity"
	SQL  string
}

// UpMigrations returns every *.up.sql file sorted by sequence prefix.
// Down migrations and README are ignored.
func UpMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		body, err := fsys.ReadFile(name)
		if err != nil {
			return nil, err
		}
		stem := strings.TrimSuffix(name, ".up.sql")
		// NNNNNN_name → seq, name
		idx := strings.Index(stem, "_")
		if idx < 0 {
			continue
		}
		out = append(out, Migration{
			Seq:  stem[:idx],
			Name: stem[idx+1:],
			SQL:  string(body),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}
