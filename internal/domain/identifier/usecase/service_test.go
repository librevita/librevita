package usecase_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/vault"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierrepo "librevita.org/internal/domain/identifier/repository"
	"librevita.org/internal/domain/identifier/usecase"
)

var (
	testClinicID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	testUserID   = uuid.MustParse("00000000-0000-0000-0000-000000000002")
)

func openTestDB(t *testing.T) *ent.Client {
	t.Helper()
	name := "identifier-test-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })
	return client
}

func newTestServices(t *testing.T, client *ent.Client) (usecase.Service, usecase.SystemsService, *identifiermodel.Registry) {
	t.Helper()
	v, err := vault.NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatalf("bbolt vault: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })

	key, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	sysRepo := identifierrepo.NewSystemRepository(client)
	reg := identifiermodel.NewRegistry()
	rows, err := sysRepo.ListActive(context.Background())
	if err != nil {
		t.Fatalf("load systems: %v", err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatalf("reload: %v", err)
	}
	log := slog.New(slog.DiscardHandler)
	idRepo := identifierrepo.NewIdentifierRepository(client)
	return usecase.NewService(idRepo, key, reg, log), usecase.NewSystemsService(sysRepo, reg, log), reg
}

func seedPatient(t *testing.T, client *ent.Client, clinicID uuid.UUID) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	p, err := client.Patient.Create().
		SetID(id).
		SetClinicID(clinicID).
		SetBlindIndex("idx-" + id.String()).
		SetEncryptedPayload([]byte("payload")).
		SetNonce([]byte("123456789012345678901234")).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create patient: %v", err)
	}
	return p.ID
}

func TestAddAndFindByValue(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	patientID := seedPatient(t, db, testClinicID)

	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(), Value: "123.456.789-09",
	})
	if err != nil {
		t.Fatalf("AddIdentifier: %v", err)
	}
	if got.System != usecase.CPFSystem {
		t.Fatalf("system = %s, want %s (detected)", got.System, usecase.CPFSystem)
	}
	if got.Value != "12345678909" {
		t.Fatalf("value = %q, want normalized %q", got.Value, "12345678909")
	}

	found, err := svc.FindByValue(context.Background(), testClinicID.String(), "12345678909")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 1 || found[0].PatientID != patientID.String() {
		t.Fatalf("FindByValue = %+v, want the patient", found)
	}

	// The formatted input finds the same document.
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "123.456.789-09")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("FindByValue(formatted) = %d hits, want 1", len(found))
	}

	// Wrong check digit never matches.
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "12345678901")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("FindByValue(invalid) = %d hits, want 0", len(found))
	}

	// Unknown value yields nothing.
	found, err = svc.FindByValue(context.Background(), testClinicID.String(), "999999990")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("FindByValue(unknown) = %d hits, want 0", len(found))
	}
}

func TestAddIdentifierExplicitSystem(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	patientID := seedPatient(t, db, testClinicID)

	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(), System: usecase.NIFSystem, Value: "999 999 990",
	})
	if err != nil {
		t.Fatalf("AddIdentifier: %v", err)
	}
	if got.System != usecase.NIFSystem {
		t.Fatalf("system = %s, want %s", got.System, usecase.NIFSystem)
	}
	if got.Value != "999999990" {
		t.Fatalf("value = %q, want %q", got.Value, "999999990")
	}
}

func TestAddIdentifierRejectsInvalidValue(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	patientID := seedPatient(t, db, testClinicID)

	_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(), Value: "12345678901",
	})
	if err == nil {
		t.Fatal("AddIdentifier with a bad check digit must fail")
	}
	var validation *usecase.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %T %v, want ValidationError", err, err)
	}
}

func TestAddIdentifierDuplicate(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	patientA := seedPatient(t, db, testClinicID)
	patientB := seedPatient(t, db, testClinicID)

	in := usecase.Input{PatientID: patientA.String(), Value: "52998224725"}
	if _, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), in); err != nil {
		t.Fatalf("first AddIdentifier: %v", err)
	}
	// Same value, same patient: allowed? No -- UNIQUE(blind_index)
	// forbids even the same patient registering twice.
	_, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), in)
	if !errors.Is(err, usecase.ErrDuplicate) {
		t.Fatalf("second AddIdentifier = %v, want ErrDuplicate", err)
	}

	// Same value, other patient: the CPF already exists.
	inB := usecase.Input{PatientID: patientB.String(), Value: "52998224725"}
	_, err = svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), inB)
	if !errors.Is(err, usecase.ErrDuplicate) {
		t.Fatalf("AddIdentifier on other patient = %v, want ErrDuplicate", err)
	}
}

func TestListAndRemove(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	patientID := seedPatient(t, db, testClinicID)

	for _, value := range []string{"52998224725", "ab1234567"} {
		if _, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
			PatientID: patientID.String(), Value: value,
		}); err != nil {
			t.Fatalf("AddIdentifier(%s): %v", value, err)
		}
	}

	got, err := svc.List(context.Background(), testClinicID.String(), patientID.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List = %d identifiers, want 2", len(got))
	}

	if err := svc.Remove(context.Background(), testClinicID.String(), patientID.String(), got[0].ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, err = svc.List(context.Background(), testClinicID.String(), patientID.String())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List after remove = %d identifiers, want 1", len(got))
	}
	// The removed value is no longer findable.
	found, err := svc.FindByValue(context.Background(), testClinicID.String(), got[0].Value)
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 0 && found[0].Value != got[0].Value {
		t.Fatalf("FindByValue after remove = %+v", found)
	}
}

