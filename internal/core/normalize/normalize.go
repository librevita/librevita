package normalize

import (
	"strings"
	"unicode"
)

// Phone normalizes phone numbers for canonical searching and indexing by stripping all non-digit characters.
// For example: "+55 (11) 98765-4321" -> "5511987654321".
func Phone(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Email normalizes email addresses for canonical searching and indexing
// by trimming whitespace and converting to lowercase in a permissive manner.
// For example: "  Doctor.John+clinic@EXAMPLE.com  " -> "doctor.john+clinic@example.com".
func Email(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// Document normalizes identification documents (CPF, CNPJ, National IDs, Passports)
// by stripping all punctuation and whitespace.
// For example: "123.456.789-00" -> "12345678900".
func Document(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// Text performs standard case-insensitive text normalization (trimming whitespace and lowercase).
func Text(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}
