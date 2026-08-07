package components

import "testing"

func TestInitials(t *testing.T) {
	cases := map[string]string{
		"":               "?",
		"Ana":            "A",
		"Ana Souza":      "AS",
		"ana souza":      "AS",
		"  Carlos Lima ": "CL",
	}
	for in, want := range cases {
		if got := Initials(in); got != want {
			t.Errorf("Initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoleBadgeClass(t *testing.T) {
	cases := map[string]string{
		"admin":        "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300",
		"physician":    "bg-indigo-100 text-indigo-800 dark:bg-indigo-900 dark:text-indigo-300",
		"receptionist": "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300",
		"patient":      "bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-300",
	}
	for role, want := range cases {
		if got := RoleBadgeClass(role); got != want {
			t.Errorf("RoleBadgeClass(%q) = %q, want %q", role, got, want)
		}
	}
}
