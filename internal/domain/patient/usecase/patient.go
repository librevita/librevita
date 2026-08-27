// Package usecase holds the patient domain service: validation and
// persistence of patient records with transparent Zero-Knowledge encryption.
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	normalizer "librevita.org/internal/core/normalize"
	"librevita.org/internal/core/policy"
	patientmodel "librevita.org/internal/domain/patient/model"
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

// Re-export domain models and contracts from patient/model.
type (
	Patient                  = patientmodel.Patient
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
	log      *slog.Logger
	policies *policy.PolicyEngine
	engine   *crypto.Engine
}

// NewService is the Fx provider.
func NewService(repo PatientRepository, log *slog.Logger, policies *policy.PolicyEngine, engine *crypto.Engine) *Service {
	return &Service{
		repo:     repo,
		log:      log,
		policies: policies,
		engine:   engine,
	}
}

// AuthorizePatientEdit evaluates the fine-grained patient.edit policy.
func (s *Service) AuthorizePatientEdit(ctx context.Context, principal *auth.Principal, id string, createdBy *string, status patientmodel.PatientStatus) error {
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

// Create validates in and inserts a patient domain entity.
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

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	p := Patient{
		ID:          id,
		ClinicID:    cUUID,
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
		Status:      patientmodel.PatientStatusActive,
		CreatedBy:   cb,
	}

	if s.engine != nil {
		if _, err := s.engine.EnsurePatientDEKForClinic(ctx, cUUID, id); err != nil {
			return nil, fmt.Errorf("usecase: provision patient dek: %w", err)
		}
	}
	created, err := s.repo.Create(ctx, p)
	if err != nil && s.engine != nil {
		_ = s.engine.DeletePatientDEKForClinic(ctx, cUUID, id)
	}
	return created, err
}

// Get returns a patient by id, scoped to the clinic.
func (s *Service) Get(ctx context.Context, clinicID, id string) (*Patient, error) {
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	return s.repo.Get(ctx, cUUID, pUUID)
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

	return s.repo.GetWithCreator(ctx, cUUID, pUUID)
}

// GetMany returns patients from one clinic, preserving the order of ids.
func (s *Service) GetMany(ctx context.Context, clinicID string, ids []string) ([]Patient, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}
	patientIDs := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		pUUID, err := uuid.Parse(id)
		if err != nil {
			return nil, fmt.Errorf("usecase: invalid patient id: %w", err)
		}
		patientIDs = append(patientIDs, pUUID)
	}
	if optimized, ok := s.repo.(patientmodel.PatientQueryRepository); ok {
		patients, err := optimized.GetMany(ctx, cUUID, patientIDs)
		if err != nil {
			return nil, err
		}
		byID := make(map[uuid.UUID]Patient, len(patients))
		for _, patient := range patients {
			byID[patient.ID] = patient
		}
		out := make([]Patient, 0, len(patientIDs))
		for _, id := range patientIDs {
			if patient, ok := byID[id]; ok {
				out = append(out, patient)
			}
		}
		return out, nil
	}
	out := make([]Patient, 0, len(patientIDs))
	for _, id := range patientIDs {
		patient, err := s.repo.Get(ctx, cUUID, id)
		if err != nil {
			return nil, err
		}
		if patient != nil {
			out = append(out, *patient)
		}
	}
	return out, nil
}

// Delete removes a patient aggregate after the caller has completed its
// crypto-shredding step. The concrete repository must support aggregate
// deletion in production.
func (s *Service) Delete(ctx context.Context, clinicID, id string) error {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return fmt.Errorf("usecase: invalid clinic id: %w", err)
	}
	pUUID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("usecase: invalid patient id: %w", err)
	}
	repo, ok := s.repo.(patientmodel.PatientDeletionRepository)
	if !ok {
		return errors.New("usecase: patient deletion repository is unavailable")
	}
	return repo.DeleteAggregate(ctx, cUUID, pUUID)
}

// Update validates in and updates the patient.
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

	existing, err := s.repo.Get(ctx, cUUID, pUUID)
	if err != nil {
		return nil, err
	}

	p := Patient{
		ID:          pUUID,
		ClinicID:    cUUID,
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
		Status:      existing.Status,
		CreatedBy:   existing.CreatedBy,
	}

	return s.repo.Update(ctx, p)
}

// SetStatus updates the patient status, scoped to the clinic.
func (s *Service) SetStatus(ctx context.Context, clinicID, id string, status patientmodel.PatientStatus) error {
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

// BulkSetStatus updates the status for multiple patients in a clinic.
func (s *Service) BulkSetStatus(ctx context.Context, clinicID string, ids []string, status patientmodel.PatientStatus) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return 0, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}
	uuids := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		u, err := uuid.Parse(id)
		if err != nil {
			return 0, fmt.Errorf("usecase: invalid patient id %q: %w", id, err)
		}
		uuids = append(uuids, u)
	}
	return s.repo.BulkSetStatus(ctx, cUUID, uuids, status)
}

