package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/pkg/urn"
)

func TestRegistryLoadAndDetect(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	cases := []struct {
		raw    string
		system string
	}{
		{"123.456.789-09", urn.Identifier("br", "cpf")},
		{"52998224725", urn.Identifier("br", "cpf")},
		{"123456789012341", urn.Identifier("br", "sus")},
		{"999999990", urn.Identifier("pt", "nif")},
		{"ab1234567", urn.Identifier("passport")},
		{"whatever id", urn.IdentifierRaw},
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
	assert.True(t, saw[urn.Identifier("pt", "nif")])
	assert.True(t, saw[urn.IdentifierRaw])
	assert.False(t, saw[urn.Identifier("passport")])
}

func TestRegistryForSystemFallsBack(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	got := reg.ForSystem(urn.Identifier("pt", "nif"))
	assert.Equal(t, urn.Identifier("pt", "nif"), got.System())

	unknown := urn.Identifier("py", "cedula")
	got = reg.ForSystem(unknown)
	assert.Equal(t, urn.IdentifierRaw, got.System())

	reg.mu.Lock()
	delete(reg.systems, urn.Identifier("pt", "nif"))
	reg.mu.Unlock()

	got = reg.ForSystem(urn.Identifier("pt", "nif"))
	assert.Equal(t, urn.IdentifierRaw, got.System())
}

func TestRegistryReloadRejectsBadRows(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	err := reg.Reload([]*IdentifierSystem{
		testSystem(urn.Identifier("bad"), "Bad", "[", TransformNone, CheckNone, 0, 1, 10),
	})
	assert.Error(t, err)

	got := reg.Detect("anything")
	assert.Equal(t, urn.IdentifierRaw, got.System())
}

func TestRegistryDetectionOrderLongestPatternStringFirst(t *testing.T) {
	reg := NewRegistry()
	rows := []*IdentifierSystem{
		testSystem(urn.Identifier("x", "short"), "Short", "[0-9]{12}", TransformDigits, CheckNone, 0, 1, 10),
		testSystem(urn.Identifier("x", "long"), "Long", "[0-9]{10,12}", TransformDigits, CheckNone, 0, 1, 10),
	}
	require.NoError(t, reg.Reload(rows))

	got := reg.Detect("123456789012")
	assert.Equal(t, urn.Identifier("x", "long"), got.System())
}

func TestRegistryReloadKeepsPreviousStateOnFailure(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Reload(seedRows()))

	before := reg.Systems()
	err := reg.Reload([]*IdentifierSystem{
		testSystem(urn.Identifier("bad"), "Bad", ")", TransformNone, CheckNone, 0, 1, 10),
	})
	assert.Error(t, err)

	after := reg.Systems()
	assert.Equal(t, len(before), len(after))
}
