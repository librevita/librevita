package usecase_test

import (
	"context"
	"librevita.org/pkg/log"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/internal/domain/identifier/usecase"
	"librevita.org/pkg/urn"
	cryptomocks "librevita.org/tests/mocks/core/crypto"
	identifiermocks "librevita.org/tests/mocks/domain/identifier/model"
)

var (
	testClinicID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	testUserID   = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	testKeyB64   = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=" // gitleaks:allow
)

func defaultTestSystems() []*identifiermodel.IdentifierSystem {
	return []*identifiermodel.IdentifierSystem{
		{
			ID:               uuid.MustParse("01990000-0000-7000-8000-000000000001"),
			System:           urn.Identifier("br", "cpf"),
			DisplayName:      "CPF (Brasil)",
			Pattern:          `^[0-9]{11}$`,
			Transform:        identifiermodel.TransformDigits,
			CheckAlgorithm:   identifiermodel.CheckMod11Desc,
			CheckBaseLen:     9,
			CheckDVCount:     2,
			CheckStartWeight: 10,
			Active:           true,
		},
		{
			ID:               uuid.MustParse("01990000-0000-7000-8000-000000000002"),
			System:           urn.Identifier("pt", "nif"),
			DisplayName:      "NIF (Portugal)",
			Pattern:          `^[0-9]{9}$`,
			Transform:        identifiermodel.TransformDigits,
			CheckAlgorithm:   identifiermodel.CheckMod11Desc,
			CheckBaseLen:     8,
			CheckDVCount:     1,
			CheckStartWeight: 9,
			Active:           true,
		},
	}
}

func setupTestServices(t *testing.T) (
	*identifiermocks.MockIdentifierRepository,
	*identifiermocks.MockSystemRepository,
	*cryptomocks.MockKeyStore,
	usecase.Service,
	usecase.SystemsService,
	*identifiermodel.Registry,
) {
	t.Helper()
	ksMock := cryptomocks.NewMockKeyStore(t)
	// In-memory keystore for patient DEKs during encryption tests
	deks := make(map[string][]byte)
	ksMock.EXPECT().GetDEK(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, urn string) ([]byte, error) {
		if dek, ok := deks[urn]; ok {
			return dek, nil
		}
		return nil, crypto.ErrKeyNotFound
	}).Maybe()
	ksMock.EXPECT().PutDEK(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, urn string, dek []byte) error {
		deks[urn] = dek
		return nil
	}).Maybe()

	key, err := crypto.NewMasterKey(testKeyB64, ksMock)
	require.NoError(t, err)

	reg := identifiermodel.NewRegistry()
	require.NoError(t, reg.Reload(defaultTestSystems()))

	idRepoMock := identifiermocks.NewMockIdentifierRepository(t)
	sysRepoMock := identifiermocks.NewMockSystemRepository(t)
	log := log.Nop()

	svc := usecase.NewService(idRepoMock, key, reg, log)
	idRepoMock.EXPECT().AllowsSystem(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()
	systemsSvc := usecase.NewSystemsService(sysRepoMock, reg)

	return idRepoMock, sysRepoMock, ksMock, svc, systemsSvc, reg
}

