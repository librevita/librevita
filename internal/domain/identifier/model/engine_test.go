package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/pkg/urn"
)

// testSystem builds a domain entity for tests.
func testSystem(system, displayName, pattern string, transform Transform,
	algo CheckAlgorithm, base, dv, start int) *IdentifierSystem {
	return &IdentifierSystem{
		System:           system,
		DisplayName:      displayName,
		Pattern:          pattern,
		Transform:        transform,
		CheckAlgorithm:   algo,
		CheckBaseLen:     base,
		CheckDVCount:     dv,
		CheckStartWeight: start,
		Active:           true,
	}
}

// mustConfigured parses a row into a strategy, failing the test.
func mustConfigured(t *testing.T, row *IdentifierSystem) *Configured {
	t.Helper()
	c, err := NewConfigured(row)
	require.NoError(t, err, "NewConfigured(%s)", row.System)
	return c
}

// seedRows reproduces the migration seeds for registry tests.
func seedRows() []*IdentifierSystem {
	return []*IdentifierSystem{
		testSystem(urn.Identifier("br", "cpf"), "CPF (Brasil)", "[0-9]{11}", TransformDigits, CheckMod11Desc, 9, 2, 10),
		testSystem(urn.Identifier("br", "sus"), "Cartão SUS (Brasil)", "[0-9]{15}", TransformDigits, CheckMod11Cyclic, 14, 1, 10),
		testSystem(urn.Identifier("pt", "nif"), "NIF (Portugal)", "[0-9]{9}", TransformDigits, CheckMod11Desc, 8, 1, 9),
		testSystem(urn.Identifier("passport"), "Passaporte", "[A-Z]{1,2}[0-9]{6,9}", TransformUpper, CheckNone, 0, 1, 10),
	}
}

func TestCPF(t *testing.T) {
	c := mustConfigured(t, seedRows()[0])

	cases := []struct {
		value string
		want  string
	}{
		{"12345678909", "12345678909"},
		{"123.456.789-09", "12345678909"},
		{"  123.456.789-09  ", "12345678909"},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := c.Normalize(tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	invalid := []string{
		"12345678900",
		"11111111111",
		"1234567890",
		"123456789012",
		"abc",
	}
	for _, raw := range invalid {
		t.Run("invalid/"+raw, func(t *testing.T) {
			_, err := c.Normalize(raw)
			assert.Error(t, err)
		})
	}
}

func TestSUS(t *testing.T) {
	c := mustConfigured(t, seedRows()[1])

	valid := "123456789012340"
	assert.True(t, c.Detect(valid))

	got, err := c.Normalize(" 123 4567 8901 2340 ")
	require.NoError(t, err)
	assert.Equal(t, valid, got)

	_, err = c.Normalize("123456789012341")
	assert.Error(t, err)
}

func TestNIF(t *testing.T) {
	c := mustConfigured(t, seedRows()[2])

	valid := "123456789"
	got, err := c.Normalize(" 123 456 789 ")
	require.NoError(t, err)
	assert.Equal(t, valid, got)

	valid2 := "999999990"
	got2, err := c.Normalize(valid2)
	require.NoError(t, err)
	assert.Equal(t, valid2, got2)

	_, err = c.Normalize("123456780")
	assert.Error(t, err)
}

func TestPassport(t *testing.T) {
	c := mustConfigured(t, seedRows()[3])

	cases := []struct {
		raw  string
		want string
	}{
		{"fx123456", "FX123456"},
		{"  a12345678  ", "A12345678"},
		{"CC123456789", "CC123456789"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := c.Normalize(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	invalid := []string{
		"1234567",
		"ABC123456",
		"A12345",
	}
	for _, raw := range invalid {
		t.Run("invalid/"+raw, func(t *testing.T) {
			_, err := c.Normalize(raw)
			assert.Error(t, err)
		})
	}
}

func TestDetect(t *testing.T) {
	c := mustConfigured(t, seedRows()[0])
	assert.True(t, c.Detect("12345678909"))
	assert.True(t, c.Detect("123.456.789-09"))
	assert.False(t, c.Detect("12345"))
}

func TestConfiguredRejectsConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		row  *IdentifierSystem
	}{
		{"empty pattern", testSystem(urn.Identifier("x"), "X", "", TransformNone, CheckNone, 0, 1, 10)},
		{"bad regex", testSystem(urn.Identifier("x"), "X", "[", TransformNone, CheckNone, 0, 1, 10)},
		{"bad transform", testSystem(urn.Identifier("x"), "X", "[0-9]{1}", "shout", CheckNone, 0, 1, 10)},
		{"cyclic with two digits", testSystem(urn.Identifier("x"), "X", "[0-9]{2}", TransformDigits, CheckMod11Cyclic, 1, 2, 10)},
		{"base without algorithm", testSystem(urn.Identifier("x"), "X", "[0-9]{2}", TransformDigits, CheckNone, 1, 1, 10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewConfigured(tc.row)
			assert.Error(t, err)
		})
	}
}

func TestRawFallback(t *testing.T) {
	raw := rawStrategy{}
	assert.True(t, raw.Detect("anything at all"))

	got, err := raw.Normalize("  some   ID 123 ")
	require.NoError(t, err)
	assert.Equal(t, "SOME ID 123", got)

	_, err = raw.Normalize("   ")
	assert.ErrorIs(t, err, ErrValueRequired)

	_, err = raw.Normalize(strings.Repeat("a", 121))
	assert.Error(t, err)
}
