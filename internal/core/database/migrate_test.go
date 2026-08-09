package database

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestUpgradeFromLegacyRoles(t *testing.T) {
	db, err := sql.Open("sqlite", "file:upgrade-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)

	// Schema as it was before the relational roles migration.
	if err := MigrateTo(ctx, db, 10, log); err != nil {
		t.Fatalf("migrate to 10: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role)
		VALUES ('user-legacy', 'legacy@example.org', 'x', 'Legacy User', 'physician')`); err != nil {
		t.Fatalf("seed legacy user: %v", err)
	}

	// Apply the relational roles migration on top.
	if err := MigrateTo(ctx, db, 11, log); err != nil {
		t.Fatalf("migrate to 11: %v", err)
	}

	// The backfill mapped the legacy text role to the physician role row.
	var roleName string
	if err := db.QueryRow(`SELECT r.name FROM users u JOIN roles r ON r.id = u.role_id WHERE u.id = 'user-legacy'`).Scan(&roleName); err != nil {
		t.Fatalf("load migrated role: %v", err)
	}
	if roleName != "physician" {
		t.Errorf("migrated role = %q, want physician", roleName)
	}

	// The text role column is gone.
	if _, err := db.Query(`SELECT role FROM users`); err == nil || !strings.Contains(err.Error(), "no such column") {
		t.Errorf("role column still queryable: %v", err)
	}

	// The four system roles were seeded.
	var roles int
	if err := db.QueryRow(`SELECT COUNT(*) FROM roles WHERE system = 1`).Scan(&roles); err != nil {
		t.Fatal(err)
	}
	if roles != 4 {
		t.Errorf("system roles = %d, want 4", roles)
	}
}
