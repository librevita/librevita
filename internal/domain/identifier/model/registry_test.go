package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryLoadAndDetect(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

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
			got := reg.Detect(tc.raw)
			assert.Equal(t, tc.system, got.System())
		})
	}
}

func TestRegistryDetectCandidatesIncludesFallback(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	candidates := reg.DetectCandidates("999999990")
	saw := map[string]bool{}
	for _, c := range candidates {
		saw[c.System()] = true
	}
	assert.True(t, saw[NIFSystem])
	assert.True(t, saw[RawSystem])
	assert.False(t, saw[PassportSystem])
}

func TestRegistryForSystemFallsBack(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	got := reg.ForSystem(NIFSystem)
	assert.Equal(t, NIFSystem, got.System())

	unknown := "urn:librevita:id:py:cedula"
	got = reg.ForSystem(unknown)
	assert.Equal(t, RawSystem, got.System())

	reg.mu.Lock()
	delete(reg.systems, NIFSystem)
	reg.mu.Unlock()

	got = reg.ForSystem(NIFSystem)
	assert.Equal(t, RawSystem, got.System())
}

func TestRegistryReloadRejectsBadRows(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	err := reg.Reload([]*IdentifierSystem{
		testSystem("urn:librevita:id:bad", "Bad", "[", TransformNone, CheckNone, 0, 1, 10),
	})
	assert.Error(t, err)

	got := reg.Detect("anything")
	assert.Equal(t, RawSystem, got.System())
}

func TestRegistryDetectionOrderLongestPatternStringFirst(t *testing.T) {
	reg := NewRegistry()
	rows := []*IdentifierSystem{
		testSystem("urn:librevita:id:x:short", "Short", "[0-9]{12}", TransformDigits, CheckNone, 0, 1, 10),
		testSystem("urn:librevita:id:x:long", "Long", "[0-9]{10,12}", TransformDigits, CheckNone, 0, 1, 10),
	}
	require.NoError(t, reg.Reload(rows))

	got := reg.Detect("123456789012")
	assert.Equal(t, "urn:librevita:id:x:long", got.System())
}

func TestRegistryReloadKeepsPreviousStateOnFailure(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	before := reg.Systems()
	err := reg.Reload([]*IdentifierSystem{
		testSystem("urn:librevita:id:bad", "Bad", ")", TransformNone, CheckNone, 0, 1, 10),
	})
	assert.Error(t, err)

	after := reg.Systems()
	assert.Equal(t, len(before), len(after))
}
