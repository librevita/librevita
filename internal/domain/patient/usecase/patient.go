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

	"librevita.org/internal/domain/patient/repository"
)

const (
	maxPatientNameLen = 120
	maxDocumentLen    = 20
	maxPhoneLen       = 20
	maxEmailLen       = 120
	maxAddressLen     = 120
	maxNotesLen       = 4000
)

// PatientStatus values for the patients.status column.
const (
	StatusActive   = "active"
	StatusInactive = "inactive"
)

var validSex = map[string]bool{
	"female": true, "male": true, "other": true, "unknown": true,
}

// ValidationError is returned for invalid patient input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// ErrNotFound is returned when the patient does not exist.
var ErrNotFound = errors.New("patient: not found")

// PatientInput is the editable profile of a patient.
type PatientInput struct {
	DisplayName string
	BirthDate   string
	Sex         string
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
	db  *sql.DB
	q   *repository.Queries
	log *slog.Logger
}

// NewService is the Fx provider.
func NewService(db *sql.DB, log *slog.Logger) *Service {
	return &Service{db: db, q: repository.New(db), log: log}
}

// Create validates in and inserts a patient for the clinic.
func (s *Service) Create(ctx context.Context, clinicID string, in PatientInput) (*repository.Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate patient id: %w", err)
	}
	patient, err := s.q.CreatePatient(ctx, repository.CreatePatientParams{
		ID:          id.String(),
		ClinicID:    clinicID,
		DisplayName: normalized.DisplayName,
		BirthDate:   nullString(normalized.BirthDate),
		Sex:         normalized.Sex,
		Document:    nullString(normalized.Document),
		Phone:       nullString(normalized.Phone),
		Email:       nullString(normalized.Email),
		Street:      nullString(normalized.Street),
		City:        nullString(normalized.City),
		State:       nullString(normalized.State),
		PostalCode:  nullString(normalized.PostalCode),
		Notes:       nullString(normalized.Notes),
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: create patient: %w", err)
	}
	return &patient, nil
}

// Get returns a patient by id.
func (s *Service) Get(ctx context.Context, id string) (*repository.Patient, error) {
	patient, err := s.q.GetPatientByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("usecase: get patient: %w", err)
	}
	return &patient, nil
}

// Update validates in and applies it to the patient.
func (s *Service) Update(ctx context.Context, id string, in PatientInput) (*repository.Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	patient, err := s.q.UpdatePatient(ctx, repository.UpdatePatientParams{
		DisplayName: normalized.DisplayName,
		BirthDate:   nullString(normalized.BirthDate),
		Sex:         normalized.Sex,
		Document:    nullString(normalized.Document),
		Phone:       nullString(normalized.Phone),
		Email:       nullString(normalized.Email),
		Street:      nullString(normalized.Street),
		City:        nullString(normalized.City),
		State:       nullString(normalized.State),
		PostalCode:  nullString(normalized.PostalCode),
		Notes:       nullString(normalized.Notes),
		ID:          id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("usecase: update patient: %w", err)
	}
	return &patient, nil
}

// List returns patients for the clinic, filtered by q when non-empty and
// by status when set, up to limit.
func (s *Service) List(ctx context.Context, clinicID, q, status string, limit int) ([]repository.Patient, error) {
	pattern := "%" + strings.TrimSpace(q) + "%"
	if strings.TrimSpace(q) == "" {
		rows, err := s.q.ListPatients(ctx, repository.ListPatientsParams{
			ClinicID: clinicID, StatusEmpty: status, StatusFilter: status, Limit: int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("usecase: list patients: %w", err)
		}
		return rows, nil
	}
	rows, err := s.q.SearchPatients(ctx, repository.SearchPatientsParams{
		ClinicID: clinicID, StatusEmpty: status, StatusFilter: status,
		DisplayName: pattern,
		Document:    sql.NullString{String: pattern, Valid: true},
		Email:       sql.NullString{String: pattern, Valid: true},
		Limit:       int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("usecase: search patients: %w", err)
	}
	return rows, nil
}

// SetStatus archives (inactive) or restores (active) a patient.
func (s *Service) SetStatus(ctx context.Context, id, status string) error {
	if status != StatusActive && status != StatusInactive {
		return &ValidationError{Msg: "invalid patient status"}
	}
	if err := s.q.UpdatePatientStatus(ctx, repository.UpdatePatientStatusParams{
		ID: id, Status: status,
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("usecase: set patient status: %w", err)
	}
	return nil
}

// Count returns the number of patient records for the clinic.
func (s *Service) Count(ctx context.Context, clinicID string) (int64, error) {
	count, err := s.q.CountPatients(ctx, clinicID)
	if err != nil {
		return 0, fmt.Errorf("usecase: count patients: %w", err)
	}
	return count, nil
}

// DocumentExists reports whether another patient in the clinic already
// holds the document. excludeID skips the record being edited.
func (s *Service) DocumentExists(ctx context.Context, clinicID, document, excludeID string) (bool, error) {
	exists, err := s.q.PatientDocumentExists(ctx, repository.PatientDocumentExistsParams{
		ClinicID: clinicID,
		Document: sql.NullString{String: document, Valid: document != ""},
		ID:       excludeID,
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
		Sex:         strings.TrimSpace(in.Sex),
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
		out.Sex = "unknown"
	}
	if !validSex[out.Sex] {
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

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