func TestAddAndFindByValue(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

	var storedRecord identifiermodel.IdentifierRecord

	idRepoMock.EXPECT().Add(mock.Anything, mock.MatchedBy(func(rec identifiermodel.IdentifierRecord) bool {
		return rec.PatientID == patientID && rec.System == urn.Identifier("br", "cpf") && rec.BlindIndex != ""
	})).RunAndReturn(func(ctx context.Context, rec identifiermodel.IdentifierRecord) (*identifiermodel.IdentifierRecord, error) {
		rec.ID = uuid.MustParse("01990000-0000-7000-8000-000000000020")
		rec.CreatedAt = time.Now()
		storedRecord = rec
		return &rec, nil
	}).Once()

	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		Value:     "123.456.789-09",
	})
	require.NoError(t, err)
	assert.Equal(t, urn.Identifier("br", "cpf"), got.System)
	assert.Equal(t, "12345678909", got.Value)

	// Find by normalized value
	idRepoMock.EXPECT().FindByBlindIndex(mock.Anything, testClinicID, mock.Anything).RunAndReturn(func(ctx context.Context, cID uuid.UUID, blind string) (*identifiermodel.IdentifierRecord, error) {
		if blind == storedRecord.BlindIndex {
			return &storedRecord, nil
		}
		return nil, usecase.ErrNotFound
	}).Maybe()

	found, err := svc.FindByValue(context.Background(), testClinicID.String(), "12345678909")
	require.NoError(t, err)
	require.Len(t, found, 1)
	assert.Equal(t, patientID.String(), found[0].PatientID)
	assert.Equal(t, "12345678909", found[0].Value)

	// Formatted input finds the same document
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "123.456.789-09")
	require.NoError(t, err)
	require.Len(t, found, 1)

	// Wrong check digit never matches
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "12345678901")
	require.NoError(t, err)
	assert.Empty(t, found)

	// Unknown value yields nothing
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "999999990")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestAddIdentifierExplicitSystem(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

	idRepoMock.EXPECT().Add(mock.Anything, mock.MatchedBy(func(rec identifiermodel.IdentifierRecord) bool {
		return rec.PatientID == patientID && rec.System == urn.Identifier("pt", "nif")
	})).RunAndReturn(func(ctx context.Context, rec identifiermodel.IdentifierRecord) (*identifiermodel.IdentifierRecord, error) {
		rec.ID = uuid.MustParse("01990000-0000-7000-8000-000000000021")
		rec.CreatedAt = time.Now()
		return &rec, nil
	}).Once()

	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		System:    urn.Identifier("pt", "nif"),
		Value:     "999 999 990",
	})
	require.NoError(t, err)
	assert.Equal(t, urn.Identifier("pt", "nif"), got.System)
	assert.Equal(t, "999999990", got.Value)
}

func TestAddIdentifierRejectsInvalidValue(t *testing.T) {
	_, _, _, svc, _, _ := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

	_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		Value:     "12345678901",
	})
	require.Error(t, err)
	var validation *usecase.ValidationError
	require.ErrorAs(t, err, &validation)
}

func TestAddIdentifierDuplicate(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	patientA := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	patientB := uuid.MustParse("01990000-0000-7000-8000-000000000011")

	in := usecase.Input{PatientID: patientA.String(), Value: "52998224725"}

	idRepoMock.EXPECT().Add(mock.Anything, mock.Anything).Return(&identifiermodel.IdentifierRecord{
		ID:        uuid.MustParse("01990000-0000-7000-8000-000000000030"),
		PatientID: patientA,
		System:    urn.Identifier("br", "cpf"),
	}, nil).Once()

	_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), in)
	require.NoError(t, err)

	// Second attempt on same patient with duplicate blind index error from repository
	idRepoMock.EXPECT().Add(mock.Anything, mock.Anything).Return(nil, usecase.ErrDuplicate).Once()
	_, err = svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), in)
	require.ErrorIs(t, err, usecase.ErrDuplicate)

	// Duplicate on other patient
	idRepoMock.EXPECT().Add(mock.Anything, mock.Anything).Return(nil, usecase.ErrDuplicate).Once()
	inB := usecase.Input{PatientID: patientB.String(), Value: "52998224725"}
	_, err = svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), inB)
	require.ErrorIs(t, err, usecase.ErrDuplicate)
}

