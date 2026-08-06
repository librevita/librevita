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

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/domain/user/repository"
)

// Domain errors. Handlers translate these into user-facing messages.
var (
	ErrEmailTaken         = errors.New("usecase: email is already registered")
	ErrInvalidCredentials = errors.New("usecase: invalid email or password")
)

// Input field limits.
const (
	maxNameLen     = 100
	maxEmailLen    = 254
	minPasswordLen = 8
	maxPasswordLen = 128
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
// becomes an admin; subsequent registrations are patients. The first-admin
// decision is atomic: concurrent registrations on an empty database never
// produce more than one admin.
type Service struct {
	db        *sql.DB
	users     *repository.Queries
	sessions  *auth.SessionManager
	audit     *audit.Logger
	log       *slog.Logger
	dummyHash string
}

// NewService is the Fx provider.
func NewService(db *sql.DB, sessions *auth.SessionManager, auditLogger *audit.Logger, log *slog.Logger) *Service {
	// Precomputed hash used to equalize the cost of login attempts for
	// unknown or deactivated accounts, preventing email enumeration by
	// response timing.
	dummyHash, err := auth.HashPassword("dummy-password-for-timing")
	if err != nil {
		// Unreachable: parameters are constant.
		log.Error("failed to precompute dummy password hash", "error", err)
	}
	return &Service{
		db:        db,
		users:     repository.New(db),
		sessions:  sessions,
		audit:     auditLogger,
		log:       log,
		dummyHash: dummyHash,
	}
}

// Register validates the input, creates the account, and starts a session.
// It returns the principal and the raw session token.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*auth.Principal, string, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	password := in.Password

	if err := validateRegistration(name, email, password); err != nil {
		s.audit.Record(ctx, audit.Event{
			Action: "register", Resource: "user", Result: audit.ResultFailure,
			Detail: err.Error(),
		})
		return nil, "", err
	}

	// The whole first-admin decision and insert run inside one transaction
	// on the single SQLite connection, so concurrent registrations are
	// serialized and exactly one account can become admin.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("usecase: begin register: %w", err)
	}
	defer tx.Rollback()

	qtx := s.users.WithTx(tx)

	count, err := qtx.CountUsers(ctx)
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

	user, err := qtx.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Role:         role.String(),
	})
	if err != nil {
		// The UNIQUE COLLATE NOCASE constraint maps to a duplicate email.
		return nil, "", ErrEmailTaken
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("usecase: commit register: %w", err)
	}

	principal, token, err := s.startSession(ctx, user)
	result := audit.ResultSuccess
	detail := ""
	if err != nil {
		result = audit.ResultFailure
		detail = err.Error()
	}
	s.audit.Record(ctx, audit.Event{
		ActorID: user.ID, ActorMail: user.Email,
		Action: "register", Resource: "user", Result: result, Detail: detail,
	})
	return principal, token, err
}

// Login verifies credentials and starts a session. All failure paths run an
// Argon2id verification so that response timing does not reveal whether an
// email exists; failures return ErrInvalidCredentials.
func (s *Service) Login(ctx context.Context, c Credentials) (*auth.Principal, string, error) {
	email := normalizeEmail(c.Email)
	user, err := s.users.GetUserByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		s.timingDummy(c.Password)
		s.auditLogin(ctx, 0, email, "unknown email")
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", fmt.Errorf("usecase: lookup email: %w", err)
	}
	if user.Active != 1 {
		s.timingDummy(c.Password)
		s.auditLogin(ctx, user.ID, email, "account deactivated")
		return nil, "", ErrInvalidCredentials
	}

	ok, err := auth.VerifyPassword(user.PasswordHash, c.Password)
	if err != nil {
		s.log.Warn("stored password hash rejected", "user_id", user.ID)
		s.timingDummy(c.Password)
		s.auditLogin(ctx, user.ID, email, "malformed stored hash")
		return nil, "", ErrInvalidCredentials
	}
	if !ok {
		s.auditLogin(ctx, user.ID, email, "wrong password")
		return nil, "", ErrInvalidCredentials
	}

	principal, token, err := s.startSession(ctx, user)
	if err != nil {
		s.auditLogin(ctx, user.ID, email, err.Error())
		return nil, "", err
	}
	s.auditLogin(ctx, user.ID, email, "")
	return principal, token, nil
}

// Logout destroys the session behind token.
func (s *Service) Logout(ctx context.Context, token string) error {
	err := s.sessions.Destroy(ctx, token)
	result := audit.ResultSuccess
	detail := ""
	if err != nil {
		result = audit.ResultFailure
		detail = err.Error()
	}
	s.audit.Record(ctx, audit.Event{
		Action: "logout", Resource: "session", Result: result, Detail: detail,
	})
	return err
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

// timingDummy spends the same Argon2id cost as a real password check.
func (s *Service) timingDummy(password string) {
	if s.dummyHash != "" {
		_, _ = auth.VerifyPassword(s.dummyHash, password)
	}
}

func (s *Service) auditLogin(ctx context.Context, userID int64, email, failure string) {
	result := audit.ResultSuccess
	if failure != "" {
		result = audit.ResultFailure
	}
	s.audit.Record(ctx, audit.Event{
		ActorID: userID, ActorMail: email,
		Action: "login", Resource: "user",
		Result: result,
		Detail: failure,
	})
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateRegistration(name, email, password string) error {
	if name == "" {
		return &ValidationError{Msg: "display name is required"}
	}
	if len(name) > maxNameLen {
		return &ValidationError{Msg: "display name is too long"}
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || !strings.Contains(email, "@") || addr.Address != email {
		return &ValidationError{Msg: "enter a valid email address"}
	}
	if len(email) > maxEmailLen {
		return &ValidationError{Msg: "email is too long"}
	}
	if len(password) < minPasswordLen {
		return &ValidationError{Msg: "password must be at least 8 characters"}
	}
	if len(password) > maxPasswordLen {
		return &ValidationError{Msg: "password must be at most 128 characters"}
	}
	return nil
}