// List returns decrypted patients matching clinic and status filters with pagination.
func (s *Service) List(ctx context.Context, clinicID, q, status, field string, limit, offset int) ([]Patient, int64, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: invalid clinic id: %w", err)
	}

	var statusFilter *patientmodel.PatientStatus
	if status != "" {
		st := patientmodel.PatientStatus(status)
		statusFilter = &st
	}

	if optimized, ok := s.repo.(patientmodel.PatientQueryRepository); ok {
		return s.listOptimized(ctx, optimized, cUUID, strings.TrimSpace(q), field, statusFilter, limit, offset)
	}

	patients, err := s.repo.ListByClinicAndStatus(ctx, cUUID, statusFilter)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list patients: %w", err)
	}

	total := len(patients)
	filtered := make([]Patient, 0, total)
	trimmedQ := strings.ToLower(strings.TrimSpace(q))

	for _, pt := range patients {
		if trimmedQ != "" {
			matchesName := matchWordPrefix(pt.DisplayName, trimmedQ)
			matchesEmail := pt.Email != nil && strings.HasPrefix(strings.ToLower(*pt.Email), trimmedQ)

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
		filtered = append(filtered, pt)
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

func (s *Service) listOptimized(
	ctx context.Context,
	repo patientmodel.PatientQueryRepository,
	clinicID uuid.UUID,
	q, field string,
	status *patientmodel.PatientStatus,
	limit, offset int,
) ([]Patient, int64, error) {
	var nameTokens []string
	var emailBlindIndex string
	if q != "" {
		hasher := fle.ResolveHasher(ctx, nil)
		if hasher == nil {
			return nil, 0, fmt.Errorf("usecase: clinic hasher is required for patient search")
		}
		searchEmail := field == "email" || (field == "" && strings.Contains(q, "@"))
		if searchEmail {
			var err error
			emailBlindIndex, err = hasher.BlindIndex("patient.email", normalizer.Email(q))
			if err != nil {
				return nil, 0, fmt.Errorf("usecase: hash patient email search: %w", err)
			}
		} else {
			for _, word := range strings.Fields(normalizer.Text(q)) {
				tokens := normalizer.NameTokens(word)
				if len(tokens) == 0 {
					continue
				}
				token := tokens[len(tokens)-1]
				hash, err := hasher.BlindIndex("patient.token", token)
				if err != nil {
					return nil, 0, fmt.Errorf("usecase: hash patient name search: %w", err)
				}
				nameTokens = append(nameTokens, hash)
			}
			if len(nameTokens) == 0 {
				return []Patient{}, 0, nil
			}
		}
	}

	candidates, total, err := repo.ListCandidates(ctx, clinicID, status, nameTokens, emailBlindIndex, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: list patient candidates: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.ID)
	}
	hydrated, err := repo.GetMany(ctx, clinicID, ids)
	if err != nil {
		return nil, 0, fmt.Errorf("usecase: hydrate patients: %w", err)
	}
	byID := make(map[uuid.UUID]Patient, len(hydrated))
	for _, patient := range hydrated {
		byID[patient.ID] = patient
	}
	out := make([]Patient, 0, len(candidates))
	for _, candidate := range candidates {
		if patient, ok := byID[candidate.ID]; ok {
			out = append(out, patient)
		}
	}
	return out, int64(total), nil
}

// ListPage returns one page of decrypted patients.
func (s *Service) ListPage(ctx context.Context, clinicID, q, field, status string, limit, offset int) ([]Patient, int64, error) {
	return s.List(ctx, clinicID, q, status, field, limit, offset)
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

func normalize(in PatientInput) (PatientInput, error) {
	out := PatientInput{
		DisplayName: strings.TrimSpace(in.DisplayName),
		BirthDate:   strings.TrimSpace(in.BirthDate),
		Sex:         patientmodel.Sex(strings.TrimSpace(in.Sex.String())),
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
	if out.Phone == "" {
		return out, &ValidationError{Msg: "patient phone is required"}
	}
	if len(out.Phone) > maxPhoneLen {
		return out, &ValidationError{Msg: "phone is too long"}
	}
	if out.Email == "" {
		return out, &ValidationError{Msg: "patient email is required"}
	}
	addr, err := mail.ParseAddress(out.Email)
	if err != nil || addr.Address != out.Email {
		return out, &ValidationError{Msg: "enter a valid email address"}
	}
	if out.Sex == "" {
		out.Sex = patientmodel.SexUnknown
	}
	if !out.Sex.Valid() {
		return out, &ValidationError{Msg: "invalid sex"}
	}
	if out.BirthDate != "" {
		if _, err := time.Parse("2006-01-02", out.BirthDate); err != nil {
			return out, &ValidationError{Msg: "enter a valid birth date (YYYY-MM-DD)"}
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
