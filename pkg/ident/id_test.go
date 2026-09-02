package ident_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/pkg/ident"
)

func TestDistinctTypes(t *testing.T) {
	clinic := ident.MustParseClinic("01990000-0000-7000-8000-0000000000c1")
	patient := ident.MustParsePatient("01990000-0000-7000-8000-0000000000d1")
	assert.Equal(t, clinic.String(), "01990000-0000-7000-8000-0000000000c1")
	assert.False(t, clinic.IsZero())
	assert.True(t, ident.PatientID{}.IsZero())
	assert.NotEqual(t, clinic.UUID(), patient.UUID())
}

func TestParseRoundTrip(t *testing.T) {
	raw := "01990000-0000-7000-8000-0000000000c1"
	got, err := ident.ParseClinic(raw)
	require.NoError(t, err)
	assert.Equal(t, raw, got.String())

	_, err = ident.ParseClinic("not-a-uuid")
	assert.Error(t, err)
}

func TestScanValue(t *testing.T) {
	want := ident.MustParsePatient("01990000-0000-7000-8000-0000000000d1")
	v, err := want.Value()
	require.NoError(t, err)

	var got ident.PatientID
	require.NoError(t, got.Scan(v))
	assert.Equal(t, want, got)
}

func TestTextJSON(t *testing.T) {
	want := ident.MustParseUser("01990000-0000-7000-8000-0000000000aa")
	b, err := json.Marshal(want)
	require.NoError(t, err)

	var got ident.UserID
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, want, got)
}

func TestNewAndFromUUID(t *testing.T) {
	a := ident.New[ident.EpisodeID]()
	assert.False(t, a.IsZero())
	assert.Equal(t, uuid.Version(7), a.UUID().Version())

	u := uuid.MustParse("01990000-0000-7000-8000-0000000000e1")
	assert.Equal(t, u, ident.FromUUID[ident.EpisodeID](u).UUID())
}
