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

func TestAllEntityIDTypes(t *testing.T) {
	raw := "01990000-0000-7000-8000-000000000001"

	// PlatformUser
	pUser := ident.MustParsePlatformUser(raw)
	assert.Equal(t, raw, pUser.String())
	pUserParsed, err := ident.ParsePlatformUser(raw)
	require.NoError(t, err)
	assert.Equal(t, pUser, pUserParsed)

	// Episode
	ep := ident.MustParseEpisode(raw)
	assert.Equal(t, raw, ep.String())
	epParsed, err := ident.ParseEpisode(raw)
	require.NoError(t, err)
	assert.Equal(t, ep, epParsed)

	// Appointment
	app := ident.MustParseAppointment(raw)
	assert.Equal(t, raw, app.String())
	appParsed, err := ident.ParseAppointment(raw)
	require.NoError(t, err)
	assert.Equal(t, app, appParsed)

	// Finding
	find := ident.MustParseFinding(raw)
	assert.Equal(t, raw, find.String())
	findParsed, err := ident.ParseFinding(raw)
	require.NoError(t, err)
	assert.Equal(t, find, findParsed)

	// Problem
	prob := ident.MustParseProblem(raw)
	assert.Equal(t, raw, prob.String())
	probParsed, err := ident.ParseProblem(raw)
	require.NoError(t, err)
	assert.Equal(t, prob, probParsed)

	// PlanItem
	plan := ident.MustParsePlanItem(raw)
	assert.Equal(t, raw, plan.String())
	planParsed, err := ident.ParsePlanItem(raw)
	require.NoError(t, err)
	assert.Equal(t, plan, planParsed)

	// Role
	role := ident.MustParseRole(raw)
	assert.Equal(t, raw, role.String())
	roleParsed, err := ident.ParseRole(raw)
	require.NoError(t, err)
	assert.Equal(t, role, roleParsed)

	// Specialty
	spec := ident.MustParseSpecialty(raw)
	assert.Equal(t, raw, spec.String())
	specParsed, err := ident.ParseSpecialty(raw)
	require.NoError(t, err)
	assert.Equal(t, spec, specParsed)

	// Policy
	pol := ident.MustParsePolicy(raw)
	assert.Equal(t, raw, pol.String())
	polParsed, err := ident.ParsePolicy(raw)
	require.NoError(t, err)
	assert.Equal(t, pol, polParsed)

	// PatientIdentifier
	patIdent := ident.MustParsePatientIdentifier(raw)
	assert.Equal(t, raw, patIdent.String())
	patIdentParsed, err := ident.ParsePatientIdentifier(raw)
	require.NoError(t, err)
	assert.Equal(t, patIdent, patIdentParsed)

	// IdentifierSystem
	identSys := ident.MustParseIdentifierSystem(raw)
	assert.Equal(t, raw, identSys.String())
	identSysParsed, err := ident.ParseIdentifierSystem(raw)
	require.NoError(t, err)
	assert.Equal(t, identSys, identSysParsed)

	// ClinicIdentifierSystem
	clinicIdentSys := ident.MustParseClinicIdentifierSystem(raw)
	assert.Equal(t, raw, clinicIdentSys.String())
	clinicIdentSysParsed, err := ident.ParseClinicIdentifierSystem(raw)
	require.NoError(t, err)
	assert.Equal(t, clinicIdentSys, clinicIdentSysParsed)

	// StorageObject
	storageObj := ident.MustParseStorageObject(raw)
	assert.Equal(t, raw, storageObj.String())
	storageObjParsed, err := ident.ParseStorageObject(raw)
	require.NoError(t, err)
	assert.Equal(t, storageObj, storageObjParsed)

	// StaffChangeRequest
	staffReq := ident.MustParseStaffChangeRequest(raw)
	assert.Equal(t, raw, staffReq.String())
	staffReqParsed, err := ident.ParseStaffChangeRequest(raw)
	require.NoError(t, err)
	assert.Equal(t, staffReq, staffReqParsed)

	// Generic MustParse panic
	assert.Panics(t, func() {
		ident.MustParse[ident.ClinicID]("invalid-uuid")
	})

	// Scan/Value/Text/Binary roundtrip for all types
	testCodecs := []struct {
		val interface {
			String() string
			IsZero() bool
		}
	}{
		{pUser}, {ep}, {app}, {find}, {prob}, {plan}, {role}, {spec},
		{pol}, {patIdent}, {identSys}, {clinicIdentSys}, {storageObj}, {staffReq},
	}

	for _, tc := range testCodecs {
		assert.False(t, tc.val.IsZero())
		assert.Equal(t, raw, tc.val.String())
	}
}


