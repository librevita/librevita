//go:build dqlite

package database

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	clinicrepo "librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/core/crypto"
	patientrepo "librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/identifier"
	userrepo "librevita.org/internal/domain/user/repository"
)

func strPtrT(s string) *string { return &s }

// TestDqliteSpike connects to the local dqlite cluster (see
// /tmp/opencode/dqlite-node) through the pure-Go driver and exercises
// the full stack: goose migrations, a real transaction, and the FLE
// round trip. Skips when the cluster is not reachable.
func TestDqliteSpike(t *testing.T) {
	db, err := openDqlite("127.0.0.1:9001,127.0.0.1:9002,127.0.0.1:9003", "librevita")
	if err != nil {
		t.Skipf("no dqlite cluster: %v", err)
	}
	defer db.Close()

	if err := Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// #nosec G202 -- table names come from the fixed list below.
	for _, table := range []string{"patient_identifiers", "patients", "users", "clinics"} {
		if _, err := db.Exec("DELETE FROM " + table); err != nil {
			t.Fatalf("clean %s: %v", table, err)
		}
	}

	// A real transaction: BEGIN/COMMIT through the wire protocol, using
	// the typed sqlc repository methods.
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := clinicrepo.New(tx).CreateClinic(context.Background(), clinicrepo.CreateClinicParams{
		ID: uuid.MustParse("00000000-0000-0000-0000-0000000000d1"), Name: "Dqlite", TaxID: strPtrT("1"), Country: "BR", Timezone: "UTC",
	}); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	// Rollback must not persist.
	tx, err = db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clinicrepo.New(tx).CreateClinic(context.Background(), clinicrepo.CreateClinicParams{
		ID: uuid.MustParse("00000000-0000-0000-0000-0000000000d2"), Name: "Rolled", TaxID: strPtrT("2"), Country: "BR", Timezone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM clinics WHERE id = '00000000-0000-0000-0000-0000000000d2'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back clinic persisted: %d", count)
	}

	// FLE round trip: encrypted identifier + blind index + duplicate.
	clinicID := "00000000-0000-0000-0000-0000000000d1"
	adminID := "00000000-0000-0000-0000-0000000000d5"
	adminRole, err := userrepo.New(db).GetRoleByName(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userrepo.New(db).CreateUser(context.Background(), userrepo.CreateUserParams{
		ID: uuid.MustParse(adminID), Email: "admin@dqlite.test", PasswordHash: "x", DisplayName: "Admin", RoleID: adminRole.ID,
	}); err != nil {
		t.Fatal(err)
	}
	patientID := uuid.MustParse("00000000-0000-0000-0000-0000000000d3")
	if _, err := patientrepo.New(db).CreatePatient(context.Background(), patientrepo.CreatePatientParams{
		ID: patientID, ClinicID: uuid.MustParse(clinicID), DisplayName: "P", Sex: "unknown",
		CreatedBy: uuid.MustParse(adminID),
	}); err != nil {
		t.Fatal(err)
	}
	vault, err := crypto.NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vault.Close() })

	key, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", vault)
	if err != nil {
		t.Fatal(err)
	}
	reg := identifier.NewRegistry()
	rows, err := identifier.LoadActiveSystems(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatal(err)
	}
	svc := identifier.NewService(db, key, reg, slog.New(slog.DiscardHandler))
	if _, err := svc.AddIdentifier(context.Background(), clinicID, adminID, identifier.Input{
		PatientID: patientID.String(), Value: "123.456.789-09",
	}); err != nil {
		t.Fatalf("add identifier: %v", err)
	}
	found, err := svc.FindByValue(context.Background(), clinicID, "12345678909")
	if err != nil || len(found) != 1 {
		t.Fatalf("find by value: %v %+v", err, found)
	}
	other := uuid.MustParse("00000000-0000-0000-0000-0000000000d4")
	if _, err := patientrepo.New(db).CreatePatient(context.Background(), patientrepo.CreatePatientParams{
		ID: other, ClinicID: uuid.MustParse(clinicID), DisplayName: "O", Sex: "unknown",
		CreatedBy: uuid.MustParse(adminID),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddIdentifier(context.Background(), clinicID, adminID, identifier.Input{
		PatientID: other.String(), Value: "12345678909",
	}); !errors.Is(err, identifier.ErrDuplicate) {
		t.Fatalf("duplicate = %v, want ErrDuplicate", err)
	}
	t.Log("dqlite spike OK: migrations, transactions, FLE")
}
