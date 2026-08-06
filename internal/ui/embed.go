// Package ui embeds the frontend assets (Tailwind CSS generated at build
// time, vendored HTMX/Alpine runtimes, and the small application script)
// into the binary.
package ui

import "embed"

//go:embed static
var static embed.FS
