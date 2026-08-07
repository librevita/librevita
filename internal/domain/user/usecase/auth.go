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

	"github.com/google/uuid"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	clinicdomain "librevita.org/internal/domain/clinic"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/user/repository"
)

// Domain errors. Handlers translate these into user-facing messages.
var (
	ErrEmailTaken         = errors.New("usecase: email is already registered")
	ErrInvalidCredentials = errors.New("usecase: invalid email or password")
	ErrAlreadyOnboarded   = errors.New("usecase: system is already onboarded")
)

// Input field limits.
const (
	maxNameLen     = 100
	maxEmailLen    = 254
	minPasswordLen = 8
	maxPasswordLen = 128

	maxClinicNameLen = 200
	maxTaxIDLen      = 30
	maxPhoneLen      = 30
	maxStreetLen     = 200
	maxCityLen       = 100
	maxStateLen      = 50
	maxPostalCodeLen = 20
	maxCountryLen    = 2
	maxTimezoneLen   = 64
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

// ClinicInput is the clinic profile collected during onboarding.
type ClinicInput struct {
	Name       string
	TaxID      string
	Phone      string
	Email      string
	Street     string
	City       string
	State      string
	PostalCode string
	Country    string
	Timezone   string
}

// Credentials is the login request.
type Credentials struct {
	Email    string
	Password string
}

// Service implements authentication workflows. Public registration always
// creates patient accounts; the admin and clinic profile are created
// together by Onboard, which is only possible on an empty system.
type Service struct {
	db        *sql.DB
	users     *repository.Queries
	clinics   *clinicrepo.Queries
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
		clinics:   clinicrepo.New(db),
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

	// Public registration always creates a patient account. The admin
	// account is created exclusively by Onboard.
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	userID, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("usecase: generate user id: %w", err)
	}

	user, err := s.users.CreateUser(ctx, repository.CreateUserParams{
		ID:           userID.String(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Role:         auth.RolePatient.String(),
	})
	if err != nil {
		// The UNIQUE COLLATE NOCASE constraint maps to a duplicate email.
		return nil, "", ErrEmailTaken
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

// setupMetaKey marks a completed onboarding. The marker is written in the
// same transaction as the admin account, so setup can run exactly once even
// if every account and the clinic are later removed.
const setupMetaKey = "setup_completed"

// IsOnboarded reports whether the system has already been set up. The
// persisted marker is authoritative; the user count is a second guard
// against a deleted marker.
func (s *Service) IsOnboarded(ctx context.Context) (bool, error) {
	_, err := s.users.GetMetaValue(ctx, setupMetaKey)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("usecase: read setup marker: %w", err)
	}
	count, err := s.users.CountUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("usecase: count users: %w", err)
	}
	return count > 0, nil
}

// UserCountByRole counts accounts with the given role (e.g. "patient").
func (s *Service) UserCountByRole(ctx context.Context, role string) (int64, error) {
	count, err := s.users.CountUsersByRole(ctx, role)
	if err != nil {
		return 0, fmt.Errorf("usecase: count users by role: %w", err)
	}
	return count, nil
}

// UserCount counts all accounts.
func (s *Service) UserCount(ctx context.Context) (int64, error) {
	count, err := s.users.CountUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("usecase: count users: %w", err)
	}
	return count, nil
}

// ListRecentUsers returns the newest accounts, newest first.
func (s *Service) ListRecentUsers(ctx context.Context, limit int) ([]repository.ListRecentUsersRow, error) {
	rows, err := s.users.ListRecentUsers(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("usecase: list recent users: %w", err)
	}
	return rows, nil
}

