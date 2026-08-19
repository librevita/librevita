package identifier

import (
	"testing"
)

func TestRegistryLoadAndDetect(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Reload(seedRows()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	cases := []struct {
		raw    string
		system string
	}{
		{"123.456.789-09", CPFSystem},
		{"52998224725", CPFSystem},
		{"123456789012341", SUSSystem},
		{"999999990", NIFSystem},
		{"ab1234567", PassportSystem},
		{"whatever id", RawSystem},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			if got := reg.Detect(tc.raw); got.System() != tc.system {
				t.Fatalf("Detect(%q) = %s, want %s", tc.raw, got.System(), tc.system)
			}
		})
	}
}

func TestRegistryDetectCandidatesIncludesFallback(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Reload(seedRows()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	candidates := reg.DetectCandidates("999999990")
	saw := map[string]bool{}
	for _, c := range candidates {
		saw[c.System()] = true
	}
	if !saw[NIFSystem] || !saw[RawSystem] {
		t.Fatalf("candidates = %v, want NIF and raw", saw)
	}
	if saw[PassportSystem] {
		t.Fatal("passport must not match a bare digit string")
	}
}

func TestRegistryForSystemFallsBack(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Reload(seedRows()); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if got := reg.ForSystem(NIFSystem); got.System() != NIFSystem {
		t.Fatalf("ForSystem(NIF) = %s", got.System())
	}
	unknown := "urn:librevita:id:py:cedula"
	if got := reg.ForSystem(unknown); got.System() != RawSystem {
		t.Fatalf("ForSystem(%s) = %s, want raw", unknown, got.System())
	}
	reg.mu.Lock()
	delete(reg.systems, NIFSystem)
	reg.mu.Unlock()
	if got := reg.ForSystem(NIFSystem); got.System() != RawSystem {
		t.Fatalf("ForSystem(deactivated) = %s, want raw", got.System())
	}
}

func TestRegistryReloadRejectsBadRows(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Reload(seedRows()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if err := reg.Reload([]*IdentifierSystem{
		testSystem("urn:librevita:id:bad", "Bad", "[", TransformNone, CheckNone, 0, 1, 10),
	}); err == nil {
		t.Fatal("Reload with a bad pattern must fail")
	}
	if got := reg.Detect("anything"); got.System() != RawSystem {
		t.Fatalf("after failed reload, Detect = %s, want raw (previous state kept)", got.System())
	}
}

func TestRegistryDetectionOrderLongestPatternStringFirst(t *testing.T) {
	reg := NewRegistry()
	rows := []*IdentifierSystem{
		testSystem("urn:librevita:id:x:short", "Short", "[0-9]{12}", TransformDigits, CheckNone, 0, 1, 10),
		testSystem("urn:librevita:id:x:long", "Long", "[0-9]{10,12}", TransformDigits, CheckNone, 0, 1, 10),
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if got := reg.Detect("123456789012"); got.System() != "urn:librevita:id:x:long" {
		t.Fatalf("Detect = %s, want the longest pattern string", got.System())
	}
}

func TestRegistryReloadKeepsPreviousStateOnFailure(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Reload(seedRows()); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	before := reg.Systems()
	if err := reg.Reload([]*IdentifierSystem{
		testSystem("urn:librevita:id:bad", "Bad", ")", TransformNone, CheckNone, 0, 1, 10),
	}); err == nil {
		t.Fatal("Reload with a bad pattern must fail")
	}
	after := reg.Systems()
	if len(before) != len(after) {
		t.Fatalf("failed reload changed the registry: %v -> %v", before, after)
	}
}
