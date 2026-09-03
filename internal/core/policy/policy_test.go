package policy

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/cel-go/cel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"librevita.org/pkg/ident"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/database"
	"librevita.org/internal/database/record"
	"librevita.org/pkg/log"
)

func pctx() context.Context {
	return clinicctx.WithTestClinic(context.Background())
}

func testPolicyEngine(t *testing.T) *PolicyEngine {
	t.Helper()
	pe, err := NewPolicyEngine(openPolicyDB(t), log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)
	return pe
}

func openPolicyDB(t *testing.T) Repository {
	t.Helper()
	name := "policy-test-" + uuid.NewString()
	db, err := sql.Open("sqlite", "file:"+name+"?mode=memory&cache=shared")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	err = database.Migrate(context.Background(), db, log.Nop())
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := record.NewClient(record.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Clinic.Create().
		SetID(clinicctx.TestClinicID).
		SetSlug("test").
		SetName("Test Clinic").
		SetCountry("BR").
		SetTimezone("America/Sao_Paulo").
		Save(context.Background())
	require.NoError(t, err)

	return NewPolicyRepository(client)
}

func TestPolicyCompilesAtStartup(t *testing.T) {
	pe := testPolicyEngine(t)
	for name := range DefaultPolicies {
		_, err := pe.Allowed(pctx(), name, &auth.Principal{Role: auth.RoleAdmin}, RequestInfo{})
		assert.NoError(t, err, "policy %q was not compiled", name)
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
		{"physician may write chart", auth.RolePhysician, "chart.write", true},
		{"receptionist may not write chart", auth.RoleReceptionist, "chart.write", false},
		{"patient may not write chart", auth.RolePatient, "chart.write", false},
		{"physician may view chart", auth.RolePhysician, "chart.view", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: tc.role}
			got, err := pe.Allowed(pctx(), tc.policy, p, RequestInfo{Method: "GET", Path: "/"})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestPolicyUnknown(t *testing.T) {
	pe := testPolicyEngine(t)
	p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}

	_, err := pe.Allowed(pctx(), "does.not.exist", p, RequestInfo{})
	assert.ErrorIs(t, err, ErrPolicyNotFound)
}

func TestPolicyRejectsNonBoolean(t *testing.T) {
	// A policy that evaluates to a string must fail at evaluation time.
	env, err := cel.NewEnv(
		cel.Variable("principal", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("request", cel.MapType(cel.StringType, cel.AnyType)),
	)
	require.NoError(t, err)
	ast, iss := env.Compile(`principal.role`)
	if iss != nil {
		require.NoError(t, iss.Err())
	}
	prog, err := env.Program(ast)
	require.NoError(t, err)

	pe := &PolicyEngine{
		progs: map[ident.ClinicID]map[string]cel.Program{
			{}: {"weird": prog},
		},
		log: log.Nop(),
	}
	p := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}

	_, err = pe.Allowed(context.Background(), "weird", p, RequestInfo{})
	assert.Error(t, err, "Allowed of non-bool policy should fail")
}

func TestPoliciesSeededFromDefaults(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)
	assert.Len(t, pe.progs[clinicctx.TestClinicID], len(DefaultPolicies))

	rows, err := pe.List(pctx())
	require.NoError(t, err)
	assert.Len(t, rows, len(DefaultPolicies))

	// Every seeded policy must have exactly one seed version.
	for _, row := range rows {
		history, err := pe.History(pctx(), row.Name, 10)
		require.NoError(t, err)
		require.Len(t, history, 1)
		assert.Equal(t, "seed", history[0].Origin)
	}
}

func TestSetUpdatesPolicy(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RolePatient}
	allowed, _ := pe.Allowed(pctx(), "admin.view", p, RequestInfo{})
	assert.False(t, allowed, "patient must not access admin.view by default")

	// Open admin.view to every role.
	err = pe.Set(pctx(), "admin.view", `principal.role in ['admin','patient']`, Actor{ID: "u1", Email: "admin@example.org"})
	require.NoError(t, err)
	allowed, _ = pe.Allowed(pctx(), "admin.view", p, RequestInfo{})
	assert.True(t, allowed, "patient must access admin.view after policy change")
}

