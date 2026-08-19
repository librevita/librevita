package identifier

import (
	"strings"
	"testing"
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
func mustConfigured(t *testing.T, row *IdentifierSystem) *configured {
	t.Helper()
	c, err := newConfigured(row)
	if err != nil {
		t.Fatalf("newConfigured(%s): %v", row.System, err)
	}
	return c
}

// seedRows reproduces the migration seeds for registry tests.
func seedRows() []*IdentifierSystem {
	return []*IdentifierSystem{
		testSystem(CPFSystem, "CPF (Brasil)", "[0-9]{11}", TransformDigits, CheckMod11Desc, 9, 2, 10),
		testSystem(SUSSystem, "Cartão SUS (Brasil)", "[0-9]{15}", TransformDigits, CheckMod11Cyclic, 14, 1, 10),
		testSystem(NIFSystem, "NIF (Portugal)", "[0-9]{9}", TransformDigits, CheckMod11Desc, 8, 1, 9),
		testSystem(PassportSystem, "Passaporte", "[A-Z]{1,2}[0-9]{6,9}", TransformUpper, CheckNone, 0, 1, 10),
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
			if err != nil {
				t.Fatalf("Normalize(%s): %v", tc.value, err)
			}
			if got != tc.want {
				t.Fatalf("Normalize(%s) = %q, want %q", tc.value, got, tc.want)
			}
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
			if _, err := c.Normalize(raw); err == nil {
				t.Fatalf("Normalize(%s) expected an error", raw)
			}
		})
	}
}

func TestSUS(t *testing.T) {
	c := mustConfigured(t, seedRows()[1])

	valid := "123456789012340"
	if !c.Detect(valid) {
		t.Fatal("Detect(valid) = false, want true")
	}
	got, err := c.Normalize(" 123 4567 8901 2340 ")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != valid {
		t.Fatalf("Normalize = %q, want %q", got, valid)
	}

	if _, err := c.Normalize("123456789012341"); err == nil {
		t.Fatal("Normalize(bad dv) expected an error")
	}
}

func TestNIF(t *testing.T) {
	c := mustConfigured(t, seedRows()[2])

	valid := "123456789"
	got, err := c.Normalize(" 123 456 789 ")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != valid {
		t.Fatalf("Normalize = %q, want %q", got, valid)
	}

	valid2 := "999999990"
	got2, err := c.Normalize(valid2)
	if err != nil || got2 != valid2 {
		t.Fatalf("Normalize(%s) = %q, err: %v", valid2, got2, err)
	}

	if _, err := c.Normalize("123456780"); err == nil {
		t.Fatal("Normalize(bad dv) expected an error")
	}
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
			if err != nil {
				t.Fatalf("Normalize(%s): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("Normalize(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}

	invalid := []string{
		"1234567",
		"ABC123456",
		"A12345",
	}
	for _, raw := range invalid {
		t.Run("invalid/"+raw, func(t *testing.T) {
			if _, err := c.Normalize(raw); err == nil {
				t.Fatalf("Normalize(%s) expected an error", raw)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	c := mustConfigured(t, seedRows()[0])
	if !c.Detect("12345678909") {
		t.Fatal("Detect(12345678909) = false, want true")
	}
	if !c.Detect("123.456.789-09") {
		t.Fatal("Detect(123.456.789-09) = false, want true")
	}
	if c.Detect("12345") {
		t.Fatal("Detect(12345) = true, want false")
	}
}

func TestConfiguredRejectsConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		row  *IdentifierSystem
	}{
		{"empty pattern", testSystem("urn:librevita:id:x", "X", "", TransformNone, CheckNone, 0, 1, 10)},
		{"bad regex", testSystem("urn:librevita:id:x", "X", "[", TransformNone, CheckNone, 0, 1, 10)},
		{"bad transform", testSystem("urn:librevita:id:x", "X", "[0-9]{1}", "shout", CheckNone, 0, 1, 10)},
		{"cyclic with two digits", testSystem("urn:librevita:id:x", "X", "[0-9]{2}", TransformDigits, CheckMod11Cyclic, 1, 2, 10)},
		{"base without algorithm", testSystem("urn:librevita:id:x", "X", "[0-9]{2}", TransformDigits, CheckNone, 1, 1, 10)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := newConfigured(tc.row); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestRawFallback(t *testing.T) {
	raw := rawStrategy{}
	if !raw.Detect("anything at all") {
		t.Fatal("raw fallback must detect everything")
	}
	got, err := raw.Normalize("  some   ID 123 ")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got != "SOME ID 123" {
		t.Fatalf("Normalize = %q, want %q", got, "SOME ID 123")
	}
	if _, err := raw.Normalize("   "); err != ErrValueRequired {
		t.Fatalf("Normalize(blank) = %v, want ErrValueRequired", err)
	}
	if _, err := raw.Normalize(strings.Repeat("a", 121)); err == nil {
		t.Fatal("Normalize(too long) = nil, want error")
	}
}