func TestListAndRemove(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

	var records []identifiermodel.IdentifierRecord
	idRepoMock.EXPECT().PatientExists(mock.Anything, testClinicID, patientID).Return(true, nil).Maybe()
	idRepoMock.EXPECT().Add(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, rec identifiermodel.IdentifierRecord) (*identifiermodel.IdentifierRecord, error) {
		rec.ID = uuid.New()
		records = append(records, rec)
		return &rec, nil
	}).Twice()

	for _, value := range []string{"52998224725", "52998224725"} {
		_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
			PatientID: patientID.String(),
			Value:     value,
		})
		require.NoError(t, err)
	}

	idRepoMock.EXPECT().ListByPatient(mock.Anything, patientID).Return(records, nil).Once()
	got, err := svc.List(context.Background(), testClinicID.String(), patientID.String())
	require.NoError(t, err)
	require.Len(t, got, 2)

	removeID := uuid.MustParse(got[0].ID)
	idRepoMock.EXPECT().Remove(mock.Anything, patientID, removeID).Return(nil).Once()
	err = svc.Remove(context.Background(), testClinicID.String(), patientID.String(), got[0].ID)
	require.NoError(t, err)

	// List after remove returns remaining item
	idRepoMock.EXPECT().ListByPatient(mock.Anything, patientID).Return(records[1:], nil).Once()
	got, err = svc.List(context.Background(), testClinicID.String(), patientID.String())
	require.NoError(t, err)
	require.Len(t, got, 1)

	// Removed value lookup yields nothing
	idRepoMock.EXPECT().FindByBlindIndex(mock.Anything, testClinicID, mock.Anything).Return(nil, usecase.ErrNotFound).Maybe()
	found, err := svc.FindByValue(context.Background(), testClinicID.String(), "52998224725")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestFindByValueScopedToClinic(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	otherClinic := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	patientID := uuid.MustParse("00000000-0000-0000-0000-000000000010")

	idRepoMock.EXPECT().Add(mock.Anything, mock.Anything).Return(&identifiermodel.IdentifierRecord{
		ID:        uuid.New(),
		PatientID: patientID,
		System:    urn.Identifier("br", "cpf"),
	}, nil).Once()

	_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		Value:     "52998224725",
	})
	require.NoError(t, err)

	// Another clinic must not see the document
	idRepoMock.EXPECT().FindByBlindIndex(mock.Anything, otherClinic, mock.Anything).Return(nil, usecase.ErrNotFound).Maybe()
	found, err := svc.FindByValue(context.Background(), otherClinic.String(), "52998224725")
	require.NoError(t, err)
	assert.Empty(t, found)
}

