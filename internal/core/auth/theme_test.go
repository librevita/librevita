package auth_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/auth"
)

// TestUITheme mirrors the CHECK constraint on users.ui_theme: only
// system/light/dark are valid, and the stored representation
// round-trips.
func TestUITheme(t *testing.T) {
	for _, theme := range []auth.UITheme{auth.UIThemeSystem, auth.UIThemeLight, auth.UIThemeDark} {
		assert.True(t, theme.Valid(), "%q must be valid", theme)
		got, ok := auth.ParseUITheme(theme.String())
		assert.True(t, ok)
		assert.Equal(t, theme, got)
	}
	assert.False(t, auth.UITheme("sepia").Valid())
	_, ok := auth.ParseUITheme("sepia")
	assert.False(t, ok)
}
