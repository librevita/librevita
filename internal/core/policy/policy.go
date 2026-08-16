// Package policy implements dynamic authorization with CEL (Common
// Expression Language), a non-Turing-complete expression language: no loops,
// recursion, or side effects, which makes authorization rules bounded, safe
// to evaluate, and auditable.
//
// Policies are stored in SQLite and seeded from DefaultPolicies on startup.
// Administrators edit them at runtime through the admin panel; every change
// is validated (compilation plus boolean output) before it becomes active.
//
// This package is transport-agnostic. The Echo middleware that enforces
// policies lives in librevita.org/internal/core/server.
package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy/repository"
	lvtypes "librevita.org/internal/types"
)

// Every expression receives the same variables:
//
//	principal  map[string]any  id (string UUIDv7), email (string), name (string), role (string)
//	request    map[string]any  method (string), path (string)
//	resource   map[string]any  the object being accessed (route policies
//	                           leave it empty; resource policies, evaluated
//	                           in the use cases, fill it, e.g. created_by)
//	context    map[string]any  ambient attributes (time, emergency flag, ...).
//	                           clinic_id is filled automatically from the
//	                           clock provider when one is wired (see
//	                           SetClockProvider), so policies can express
//	                           per-clinic rules without a schema change.
//
// Policy names are permissions. Routes reference them with Require(name) in
// the server package. Resource-level policies (patient.edit) are enforced
// inside the use cases with AllowedResource, where the object attributes
// are available; the route middleware only performs the coarse filter.
var DefaultPolicies = map[string]string{
	// Authenticated dashboard, available to every active account.
	"dashboard.view": `principal.role in ['admin', 'physician', 'receptionist', 'patient']`,

	// Self-service profile preferences (UI theme, personal timezone):
	// every authenticated account manages its own settings.
	"profile.update": `principal.role in ['admin', 'physician', 'receptionist', 'patient']`,

	// Admin area, restricted to the admin role.
	"admin.view": `principal.role == 'admin'`,

	// Account creation. Registration is never public: the policy decides
	// who may create accounts. The default restricts it to admins; an
	// operator may tighten it to a single user, for example
	// `principal.email == 'hr@example.org'`, or close it entirely with
	// `false`.
	"users.register": `principal.role == 'admin'`,

	// User management: create staff accounts, change roles and status.
	"users.manage": `principal.role == 'admin'`,

	// Physician directory: visible to staff, edited directly by admins,
	// and changeable by receptionists through admin-approved requests.
	"staff.view":    `principal.role in ['admin', 'physician', 'receptionist']`,
	"staff.edit":    `principal.role == 'admin'`,
	"staff.request": `principal.role in ['admin', 'receptionist']`,
	"staff.approve": `principal.role == 'admin'`,

	// Clinic calendar: the schedule is open to the clinical staff.
	"calendar.view": `principal.role in ['admin', 'physician', 'receptionist']`,

	// Patient registry, available to the clinical roles.
	"patient.view": `principal.role in ['admin', 'physician', 'receptionist']`,

	// Patient record changes, evaluated against the record itself: an
	// admin edits anything, a physician only the patients they registered
	// (resource.created_by). Enforced in the patient use cases.
	"patient.edit": `principal.role == 'admin' || (principal.role == 'physician' && resource.created_by == principal.id)`,

	// Clinical file attachments: reading is open to the clinical roles,
	// writing is restricted to admins and physicians. Belonging to the
	// patient is enforced per request (domain + resource), so a bare
	// file id never resolves an attachment of another patient.
	"patient.document.read":  `principal.role in ['admin', 'physician', 'receptionist']`,
	"patient.document.write": `principal.role in ['admin', 'physician']`,
}

// RequestInfo is the request side of the policy evaluation context.
type RequestInfo struct {
	Method string
	Path   string
}

// criticalPolicies guard their own management: if an admin saved an
// expression that denies the admin role, the admin panel would become
// unreachable with no recovery path. Set validates changes to these
// policies against an admin fixture and rejects self-lockout.
var criticalPolicies = map[string]bool{
	"admin.view": true,
}

// adminFixture is the representative principal used to reject self-lockout.
var adminFixture = auth.Principal{
	ID:    "00000000-0000-7000-8000-000000000000",
	Email: "admin@librevita.example",
	Name:  "Administrator",
	Role:  auth.RoleAdmin,
}

// ErrPolicyNotFound is returned when a route references an unknown policy.
var ErrPolicyNotFound = errors.New("policy: policy not found")

// PolicyEngine compiles policies once and keeps them in memory. Writes
// (startup seeding and admin updates) validate the expression first, so a
// broken policy is never activated.
type PolicyEngine struct {
	env   *cel.Env
	mu    sync.RWMutex
	progs map[string]cel.Program
	db    *sql.DB
	log   *slog.Logger

	// clinicID resolves the installation's clinic for the context
	// variable. The interface keeps this core package free of domain
	// imports; the fx module wires the real provider at boot.
	clinicID clinicIDResolver

	// setMu serializes Set so that the in-memory program always matches the
	// last committed database version, even under concurrent edits.
	setMu sync.Mutex
}