func TestSetRejectsInvalidExpression(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)

	// Broken CEL.
	err = pe.Set(pctx(), "admin.view", `principal.role ==`, Actor{ID: "u1", Email: "admin@example.org"})
	assert.Error(t, err, "Set with broken CEL must fail")

	// Compiles but does not evaluate to a boolean.
	err = pe.Set(pctx(), "admin.view", `principal.role`, Actor{ID: "u1", Email: "admin@example.org"})
	assert.Error(t, err, "Set with non-boolean policy must fail")

	// The previous program must stay active.
	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RolePatient}
	allowed, _ := pe.Allowed(pctx(), "admin.view", p, RequestInfo{})
	assert.False(t, allowed, "failed Set must not activate a new policy")
}

func TestSetPersistsAcrossRestart(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)

	err = pe.Set(pctx(), "users.register", `false`, Actor{ID: "u1", Email: "admin@example.org"})
	require.NoError(t, err)

	// A new engine over the same database must pick up the stored value.
	pe2, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe2.Load(pctx())
	require.NoError(t, err)

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	allowed, _ := pe2.Allowed(pctx(), "users.register", p, RequestInfo{})
	assert.False(t, allowed, "stored policy `false` must survive a restart")
}

func TestConcurrentReadsDuringSet(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			err := pe.Set(pctx(), "admin.view", `principal.role == 'admin'`, Actor{ID: "u1", Email: "admin@example.org"})
			assert.NoError(t, err)
		}
	}()
	for i := 0; i < 2000; i++ {
		_, err := pe.Allowed(pctx(), "admin.view", p, RequestInfo{})
		require.NoError(t, err)
	}
	<-done
}

func TestSetRecordsVersionWithActor(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	// Seeding records one initial version with origin "seed".
	rows, err := pe.History(pctx(), "users.register", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, PolicyOriginSeed.String(), rows[0].Origin)
	assert.Nil(t, rows[0].ChangedByEmail)

	err = pe.Set(pctx(), "users.register", `principal.role == 'admin'`, Actor{ID: "u-1", Email: "ana@example.org"})
	require.NoError(t, err)
	err = pe.Set(pctx(), "users.register", `false`, Actor{ID: "u-2", Email: "bruno@example.org"})
	require.NoError(t, err)

	rows, err = pe.History(pctx(), "users.register", 10)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	// Newest first.
	assert.Equal(t, "false", rows[0].Expression)
	assert.Equal(t, `principal.role == 'admin'`, rows[1].Expression)
	assert.Equal(t, PolicyOriginSeed.String(), rows[2].Origin)

	assert.Equal(t, PolicyOriginAdmin.String(), rows[0].Origin)
	assert.Equal(t, PolicyOriginAdmin.String(), rows[1].Origin)

	require.NotNil(t, rows[0].ChangedByEmail)
	assert.Equal(t, "bruno@example.org", *rows[0].ChangedByEmail)

	require.NotNil(t, rows[1].ChangedBy)
	assert.Equal(t, "u-1", *rows[1].ChangedBy)
}

func TestSetRejectedChangeHasNoVersion(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	err = pe.Set(pctx(), "admin.view", `principal.role ==`, Actor{ID: "u-1", Email: "ana@example.org"})
	assert.Error(t, err, "Set with broken CEL must fail")

	rows, err := pe.History(pctx(), "admin.view", 10)
	require.NoError(t, err)
	assert.Len(t, rows, 1, "rejected change created unexpected version rows")
}

