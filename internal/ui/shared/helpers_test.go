package shared

import (
	"strings"
	"testing"
)

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
		"patient":      "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-300",
	}
	// Custom roles derive a stable color from the name.
	if got := RoleBadgeClass("patient"); got != RoleBadgeClass("patient") {
		t.Errorf("RoleBadgeClass must be stable, got %q twice", got)
	}
	for _, role := range []string{"psychologist", "manager", "social-worker"} {
		got := RoleBadgeClass(role)
		if !strings.HasPrefix(got, "bg-") || !strings.Contains(got, "dark:bg-") {
			t.Errorf("RoleBadgeClass(%q) = %q, want a palette pair", role, got)
		}
	}
	for role, want := range cases {
		if got := RoleBadgeClass(role); got != want {
			t.Errorf("RoleBadgeClass(%q) = %q, want %q", role, got, want)
		}
	}
}

func TestBtnClass(t *testing.T) {
	cases := map[string]string{
		"primary":   "btn",
		"secondary": "btn-secondary",
		"danger":    "btn-danger",
		"unknown":   "btn",
	}
	for kind, want := range cases {
		if got := BtnClass(kind); got != want {
			t.Errorf("btnClass(%q) = %q, want %q", kind, got, want)
		}
	}
}

func TestInputAndCheckboxClass(t *testing.T) {
	if got := InputClass(""); got != "input" {
		t.Errorf("InputClass(\"\") = %q, want \"input\"", got)
	}
	if got := InputClass("border-red-500"); got != "input border-red-500" {
		t.Errorf("InputClass(\"border-red-500\") = %q, want \"input border-red-500\"", got)
	}

	base := "h-4 w-4 rounded border-gray-300 text-indigo-600 focus:ring-indigo-500 dark:border-gray-600 dark:bg-gray-700 dark:ring-offset-gray-800"
	if got := CheckboxClass(""); got != base {
		t.Errorf("CheckboxClass(\"\") = %q, want base", got)
	}
	if got := CheckboxClass("extra"); got != base+" extra" {
		t.Errorf("CheckboxClass(\"extra\") = %q, want base extra", got)
	}
}
