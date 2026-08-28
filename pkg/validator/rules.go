package validator

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

// NotBlank checks if a string contains non-whitespace characters.
func NotBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

// MinRunes checks if a string has at least n unicode code points (runes).
func MinRunes(value string, n int) bool {
	return utf8.RuneCountInString(value) >= n
}

// MaxRunes checks if a string has at most n unicode code points (runes).
func MaxRunes(value string, n int) bool {
	return utf8.RuneCountInString(value) <= n
}

// BetweenRunes checks if the rune count of a string is between min and max inclusive.
func BetweenRunes(value string, min, max int) bool {
	count := utf8.RuneCountInString(value)
	return count >= min && count <= max
}

// ValidEmail verifies that the email address is syntactically valid according to RFC 5322,
// contains no whitespace, and includes a domain part with a dot.
func ValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return false
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// In checks if a value is present in a list of allowed values.
func In[T comparable](value T, allowed ...T) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// NotIn checks if a value is absent from a list of blocked values.
func NotIn[T comparable](value T, blocked ...T) bool {
	for _, b := range blocked {
		if value == b {
			return false
		}
	}
	return true
}

// Matches checks if a string matches the provided compiled regular expression.
func Matches(value string, rx *regexp.Regexp) bool {
	if rx == nil {
		return false
	}
	return rx.MatchString(value)
}

// ValidUUID checks if a string is a valid UUID representation.
func ValidUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
