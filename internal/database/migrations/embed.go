// Package migrations embeds dialect-specific database migration assets.
package migrations

import "embed"

// FS contains all embedded Goose migrations for SQLite and PostgreSQL.
//
//go:embed sqlite/*.sql postgres/*.sql
var FS embed.FS
