package database

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/config"
)

func TestMigrateSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", "file:migrate-sqlite-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	if err := Migrate(ctx, db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	// Verify all expected business tables are created by SQLite migrations.
	rows, err := db.Query(`SELECT name FROM pragma_table_list WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables[name] = true
	}
	expected := []string{
		"roles", "users", "sessions", "audit_log", "clinics", "meta",
		"policies", "policy_versions", "patients", "specialties",
		"user_specialties", "staff_change_requests", "storage_objects",
		"identifier_systems", "patient_identifiers", "appointments", "episodes",
	}
	for _, name := range expected {
		if !tables[name] {
			t.Errorf("sqlite table %q was not created by migrations", name)
		}
	}
}

func TestMigratePostgres(t *testing.T) {
	// Connect to local dev postgres container if available
	db, err := sql.Open("pgx", "postgres://postgres:postgres@localhost:5433/dev?sslmode=disable")
	if err != nil {
		t.Skipf("skipping postgres migration test: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping postgres migration test (database not reachable): %v", err)
	}

	if err := MigrateWithDriver(ctx, db, config.DriverPostgres, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}

	// Verify all expected business tables are created in PostgreSQL
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables[name] = true
	}
	expected := []string{
		"roles", "users", "sessions", "audit_log", "clinics", "meta",
		"policies", "policy_versions", "patients", "specialties",
		"user_specialties", "staff_change_requests", "storage_objects",
		"identifier_systems", "patient_identifiers", "appointments", "episodes",
	}
	for _, name := range expected {
		if !tables[name] {
			t.Errorf("postgres table %q was not created by migrations", name)
		}
	}
}
