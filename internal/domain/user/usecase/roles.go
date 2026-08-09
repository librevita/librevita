package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/domain/user/repository"
)

// Role errors.
var (
	ErrDuplicateRole = errors.New("usecase: role already exists")
	ErrSystemRole    = errors.New("usecase: system roles cannot be renamed or deleted")
	ErrRoleInUse     = errors.New("usecase: role is assigned to accounts")
)

const maxRoleNameLen = 40

// ListRoles returns every role, system roles first.
func (s *Service) ListRoles(ctx context.Context) ([]repository.Role, error) {
	rows, err := s.users.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("usecase: list roles: %w", err)
	}
	return rows, nil
}

// CreateRole adds a new role. The four seeded roles are the system ones;
// new roles are ordinary rows the administrator manages freely. Clinical
// roles appear in the physician directory and take part in the staff
// change workflow.
func (s *Service) CreateRole(ctx context.Context, name string, clinical bool) (*repository.Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "role name is required"}
	}
	if len(name) > maxRoleNameLen {
		return nil, &ValidationError{Msg: "role name is too long"}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate role id: %w", err)
	}
	isClinical := int64(0)
	if clinical {
		isClinical = 1
	}
	role, err := s.users.CreateRole(ctx, repository.CreateRoleParams{ID: id.String(), Name: name, IsClinical: isClinical})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicateRole
		}
		return nil, fmt.Errorf("usecase: create role: %w", err)
	}
	return &role, nil
}

// RenameRole changes a non-system role's name. The CEL policies match on
// names, so renaming a role also means updating the policies that
// reference it.
func (s *Service) RenameRole(ctx context.Context, id, name string) (*repository.Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "role name is required"}
	}
	if len(name) > maxRoleNameLen {
		return nil, &ValidationError{Msg: "role name is too long"}
	}
	current, err := s.users.GetRoleByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System == 1 {
		return nil, ErrSystemRole
	}
	role, err := s.users.RenameRole(ctx, repository.RenameRoleParams{Name: name, ID: id})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicateRole
		}
		return nil, fmt.Errorf("usecase: rename role: %w", err)
	}
	return &role, nil
}

// DeleteRole removes a non-system role that no account uses.
func (s *Service) DeleteRole(ctx context.Context, id string) error {
	current, err := s.users.GetRoleByID(ctx, id)
	if err != nil {
		return fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System == 1 {
		return ErrSystemRole
	}
	count, err := s.users.CountUsersByRoleID(ctx, id)
	if err != nil {
		return fmt.Errorf("usecase: count role users: %w", err)
	}
	if count > 0 {
		return ErrRoleInUse
	}
	if err := s.users.DeleteRole(ctx, id); err != nil {
		return fmt.Errorf("usecase: delete role: %w", err)
	}
	return nil
}


// SetRoleClinical marks a non-system role as clinical staff, which puts
// its accounts in the physician directory and the change workflow.
func (s *Service) SetRoleClinical(ctx context.Context, id string, clinical bool) error {
	current, err := s.users.GetRoleByID(ctx, id)
	if err != nil {
		return fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System == 1 {
		return ErrSystemRole
	}
	isClinical := int64(0)
	if clinical {
		isClinical = 1
	}
	if _, err := s.users.SetRoleClinical(ctx, repository.SetRoleClinicalParams{
		IsClinical: isClinical, ID: id,
	}); err != nil {
		return fmt.Errorf("usecase: set role clinical: %w", err)
	}
	return nil
}


// RoleByID loads a role.
func (s *Service) RoleByID(ctx context.Context, id string) (*repository.Role, error) {
	role, err := s.users.GetRoleByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("usecase: load role: %w", err)
	}
	return &role, nil
}
