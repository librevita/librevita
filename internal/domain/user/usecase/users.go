package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/pkg/validator"
)

// CreateUserInput is the staff account creation request.
type CreateUserInput struct {
	Name     string
	Email    string
	Password string
	Role     string
}

// UpdateUserInput is the staff account update request.
type UpdateUserInput struct {
	Name   string
	Email  string
	Role   string
	Active bool
}

// CreateUser creates a staff account with the given role.
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (*User, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	roleName := in.Role

	if err := validateRegistration(name, email, in.Password); err != nil {
		return nil, err
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	userID, err := uuid.NewV7()
	if err != nil {
		return nil, errors.Wrap(err, "usecase: generate user id")
	}

	roleRow, err := s.roleRepo.GetByName(ctx, roleName)
	if err != nil {
		return nil, &ValidationError{Msg: "unsupported role"}
	}

	u, err := s.userRepo.Create(ctx, &User{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		RoleID:       roleRow.ID,
		RoleName:     roleRow.Name,
		Active:       true,
	})
	if err != nil {
		return nil, ErrEmailTaken
	}
	return u, nil
}

// UpdateUser changes the profile, role, or status of an account.
func (s *Service) UpdateUser(ctx context.Context, id string, actorID string, in UpdateUserInput) (*User, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	roleName := in.Role

	v := validator.New()
	v.Field(name, "name", "display name").
		Required().
		Max(maxNameLen)

	v.Field(email, "email", "email").
		Email().
		Max(maxEmailLen)

	if err := v.Err(); err != nil {
		return nil, err
	}

	if id == actorID {
		if roleName != auth.RoleAdmin.String() || !in.Active {
			return nil, ErrCannotDemoteSelf
		}
	}

	roleRow, err := s.roleRepo.GetByName(ctx, roleName)
	if err != nil {
		return nil, &ValidationError{Msg: "unsupported role"}
	}

	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: invalid user id")
	}

	current, err := s.userRepo.GetByID(ctx, uUUID)
	if err != nil {
		return nil, err
	}

	demotingOrDeactivating := (current.RoleName == auth.RoleAdmin.String() && current.Active) &&
		(roleName != auth.RoleAdmin.String() || !in.Active)

	if demotingOrDeactivating {
		activeAdmins, err := s.userRepo.CountActiveAdmins(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "usecase: count active admins")
		}
		if activeAdmins <= 1 {
			return nil, ErrLastActiveAdmin
		}
	}

	u, err := s.userRepo.Update(ctx, &User{
		ID:          uUUID,
		Email:       email,
		DisplayName: name,
		RoleID:      roleRow.ID,
		RoleName:    roleRow.Name,
		Active:      in.Active,
	})
	if err != nil {
		return nil, err
	}
	return u, nil
}

func validateEmail(email string) error {
	v := validator.New()
	v.Field(email, "email", "email").
		Email().
		Max(maxEmailLen)
	return v.Err()
}

// ListUsersPage returns one page of accounts matching q (word-prefix search).
func (s *Service) ListUsersPage(ctx context.Context, q string, limit, offset int) ([]ListUsersRow, int64, error) {
	return s.userRepo.ListPage(ctx, q, limit, offset)
}

// GetUser loads a single account with its role name.
func (s *Service) GetUser(ctx context.Context, id string) (*GetUserByIDRow, error) {
	return s.UserByID(ctx, id)
}

// CountStaff counts the accounts with clinical or administrative roles.
func (s *Service) CountStaff(ctx context.Context) (int64, error) {
	roles := []string{auth.RoleAdmin.String(), auth.RolePhysician.String(), auth.RoleReceptionist.String()}
	return s.userRepo.CountStaff(ctx, roles)
}

// UpdatePreferences stores the user's UI theme and personal timezone.
func (s *Service) UpdatePreferences(ctx context.Context, userID, timezone string, theme auth.UITheme) error {
	timezone = strings.TrimSpace(timezone)
	v := validator.New()
	v.Validatable(theme, "theme", "invalid UI theme")
	if timezone != "" {
		_, err := time.LoadLocation(timezone)
		v.Check(err == nil, "timezone", "unknown timezone")
	}
	if err := v.Err(); err != nil {
		return err
	}
	uUUID, err := uuid.Parse(userID)
	if err != nil {
		return errors.Wrap(err, "usecase: invalid user id")
	}
	return s.userRepo.UpdatePreferences(ctx, uUUID, timezone, string(theme))
}
