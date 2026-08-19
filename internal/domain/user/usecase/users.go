package usecase

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/types"
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
		return nil, fmt.Errorf("usecase: generate user id: %w", err)
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

	if name == "" {
		return nil, &ValidationError{Msg: "display name is required"}
	}
	if len(name) > maxNameLen {
		return nil, &ValidationError{Msg: "display name is too long"}
	}
	if err := validateEmail(email); err != nil {
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
		return nil, fmt.Errorf("usecase: invalid user id: %w", err)
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
			return nil, fmt.Errorf("usecase: count active admins: %w", err)
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
	addr, err := mail.ParseAddress(email)
	if err != nil || !strings.Contains(email, "@") || addr.Address != email {
		return &ValidationError{Msg: "enter a valid email address"}
	}
	if len(email) > maxEmailLen {
		return &ValidationError{Msg: "email is too long"}
	}
	return nil
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
func (s *Service) UpdatePreferences(ctx context.Context, userID, timezone string, theme types.UITheme) error {
	timezone = strings.TrimSpace(timezone)
	if !theme.Valid() {
		return &ValidationError{Msg: "invalid UI theme"}
	}
	if timezone != "" {
		if _, err := time.LoadLocation(timezone); err != nil {
			return &ValidationError{Msg: "unknown timezone"}
		}
	}
	uUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("usecase: invalid user id: %w", err)
	}
	return s.userRepo.UpdatePreferences(ctx, uUUID, timezone, string(theme))
}