func TestPolicyIDIsStableUUIDv7(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	err = pe.Set(pctx(), "admin.view", `principal.role == 'admin'`, Actor{ID: "u-1", Email: "a@example.org"})
	require.NoError(t, err)
	err = pe.Set(pctx(), "admin.view", `principal.role in ['admin','physician']`, Actor{ID: "u-1", Email: "a@example.org"})
	require.NoError(t, err)

	rows, err := pe.List(pctx())
	require.NoError(t, err)

	var firstID string
	for _, row := range rows {
		if row.Name == "admin.view" {
			firstID = row.ID
			break
		}
	}
	require.NotEmpty(t, firstID, "admin.view not listed")

	id, err := uuid.Parse(firstID)
	require.NoError(t, err)
	assert.Equal(t, 7, int(id.Version()), "policy id %q is not UUIDv7", firstID)

	// Updates must not change the id.
	err = pe.Set(pctx(), "admin.view", `true`, Actor{ID: "u-1", Email: "a@example.org"})
	require.NoError(t, err)

	rows, err = pe.List(pctx())
	require.NoError(t, err)
	for _, row := range rows {
		if row.Name == "admin.view" {
			assert.Equal(t, firstID, row.ID, "policy id changed across updates")
		}
	}
}

func TestSetRejectsSelfLockout(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	actor := Actor{ID: "u-1", Email: "admin@example.org"}

	// Deny-everything and deny-admin expressions must be rejected.
	for _, expr := range []string{`false`, `principal.role == 'physician'`, `principal.email == 'other@example.org'`} {
		err := pe.Set(pctx(), "admin.view", expr, actor)
		assert.Error(t, err, "Set(admin.view, %q) must be rejected as self-lockout", expr)
	}

	// Expressions that keep allowing the admin role are fine.
	for _, expr := range []string{`principal.role == 'admin'`, `principal.role in ['admin','physician']`, `true`} {
		err := pe.Set(pctx(), "admin.view", expr, actor)
		assert.NoError(t, err, "Set(admin.view, %q) failed", expr)
	}

	// Non-critical policies are not guarded.
	err = pe.Set(pctx(), "users.register", `false`, actor)
	assert.NoError(t, err, "Set(users.register, false) must be allowed")
}

func TestRejectedSelfLockoutKeepsPreviousPolicy(t *testing.T) {
	db := openPolicyDB(t)
	pe, err := NewPolicyEngine(db, log.Nop())
	require.NoError(t, err)
	err = pe.Load(pctx())
	require.NoError(t, err)

	err = pe.Set(pctx(), "admin.view", `false`, Actor{ID: "u-1", Email: "a@example.org"})
	assert.Error(t, err, "self-lockout must be rejected")

	p := &auth.Principal{ID: "id", Email: "u@example.org", Name: "User", Role: auth.RoleAdmin}
	allowed, _ := pe.Allowed(pctx(), "admin.view", p, RequestInfo{})
	assert.True(t, allowed, "rejected change must keep the admin allowed")
}

