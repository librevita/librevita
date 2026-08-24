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

func isStopWord(w string) bool {
	switch w {
	case "da", "de", "do", "das", "dos", "e",
		"del", "van", "von", "der", "the", "and", "of":
		return true
	default:
		return false
	}
}

// NameTokens extracts search tokens (words and prefix n-grams >= 3 chars) from a person's name.
// For example: "Carlos Silva" -> ["car", "carl", "carlo", "carlos", "sil", "silv", "silva"].
func NameTokens(name string) []string {
	return NameTokensWithMinLen(name, 3)
}

// NameTokensWithMinLen extracts unique words and prefix n-grams starting from minPrefixLen characters.
func NameTokensWithMinLen(name string, minPrefixLen int) []string {
	if name == "" {
		return nil
	}
	clean := strings.ToLower(strings.TrimSpace(name))
	words := strings.Fields(clean)
	if len(words) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	var tokens []string

	addToken := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			tokens = append(tokens, t)
		}
	}

	for _, w := range words {
		w = strings.TrimFunc(w, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w == "" || isStopWord(w) {
			continue
		}

		runes := []rune(w)
		wLen := len(runes)

		if wLen < minPrefixLen {
			addToken(w)
			continue
		}

		for i := minPrefixLen; i <= wLen; i++ {
			addToken(string(runes[:i]))
		}
	}

	return tokens
}
