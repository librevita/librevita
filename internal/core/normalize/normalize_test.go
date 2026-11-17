package normalize_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/normalize"
)

func TestPhone(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"+55 (11) 98765-4321", "5511987654321"},
		{"(21) 91234-5678", "21912345678"},
		{"  +1 (555) 234-5678  ", "15552345678"},
		{"1234567890", "1234567890"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Phone(tt.input))
	}
}

func TestEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"  Doctor.John+clinic@EXAMPLE.COM  ", "doctor.john+clinic@example.com"},
		{"patient.name@hospital.org.br", "patient.name@hospital.org.br"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Email(tt.input))
	}
}

func TestDocument(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"123.456.789-00", "12345678900"},
		{"12.345.678/0001-90", "12345678000190"},
		{"  AB-123.456  ", "ab123456"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Document(tt.input))
	}
}

func TestText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"  Maria Joana da Silva  ", "maria joana da silva"},
		{"Dr. John Doe", "dr. john doe"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.Text(tt.input))
	}
}

func TestNameTokens(t *testing.T) {
	assert.Nil(t, normalize.NameTokens(""))
	assert.Nil(t, normalize.NameTokens("   "))

	tokens := normalize.NameTokens("Carlos Eduardo da Silva")
	expected := []string{
		"c", "ca", "car", "carl", "carlo", "carlos",
		"e", "ed", "edu", "edua", "eduar", "eduard", "eduardo",
		"s", "si", "sil", "silv", "silva",
	}
	assert.Equal(t, expected, tokens)

	shortTokens := normalize.NameTokens("Ana Li")
	assert.Equal(t, []string{"a", "an", "ana", "l", "li"}, shortTokens)
}

func TestStripDiacritics(t *testing.T) {
	assert.Equal(t, "concessao", normalize.StripDiacritics("concessão"))
	assert.Equal(t, "luis", normalize.StripDiacritics("luís"))
	assert.Equal(t, "muller", normalize.StripDiacritics("müller"))
	assert.Equal(t, "joao", normalize.StripDiacritics("joão"))
}

func TestNormalizeBrazilian(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Walter", "valter"},
		{"Watanabe", "vatanabe"},
		{"Wanderley", "vanderlei"},
		{"Luiz", "luis"},
		{"Muniz", "munis"},
		{"Phelipe", "felipe"},
		{"Ely", "eli"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, normalize.NormalizeBrazilian(tt.input))
	}
}

func TestPhoneticTokens(t *testing.T) {
	tokens := normalize.PhoneticTokens("Schmidt", 2)
	assert.NotEmpty(t, tokens)
	for _, tok := range tokens {
		assert.True(t, strings.HasPrefix(tok, "ph:"))
	}

	// Empty / short
	assert.Nil(t, normalize.PhoneticTokens("", 2))
	assert.Nil(t, normalize.PhoneticTokens("a", 2))
}

func TestMultiLanguageNameTokens_Intersections(t *testing.T) {
	// Pairs of (storedName, queryVariation) that MUST intersect on at least one token
	pairs := []struct {
		stored string
		query  string
		reason string
	}{
		{"Carlos Schmidt", "Shimit", "German surname vs phonetic PT-BR transcription"},
		{"Carlos Schmidt", "Schmit", "German surname spelling variation"},
		{"Valter Silva", "Walter", "German/Brazilian W vs V"},
		{"Rossi Chiari", "Kiari", "Italian hard C (ch) sound"},
		{"Luiz Santos", "Luís", "Portuguese Z vs S"},
		{"Phelipe Ramos", "Felipe", "Ph vs F"},
		{"Sandro Watanabe", "Vatanabe", "Japanese W vs V in Brazil"},
		{"Helena Kowalski", "Kovalski", "Slavic W as V"},
		{"Yara Nogueira", "Iara", "Y vs I full name"},
		{"Yara Nogueira", "Iar", "Y vs I prefix n-gram"},
		{"Phelipe Ramos", "Fel", "Ph vs F prefix n-gram"},
		{"Valter Silva", "Wal", "V vs W prefix n-gram"},
	}

	for _, p := range pairs {
		storedTokens := normalize.MultiLanguageNameTokens(p.stored)
		queryTokens := normalize.MultiLanguageNameTokens(p.query)

		tokenSet := make(map[string]struct{})
		for _, tok := range storedTokens {
			tokenSet[tok] = struct{}{}
		}

		var matched []string
		for _, qTok := range queryTokens {
			if _, ok := tokenSet[qTok]; ok {
				matched = append(matched, qTok)
			}
		}

		assert.NotEmptyf(t, matched, "Expected match between %q and %q (%s). Stored: %v, Query: %v",
			p.stored, p.query, p.reason, storedTokens, queryTokens)
	}
}

func BenchmarkMultiLanguageNameTokens(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = normalize.MultiLanguageNameTokens("Carlos Eduardo da Silva Schmidt")
	}
}

func BenchmarkNormalizeBrazilian(b *testing.B) {
	words := []string{"Walter", "Schmidt", "Shimit", "Luiz", "Phelipe", "Watanabe", "Chiari"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, w := range words {
			_ = normalize.NormalizeBrazilian(w)
		}
	}
}
