// Package usecase implements authentication workflows for the user domain.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/pkg/validator"
)

// Input field limits.
const (
	maxNameLen     = 100
	maxEmailLen    = 254
	minPasswordLen = 8
	maxPasswordLen = 128
)

// ValidationError reports invalid registration input.
type ValidationError = validator.ValidationError

// RegisterInput is the account creation request.
type RegisterInput struct {
	Name      string
	Email     string
	Password  string
	PatientID string
}

// Credentials is the login request.
type Credentials struct {
	Email    string
	Password string
}

// Service implements authentication workflows and user domain orchestration.
type Service struct {
	userRepo      UserRepository
	roleRepo      RoleRepository
	specialtyRepo SpecialtyRepository
	staffReqRepo  StaffRequestRepository
	setupRepo     SetupRepository
	sessions      *auth.SessionManager
	audit         *audit.Logger
	log           *slog.Logger
	dummyHash     string
}

// NewService is the Fx provider.
func NewService(
	userRepo UserRepository,
	roleRepo RoleRepository,
	specialtyRepo SpecialtyRepository,
	staffReqRepo StaffRequestRepository,
	setupRepo SetupRepository,
	sessions *auth.SessionManager,
	auditLogger *audit.Logger,
	log *slog.Logger,
) *Service {
	dummyHash, err := auth.HashPassword("dummy-password-for-timing")
	if err != nil {
		log.Error("failed to precompute dummy password hash", "error", err)
	}
	return &Service{
		userRepo:      userRepo,
		roleRepo:      roleRepo,
		specialtyRepo: specialtyRepo,
		staffReqRepo:  staffReqRepo,
		setupRepo:     setupRepo,
		sessions:      sessions,
		audit:         auditLogger,
		log:           log,
		dummyHash:     dummyHash,
	}
}

// Register validates the input, creates the account, and starts a session.
func (s *Service) Register(ctx context.Context, in RegisterInput) (*auth.Principal, string, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.Email)
	password := in.Password

	if err := validateRegistration(name, email, password); err != nil {
		s.audit.Record(ctx, audit.Event{
			Action: "register", Resource: "user", Result: audit.AuditResultFailure,
			Detail: err.Error(),
		})
		return nil, "", err
	}

	patientUUID, err := uuid.Parse(strings.TrimSpace(in.PatientID))
	if err != nil {
		s.audit.Record(ctx, audit.Event{
			Action: "register", Resource: "user", Result: audit.AuditResultFailure,
			Detail: "patient is required",
		})
		return nil, "", &ValidationError{Msg: "patient is required"}
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, "", err
	}

	userID, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("usecase: generate user id: %w", err)
	}

	patientRole, err := s.roleRepo.GetByName(ctx, auth.RolePatient.String())
	if err != nil {
		return nil, "", fmt.Errorf("usecase: resolve patient role: %w", err)
	}

	u, err := s.userRepo.Create(ctx, &User{
		ID:           userID,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		RoleID:       patientRole.ID,
		Active:       true,
	})
	if err != nil {
		return nil, "", ErrEmailTaken
	}
	if err := s.userRepo.BindPortalPatient(ctx, u.ID, patientUUID); err != nil {
		return nil, "", &ValidationError{Msg: "could not link the patient portal"}
	}

	principal, token, err := s.startSession(ctx, u.ID.String(), u.Email, u.DisplayName, auth.RolePatient.String())
	result := audit.AuditResultSuccess
	detail := ""
	if err != nil {
		result = audit.AuditResultFailure
		detail = err.Error()
	}
	s.audit.Record(ctx, audit.Event{
		ActorID: u.ID.String(), ActorMail: u.Email,
		Action: "register", Resource: "user", Result: result, Detail: detail,
	})
	return principal, token, err
}

// IsOnboarded reports whether the system has already been set up.
func (s *Service) IsOnboarded(ctx context.Context) (bool, error) {
	return s.setupRepo.IsOnboarded(ctx)
}

// UserCountByRole counts accounts with the given role.
func (s *Service) UserCountByRole(ctx context.Context, roleName string) (int64, error) {
	return s.userRepo.CountByRole(ctx, roleName)
}

// UserCount counts all accounts.
func (s *Service) UserCount(ctx context.Context) (int64, error) {
	return s.userRepo.Count(ctx)
}

// ListRecentUsers returns the newest accounts, newest first.
func (s *Service) ListRecentUsers(ctx context.Context, limit int) ([]ListRecentUsersRow, error) {
	return s.userRepo.ListRecent(ctx, limit)
}

// UserByID returns the account with the given id.
func (s *Service) UserByID(ctx context.Context, id string) (*GetUserByIDRow, error) {
	uUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid user id: %w", err)
	}
	return s.userRepo.GetByID(ctx, uUUID)
}

