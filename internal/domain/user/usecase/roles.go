package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const maxRoleNameLen = 40

// ListRoles returns every role, system roles first.
func (s *Service) ListRoles(ctx context.Context) ([]Role, error) {
	return s.roleRepo.List(ctx)
}

// CreateRole adds a new role.
func (s *Service) CreateRole(ctx context.Context, name string, clinical bool) (*Role, error) {
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
	return s.roleRepo.Create(ctx, &Role{
		ID:         id,
		Name:       name,
		IsClinical: clinical,
	})
}

// RenameRole changes a non-system role's name.
func (s *Service) RenameRole(ctx context.Context, id, name string) (*Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "role name is required"}
	}
	if len(name) > maxRoleNameLen {
		return nil, &ValidationError{Msg: "role name is too long"}
	}
	rUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid role id: %w", err)
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return nil, fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System {
		return nil, ErrSystemRole
	}
	return s.roleRepo.Update(ctx, &Role{
		ID:         rUUID,
		Name:       name,
		IsClinical: current.IsClinical,
	})
}

// DeleteRole removes a non-system role that no account uses.
func (s *Service) DeleteRole(ctx context.Context, id string) error {
	rUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("usecase: invalid role id: %w", err)
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System {
		return ErrSystemRole
	}
	count, err := s.roleRepo.CountUsersWithRole(ctx, rUUID)
	if err != nil {
		return fmt.Errorf("usecase: count role users: %w", err)
	}
	if count > 0 {
		return ErrRoleInUse
	}
	return s.roleRepo.Delete(ctx, rUUID)
}

// SetRoleClinical marks a non-system role as clinical staff.
func (s *Service) SetRoleClinical(ctx context.Context, id string, clinical bool) error {
	rUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("usecase: invalid role id: %w", err)
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return fmt.Errorf("usecase: load role: %w", err)
	}
	if current.System {
		return ErrSystemRole
	}
	_, err = s.roleRepo.Update(ctx, &Role{
		ID:         rUUID,
		Name:       current.Name,
		IsClinical: clinical,
	})
	if err != nil {
		return fmt.Errorf("usecase: set role clinical: %w", err)
	}
	return nil
}

// RoleByID loads a role.
func (s *Service) RoleByID(ctx context.Context, id string) (*Role, error) {
	rUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid role id: %w", err)
	}
	return s.roleRepo.GetByID(ctx, rUUID)
}
