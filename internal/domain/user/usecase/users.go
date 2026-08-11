package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/domain/user/repository"
	"librevita.org/internal/types"
)

// User errors.
var (
	ErrUserNotFound     = errors.New("usecase: user not found")
	ErrCannotDemoteSelf = errors.New("usecase: cannot change your own role or status")
	ErrLastActiveAdmin  = errors.New("usecase: cannot deactivate or demote the last active admin")
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

// CreateUser creates a staff account with the given role. The requester
// must hold the users.manage policy (enforced by the route middleware).
func (s *Service) CreateUser(ctx context.Context, in CreateUserInput) (*repository.User, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	role := in.Role

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

	roleRow, err := s.users.GetRoleByName(ctx, role)
	if err != nil {
		return nil, &ValidationError{Msg: "unsupported role"}
	}
	user, err := s.users.CreateUser(ctx, repository.CreateUserParams{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		RoleID:       roleRow.ID,
	})
	if err != nil {
		return nil, ErrEmailTaken
	}
	return &user, nil
}

// UpdateUser changes the profile, role, or status of an account. It
// refuses to lock the system out: an admin cannot demote or deactivate
// themselves, and the last active admin cannot be demoted or
// deactivated.
func (s *Service) UpdateUser(ctx context.Context, id string, actorID string, in UpdateUserInput) (*repository.User, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	role := in.Role

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
		if role != auth.RoleAdmin.String() || !in.Active {
			return nil, ErrCannotDemoteSelf
		}
	}

	roleRow, err := s.users.GetRoleByName(ctx, role)
	if err != nil {
		return nil, &ValidationError{Msg: "unsupported role"}
	}

	current, err := s.users.GetUserByID(ctx, uuid.MustParse(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("usecase: load user: %w", err)
	}

	demotingOrDeactivating := (current.RoleName == auth.RoleAdmin.String() && current.Active) &&
		(role != auth.RoleAdmin.String() || !in.Active)

	var user repository.User
	if demotingOrDeactivating {
		// The last-active-admin check lives inside the UPDATE itself
		// (single statement = atomic), so two concurrent admins cannot
		// both pass a separate count check and leave no active admin.
		u, err := s.users.UpdateUserGuarded(ctx, repository.UpdateUserGuardedParams{
			Email: email, DisplayName: name, RoleID: roleRow.ID, Active: in.Active, ID: uuid.MustParse(id), Column6: 1,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrLastActiveAdmin
			}
			return nil, ErrEmailTaken
		}
		user = u
	} else {
		u, err := s.users.UpdateUser(ctx, repository.UpdateUserParams{
			ID: uuid.MustParse(id), Email: email, DisplayName: name, RoleID: roleRow.ID, Active: in.Active,
		})
		if err != nil {
			return nil, ErrEmailTaken
		}
		user = u
	}
	return &user, nil
}

// validateEmail checks the email without the password rules.
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

// ListUsersPage returns one page of accounts matching q (name or email,
// case-insensitive), newest first, together with the total match count.
// instr() matches the term literally, so LIKE wildcards have no effect.
func (s *Service) ListUsersPage(ctx context.Context, q string, limit, offset int) ([]repository.ListUsersRow, int64, error) {
	rows, err := s.users.ListUsers(ctx, repository.ListUsersParams{
		Column1: q,
		Limit:   int64(limit),
		Offset:  int64(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list users: %w", err)
	}
	total, err := s.users.CountUsersMatching(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: count users: %w", err)
	}
	return rows, total, nil
}

// GetUser loads a single account with its role name.
func (s *Service) GetUser(ctx context.Context, id string) (*repository.GetUserByIDRow, error) {
	user, err := s.users.GetUserByID(ctx, uuid.MustParse(id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("usecase: get user: %w", err)
	}
	return &user, nil
}

// CountStaff counts the accounts with clinical or administrative roles.
func (s *Service) CountStaff(ctx context.Context) (int64, error) {
	var total int64
	for _, role := range []string{auth.RoleAdmin.String(), auth.RolePhysician.String(), auth.RoleReceptionist.String()} {
		count, err := s.users.CountUsersByRole(ctx, role)
		if err != nil {
			return 0, fmt.Errorf("usecase: count staff by role: %w", err)
		}
		total += count
	}
	return total, nil
}

// UpdatePreferences stores the user's UI theme and personal timezone.
// An empty timezone inherits the clinic timezone. The theme is a closed
// set (types.UITheme, mirrored by the database CHECK constraint) and
// the timezone must be a valid IANA zone.
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
	if _, err := s.users.UpdateUserPreferences(ctx, repository.UpdateUserPreferencesParams{
		Timezone: timezone, UiTheme: theme, ID: uuid.MustParse(userID),
	}); err != nil {
		return fmt.Errorf("usecase: update preferences: %w", err)
	}
	return nil
}
