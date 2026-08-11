// Package migrations exposes the ordered SQLite schema embedded in the binary.
package migrations

import "embed"

// Files contains every numbered SQL migration in this directory.
//
//go:embed *.sql
var Files embed.FS
