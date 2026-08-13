//go:build dqlite

package database

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/domain/patient/identifier"
)

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

	// A real transaction: BEGIN/COMMIT through the wire protocol.
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(`INSERT INTO clinics (id, name, tax_id) VALUES (?, ?, ?)`,
		"00000000-0000-0000-0000-0000000000d1", "Dqlite", "1"); err != nil {
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
	if _, err := tx.Exec(`INSERT INTO clinics (id, name, tax_id) VALUES (?, ?, ?)`,
		"00000000-0000-0000-0000-0000000000d2", "Rolled", "2"); err != nil {
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
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role_id)
		VALUES (?, 'admin@dqlite.test', 'x', 'Admin', (SELECT id FROM roles WHERE name = 'admin'))`, adminID); err != nil {
		t.Fatal(err)
	}
	patientID := uuid.MustParse("00000000-0000-0000-0000-0000000000d3")
	if _, err := db.Exec(`INSERT INTO patients (id, clinic_id, display_name) VALUES (?, ?, 'P')`,
		patientID.String(), clinicID); err != nil {
		t.Fatal(err)
	}
	key, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=")
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
	if _, err := db.Exec(`INSERT INTO patients (id, clinic_id, display_name) VALUES (?, ?, 'O')`,
		other.String(), clinicID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddIdentifier(context.Background(), clinicID, adminID, identifier.Input{
		PatientID: other.String(), Value: "12345678909",
	}); !errors.Is(err, identifier.ErrDuplicate) {
		t.Fatalf("duplicate = %v, want ErrDuplicate", err)
	}
	t.Log("dqlite spike OK: migrations, transactions, FLE")
}
