package shared

import "strings"

// Initials renders up to two initials from a display name.
func Initials(name string) string {
	if name == "" {
		return "?"
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return strings.ToUpper(parts[0][:1])
	}
	return strings.ToUpper(parts[0][:1] + parts[1][:1])
}

// RoleBadgeClass returns the badge colors for an account role. Custom
// roles pick a color deterministically from their name, so every role
// stays distinguishable without new palette entries.
func RoleBadgeClass(role string) string {
	switch role {
	case "admin":
		return "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300"
	case "physician":
		return "bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-300"
	case "receptionist":
		return "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300"
	default:
		palettes := []string{
			"gray", "red", "indigo", "green", "blue", "purple", "amber",
		}
		// Sum of the runes; stable for a given name.
		sum := 0
		for _, r := range role {
			sum += int(r)
		}
		base := palettes[sum%len(palettes)]
		return "bg-" + base + "-100 text-" + base + "-800 dark:bg-" + base + "-900 dark:text-" + base + "-300"
	}
}

// BtnClass returns the style class for a button kind. Unknown kinds fall
// back to the primary button.
func BtnClass(kind string) string {
	switch kind {
	case "secondary":
		return "btn-secondary"
	case "danger":
		return "btn-danger"
	default:
		return "btn"
	}
}

// InputClass joins the base form control class with optional extra
// classes, avoiding a trailing space when extras are empty.
func InputClass(extra string) string {
	if extra == "" {
		return "input"
	}
	return "input " + extra
}

// CheckboxClass joins the base checkbox classes with optional extras.
func CheckboxClass(extra string) string {
	base := "h-4 w-4 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500 dark:border-gray-600 dark:bg-gray-700 dark:ring-offset-gray-800"
	if extra == "" {
		return base
	}
	return base + " " + extra
}