// Onboard creates the first clinic admin, system roles, and policies for the
// clinic already in context (the Host shell).
func (s *Service) Onboard(ctx context.Context, admin RegisterInput, systemIDs []uuid.UUID) (*auth.Principal, string, error) {
	name := strings.TrimSpace(admin.Name)
	email := normalizeEmail(admin.Email)

	if err := validateRegistration(name, email, admin.Password); err != nil {
		s.auditOnboard(ctx, err.Error())
		return nil, "", err
	}
	if _, err := clinicctx.MustClinicID(ctx); err != nil {
		s.auditOnboard(ctx, err.Error())
		return nil, "", err
	}

	hash, err := auth.HashPassword(admin.Password)
	if err != nil {
		return nil, "", err
	}

	adminID, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("usecase: generate admin id: %w", err)
	}

	adminUser := &User{
		ID:           adminID,
		Email:        email,
		PasswordHash: hash,
		DisplayName:  name,
		Active:       true,
	}

	createdUser, err := s.setupRepo.Onboard(ctx, adminUser, systemIDs)
	if err != nil {
		if errors.Is(err, ErrAlreadyOnboarded) {
			return nil, "", ErrAlreadyOnboarded
		}
		return nil, "", err
	}

	principal, token, err := s.startSession(ctx, createdUser.ID.String(), createdUser.Email, createdUser.DisplayName, auth.RoleAdmin.String())
	result := audit.AuditResultSuccess
	detail := ""
	if err != nil {
		result = audit.AuditResultFailure
		detail = err.Error()
	}
	s.audit.Record(ctx, audit.Event{
		ActorID: createdUser.ID.String(), ActorMail: createdUser.Email,
		Action: "onboard", Resource: "setup", Result: result, Detail: detail,
	})
	return principal, token, err
}

// Login verifies credentials and starts a session.
func (s *Service) Login(ctx context.Context, c Credentials) (*auth.Principal, string, error) {
	email := normalizeEmail(c.Email)
	u, err := s.userRepo.GetByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		s.timingDummy(c.Password)
		s.auditLogin(ctx, "", email, "unknown email")
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", fmt.Errorf("usecase: lookup email: %w", err)
	}
	if !u.Active {
		s.timingDummy(c.Password)
		s.auditLogin(ctx, u.ID.String(), email, "account deactivated")
		return nil, "", ErrInvalidCredentials
	}

	ok, err := auth.VerifyPassword(u.PasswordHash, c.Password)
	if err != nil {
		s.log.Warn("stored password hash rejected", "user_id", u.ID)
		s.timingDummy(c.Password)
		s.auditLogin(ctx, u.ID.String(), email, "malformed stored hash")
		return nil, "", ErrInvalidCredentials
	}
	if !ok {
		s.auditLogin(ctx, u.ID.String(), email, "wrong password")
		return nil, "", ErrInvalidCredentials
	}

	principal, token, err := s.startSession(ctx, u.ID.String(), u.Email, u.DisplayName, u.RoleName)
	if err != nil {
		s.auditLogin(ctx, u.ID.String(), email, err.Error())
		return nil, "", err
	}
	s.auditLogin(ctx, u.ID.String(), email, "")
	return principal, token, nil
}

// Logout destroys the session behind token.
func (s *Service) Logout(ctx context.Context, token string) error {
	err := s.sessions.Destroy(ctx, token)
	result := audit.AuditResultSuccess
	detail := ""
	if err != nil {
		result = audit.AuditResultFailure
		detail = err.Error()
	}
	s.audit.Record(ctx, audit.Event{
		Action: "logout", Resource: "session", Result: result, Detail: detail,
	})
	return err
}

func (s *Service) startSession(ctx context.Context, id, email, name, roleName string) (*auth.Principal, string, error) {
	principal := &auth.Principal{
		ID:    id,
		Email: email,
		Name:  name,
		Role:  auth.Role(roleName),
	}
	if cid, ok := clinicctx.ClinicID(ctx); ok {
		principal.ClinicID = cid.String()
	}

	token, err := s.sessions.Create(ctx, *principal)
	if err != nil {
		return nil, "", err
	}
	return principal, token, nil
}

func (s *Service) timingDummy(password string) {
	if s.dummyHash != "" {
		_, _ = auth.VerifyPassword(s.dummyHash, password)
	}
}

func (s *Service) auditLogin(ctx context.Context, userID string, email, failure string) {
	result := audit.AuditResultSuccess
	if failure != "" {
		result = audit.AuditResultFailure
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
	v := validator.New()

	v.Field(name, "name", "display name").
		Required().
		Max(maxNameLen)

	v.Field(email, "email", "email").
		Required().
		Email().
		Max(maxEmailLen)

	v.Field(password, "password", "password").
		Required().
		Min(minPasswordLen).
		Max(maxPasswordLen)

	return v.Err()
}

func (s *Service) auditOnboard(ctx context.Context, failure string) {
	s.audit.Record(ctx, audit.Event{
		Action: "onboard", Resource: "setup", Result: audit.AuditResultFailure, Detail: failure,
	})
}