func TestAdministratorRegistersNewSystem(t *testing.T) {
	idRepoMock, sysRepoMock, _, svc, systems, reg := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	createdID := uuid.MustParse("01990000-0000-7000-8000-000000000099")

	createdSystem := &identifiermodel.IdentifierSystem{
		ID:               createdID,
		System:           urn.Identifier("py", "cedula"),
		DisplayName:      "Cédula de Identidad (Paraguay)",
		Pattern:          "[0-9]{8}",
		Transform:        identifiermodel.TransformDigits,
		CheckAlgorithm:   identifiermodel.CheckMod11Desc,
		CheckBaseLen:     7,
		CheckDVCount:     1,
		CheckStartWeight: 8,
		Active:           true,
	}

	sysListWithNew := append(defaultTestSystems(), createdSystem)
	sysRepoMock.EXPECT().Create(mock.Anything, mock.MatchedBy(func(sys *identifiermodel.IdentifierSystem) bool {
		return sys.System == urn.Identifier("py", "cedula")
	})).Return(createdSystem, nil).Once()
	sysRepoMock.EXPECT().ListActive(mock.Anything).Return(sysListWithNew, nil).Once()

	created, err := systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:           urn.Identifier("py", "cedula"),
		DisplayName:      "Cédula de Identidad (Paraguay)",
		Pattern:          "[0-9]{8}",
		Transform:        usecase.TransformDigits,
		CheckAlgorithm:   usecase.CheckMod11Desc,
		CheckBaseLen:     7,
		CheckDVCount:     1,
		CheckStartWeight: 8,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, len(reg.Systems())) // 2 default + 1 new = 3 active systems

	var storedRecord identifiermodel.IdentifierRecord
	idRepoMock.EXPECT().Add(mock.Anything, mock.MatchedBy(func(rec identifiermodel.IdentifierRecord) bool {
		return rec.System == urn.Identifier("py", "cedula")
	})).RunAndReturn(func(ctx context.Context, rec identifiermodel.IdentifierRecord) (*identifiermodel.IdentifierRecord, error) {
		rec.ID = uuid.New()
		storedRecord = rec
		return &rec, nil
	}).Once()

	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		Value:     "12345679",
	})
	require.NoError(t, err)
	assert.Equal(t, urn.Identifier("py", "cedula"), got.System)

	// Lookup through blind index
	idRepoMock.EXPECT().FindByBlindIndex(mock.Anything, testClinicID, mock.Anything).RunAndReturn(func(ctx context.Context, cID uuid.UUID, blind string) (*identifiermodel.IdentifierRecord, error) {
		if blind == storedRecord.BlindIndex {
			return &storedRecord, nil
		}
		return nil, usecase.ErrNotFound
	}).Maybe()

	found, err := svc.FindByValue(context.Background(), testClinicID.String(), "1234.567-9")
	require.NoError(t, err)
	require.Len(t, found, 1)

	// Deactivating system
	sysRepoMock.EXPECT().SetActive(mock.Anything, created.ID, false).Return(nil).Once()
	sysRepoMock.EXPECT().ListActive(mock.Anything).Return(defaultTestSystems(), nil).Once()
	err = systems.SetActive(context.Background(), created.ID.String(), false)
	require.NoError(t, err)

	// Adding after deactivation falls back to raw system
	idRepoMock.EXPECT().Add(mock.Anything, mock.MatchedBy(func(rec identifiermodel.IdentifierRecord) bool {
		return rec.System == urn.IdentifierRaw
	})).Return(&identifiermodel.IdentifierRecord{
		ID:        uuid.New(),
		PatientID: patientID,
		System:    urn.IdentifierRaw,
	}, nil).Once()

	got, err = svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(),
		Value:     "12345678",
	})
	require.NoError(t, err)
	assert.Equal(t, urn.IdentifierRaw, got.System)
}

func TestUpdateSystemPreservesActiveState(t *testing.T) {
	_, sysRepoMock, _, _, systems, _ := setupTestServices(t)
	ctx := context.Background()
	systemID := uuid.MustParse("01990000-0000-7000-8000-000000000099")

	createdSystem := &identifiermodel.IdentifierSystem{
		ID:             systemID,
		System:         urn.Identifier("py", "cedula"),
		DisplayName:    "Cedula",
		Pattern:        `^\d{6,8}-\d$`,
		Transform:      identifiermodel.TransformNone,
		CheckAlgorithm: identifiermodel.CheckNone,
		Active:         true,
	}

	sysRepoMock.EXPECT().Create(mock.Anything, mock.Anything).Return(createdSystem, nil).Once()
	sysRepoMock.EXPECT().ListActive(mock.Anything).Return(append(defaultTestSystems(), createdSystem), nil).Once()

	created, err := systems.Create(ctx, testUserID.String(), usecase.SystemInput{
		System:         urn.Identifier("py", "cedula"),
		DisplayName:    "Cedula",
		Pattern:        `^\d{6,8}-\d$`,
		Transform:      usecase.TransformNone,
		CheckAlgorithm: usecase.CheckNone,
	})
	require.NoError(t, err)

	// SetActive to false
	sysRepoMock.EXPECT().SetActive(mock.Anything, created.ID, false).Return(nil).Once()
	sysRepoMock.EXPECT().ListActive(mock.Anything).Return(defaultTestSystems(), nil).Once()
	err = systems.SetActive(ctx, created.ID.String(), false)
	require.NoError(t, err)

	// Update inactive system
	updatedSystem := &identifiermodel.IdentifierSystem{
		ID:             systemID,
		System:         urn.Identifier("py", "cedula"),
		DisplayName:    "Cedula de Identidad",
		Pattern:        `^\d{6,8}-\d$`,
		Transform:      identifiermodel.TransformNone,
		CheckAlgorithm: identifiermodel.CheckNone,
		Active:         false,
	}
	sysRepoMock.EXPECT().Update(mock.Anything, mock.MatchedBy(func(s *identifiermodel.IdentifierSystem) bool {
		return s.ID == created.ID
	})).Return(updatedSystem, nil).Once()
	sysRepoMock.EXPECT().ListActive(mock.Anything).Return(defaultTestSystems(), nil).Once()

	updated, err := systems.Update(ctx, created.ID.String(), usecase.SystemInput{
		System:         urn.Identifier("py", "cedula"),
		DisplayName:    "Cedula de Identidad",
		Pattern:        `^\d{6,8}-\d$`,
		Transform:      usecase.TransformNone,
		CheckAlgorithm: usecase.CheckNone,
	})
	require.NoError(t, err)
	assert.False(t, updated.Active)
}

