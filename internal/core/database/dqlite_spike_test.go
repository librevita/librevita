//go:build dqlite

package database

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/clinic"
	"librevita.org/ent/role"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/vault"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierrepo "librevita.org/internal/domain/identifier/repository"
	identifierusecase "librevita.org/internal/domain/identifier/usecase"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/testutil"
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

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// A real transaction: BEGIN/COMMIT through Ent.
	tx, err := client.Tx(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Clinic.Create().
		SetID(uuid.MustParse("00000000-0000-0000-0000-0000000000d1")).
		SetName("Dqlite").
		SetTaxID("1").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(context.Background()); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Rollback must not persist.
	tx, err = client.Tx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Clinic.Create().
		SetID(uuid.MustParse("00000000-0000-0000-0000-0000000000d2")).
		SetName("Rolled").
		SetTaxID("2").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	count, err := client.Clinic.Query().Where(clinic.IDEQ(uuid.MustParse("00000000-0000-0000-0000-0000000000d2"))).Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back clinic persisted: %d", count)
	}

	// FLE round trip: encrypted identifier + blind index + duplicate.
	clinicID := "00000000-0000-0000-0000-0000000000d1"
	adminID := "00000000-0000-0000-0000-0000000000d5"
	adminRole, err := client.Role.Query().Where(role.NameEQ("admin")).Only(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.User.Create().
		SetID(uuid.MustParse(adminID)).
		SetEmail("admin@dqlite.test").
		SetPasswordHash("x").
		SetDisplayName("Admin").
		SetRoleID(adminRole.ID).
		Save(context.Background()); err != nil {
		t.Fatal(err)
	}

	v, err := vault.NewBBoltVault(filepath.Join(t.TempDir(), "keys.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })

	engine, err := crypto.NewEngine("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v)
	if err != nil {
		t.Fatal(err)
	}

	patientSvc := usecase.NewService(client, engine, slog.New(slog.DiscardHandler), nil)
	createdPt, err := patientSvc.Create(context.Background(), clinicID, adminID, usecase.PatientInput{
		DisplayName: "P",
		Sex:         "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	patientID := createdPt.ID

	reg := identifiermodel.NewRegistry()
	sysRepo := identifierrepo.NewSystemRepository(client)
	rows, err := sysRepo.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Reload(rows); err != nil {
		t.Fatal(err)
	}
	idRepo := identifierrepo.NewIdentifierRepository(client)
	svc := identifierusecase.NewService(idRepo, engine, reg, slog.New(slog.DiscardHandler))
	if _, err := svc.AddIdentifier(context.Background(), clinicID, adminID, identifierusecase.Input{
		PatientID: patientID.String(), Value: "123.456.789-09",
	}); err != nil {
		t.Fatalf("add identifier: %v", err)
	}
	found, err := svc.FindByValue(context.Background(), clinicID, "12345678909")
	if err != nil || len(found) != 1 {
		t.Fatalf("find by value: %v %+v", err, found)
	}

	otherPt, err := patientSvc.Create(context.Background(), clinicID, adminID, usecase.PatientInput{
		DisplayName: "O",
		Sex:         "unknown",
	})
	if err != nil {
		t.Fatal(err)
	}
	other := otherPt.ID
	if _, err := svc.AddIdentifier(context.Background(), clinicID, adminID, identifierusecase.Input{
		PatientID: other.String(), Value: "12345678909",
	}); !errors.Is(err, identifierusecase.ErrDuplicate) {
		t.Fatalf("duplicate = %v, want ErrDuplicate", err)
	}
	t.Log("dqlite spike OK: migrations, transactions, FLE")
}
