package normalize

import (
	"strings"
	"unicode"

	"github.com/f1monkey/phonetic/beidermorse"
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

// IsStopWord reports whether w is a common name connective preposition/article.
func IsStopWord(w string) bool {
	return isStopWord(w)
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

// NameTokens extracts search tokens (words and prefix n-grams >= 1 chars) from a person's name.
// For example: "Carlos Silva" -> ["c", "ca", "car", "carl", "carlo", "carlos", "s", "si", "sil", "silv", "silva"].
func NameTokens(name string) []string {
	return NameTokensWithMinLen(name, 1)
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

var bmpmEncoder = beidermorse.MustNewEncoder(
	beidermorse.WithAccuracy(beidermorse.Exact),
	beidermorse.WithBufferReuse(true),
)

// StripDiacritics removes common accents and diacritics from Latin characters.
func StripDiacritics(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ã', 'â', 'ä', 'Á', 'À', 'Ã', 'Â', 'Ä':
			b.WriteByte('a')
		case 'é', 'è', 'ê', 'ë', 'É', 'È', 'Ê', 'Ë':
			b.WriteByte('e')
		case 'í', 'ì', 'î', 'ï', 'Í', 'Ì', 'Î', 'Ï':
			b.WriteByte('i')
		case 'ó', 'ò', 'õ', 'ô', 'ö', 'Ó', 'Ò', 'Õ', 'Ô', 'Ö':
			b.WriteByte('o')
		case 'ú', 'ù', 'û', 'ü', 'Ú', 'Ù', 'Û', 'Ü':
			b.WriteByte('u')
		case 'ç', 'Ç':
			b.WriteByte('c')
		case 'ñ', 'Ñ':
			b.WriteByte('n')
		case 'ý', 'ÿ', 'Ý', 'Ÿ':
			b.WriteByte('i')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isVowelRune(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	default:
		return false
	}
}

// NormalizeBrazilian unifies common Portuguese and Brazilian spelling and phonetic variations
// using the active sandboxed Expr rule engine.
func NormalizeBrazilian(w string) string {
	return ActiveRuleEngine().Normalize(w)
}

// PhoneticTokens returns up to maxTokens Beider-Morse phonetic codes for a word,
// prefixed with "ph:" to avoid namespace collision with textual tokens.
func PhoneticTokens(w string, maxTokens int) []string {
	clean := strings.TrimFunc(strings.ToLower(w), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if clean == "" || len(clean) < 2 {
		return nil
	}

	rawTokens := bmpmEncoder.Encode(clean)
	if len(rawTokens) == 0 {
		return nil
	}

	limit := maxTokens
	if limit <= 0 || limit > len(rawTokens) {
		limit = len(rawTokens)
	}

	seen := make(map[string]struct{}, limit)
	out := make([]string, 0, limit)
	for _, tok := range rawTokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		phTok := "ph:" + tok
		if _, exists := seen[phTok]; !exists {
			seen[phTok] = struct{}{}
			out = append(out, phTok)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

// MultiLanguageNameTokens extracts search tokens combining prefix n-grams (>= 3 chars)
// and multi-language phonetic tokens (Beider-Morse BMPM + Brazilian Portuguese normalization).
func MultiLanguageNameTokens(name string) []string {
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

	// 1. Prefix n-grams (orthographic, language-agnostic)
	for _, tok := range NameTokensWithMinLen(name, 1) {
		addToken(tok)
	}

	// 2. Multi-language phonetic and Brazilian normalized tokens
	for _, rawWord := range words {
		w := strings.TrimFunc(rawWord, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if w == "" || isStopWord(w) {
			continue
		}

		// A. Direct Beider-Morse tokens on original clean word
		for _, ph := range PhoneticTokens(w, 4) {
			addToken(ph)
		}

		// B. Brazilian normalized tokens
		br := NormalizeBrazilian(w)
		if br != "" && br != w {
			for _, tok := range NameTokensWithMinLen(br, 1) {
				addToken(tok)
			}
			for _, ph := range PhoneticTokens(br, 4) {
				addToken(ph)
			}
		}

		// C. Souza / Sousa equivalence
		if strings.Contains(w, "ou") {
			oVar := strings.ReplaceAll(w, "ou", "o")
			for _, tok := range NameTokensWithMinLen(oVar, 1) {
				addToken(tok)
			}
			for _, ph := range PhoneticTokens(oVar, 4) {
				addToken(ph)
			}
		}
		if strings.Contains(w, "sou") {
			zVar := strings.ReplaceAll(w, "sou", "souz")
			for _, tok := range NameTokensWithMinLen(zVar, 1) {
				addToken(tok)
			}
			for _, ph := range PhoneticTokens(zVar, 4) {
				addToken(ph)
			}
		}
	}

	return tokens
}
