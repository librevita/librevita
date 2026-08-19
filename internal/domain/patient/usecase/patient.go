// Package usecase holds the patient domain service: validation and
// persistence of patient records with Zero-Knowledge encryption.
package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/policy"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/internal/types"
)

const (
	maxPatientNameLen = 120
	maxPhoneLen       = 20
	maxEmailLen       = 120
	maxAddressLen     = 120
	maxNotesLen       = 4000
)

// ValidationError is returned for invalid patient input.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// Re-export domain models, records, and contracts from patient/model.
type (
	PatientPayload           = patientmodel.PatientPayload
	Patient                  = patientmodel.Patient
	PatientRecord            = patientmodel.PatientRecord
	PatientRecordWithCreator = patientmodel.PatientRecordWithCreator
	GetPatientWithCreatorRow = patientmodel.GetPatientWithCreatorRow
	PatientInput             = patientmodel.PatientInput
	PatientRepository        = patientmodel.PatientRepository
)

var (
	ErrNotFound  = patientmodel.ErrNotFound
	ErrForbidden = patientmodel.ErrForbidden
)

// Service persists and validates patient records.
type Service struct {
	repo     PatientRepository
	crypto   *crypto.Engine
	log      *slog.Logger
	policies *policy.PolicyEngine
}

// NewService is the Fx provider.
func NewService(repo PatientRepository, engine *crypto.Engine, log *slog.Logger, policies *policy.PolicyEngine) *Service {
	return &Service{
		repo:     repo,
		crypto:   engine,
		log:      log,
		policies: policies,
	}
}

func patientURN(id uuid.UUID) string {
	return "urn:librevita:patient:" + id.String()
}

// AuthorizePatientEdit evaluates the fine-grained patient.edit policy.
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

