package policy

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/database"
)

func testPolicyEngine(t *testing.T) *PolicyEngine {
	t.Helper()
	pe, err := NewPolicyEngine(openPolicyDB(t), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return pe
}

func openPolicyDB(t *testing.T) *sql.DB {
	t.Helper()
	name := "policy-test-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestPolicyCompilesAtStartup(t *testing.T) {
	pe := testPolicyEngine(t)
	for name := range DefaultPolicies {
		if _, ok := pe.progs[name]; !ok {
			t.Errorf("policy %q was not compiled", name)
		}
	}
}

func TestPolicyEvaluation(t *testing.T) {
	pe := testPolicyEngine(t)

	cases := []struct {
		name     string
		role     auth.Role
		policy   string
		expected bool
	}{
		{"admin allowed", auth.RoleAdmin, "admin.view", true},
		{"physician denied", auth.RolePhysician, "admin.view", false},
		{"receptionist denied", auth.RoleReceptionist, "admin.view", false},
		{"patient denied", auth.RolePatient, "admin.view", false},
		{"patient dashboard", auth.RolePatient, "dashboard.view", true},
		{"admin may register", auth.RoleAdmin, "users.register", true},
		{"physician may not register", auth.RolePhysician, "users.register", false},
		{"patient may not register", auth.RolePatient, "users.register", false},
		{"admin may manage users", auth.RoleAdmin, "users.manage", true},
		{"physician may not manage users", auth.RolePhysician, "users.manage", false},
		{"receptionist may not manage users", auth.RoleReceptionist, "users.manage", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: tc.role}
			got, err := pe.Allowed(context.Background(), tc.policy, p, RequestInfo{Method: "GET", Path: "/"})
			if err != nil {
				t.Fatalf("Allowed: %v", err)
			}
			if got != tc.expected {
				t.Fatalf("Allowed = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestPolicyUnknown(t *testing.T) {
	pe := testPolicyEngine(t)
	p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}

	_, err := pe.Allowed(context.Background(), "does.not.exist", p, RequestInfo{})
	if !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("Allowed = %v, want ErrPolicyNotFound", err)
	}
}

func TestPolicyRejectsNonBoolean(t *testing.T) {
	// A policy that evaluates to a string must fail at evaluation time.
	env, err := cel.NewEnv(
		cel.Variable("principal", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("request", cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		t.Fatal(err)
	}
	ast, iss := env.Compile(`principal.role`)
	if iss != nil && iss.Err() != nil {
		t.Fatal(iss.Err())
	}
	prog, err := env.Program(ast)
	if err != nil {
		t.Fatal(err)
	}

	pe := &PolicyEngine{progs: map[string]cel.Program{"weird": prog}, log: slog.New(slog.DiscardHandler)}
	p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}

	if _, err := pe.Allowed(context.Background(), "weird", p, RequestInfo{}); err == nil {
		t.Fatal("Allowed of non-bool policy should fail")
	}
}

func TestPoliciesSeededFromDefaults(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pe.progs) != len(DefaultPolicies) {
		t.Fatalf("compiled %d policies, want %d", len(pe.progs), len(DefaultPolicies))
	}

	rows, err := pe.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(DefaultPolicies) {
		t.Fatalf("stored %d policies, want %d", len(rows), len(DefaultPolicies))
	}

	// Every seeded policy must have exactly one seed version.
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM policy_versions WHERE origin = 'seed'`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != len(DefaultPolicies) {
		t.Fatalf("seed versions = %d, want %d", versions, len(DefaultPolicies))
	}
}

func TestSetUpdatesPolicy(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RolePatient}
	if allowed, _ := pe.Allowed(context.Background(), "admin.view", p, RequestInfo{}); allowed {
		t.Fatal("patient must not access admin.view by default")
	}

	// Open admin.view to every role.
	if err := pe.Set(context.Background(), "admin.view", `principal.role in ['admin','patient']`, Actor{ID: "u1", Email: "admin@example.org"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if allowed, _ := pe.Allowed(context.Background(), "admin.view", p, RequestInfo{}); !allowed {
		t.Fatal("patient must access admin.view after the policy change")
	}
}

func TestSetRejectsInvalidExpression(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	// Broken CEL.
	if err := pe.Set(context.Background(), "admin.view", `principal.role ==`, Actor{ID: "u1", Email: "admin@example.org"}); err == nil {
		t.Fatal("Set with broken CEL must fail")
	}
	// Compiles but does not evaluate to a boolean.
	if err := pe.Set(context.Background(), "admin.view", `principal.role`, Actor{ID: "u1", Email: "admin@example.org"}); err == nil {
		t.Fatal("Set with non-boolean policy must fail")
	}

	// The previous program must stay active.
	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RolePatient}
	if allowed, _ := pe.Allowed(context.Background(), "admin.view", p, RequestInfo{}); allowed {
		t.Fatal("failed Set must not activate a new policy")
	}
}

func TestSetPersistsAcrossRestart(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Set(context.Background(), "users.register", `false`, Actor{ID: "u1", Email: "admin@example.org"}); err != nil {
		t.Fatal(err)
	}

	// A new engine over the same database must pick up the stored value.
	pe2, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe2.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	if allowed, _ := pe2.Allowed(context.Background(), "users.register", p, RequestInfo{}); allowed {
		t.Fatal("stored policy `false` must survive a restart")
	}
}

func TestConcurrentReadsDuringSet(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if err := pe.Set(context.Background(), "admin.view", `principal.role == 'admin'`, Actor{ID: "u1", Email: "admin@example.org"}); err != nil {
				t.Error(err)
			}
		}
	}()
	for i := 0; i < 2000; i++ {
		if _, err := pe.Allowed(context.Background(), "admin.view", p, RequestInfo{}); err != nil {
			t.Fatal(err)
		}
	}
	<-done
}

func TestSetRecordsVersionWithActor(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Seeding records one initial version with origin "seed".
	rows, err := pe.History(context.Background(), "users.register", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("seed created %d version rows, want 1", len(rows))
	}
	if rows[0].Origin != OriginSeed || rows[0].ChangedByEmail != nil {
		t.Fatalf("seed version must have origin seed and no actor: %+v", rows[0])
	}

	if err := pe.Set(context.Background(), "users.register", `principal.role == 'admin'`, Actor{ID: "u-1", Email: "ana@example.org"}); err != nil {
		t.Fatal(err)
	}
	if err := pe.Set(context.Background(), "users.register", `false`, Actor{ID: "u-2", Email: "bruno@example.org"}); err != nil {
		t.Fatal(err)
	}

	rows, err = pe.History(context.Background(), "users.register", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("history has %d entries, want 3", len(rows))
	}
	// Newest first.
	if rows[0].Expression != "false" || rows[1].Expression != `principal.role == 'admin'` || rows[2].Origin != OriginSeed {
		t.Fatalf("unexpected history order: %+v", rows)
	}
	if rows[0].Origin != OriginAdmin || rows[1].Origin != OriginAdmin {
		t.Fatalf("admin edits must carry origin admin: %+v", rows)
	}
	if rows[0].ChangedByEmail == nil || *rows[0].ChangedByEmail != "bruno@example.org" {
		t.Fatalf("missing actor on newest version: %+v", rows[0])
	}
	if rows[1].ChangedBy == nil || *rows[1].ChangedBy != "u-1" {
		t.Fatalf("missing actor id on first edit: %+v", rows[1])
	}
}

func TestSetRejectedChangeHasNoVersion(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := pe.Set(context.Background(), "admin.view", `principal.role ==`, Actor{ID: "u-1", Email: "ana@example.org"}); err == nil {
		t.Fatal("Set with broken CEL must fail")
	}

	rows, err := pe.History(context.Background(), "admin.view", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rejected change created %d version rows, want 1 (seed only)", len(rows))
	}
}

func TestPolicyIDIsStableUUIDv7(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := pe.Set(context.Background(), "admin.view", `principal.role == 'admin'`, Actor{ID: "u-1", Email: "a@example.org"}); err != nil {
		t.Fatal(err)
	}
	if err := pe.Set(context.Background(), "admin.view", `principal.role in ['admin','physician']`, Actor{ID: "u-1", Email: "a@example.org"}); err != nil {
		t.Fatal(err)
	}

	rows, err := pe.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var firstID string
	for _, row := range rows {
		if row.Name == "admin.view" {
			firstID = row.ID
			break
		}
	}
	if firstID == "" {
		t.Fatal("admin.view not listed")
	}

	id, err := uuid.Parse(firstID)
	if err != nil {
		t.Fatalf("policy id %q is not a UUID: %v", firstID, err)
	}
	if id.Version() != 7 {
		t.Fatalf("policy id %q is not UUIDv7 (version %d)", firstID, id.Version())
	}

	// Updates must not change the id.
	if err := pe.Set(context.Background(), "admin.view", `true`, Actor{ID: "u-1", Email: "a@example.org"}); err != nil {
		t.Fatal(err)
	}
	rows, err = pe.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Name == "admin.view" && row.ID != firstID {
			t.Fatalf("policy id changed across updates: %q -> %q", firstID, row.ID)
		}
	}
}

func TestSetRejectsSelfLockout(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	actor := Actor{ID: "u-1", Email: "admin@example.org"}

	// Deny-everything and deny-admin expressions must be rejected.
	for _, expr := range []string{`false`, `principal.role == 'physician'`, `principal.email == 'other@example.org'`} {
		if err := pe.Set(context.Background(), "admin.view", expr, actor); err == nil {
			t.Fatalf("Set(admin.view, %q) must be rejected as self-lockout", expr)
		}
	}

	// Expressions that keep allowing the admin role are fine.
	for _, expr := range []string{`principal.role == 'admin'`, `principal.role in ['admin','physician']`, `true`} {
		if err := pe.Set(context.Background(), "admin.view", expr, actor); err != nil {
			t.Fatalf("Set(admin.view, %q) failed: %v", expr, err)
		}
	}

	// Non-critical policies are not guarded.
	if err := pe.Set(context.Background(), "users.register", `false`, actor); err != nil {
		t.Fatalf("Set(users.register, false) must be allowed: %v", err)
	}
}

func TestRejectedSelfLockoutKeepsPreviousPolicy(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := pe.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := pe.Set(context.Background(), "admin.view", `false`, Actor{ID: "u-1", Email: "a@example.org"}); err == nil {
		t.Fatal("self-lockout must be rejected")
	}

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	if allowed, _ := pe.Allowed(context.Background(), "admin.view", p, RequestInfo{}); !allowed {
		t.Fatal("rejected change must keep the admin allowed")
	}
}
