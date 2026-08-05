// Package db embeds persistence assets in the binary.
//
// This package lives at the repository root because //go:embed cannot refer
// to parent directories. internal/core/database consumes these assets here.
package db

import "embed"

// Migrations contains all SQL files under db/migrations.
//
//go:embed migrations
var Migrations embed.FS
