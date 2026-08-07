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

// RoleBadgeClass returns the badge colors for an account role.
func RoleBadgeClass(role string) string {
	switch role {
	case "admin":
		return "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300"
	case "physician":
		return "bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-300"
	case "receptionist":
		return "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300"
	default:
		return "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300"
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
