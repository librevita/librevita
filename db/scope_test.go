package db

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Scope guard for the tenant model.
//
// The installation is single-clinic (see internal/domain/clinic), and
// clinic_id on clinical tables is future-proofing. Two tables are
// tenant-scoped today: patients and specialties. Every sqlc query that
// reads or writes them must carry clinic_id in the SQL itself, so the
// scope is enforced by the query, not by a caller's discipline.
//
// Tables not listed here are intentionally global (roles, policies,
// users, audit_log, storage_objects, identifier_systems, ...) and are
// not checked. When a new tenant table is added, list it below and the
// guard starts enforcing it.
//
// Exceptions: a query that joins a tenant table through an
// account-scoped association (user_specialties) is scoped by user id
// and checked in the service layer; listing the tenant table in such a
// join does not require clinic_id in the SQL.
var (
	tenantTables = []string{"patients", "specialties"}

	// tenantRef matches a FROM/JOIN/UPDATE/INTO reference to a tenant
	// table (patient_identifiers is deliberately excluded: it carries
	// no clinic_id and its rows are only reachable through scoped
	// lookups).
	tenantRef = regexp.MustCompile(`(?i)\b(FROM|JOIN|UPDATE|INTO)\s+(patients|specialties)\b`)

	// accountScopedJoin marks queries that reach a tenant table through
	// the account association, scoped by user id in the service layer.
	accountScopedJoin = regexp.MustCompile(`(?i)JOIN\s+user_specialties`)

	clinicRef = regexp.MustCompile(`(?i)\bclinic_id\b`)
)

func TestTenantQueriesCarryClinicScope(t *testing.T) {
	queries, err := filepath.Glob("query/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) == 0 {
		t.Fatal("no query files found")
	}

	checked := 0
	for _, file := range queries {
		// #nosec G304 -- file comes from filepath.Glob over the repo's
		// own query directory, not from user input.
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, block := range queryBlocks(string(src)) {
			if !tenantRef.MatchString(block) {
				continue
			}
			if accountScopedJoin.MatchString(block) {
				continue
			}
			checked++
			if !clinicRef.MatchString(block) {
				t.Errorf("%s: query %q touches a tenant table (%s) without clinic_id",
					file, queryName(block), tenantTables)
			}
		}
	}
	if checked == 0 {
		t.Fatal("guard did not find any tenant query; tenantTables likely wrong")
	}
}

// queryBlocks splits a sqlc query file into per-query blocks.
func queryBlocks(src string) []string {
	var blocks []string
	current := ""
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-- name:") && strings.TrimSpace(current) != "" {
			blocks = append(blocks, current)
			current = ""
		}
		current += line + "\n"
	}
	if strings.TrimSpace(current) != "" {
		blocks = append(blocks, current)
	}
	return blocks
}

func queryName(block string) string {
	for _, line := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "-- name:") {
			return strings.Fields(strings.TrimSpace(line))[2]
		}
	}
	return "(unnamed)"
}
