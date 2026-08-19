// Package policy implements dynamic authorization with CEL (Common
// Expression Language), a non-Turing-complete expression language: no loops,
// recursion, or side effects, which makes authorization rules bounded, safe
// to evaluate, and auditable.
//
// Policies are stored in the database and seeded from DefaultPolicies on startup.
// Administrators edit them at runtime through the admin panel; every change
// is validated (compilation plus boolean output) before it becomes active.
//
// This package is transport-agnostic. The Echo middleware that enforces
// policies lives in librevita.org/internal/core/server.
package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	"librevita.org/internal/core/auth"
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

// PolicyRow represents a policy for listing and admin views.
type PolicyRow struct {
	ID         string
	Name       string
	Expression string
	UpdatedAt  time.Time
}

// PolicyVersionRow represents a historical version of a policy.
type PolicyVersionRow struct {
	ID             int64
	PolicyID       string
	Expression     string
	Origin         string
	CreatedAt      time.Time
	ChangedBy      *string
	ChangedByEmail *string
}

// Actor identifies who changed a policy. The zero value means the system
// (for example, startup seeding).
type Actor struct {
	ID    string // User UUIDv7.
	Email string
}

// Repository defines the storage contract for policies.
type Repository interface {
	SeedDefaults(ctx context.Context, defaults map[string]string) error
	List(ctx context.Context) ([]PolicyRow, error)
	Set(ctx context.Context, name, expression string, actor Actor, origin string) error
	History(ctx context.Context, name string, limit int) ([]PolicyVersionRow, error)
	Count(ctx context.Context) (int64, error)
}

// PolicyEngine compiles policies once and keeps them in memory. Writes
// (startup seeding and admin updates) validate the expression first, so a
// broken policy is never activated.
type PolicyEngine struct {
	env   *cel.Env
	mu    sync.RWMutex
	progs map[string]cel.Program
	repo  Repository
	log   *slog.Logger

	clinicID clinicIDResolver
	setMu    sync.Mutex
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

// NewPolicyEngine is the Fx provider.
func NewPolicyEngine(repo Repository, log *slog.Logger) (*PolicyEngine, error) {
	if repo == nil {
		return nil, errors.New("policy: requires the policy repository")
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
		repo:  repo,
		log:   log,
	}, nil
}

// Load seeds DefaultPolicies that are missing from the database and compiles
// every stored policy.
func (pe *PolicyEngine) Load(ctx context.Context) error {
	if err := pe.repo.SeedDefaults(ctx, DefaultPolicies); err != nil {
		return err
	}

	rows, err := pe.repo.List(ctx)
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

// Set validates and activates a new expression for name, persisting it and
// recording a versioned change history entry with the actor.
func (pe *PolicyEngine) Set(ctx context.Context, name, expression string, actor Actor) error {
	prog, err := pe.compile(name, expression)
	if err != nil {
		return err
	}

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

	if err := pe.repo.Set(ctx, name, expression, actor, origin.String()); err != nil {
		return err
	}

	pe.mu.Lock()
	pe.progs[name] = prog
	pe.mu.Unlock()
	return nil
}

// History returns the most recent limit changes of name, newest first.
func (pe *PolicyEngine) History(ctx context.Context, name string, limit int) ([]PolicyVersionRow, error) {
	return pe.repo.History(ctx, name, limit)
}

// List returns the stored policies sorted by name.
func (pe *PolicyEngine) List(ctx context.Context) ([]PolicyRow, error) {
	return pe.repo.List(ctx)
}

// Count returns the number of stored policies.
func (pe *PolicyEngine) Count(ctx context.Context) (int64, error) {
	return pe.repo.Count(ctx)
}

// Allowed evaluates the named policy for p and req.
func (pe *PolicyEngine) Allowed(ctx context.Context, name string, p *auth.Principal, req RequestInfo) (bool, error) {
	pe.mu.RLock()
	prog, ok := pe.progs[name]
	pe.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPolicyNotFound, name)
	}
	ctxMap := pe.contextMap(ctx)
	return evaluate(prog, p, req, nil, ctxMap)
}

// AllowedResource evaluates the named policy with an object payload in
// resource (e.g. `created_by`, `patient_id`) and optional explicit context.
func (pe *PolicyEngine) AllowedResource(ctx context.Context, name string, p *auth.Principal, req RequestInfo, resource, explicitCtx map[string]any) (bool, error) {
	pe.mu.RLock()
	prog, ok := pe.progs[name]
	pe.mu.RUnlock()
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPolicyNotFound, name)
	}
	ctxMap := pe.contextMap(ctx)
	for k, v := range explicitCtx {
		ctxMap[k] = v
	}
	return evaluate(prog, p, req, resource, ctxMap)
}

func (pe *PolicyEngine) contextMap(ctx context.Context) map[string]any {
	pe.mu.RLock()
	clocks := pe.clinicID
	pe.mu.RUnlock()

	out := map[string]any{
		"clinic_id": "",
	}
	if clocks != nil {
		if cid, err := clocks.ClinicID(ctx); err == nil && cid != "" {
			out["clinic_id"] = cid
		}
	}
	return out
}

func (pe *PolicyEngine) compile(name, expression string) (cel.Program, error) {
	ast, issues := pe.env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("policy: %q compile error: %w", name, issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("policy: %q expression must evaluate to a boolean, got %s", name, ast.OutputType().TypeName())
	}
	prog, err := pe.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("policy: %q program error: %w", name, err)
	}
	return prog, nil
}

// ValidateSyntax compiles expression and checks that it produces a boolean,
// returning a human-readable error or nil. Used by the admin form before
// submission.
func (pe *PolicyEngine) ValidateSyntax(expression string) error {
	_, err := pe.compile("validate", expression)
	return err
}

func evaluate(prog cel.Program, p *auth.Principal, req RequestInfo, resource, ctx map[string]any) (bool, error) {
	principalMap := map[string]any{
		"id":    "",
		"email": "",
		"name":  "",
		"role":  "",
	}
	if p != nil {
		principalMap["id"] = p.ID
		principalMap["email"] = p.Email
		principalMap["name"] = p.Name
		principalMap["role"] = string(p.Role)
	}

	requestMap := map[string]any{
		"method": req.Method,
		"path":   req.Path,
	}

	if resource == nil {
		resource = map[string]any{}
	}
	if ctx == nil {
		ctx = map[string]any{}
	}

	out, _, err := prog.Eval(map[string]any{
		"principal": principalMap,
		"request":   requestMap,
		"resource":  resource,
		"context":   ctx,
	})
	if err != nil {
		return false, fmt.Errorf("eval: %w", err)
	}
	b, ok := out.(types.Bool)
	if !ok {
		return false, fmt.Errorf("policy evaluated to non-bool %T", out)
	}
	return bool(b), nil
}

var adminFixture = auth.Principal{
	ID:    "01990000-0000-7000-8000-000000000001",
	Email: "admin@example.org",
	Name:  "Admin",
	Role:  auth.RoleAdmin,
}

var criticalPolicies = map[string]bool{
	"admin.view":   true,
	"users.manage": true,
}

var ErrPolicyNotFound = errors.New("policy: not found")
