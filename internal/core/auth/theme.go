package auth

// UITheme is the color scheme of the web interface. It mirrors the
// CHECK constraint on users.ui_theme (see
// db/migrations/00002_auth.sql).
type UITheme string

const (
	// UIThemeSystem follows the browser or operating system scheme.
	UIThemeSystem UITheme = "system"
	// UIThemeLight forces the light scheme.
	UIThemeLight UITheme = "light"
	// UIThemeDark forces the dark scheme.
	UIThemeDark UITheme = "dark"
)

// Valid reports whether t is one of the options the database CHECK
// constraint accepts.
func (t UITheme) Valid() bool {
	switch t {
	case UIThemeSystem, UIThemeLight, UIThemeDark:
		return true
	}
	return false
}

// String returns the stored representation of t.
func (t UITheme) String() string {
	return string(t)
}

// ParseUITheme converts a stored value back to the enum. ok is false
// when the value is not one of the CHECK options.
func ParseUITheme(s string) (UITheme, bool) {
	t := UITheme(s)
	return t, t.Valid()
}
