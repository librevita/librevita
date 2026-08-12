package identifier

import (
	"strings"
	"testing"

	"librevita.org/internal/domain/patient/repository"
)

// testSystem builds a repository row for tests.
func testSystem(system, displayName, pattern string, transform Transform,
	algo CheckAlgorithm, base, dv, start int) repository.IdentifierSystem {
	return repository.IdentifierSystem{
		System:           system,
		DisplayName:      displayName,
		Pattern:          pattern,
		Transform:        string(transform),
		CheckAlgorithm:   string(algo),
		CheckBaseLen:     int64(base),
		CheckDvCount:     int64(dv),
		CheckStartWeight: int64(start),
		Active:           1,
	}
}

// mustConfigured parses a row into a strategy, failing the test.
func mustConfigured(t *testing.T, row repository.IdentifierSystem) *configured {
	t.Helper()
	c, err := newConfigured(row)
	if err != nil {
		t.Fatalf("newConfigured(%s): %v", row.System, err)
	}
	return c
}

// seedRows reproduces the migration seeds for registry tests.
func seedRows() []repository.IdentifierSystem {
	return []repository.IdentifierSystem{
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
		ok    bool
	}{
		{"123.456.789-09", true}, // canonical example, valid
		{"52998224725", true},    // documentation example, valid
		{"529.982.247-25", true}, // same, formatted
		{"12345678901", false},   // wrong check digit
		{"11111111111", false},   // all equal
		{"1234567890", false},    // wrong length
		{"1234567890a", false},   // non-digit
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := c.Normalize(tc.value)
			if tc.ok && err != nil {
				t.Fatalf("Normalize(%q) = %v, want valid", tc.value, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("Normalize(%q) = %q, want error", tc.value, got)
			}
			if tc.ok && got != "12345678909" && got != "52998224725" {
				t.Fatalf("Normalize(%q) = %q, unexpected canonical form", tc.value, got)
			}
		})
	}
}

func TestNIF(t *testing.T) {
	c := mustConfigured(t, seedRows()[2])

	cases := []struct {
		value string
		ok    bool
	}{
		{"999999990", true},  // documented valid test number
		{"123456789", true},  // check digit 9, valid
		{"999999999", false}, // wrong check digit
		{"123456780", false}, // wrong check digit
		{"99999999", false},  // wrong length
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := c.Normalize(tc.value)
			if tc.ok && err != nil {
				t.Fatalf("Normalize(%q) = %v, want valid", tc.value, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("Normalize(%q) = %q, want error", tc.value, got)
			}
		})
	}
}

func TestSUS(t *testing.T) {
	c := mustConfigured(t, seedRows()[1])

	cases := []struct {
		value string
		ok    bool
	}{
		{"123456789012340", true},  // computed from the DATASUS scheme
		{"123456789012341", false}, // wrong check digit
		{"111111111111111", false}, // all equal
		{"12345678901234", false},  // wrong length
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := c.Normalize(tc.value)
			if tc.ok && err != nil {
				t.Fatalf("Normalize(%q) = %v, want valid", tc.value, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("Normalize(%q) = %q, want error", tc.value, got)
			}
		})
	}
}

func TestPassport(t *testing.T) {
	c := mustConfigured(t, seedRows()[3])

	cases := []struct {
		value string
		want  string
		ok    bool
	}{
		{"ab1234567", "AB1234567", true}, // case-folded to uppercase
		{"C123456", "C123456", true},
		{"ABCD123456", "", false}, // too many letters
		{"AB12345", "", false},    // too few digits
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			got, err := c.Normalize(tc.value)
			if tc.ok {
				if err != nil {
					t.Fatalf("Normalize(%q) = %v, want %q", tc.value, err, tc.want)
				}
				if got != tc.want {
					t.Fatalf("Normalize(%q) = %q, want %q", tc.value, got, tc.want)
				}
			} else if err == nil {
				t.Fatalf("Normalize(%q) = %q, want error", tc.value, got)
			}
		})
	}
}

func TestConfiguredDetect(t *testing.T) {
	c := mustConfigured(t, seedRows()[0])
	for _, value := range []string{"123.456.789-09", "52998224725", " 12345678909 "} {
		if !c.Detect(value) {
			t.Fatalf("Detect(%q) = false, want true", value)
		}
	}
	if c.Detect("12345") {
		t.Fatal("Detect(12345) = true, want false")
	}
}

func TestConfiguredRejectsConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		row  repository.IdentifierSystem
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