func TestListSkipsUndecryptableIdentifiers(t *testing.T) {
	idRepoMock, _, _, svc, _, _ := setupTestServices(t)
	patientID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

	// Corrupted ciphertext record
	badRecord := identifiermodel.IdentifierRecord{
		ID:              uuid.New(),
		PatientID:       patientID,
		System:          urn.Identifier("br", "cpf"),
		ValueCiphertext: []byte("invalid-ciphertext"),
		BlindIndex:      "0000000000000000000000000000000000000000000000000000000000000000",
		CreatedBy:       &testUserID,
		CreatedAt:       time.Now(),
	}

	idRepoMock.EXPECT().PatientExists(mock.Anything, testClinicID, patientID).Return(true, nil).Once()
	idRepoMock.EXPECT().ListByPatient(mock.Anything, patientID).Return([]identifiermodel.IdentifierRecord{badRecord}, nil).Once()

	got, err := svc.List(context.Background(), testClinicID.String(), patientID.String())
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCreateRejectsNonIdentifierURN(t *testing.T) {
	_, _, _, _, systems, _ := setupTestServices(t)
	_, err := systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:         "com.example:doc",
		DisplayName:    "Outside",
		Pattern:        "[0-9]{8}",
		Transform:      usecase.TransformDigits,
		CheckAlgorithm: usecase.CheckNone,
	})
	var v *usecase.ValidationError
	require.ErrorAs(t, err, &v)
	assert.Contains(t, v.Msg, urn.IdentifierPrefix)

	_, err = systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:         urn.IdentifierPrefix,
		DisplayName:    "Empty",
		Pattern:        "[0-9]{8}",
		Transform:      usecase.TransformDigits,
		CheckAlgorithm: usecase.CheckNone,
	})
	require.ErrorAs(t, err, &v)
	assert.Contains(t, v.Msg, urn.IdentifierPrefix)

	_, err = systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:         urn.IdentifierPrefix + "br:",
		DisplayName:    "Empty segment",
		Pattern:        "[0-9]{8}",
		Transform:      usecase.TransformDigits,
		CheckAlgorithm: usecase.CheckNone,
	})
	require.ErrorAs(t, err, &v)
	assert.Contains(t, v.Msg, urn.IdentifierPrefix)
}

func TestCreateRejectsReservedRawIdentifier(t *testing.T) {
	_, _, _, _, systems, _ := setupTestServices(t)
	_, err := systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:         urn.IdentifierRaw,
		DisplayName:    "Raw",
		Pattern:        "[0-9]{8}",
		Transform:      usecase.TransformDigits,
		CheckAlgorithm: usecase.CheckNone,
	})
	var v *usecase.ValidationError
	require.ErrorAs(t, err, &v)
	assert.Contains(t, v.Error(), urn.IdentifierRaw)
}