// Onboard creates the initial admin account and the clinic profile in one
// transaction. It succeeds only on a system that has never been set up;
// concurrent setup attempts are serialized by the single SQLite connection,
// so exactly one can win. It returns the principal and the raw session
// token.
func (s *Service) Onboard(ctx context.Context, admin RegisterInput, clinic ClinicInput) (*auth.Principal, string, error) {
	name := strings.TrimSpace(admin.Name)
	email := normalizeEmail(admin.Email)

	if err := validateRegistration(name, email, admin.Password); err != nil {
		s.auditOnboard(ctx, err.Error())
		return nil, "", err
	}
	if err := validateClinic(clinic); err != nil {
		s.auditOnboard(ctx, err.Error())
		return nil, "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", fmt.Errorf("usecase: begin onboard: %w", err)
	}
	defer tx.Rollback()

	qtx := s.users.WithTx(tx)

	if _, err := qtx.GetMetaValue(ctx, setupMetaKey); err == nil {
		return nil, "", ErrAlreadyOnboarded
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, "", fmt.Errorf("usecase: read setup marker: %w", err)
	}

	count, err := qtx.CountUsers(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("usecase: count users: %w", err)
	}
	if count > 0 {
		return nil, "", ErrAlreadyOnboarded
	}

	clinicID, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("usecase: generate clinic id: %w", err)
	}
	if _, err := s.clinics.WithTx(tx).CreateClinic(ctx, clinicrepo.CreateClinicParams{
		ID:         clinicID.String(),
		Name:       strings.TrimSpace(clinic.Name),
		TaxID:      strPtr(clinic.TaxID),
		Phone:      strPtr(clinic.Phone),
		Email:      strPtr(normalizeEmail(clinic.Email)),
		Street:     strPtr(strings.TrimSpace(clinic.Street)),
		City:       strPtr(strings.TrimSpace(clinic.City)),
		State:      strPtr(strings.TrimSpace(clinic.State)),
		PostalCode: strPtr(strings.TrimSpace(clinic.PostalCode)),
		Country:    strings.ToUpper(strings.TrimSpace(orDefault(clinic.Country, "BR"))),
		Timezone:   strings.TrimSpace(orDefault(clinic.Timezone, clinicdomain.DefaultTimezone)),
	}); err != nil {
		return nil, "", fmt.Errorf("usecase: create clinic: %w", err)
	}

	hash, err := auth.HashPassword(admin.Password)
	if err != nil {
		return nil, "", err
	}

	adminID, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("usecase: generate admin id: %w", err)
	}

	user, err := qtx.CreateUser(ctx, repository.CreateUserParams{
		ID:           adminID.String(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Role:         auth.RoleAdmin.String(),
	})
	if err != nil {
		// Unreachable on an empty system, but keep the mapping honest.
		return nil, "", ErrEmailTaken
	}

	if err := qtx.SetMeta(ctx, repository.SetMetaParams{Key: setupMetaKey, Value: "1"}); err != nil {
		return nil, "", fmt.Errorf("usecase: mark setup complete: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, "", fmt.Errorf("usecase: commit onboard: %w", err)
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
		Action: "onboard", Resource: "setup", Result: result, Detail: detail,
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
		s.auditLogin(ctx, "", email, "unknown email")
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

func (s *Service) auditLogin(ctx context.Context, userID string, email, failure string) {
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

func (s *Service) auditOnboard(ctx context.Context, failure string) {
	s.audit.Record(ctx, audit.Event{
		Action: "onboard", Resource: "setup", Result: audit.ResultFailure, Detail: failure,
	})
}

func validateClinic(c ClinicInput) error {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return &ValidationError{Msg: "clinic name is required"}
	}
	if len(name) > maxClinicNameLen {
		return &ValidationError{Msg: "clinic name is too long"}
	}
	if len(c.TaxID) > maxTaxIDLen {
		return &ValidationError{Msg: "tax id is too long"}
	}
	if len(c.Phone) > maxPhoneLen {
		return &ValidationError{Msg: "phone is too long"}
	}
	if email := normalizeEmail(c.Email); email != "" {
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Address != email {
			return &ValidationError{Msg: "enter a valid clinic email address"}
		}
	}
	if len(c.Street) > maxStreetLen || len(c.City) > maxCityLen ||
		len(c.State) > maxStateLen || len(c.PostalCode) > maxPostalCodeLen {
		return &ValidationError{Msg: "address fields are too long"}
	}
	if country := strings.ToUpper(strings.TrimSpace(c.Country)); country != "" && len(country) > maxCountryLen {
		return &ValidationError{Msg: "country must be a two-letter code"}
	}
	if len(c.Timezone) > maxTimezoneLen {
		return &ValidationError{Msg: "timezone is too long"}
	}
	if tz := strings.TrimSpace(c.Timezone); tz != "" && !clinicdomain.ValidTimezone(tz) {
		return &ValidationError{Msg: "pick a timezone from the list"}
	}
	return nil
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
