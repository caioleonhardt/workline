// Package migrations embeds the numbered SQL migration files.
package migrations

import "embed"

// FS holds NNNN_name.sql files, applied in order by internal/store.
//
//go:embed *.sql
var FS embed.FS