// clinicIDResolver is the minimal surface of the clinic profile the
// policy engine needs to expose context.clinic_id.
type clinicIDResolver interface {
	ClinicID(ctx context.Context) (string, error)
}

// SetClockProvider wires the clinic resolver so every evaluation can
// expose context.clinic_id. Called at boot; tests may leave it unset,
// in which case the context variable stays empty.
func (pe *PolicyEngine) SetClockProvider(clocks clinicIDResolver) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.clinicID = clocks
}

// NewPolicyEngine is the Fx provider. It only validates the environment;
// Load, called from the Fx OnStart hook after migrations run, seeds and
// compiles the policies. It requires the SQLite backend.
func NewPolicyEngine(db *sql.DB, log *slog.Logger) (*PolicyEngine, error) {
	if db == nil {
		return nil, errors.New("policy: requires the SQLite backend")
	}

	env, err := cel.NewEnv(
		cel.Variable("principal", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("request", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("resource", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("context", cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		return nil, fmt.Errorf("policy: cel environment: %w", err)
	}

	return &PolicyEngine{
		env:   env,
		progs: make(map[string]cel.Program),
		db:    db,
		log:   log,
	}, nil
}

// Load seeds DefaultPolicies that are missing from the database and compiles
// every stored policy. Compilation failures are returned so that a broken
// policy is a startup failure, not a runtime outage. Every seeded policy
// receives an initial version with origin "seed" so the creation is
// traceable.
func (pe *PolicyEngine) Load(ctx context.Context) error {
	queries := repository.New(pe.db)
	for name, expr := range DefaultPolicies {
		_, err := queries.GetPolicyByName(ctx, name)
		if errors.Is(err, sql.ErrNoRows) {
			id, err := uuid.NewV7()
			if err != nil {
				return fmt.Errorf("policy: seed id for %q: %w", name, err)
			}
			created, err := queries.CreatePolicy(ctx, repository.CreatePolicyParams{
				ID: id, Name: name, Expression: expr,
			})
			if err != nil {
				return fmt.Errorf("policy: seed %q: %w", name, err)
			}
			if err := queries.CreatePolicyVersion(ctx, repository.CreatePolicyVersionParams{
				PolicyID: created.ID.String(), Expression: expr, Origin: lvtypes.PolicyOriginSeed.String(),
			}); err != nil {
				return fmt.Errorf("policy: seed version %q: %w", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("policy: read %q: %w", name, err)
		}
	}

	rows, err := queries.ListPolicies(ctx)
	if err != nil {
		return fmt.Errorf("policy: list: %w", err)
	}
	for _, row := range rows {
		prog, err := pe.compile(row.Name, row.Expression)
		if err != nil {
			return err
		}
		pe.mu.Lock()
		pe.progs[row.Name] = prog
		pe.mu.Unlock()
	}
	return nil
}

// Actor identifies who changed a policy. The zero value means the system
// (for example, startup seeding).
type Actor struct {
	ID    string // User UUIDv7.
	Email string
}

// Set validates and activates a new expression for name, persisting it and
// recording a versioned change history entry with the actor. The policy
// update and its history row commit atomically; an invalid expression leaves
// both the previous program and the history untouched. Concurrent calls are
// serialized so the in-memory program always matches the last committed
// version.
func (pe *PolicyEngine) Set(ctx context.Context, name, expression string, actor Actor) error {
	prog, err := pe.compile(name, expression)
	if err != nil {
		return err
	}

	// Critical policies must never deny the admin role; the admin panel
	// is the only place that could restore them.
	if criticalPolicies[name] {
		allowed, err := evaluate(prog, &adminFixture, RequestInfo{Method: "GET", Path: "/admin"}, nil, nil)
		if err != nil {
			return fmt.Errorf("policy: %q would break admin access: %w", name, err)
		}
		if !allowed {
			return fmt.Errorf("policy: change to %q would deny access to the admin panel (self-lockout rejected)", name)
		}
	}

	pe.setMu.Lock()
	defer pe.setMu.Unlock()

	origin := lvtypes.PolicyOriginSystem
	if actor.ID != "" {
		origin = lvtypes.PolicyOriginAdmin
	}

	tx, err := pe.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("policy: begin update %q: %w", name, err)
	}
	defer tx.Rollback()

	qtx := repository.New(pe.db).WithTx(tx)

	// Policies are referenced by name; the id is assigned once at creation
	// and preserved across updates.
	policy, err := qtx.GetPolicyByName(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("policy: generate id for %q: %w", name, err)
		}
		created, err := qtx.CreatePolicy(ctx, repository.CreatePolicyParams{
			ID: id, Name: name, Expression: expression,
		})
		if err != nil {
			return fmt.Errorf("policy: create %q: %w", name, err)
		}
		policy = repository.GetPolicyByNameRow(created)
	} else if err != nil {
		return fmt.Errorf("policy: read %q: %w", name, err)
	} else {
		if err := qtx.UpdatePolicyExpression(ctx, repository.UpdatePolicyExpressionParams{
			ID: policy.ID, Expression: expression,
		}); err != nil {
			return fmt.Errorf("policy: store %q: %w", name, err)
		}
	}

	var actorID, actorMail *string
	if actor.ID != "" {
		id, mail := actor.ID, actor.Email
		actorID, actorMail = &id, &mail
	}
	if err := qtx.CreatePolicyVersion(ctx, repository.CreatePolicyVersionParams{
		PolicyID: policy.ID.String(), Expression: expression, Origin: origin.String(),
		ChangedBy: actorID, ChangedByEmail: actorMail,
	}); err != nil {
		return fmt.Errorf("policy: version %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("policy: commit update %q: %w", name, err)
	}

	pe.mu.Lock()
	pe.progs[name] = prog
	pe.mu.Unlock()
	return nil
}

// History returns the most recent limit changes of name, newest first.
func (pe *PolicyEngine) History(ctx context.Context, name string, limit int) ([]repository.ListPolicyVersionsRow, error) {
	rows, err := repository.New(pe.db).ListPolicyVersions(ctx, repository.ListPolicyVersionsParams{
		Name: name, Limit: int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("policy: history %q: %w", name, err)
	}
	return rows, nil
}

// List returns the stored policies sorted by name.
func (pe *PolicyEngine) List(ctx context.Context) ([]repository.ListPoliciesRow, error) {
	rows, err := repository.New(pe.db).ListPolicies(ctx)
	if err != nil {
		return nil, fmt.Errorf("policy: list: %w", err)
	}
	return rows, nil
}

// Count returns the number of stored policies.
func (pe *PolicyEngine) Count(ctx context.Context) (int64, error) {
	count, err := repository.New(pe.db).CountPolicies(ctx)
	if err != nil {
		return 0, fmt.Errorf("policy: count: %w", err)
	}
	return count, nil
}

// Allowed evaluates the named policy for p and req.
func (pe *PolicyEngine) Allowed(ctx context.Context, name string, p *auth.Principal, req RequestInfo) (bool, error) {
	pe.mu.RLock()
	prog, ok := pe.progs[name]
	pe.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPolicyNotFound, name)
	}

	ambient := map[string]any{}
	pe.fillAmbient(ctx, ambient)
	allowed, err := evaluate(prog, p, req, nil, ambient)
	if err != nil {
		return false, fmt.Errorf("policy: %q evaluation: %w", name, err)
	}
	return allowed, nil
}

// AllowedResource evaluates a resource-level policy (e.g. patient.edit)
// against the object attributes and ambient context. Route policies
// without resource/context can be evaluated here too: the variables are
// simply empty maps. The use cases audit denials themselves.
func (pe *PolicyEngine) AllowedResource(ctx context.Context, name string, p *auth.Principal, req RequestInfo, resource, ambient map[string]any) (bool, error) {
	pe.mu.RLock()
	prog, ok := pe.progs[name]
	pe.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPolicyNotFound, name)
	}
	if resource == nil {
		resource = map[string]any{}
	}
	if ambient == nil {
		ambient = map[string]any{}
	}
	pe.fillAmbient(ctx, ambient)
	allowed, err := evaluate(prog, p, req, resource, ambient)
	if err != nil {
		return false, fmt.Errorf("policy: %q evaluation: %w", name, err)
	}
	return allowed, nil
}

// fillAmbient adds the clinic id to the context variable unless the
// caller already provided it. The key is always present: without a
// resolver (or before onboarding) it is the empty string, so
// expressions comparing it evaluate to false instead of failing with a
// missing-key error.
func (pe *PolicyEngine) fillAmbient(ctx context.Context, ambient map[string]any) {
	if _, present := ambient["clinic_id"]; present {
		return
	}
	pe.mu.RLock()
	resolver := pe.clinicID
	pe.mu.RUnlock()
	ambient["clinic_id"] = ""
	if resolver == nil {
		return
	}
	if id, err := resolver.ClinicID(ctx); err == nil {
		ambient["clinic_id"] = id
	}
}

// evaluate runs prog against the full activation and requires a boolean
// result.
func evaluate(prog cel.Program, p *auth.Principal, req RequestInfo, resource, ambient map[string]any) (bool, error) {
	out, _, err := prog.Eval(map[string]any{
		"principal": map[string]any{
			"id":    p.ID,
			"email": p.Email,
			"name":  p.Name,
			"role":  p.Role.String(),
		},
		"request": map[string]any{
			"method": req.Method,
			"path":   req.Path,
		},
		"resource": resource,
		"context":  ambient,
	})
	if err != nil {
		return false, err
	}

	switch out {
	case types.True:
		return true, nil
	case types.False:
		return false, nil
	default:
		return false, fmt.Errorf("did not evaluate to bool")
	}
}

// compile validates and compiles an expression. It requires a successful
// compilation and a boolean output type, so a typo or a non-boolean policy
// is rejected before it can ever affect a request.
func (pe *PolicyEngine) compile(name, expression string) (cel.Program, error) {
	ast, iss := pe.env.Compile(expression)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("policy: %q does not compile: %w", name, iss.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("policy: %q must evaluate to a boolean", name)
	}
	prog, err := pe.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("policy: %q program: %w", name, err)
	}
	return prog, nil
}
