// Package usecase holds the patient domain service: validation and
// persistence of patient records.
package usecase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/types"
)

const (
	maxPatientNameLen = 120
	maxDocumentLen    = 20
	maxPhoneLen       = 20
	maxEmailLen       = 120
	maxAddressLen     = 120
	maxNotesLen       = 4000
)

// ValidationError is returned for invalid patient input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ErrNotFound is returned when the patient does not exist.
var ErrNotFound = errors.New("patient: not found")

// PatientInput is the editable profile of a patient.
type PatientInput struct {
	DisplayName string
	BirthDate   string
	Sex         types.Sex
	Document    string
	Phone       string
	Email       string
	Street      string
	City        string
	State       string
	PostalCode  string
	Notes       string
}

// Service persists and validates patient records.
type Service struct {
	db       *sql.DB
	q        *repository.Queries
	log      *slog.Logger
	policies *policy.PolicyEngine
}

// NewService is the Fx provider.
func NewService(db *sql.DB, log *slog.Logger, policies *policy.PolicyEngine) *Service {
	return &Service{db: db, q: repository.New(db), log: log, policies: policies}
}

// ErrForbidden reports a resource-level policy denial.
var ErrForbidden = errors.New("usecase: permission denied")

// AuthorizePatientEdit evaluates the fine-grained patient.edit policy
// against the record itself: an admin edits anything, a physician only
// the patients they registered. Callers audit the denial.
func (s *Service) AuthorizePatientEdit(ctx context.Context, principal *auth.Principal, id string, createdBy *string, status types.PatientStatus) error {
	resource := map[string]any{
		"id":         id,
		"created_by": orEmpty(createdBy),
		"status":     status.String(),
	}
	allowed, err := s.policies.AllowedResource(ctx, "patient.edit", principal,
		policy.RequestInfo{Method: "POST", Path: "/patients/" + id}, resource, nil)
	if err != nil {
		return fmt.Errorf("usecase: authorize patient edit: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

// Create validates in and inserts a patient for the clinic, recording
// createdBy (the user id of the registrar).
func (s *Service) Create(ctx context.Context, clinicID, createdBy string, in PatientInput) (*repository.Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate patient id: %w", err)
	}
	patient, err := s.q.CreatePatient(ctx, repository.CreatePatientParams{
		ID:          id,
		ClinicID:    uuid.MustParse(clinicID),
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex.String(),
		Document:    strPtr(normalized.Document),
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
		CreatedBy:   uuidOrNil(createdBy),
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: create patient: %w", err)
	}
	return &patient, nil
}

// GetWithCreator returns a patient with the registrar's email.
func (s *Service) GetWithCreator(ctx context.Context, id string) (*repository.GetPatientWithCreatorRow, error) {
	row, err := s.q.GetPatientWithCreator(ctx, uuid.MustParse(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("usecase: get patient with creator: %w", err)
	}
	return &row, nil
}

// Get returns a patient by id.
func (s *Service) Get(ctx context.Context, id string) (*repository.Patient, error) {
	patient, err := s.q.GetPatientByID(ctx, uuid.MustParse(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("usecase: get patient: %w", err)
	}
	return &patient, nil
}

// Update validates in and applies it to the patient.
func (s *Service) Update(ctx context.Context, clinicID, id string, in PatientInput) (*repository.Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	patient, err := s.q.UpdatePatient(ctx, repository.UpdatePatientParams{
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex.String(),
		Document:    strPtr(normalized.Document),
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
		ID:          uuid.MustParse(id),
		ClinicID:    uuid.MustParse(clinicID),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("usecase: update patient: %w", err)
	}
	return &patient, nil
}

// SetStatus updates the patient status, scoped to the clinic.
func (s *Service) SetStatus(ctx context.Context, clinicID, id string, status types.PatientStatus) error {
	err := s.q.UpdatePatientStatus(ctx, repository.UpdatePatientStatusParams{
		Status: status.String(), ID: uuid.MustParse(id), ClinicID: uuid.MustParse(clinicID),
	})
	if err != nil {
		return fmt.Errorf("usecase: set patient status: %w", err)
	}
	return nil
}

// ListPage returns one page of patients matching q and status, ordered
// by display name, together with the total match count.
func (s *Service) ListPage(ctx context.Context, clinicID, q, status string, limit, offset int) ([]repository.Patient, int64, error) {
	// The SQL pattern matches whole-word prefixes: the term must start a
	// word in the name or a document/email value.
	pattern := strings.TrimSpace(q)
	rows, err := s.q.ListPatientsPage(ctx, repository.ListPatientsPageParams{
		ClinicID: uuid.MustParse(clinicID), StatusEmpty: status, StatusFilter: status,
		QueryEmpty: q, Pattern: strPtr(pattern), Limit: int64(limit), Offset: int64(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list patients page: %w", err)
	}
	total, err := s.q.CountPatientsMatching(ctx, repository.CountPatientsMatchingParams{
		ClinicID: uuid.MustParse(clinicID), StatusEmpty: status, StatusFilter: status,
		QueryEmpty: q, Pattern: strPtr(pattern),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: count patients: %w", err)
	}
	return rows, total, nil
}

func (s *Service) Count(ctx context.Context, clinicID string) (int64, error) {
	count, err := s.q.CountPatients(ctx, uuid.MustParse(clinicID))
	if err != nil {
		return 0, fmt.Errorf("usecase: count patients: %w", err)
	}
	return count, nil
}

// DocumentExists reports whether another patient in the clinic already
// holds the document. excludeID skips the record being edited.
func (s *Service) DocumentExists(ctx context.Context, clinicID, document, excludeID string) (bool, error) {
	exists, err := s.q.PatientDocumentExists(ctx, repository.PatientDocumentExistsParams{
		ClinicID: uuid.MustParse(clinicID),
		Document: strPtr(document),
		ID:       uuid.MustParse(excludeID),
	})
	if err != nil {
		return false, fmt.Errorf("usecase: check patient document: %w", err)
	}
	return exists, nil
}

func normalize(in PatientInput) (PatientInput, error) {
	out := PatientInput{
		DisplayName: strings.TrimSpace(in.DisplayName),
		BirthDate:   strings.TrimSpace(in.BirthDate),
		Sex:         types.Sex(strings.TrimSpace(in.Sex.String())),
		Document:    strings.TrimSpace(in.Document),
		Phone:       strings.TrimSpace(in.Phone),
		Email:       strings.TrimSpace(in.Email),
		Street:      strings.TrimSpace(in.Street),
		City:        strings.TrimSpace(in.City),
		State:       strings.TrimSpace(in.State),
		PostalCode:  strings.TrimSpace(in.PostalCode),
		Notes:       strings.TrimSpace(in.Notes),
	}
	if out.DisplayName == "" {
		return out, &ValidationError{Msg: "patient name is required"}
	}
	if len(out.DisplayName) > maxPatientNameLen {
		return out, &ValidationError{Msg: "patient name is too long"}
	}
	if out.Sex == "" {
		out.Sex = types.SexUnknown
	}
	if !out.Sex.Valid() {
		return out, &ValidationError{Msg: "invalid sex"}
	}
	if out.BirthDate != "" {
		if _, err := time.Parse("2006-01-02", out.BirthDate); err != nil {
			return out, &ValidationError{Msg: "enter a valid birth date (YYYY-MM-DD)"}
		}
	}
	if len(out.Document) > maxDocumentLen {
		return out, &ValidationError{Msg: "document is too long"}
	}
	if len(out.Phone) > maxPhoneLen {
		return out, &ValidationError{Msg: "phone is too long"}
	}
	if out.Email != "" {
		addr, err := mail.ParseAddress(out.Email)
		if err != nil || addr.Address != out.Email {
			return out, &ValidationError{Msg: "enter a valid email address"}
		}
	}
	if len(out.Email) > maxEmailLen || len(out.Street) > maxAddressLen ||
		len(out.City) > maxAddressLen || len(out.State) > maxAddressLen ||
		len(out.PostalCode) > maxAddressLen {
		return out, &ValidationError{Msg: "address fields are too long"}
	}
	if len(out.Notes) > maxNotesLen {
		return out, &ValidationError{Msg: "notes are too long"}
	}
	return out, nil
}

// uuidOrNil maps an empty optional id to the Nil uuid the storage layer
// stores as NULL.
func uuidOrNil(s string) uuid.UUID {
	if s == "" {
		return uuid.Nil
	}
	return uuid.MustParse(s)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
