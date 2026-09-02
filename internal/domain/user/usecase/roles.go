package usecase

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"

	"librevita.org/pkg/ident"
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
	roleID := ident.New[ident.RoleID]()
	return s.roleRepo.Create(ctx, &Role{
		ID:         roleID,
		Name:       name,
		IsClinical: clinical,
	})
}

// RenameRole changes a non-system role's name.
func (s *Service) RenameRole(ctx context.Context, roleID, name string) (*Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, &ValidationError{Msg: "role name is required"}
	}
	if len(name) > maxRoleNameLen {
		return nil, &ValidationError{Msg: "role name is too long"}
	}
	rUUID, err := ident.ParseRole(roleID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid role id")
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: load role")
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
func (s *Service) DeleteRole(ctx context.Context, roleID string) error {
	rUUID, err := ident.ParseRole(roleID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid role id")
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return errors.Wrap(err, "usecase: load role")
	}
	if current.System {
		return ErrSystemRole
	}
	count, err := s.roleRepo.CountUsersWithRole(ctx, rUUID)
	if err != nil {
		return errors.Wrap(err, "usecase: count role users")
	}
	if count > 0 {
		return ErrRoleInUse
	}
	return s.roleRepo.Delete(ctx, rUUID)
}

// SetRoleClinical marks a non-system role as clinical staff.
func (s *Service) SetRoleClinical(ctx context.Context, roleID string, clinical bool) error {
	rUUID, err := ident.ParseRole(roleID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid role id")
	}
	current, err := s.roleRepo.GetByID(ctx, rUUID)
	if err != nil {
		return errors.Wrap(err, "usecase: load role")
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
		return errors.Wrap(err, "usecase: set role clinical")
	}
	return nil
}

// RoleByID loads a role.
func (s *Service) RoleByID(ctx context.Context, roleID string) (*Role, error) {
	rUUID, err := ident.ParseRole(roleID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid role id")
	}
	return s.roleRepo.GetByID(ctx, rUUID)
}