func TestFindByValueScopedToClinic(t *testing.T) {
	db := openTestDB(t)
	svc, _, _ := newTestServices(t, db)
	otherClinic := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	patientID := seedPatient(t, db, testClinicID)

	if _, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(), Value: "52998224725",
	}); err != nil {
		t.Fatalf("AddIdentifier: %v", err)
	}

	// Another clinic must not see the document.
	found, err := svc.FindByValue(context.Background(), otherClinic.String(), "52998224725")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("cross-clinic FindByValue = %d hits, want 0", len(found))
	}
}

func TestAdministratorRegistersNewSystem(t *testing.T) {
	db := openTestDB(t)
	svc, systems, reg := newTestServices(t, db)
	patientID := seedPatient(t, db, testClinicID)

	// A Paraguayan cédula: 8 digits with the standard scheme.
	created, err := systems.Create(context.Background(), testUserID.String(), usecase.SystemInput{
		System:           "urn:librevita:id:py:cedula",
		DisplayName:      "Cédula de Identidad (Paraguay)",
		Pattern:          "[0-9]{8}",
		Transform:        usecase.TransformDigits,
		CheckAlgorithm:   usecase.CheckMod11Desc,
		CheckBaseLen:     7,
		CheckDVCount:     1,
		CheckStartWeight: 8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(reg.Systems()) == 4 {
		t.Fatal("registry was not reloaded after Create")
	}

	// 1.234.567-9 is the cédula with its check digit.
	got, err := svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: patientID.String(), Value: "12345679",
	})
	if err != nil {
		t.Fatalf("AddIdentifier: %v", err)
	}
	if got.System != "urn:librevita:id:py:cedula" {
		t.Fatalf("system = %s, want the configured cédula", got.System)
	}

	// Lookup works through the blind index.
	found, err := svc.FindByValue(context.Background(), testClinicID.String(), "1234.567-9")
	if err != nil {
		t.Fatalf("FindByValue: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("FindByValue = %d hits, want 1", len(found))
	}

	// Deactivating the system keeps the row readable but the value no
	// longer matches (the raw fallback yields a different blind index).
	if err := systems.SetActive(context.Background(), created.ID.String(), false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	got, err = svc.AddIdentifier(context.Background(), testClinicID.String(), testUserID.String(), usecase.Input{
		PatientID: seedPatient(t, db, testClinicID).String(), Value: "12345678",
	})
	if err != nil {
		t.Fatalf("AddIdentifier after deactivation: %v", err)
	}
	if got.System != usecase.RawSystem {
		t.Fatalf("system after deactivation = %s, want raw", got.System)
	}
}

func TestUpdateSystemPreservesActiveState(t *testing.T) {
	db := openTestDB(t)
	_, systems, _ := newTestServices(t, db)
	ctx := context.Background()

	created, err := systems.Create(ctx, testUserID.String(), usecase.SystemInput{
		System: "urn:librevita:id:py:cedula", DisplayName: "Cedula", Pattern: `^\d{6,8}-\d$`,
		Transform: usecase.TransformNone, CheckAlgorithm: usecase.CheckNone, CheckDVCount: 1, CheckStartWeight: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := systems.SetActive(ctx, created.ID.String(), false); err != nil {
		t.Fatal(err)
	}
	updated, err := systems.Update(ctx, created.ID.String(), usecase.SystemInput{
		System: "urn:librevita:id:py:cedula", DisplayName: "Cedula de Identidad", Pattern: `^\d{6,8}-\d$`,
		Transform: usecase.TransformNone, CheckAlgorithm: usecase.CheckNone, CheckDVCount: 1, CheckStartWeight: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Active {
		t.Errorf("updated system active = %v, want false (editing a deactivated system must not reactivate it)", updated.Active)
	}
}

func TestListSkipsUndecryptableIdentifiers(t *testing.T) {
	client := openTestDB(t)
	svc, _, _ := newTestServices(t, client)
	patientID := seedPatient(t, client, testClinicID)

	badID := uuid.Must(uuid.NewV7())
	_, err := client.PatientIdentifier.Create().
		SetID(badID).
		SetPatientID(patientID).
		SetSystem(usecase.CPFSystem).
		SetValueCiphertext([]byte("invalid-ciphertext")).
		SetNonce([]byte("invalid-nonce-24-bytes--")).
		SetBlindIndex("0000000000000000000000000000000000000000000000000000000000000000").
		SetCreatedBy(testUserID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("insert bad identifier: %v", err)
	}

	got, err := svc.List(context.Background(), testClinicID.String(), patientID.String())
	if err != nil {
		t.Fatalf("List returned error for undecryptable row: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("List = %d, want 0", len(got))
	}
}
