// Package usecase implements authentication workflows for the user domain.
package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/domain/user/repository"
)

// Domain errors. Handlers translate these into user-facing messages.
var (
	ErrEmailTaken         = errors.New("usecase: email is already registered")
	ErrInvalidCredentials = errors.New("usecase: invalid email or password")
)

// ValidationError reports invalid registration input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return "usecase: " + e.Msg }

// RegisterInput is the account creation request.
type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

// Credentials is the login request.
type Credentials struct {
	Email    string
	Password string
}

// Service is the authentication use case. The first registered account
// becomes an admin; subsequent registrations are patients.
type Service struct {
	users    *repository.Queries
	sessions *auth.SessionManager
	log      *slog.Logger
}

// NewService is the Fx provider.
func NewService(db *sql.DB, sessions *auth.SessionManager, log *slog.Logger) *Service {
	return &Service{users: repository.New(db), sessions: sessions, log: log}
}

// Register validates the input, creates the account, and starts a session.
// It returns the principal and the raw session token.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*auth.Principal, string, error) {
	name := strings.TrimSpace(in.Name)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	password := in.Password

	if name == "" {
		return nil, "", &ValidationError{Msg: "display name is required"}
	}
	if _, err := mail.ParseAddress(email); err != nil || !strings.Contains(email, "@") {
		return nil, "", &ValidationError{Msg: "enter a valid email address"}
	}
	if len(password) < 8 {
		return nil, "", &ValidationError{Msg: "password must be at least 8 characters"}
	}

	if _, err := s.users.GetUserByEmail(ctx, email); err == nil {
		return nil, "", ErrEmailTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("usecase: lookup email: %w", err)
	}

	count, err := s.users.CountUsers(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("usecase: count users: %w", err)
	}

	role := auth.RolePatient
	if count == 0 {
		role = auth.RoleAdmin
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	user, err := s.users.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Role:         role.String(),
	})
	if err != nil {
		return nil, "", fmt.Errorf("usecase: create user: %w", err)
	}

	return s.startSession(ctx, user)
}

// Login verifies credentials and starts a session. Failures return
// ErrInvalidCredentials without disclosing whether the email exists.
func (s *Service) Login(ctx context.Context, c Credentials) (*auth.Principal, string, error) {
	email := strings.ToLower(strings.TrimSpace(c.Email))
	user, err := s.users.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", fmt.Errorf("usecase: lookup email: %w", err)
	}
	if user.Active != 1 {
		return nil, "", ErrInvalidCredentials
	}

	ok, err := auth.VerifyPassword(user.PasswordHash, c.Password)
	if err != nil {
		s.log.Warn("stored password hash rejected", "user_id", user.ID)
		return nil, "", ErrInvalidCredentials
	}
	if !ok {
		return nil, "", ErrInvalidCredentials
	}

	return s.startSession(ctx, user)
}

// Logout destroys the session behind token.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.sessions.Destroy(ctx, token)
}

func (s *Service) startSession(ctx context.Context, user repository.User) (*auth.Principal, string, error) {
	role, err := auth.ParseRole(user.Role)
	if err != nil {
		return nil, "", err
	}

	principal := &auth.Principal{
		ID:    user.ID,
		Email: user.Email,
		Name:  user.DisplayName,
		Role:  role,
	}

	token, err := s.sessions.Create(ctx, *principal)
	if err != nil {
		return nil, "", err
	}
	return principal, token, nil
}
