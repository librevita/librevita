// Package policy implements authorization with CEL (Common Expression
// Language), a non-Turing-complete expression language: no loops, recursion,
// or side effects, which makes authorization rules bounded, safe to
// evaluate, and auditable.
//
// This package is transport-agnostic. The Echo middleware that enforces
// policies lives in librevita.org/internal/core/server.
package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"

	"librevita.org/internal/core/auth"
)

// Every expression receives the same variables:
//
//	principal  map[string]any  id (int), email (string), name (string), role (string)
//	request    map[string]any  method (string), path (string)
//
// Policy names are permissions. Routes reference them with Require(name) in
// the server package.
var DefaultPolicies = map[string]string{
	// Authenticated dashboard, available to every active account.
	"dashboard.view": `principal.role in ['admin', 'physician', 'receptionist', 'patient']`,

	// Admin area, restricted to the admin role.
	"admin.view": `principal.role == 'admin'`,
}

// RequestInfo is the request side of the policy evaluation context.
type RequestInfo struct {
	Method string
	Path   string
}

// ErrPolicyNotFound is returned when a route references an unknown policy.
var ErrPolicyNotFound = errors.New("policy: policy not found")

// PolicyEngine compiles DefaultPolicies once at startup and evaluates them
// per request. Compilation failures are startup failures.
type PolicyEngine struct {
	progs map[string]cel.Program
	log   *slog.Logger
}

// NewPolicyEngine is the Fx provider. It fails fast when a policy does not
// compile or does not evaluate to a boolean.
func NewPolicyEngine(log *slog.Logger) (*PolicyEngine, error) {
	env, err := cel.NewEnv(
		cel.Variable("principal", cel.MapType(cel.StringType, cel.AnyType)),
		cel.Variable("request", cel.MapType(cel.StringType, cel.AnyType)),
	)
	if err != nil {
		return nil, fmt.Errorf("policy: cel environment: %w", err)
	}

	progs := make(map[string]cel.Program, len(DefaultPolicies))
	for name, expr := range DefaultPolicies {
		ast, iss := env.Compile(expr)
		if iss != nil && iss.Err() != nil {
			return nil, fmt.Errorf("policy: %q does not compile: %w", name, iss.Err())
		}
		prog, err := env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("policy: %q program: %w", name, err)
		}
		progs[name] = prog
	}
	return &PolicyEngine{progs: progs, log: log}, nil
}

// Allowed evaluates the named policy for p and req.
func (pe *PolicyEngine) Allowed(ctx context.Context, name string, p *auth.Principal, req RequestInfo) (bool, error) {
	prog, ok := pe.progs[name]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrPolicyNotFound, name)
	}

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
	})
	if err != nil {
		return false, fmt.Errorf("policy: %q evaluation: %w", name, err)
	}

	switch out {
	case types.True:
		return true, nil
	case types.False:
		return false, nil
	default:
		return false, fmt.Errorf("policy: %q did not evaluate to bool", name)
	}
}
