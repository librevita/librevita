package types

import "testing"

// TestUITheme mirrors the CHECK constraint on users.ui_theme: only
// system/light/dark are valid, and the stored representation
// round-trips.
func TestUITheme(t *testing.T) {
	for _, theme := range []UITheme{UIThemeSystem, UIThemeLight, UIThemeDark} {
		if !theme.Valid() {
			t.Fatalf("%q must be valid", theme)
		}
		if got, ok := ParseUITheme(theme.String()); !ok || got != theme {
			t.Fatalf("ParseUITheme(%q) = %q, %v; want %q, true", theme.String(), got, ok, theme)
		}
	}
	if UITheme("sepia").Valid() {
		t.Fatal("theme outside the CHECK set must not be valid")
	}
	if _, ok := ParseUITheme("sepia"); ok {
		t.Fatal("ParseUITheme must reject values outside the CHECK set")
	}
}
