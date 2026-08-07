// Package ui embeds the frontend assets (Tailwind CSS generated at build
// time, the vendored HTMX runtime, and the first-party application scripts)
// into the binary.
package ui

import "embed"

//go:embed static
var static embed.FS