// Create validates in, encrypts PII/PHI, and inserts a patient record.
func (s *Service) Create(ctx context.Context, clinicID, createdBy string, in PatientInput) (*Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("usecase: generate patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	blindHex, err := s.crypto.BlindIndex("patient.display_name", strings.ToLower(normalized.DisplayName))
	if err != nil {
		return nil, fmt.Errorf("usecase: compute blind index: %w", err)
	}

	payload := PatientPayload{
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex,
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
	}

	ciphertext, nonce, err := s.crypto.EncryptStruct(ctx, patientURN(id), []byte("patient"), payload)
	if err != nil {
		return nil, fmt.Errorf("usecase: encrypt patient: %w", err)
	}

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	rec := PatientRecord{
		ID:               id,
		ClinicID:         cUUID,
		BlindIndex:       blindHex,
		EncryptedPayload: ciphertext,
		Nonce:            nonce,
		Status:           types.PatientStatusActive,
		CreatedBy:        cb,
	}

	saved, err := s.repo.Create(ctx, rec)
	if err != nil {
		return nil, fmt.Errorf("usecase: create patient: %w", err)
	}

	return s.toPatientModel(saved, &payload), nil
}

// Get returns a patient by id, scoped to the clinic, with transparent decryption.
func (s *Service) Get(ctx context.Context, clinicID, id string) (*Patient, error) {
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	p, err := s.repo.Get(ctx, cUUID, pUUID)
	if err != nil {
		return nil, err
	}

	payload, err := s.decryptPayload(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("usecase: decrypt patient %s: %w", id, err)
	}

	return s.toPatientModel(p, payload), nil
}

// GetWithCreator returns a patient with the registrar's email, scoped to the clinic.
func (s *Service) GetWithCreator(ctx context.Context, clinicID, id string) (*GetPatientWithCreatorRow, error) {
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	res, err := s.repo.GetWithCreator(ctx, cUUID, pUUID)
	if err != nil {
		return nil, err
	}

	payload, err := s.decryptPayload(ctx, &res.Record)
	if err != nil {
		return nil, fmt.Errorf("usecase: decrypt patient %s: %w", id, err)
	}

	pt := s.toPatientModel(&res.Record, payload)
	return &GetPatientWithCreatorRow{
		ID:           pt.ID,
		ClinicID:     pt.ClinicID,
		DisplayName:  pt.DisplayName,
		BirthDate:    pt.BirthDate,
		Sex:          pt.Sex,
		Phone:        pt.Phone,
		Email:        pt.Email,
		Street:       pt.Street,
		City:         pt.City,
		State:        pt.State,
		PostalCode:   pt.PostalCode,
		Notes:        pt.Notes,
		Status:       pt.Status,
		CreatedBy:    pt.CreatedBy,
		CreatedAt:    pt.CreatedAt,
		UpdatedAt:    pt.UpdatedAt,
		CreatorEmail: res.CreatorEmail,
		CreatorName:  res.CreatorName,
	}, nil
}

// Update validates in, encrypts new payload, and updates the patient.
func (s *Service) Update(ctx context.Context, clinicID, id string, in PatientInput) (*Patient, error) {
	normalized, err := normalize(in)
	if err != nil {
		return nil, err
	}
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	blindHex, err := s.crypto.BlindIndex("patient.display_name", strings.ToLower(normalized.DisplayName))
	if err != nil {
		return nil, fmt.Errorf("usecase: compute blind index: %w", err)
	}

	payload := PatientPayload{
		DisplayName: normalized.DisplayName,
		BirthDate:   strPtr(normalized.BirthDate),
		Sex:         normalized.Sex,
		Phone:       strPtr(normalized.Phone),
		Email:       strPtr(normalized.Email),
		Street:      strPtr(normalized.Street),
		City:        strPtr(normalized.City),
		State:       strPtr(normalized.State),
		PostalCode:  strPtr(normalized.PostalCode),
		Notes:       strPtr(normalized.Notes),
	}

	ciphertext, nonce, err := s.crypto.EncryptStruct(ctx, patientURN(pUUID), []byte("patient"), payload)
	if err != nil {
		return nil, fmt.Errorf("usecase: encrypt patient: %w", err)
	}

	rec := PatientRecord{
		ID:               pUUID,
		ClinicID:         cUUID,
		BlindIndex:       blindHex,
		EncryptedPayload: ciphertext,
		Nonce:            nonce,
		Status:           types.PatientStatusActive,
	}

	// Preserve existing status on update
	existing, err := s.repo.Get(ctx, cUUID, pUUID)
	if err != nil {
		return nil, err
	}
	rec.Status = existing.Status
	rec.CreatedBy = existing.CreatedBy

	updated, err := s.repo.Update(ctx, rec)
	if err != nil {
		return nil, err
	}

	return s.toPatientModel(updated, &payload), nil
}

// SetStatus updates the patient status, scoped to the clinic.
func (s *Service) SetStatus(ctx context.Context, clinicID, id string, status types.PatientStatus) error {
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	count, err := s.repo.BulkSetStatus(ctx, cUUID, []uuid.UUID{pUUID}, status)
	if err != nil {
		return fmt.Errorf("usecase: set patient status: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPage returns one page of decrypted patients.
func (s *Service) ListPage(ctx context.Context, clinicID, q, field, status string, limit, offset int) ([]Patient, int64, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	var statusFilter *types.PatientStatus
	if status != "" {
		st := types.PatientStatus(status)
		statusFilter = &st
	}

	records, err := s.repo.ListByClinicAndStatus(ctx, cUUID, statusFilter)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list patients: %w", err)
	}

	total := len(records)
	filtered := make([]Patient, 0, total)
	trimmedQ := strings.ToLower(strings.TrimSpace(q))

	for _, rec := range records {
		payload, err := s.decryptPayload(ctx, &rec)
		if err != nil {
			s.log.Error("usecase: list decrypt patient failed", "id", rec.ID, "error", err)
			continue
		}
		if trimmedQ != "" {
			matchesName := matchWordPrefix(payload.DisplayName, trimmedQ)
			matchesEmail := payload.Email != nil && strings.HasPrefix(strings.ToLower(*payload.Email), trimmedQ)

			switch field {
			case "name":
				if !matchesName {
					continue
				}
			case "email":
				if !matchesEmail {
					continue
				}
			default:
				if !matchesName && !matchesEmail {
					continue
				}
			}
		}
		pt := s.toPatientModel(&rec, payload)
		filtered = append(filtered, *pt)
	}

	// Apply pagination over filtered list
	start := offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + limit
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], int64(len(filtered)), nil
}

func matchWordPrefix(text, query string) bool {
	words := strings.Fields(strings.ToLower(text))
	for _, w := range words {
		if strings.HasPrefix(w, query) {
			return true
		}
	}
	return false
}

// Count returns total number of patients for the clinic.
func (s *Service) Count(ctx context.Context, clinicID string) (int64, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return 0, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}
	count, err := s.repo.Count(ctx, cUUID)
	if err != nil {
		return 0, fmt.Errorf("usecase: count patients: %w", err)
	}
	return int64(count), nil
}

func (s *Service) decryptPayload(ctx context.Context, rec *PatientRecord) (*PatientPayload, error) {
	var payload PatientPayload
	if err := s.crypto.DecryptInto(ctx, patientURN(rec.ID), []byte("patient"), rec.EncryptedPayload, rec.Nonce, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (s *Service) toPatientModel(rec *PatientRecord, payload *PatientPayload) *Patient {
	return &Patient{
		ID:          rec.ID,
		ClinicID:    rec.ClinicID,
		DisplayName: payload.DisplayName,
		BirthDate:   payload.BirthDate,
		Sex:         payload.Sex,
		Phone:       payload.Phone,
		Email:       payload.Email,
		Street:      payload.Street,
		City:        payload.City,
		State:       payload.State,
		PostalCode:  payload.PostalCode,
		Notes:       payload.Notes,
		Status:      rec.Status,
		CreatedBy:   rec.CreatedBy,
		CreatedAt:   rec.CreatedAt,
		UpdatedAt:   rec.UpdatedAt,
	}
}

func normalize(in PatientInput) (PatientInput, error) {
	out := PatientInput{
		DisplayName: strings.TrimSpace(in.DisplayName),
		BirthDate:   strings.TrimSpace(in.BirthDate),
		Sex:         types.Sex(strings.TrimSpace(in.Sex.String())),
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