func TestPatientEditResourcePolicy(t *testing.T) {
	pe := testPolicyEngine(t)

	admin := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "admin@example.org", Name: "Admin", Role: auth.RoleAdmin}
	owner := &auth.Principal{ID: "01990000-0000-7000-8000-000000000002", Email: "dr.owner@example.org", Name: "Dr. Owner", Role: auth.RolePhysician}
	other := &auth.Principal{ID: "01990000-0000-7000-8000-000000000003", Email: "dr.other@example.org", Name: "Dr. Other", Role: auth.RolePhysician}
	rec := &auth.Principal{ID: "01990000-0000-7000-8000-000000000004", Email: "rec@example.org", Name: "Rec", Role: auth.RoleReceptionist}

	req := RequestInfo{Method: "POST", Path: "/patients/01990000-0000-7000-8000-000000000099"}
	resource := map[string]any{"id": "01990000-0000-7000-8000-000000000099", "created_by": owner.ID, "status": "active"}

	cases := []struct {
		name      string
		principal *auth.Principal
		want      bool
	}{
		{"admin edits anything", admin, true},
		{"owner physician edits", owner, true},
		{"other physician denied", other, false},
		{"receptionist denied", rec, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pe.AllowedResource(pctx(), "patient.edit", tc.principal, req, resource, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// fakeClinicID is a fixed clinic resolver for tests.
type fakeClinicID struct{ id string }

func (f fakeClinicID) ClinicID(context.Context) (string, error) { return f.id, nil }

func TestContextClinicID(t *testing.T) {
	pe := testPolicyEngine(t)
	admin := &auth.Principal{ID: "u1", Email: "a@example.org", Name: "A", Role: auth.RoleAdmin}
	req := RequestInfo{Method: "GET", Path: "/x"}

	err := pe.Set(pctx(), "test.clinic", `context.clinic_id == 'clinic-123'`, Actor{ID: "u1", Email: "a@example.org"})
	require.NoError(t, err)

	// Without a resolver the context variable is empty and the policy must deny.
	allowed, err := pe.Allowed(pctx(), "test.clinic", admin, req)
	require.NoError(t, err)
	assert.False(t, allowed, "policy must deny without a clinic resolver")

	// With the resolver wired, the clinic id arrives automatically.
	pe.SetClockProvider(fakeClinicID{id: "clinic-123"})
	allowed, err = pe.Allowed(pctx(), "test.clinic", admin, req)
	require.NoError(t, err)
	assert.True(t, allowed, "policy must allow when the resolver matches")

	// A caller-provided value wins over the resolver.
	pe.SetClockProvider(fakeClinicID{id: "other-clinic"})
	allowed, err = pe.AllowedResource(pctx(), "test.clinic", admin, req, nil,
		map[string]any{"clinic_id": "clinic-123"})
	require.NoError(t, err)
	assert.True(t, allowed, "explicit clinic_id must not be overwritten by the resolver")
}

func TestPolicyResetAndHistory(t *testing.T) {
	pe := testPolicyEngine(t)
	ctx := pctx()
	admin := &auth.Principal{ID: "u1", Email: "a@example.org", Name: "A", Role: auth.RoleAdmin}
	req := RequestInfo{Method: "GET", Path: "/"}

	// 1. Count
	count, err := pe.Count(ctx)
	require.NoError(t, err)
	assert.True(t, count > 0)

	// 2. Set custom policy
	err = pe.Set(ctx, "dashboard.view", "false", Actor{ID: "u1", Email: "a@example.org"})
	require.NoError(t, err)
	allowed, err := pe.Allowed(ctx, "dashboard.view", admin, req)
	require.NoError(t, err)
	assert.False(t, allowed)

	// 3. History
	hist, err := pe.History(ctx, "dashboard.view", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, hist)

	// 4. ValidateSyntax
	assert.NoError(t, pe.ValidateSyntax("principal.role == 'admin'"))
	assert.Error(t, pe.ValidateSyntax("invalid CEL syntax [[("))

	// 5. Reset to default
	require.NoError(t, pe.Set(ctx, "dashboard.view", DefaultPolicies["dashboard.view"], Actor{ID: "u1", Email: "a@example.org"}))
	allowedAfterReset, err := pe.Allowed(ctx, "dashboard.view", admin, req)
	require.NoError(t, err)
	assert.True(t, allowedAfterReset)
}

func TestPolicyLoadWithoutClinic(t *testing.T) {
	repo := openPolicyDB(t)
	pe, err := NewPolicyEngine(repo, log.Nop())
	require.NoError(t, err)

	// Load without clinic in context should succeed gracefully
	assert.NoError(t, pe.Load(context.Background()))
}

func TestPolicyModuleLifecycle(t *testing.T) {
	pe := testPolicyEngine(t)
	lc := fxtest.NewLifecycle(t)

	registerLifecycle(lc, pe)
	wireClinicContext(pe, nil)

	require.NoError(t, lc.Start(pctx()))
	require.NoError(t, lc.Stop(pctx()))
	assert.NotNil(t, Module)
}
