package policy

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/cel-go/cel"

	"librevita.org/internal/core/auth"
)

func testPolicyEngine(t *testing.T) *PolicyEngine {
	t.Helper()
	pe, err := NewPolicyEngine(slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewPolicyEngine: %v", err)
	}
	return pe
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
