package database

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestStrictTables(t *testing.T) {
	db, err := sql.Open("sqlite", "file:strict-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	if err := Migrate(ctx, db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Every business table must be STRICT, so wrong column types are
	// hard errors instead of silent coercion.
	rows, err := db.Query(`SELECT name, strict FROM pragma_table_list WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	tables := map[string]int64{}
	for rows.Next() {
		var name string
		var strict int64
		if err := rows.Scan(&name, &strict); err != nil {
			t.Fatal(err)
		}
		tables[name] = strict
	}
	expected := []string{"roles", "users", "sessions", "audit_log", "clinics", "meta", "policies", "policy_versions", "patients", "specialties", "user_specialties", "staff_change_requests"}
	for _, name := range expected {
		if tables[name] != 1 {
			t.Errorf("table %q is not STRICT (strict=%d)", name, tables[name])
		}
	}

	// Prove the strict typing is enforced: a text value in an INTEGER
	// column must be rejected.
	if _, err := db.Exec(`INSERT INTO roles (id, name, system) VALUES ('x', 'test-role', 'not-an-int')`); err == nil {
		t.Errorf("STRICT accepted a text value in an INTEGER column")
	} else if !strings.Contains(err.Error(), "cannot store TEXT value") {
		t.Errorf("unexpected error for strict violation: %v", err)
	}
}
