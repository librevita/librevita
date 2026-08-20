package auth_test

import (
	"testing"

	"librevita.org/internal/core/auth"
)

// TestUITheme mirrors the CHECK constraint on users.ui_theme: only
// system/light/dark are valid, and the stored representation
// round-trips.
func TestUITheme(t *testing.T) {
	for _, theme := range []auth.UITheme{auth.UIThemeSystem, auth.UIThemeLight, auth.UIThemeDark} {
		if !theme.Valid() {
			t.Fatalf("%q must be valid", theme)
		}
		if got, ok := auth.ParseUITheme(theme.String()); !ok || got != theme {
			t.Fatalf("ParseUITheme(%q) = %q, %v; want %q, true", theme.String(), got, ok, theme)
		}
	}
	if auth.UITheme("sepia").Valid() {
		t.Fatal("theme outside the CHECK set must not be valid")
	}
	if _, ok := auth.ParseUITheme("sepia"); ok {
		t.Fatal("ParseUITheme must reject values outside the CHECK set")
	}
}
