// Package migrations exposes the SQL migrations as an embed.FS so other
// packages (notably internal/db) can run them at startup without reading
// from disk.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
