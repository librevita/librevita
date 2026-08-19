package identifier

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"librevita.org/internal/core/crypto"
	patientmodel "librevita.org/internal/domain/patient/model"
)

// ErrDuplicate reports that the same (system, value) already exists for another patient.
var ErrDuplicate = patientmodel.ErrDuplicate

// ErrNotFound is returned when the identifier does not exist.
var ErrNotFound = patientmodel.ErrNotFound

// Input is a document as typed at reception.
type Input struct {
	PatientID string
	System    string
	Value     string
}

// Identifier is a decrypted identifier.
type Identifier struct {
	ID        string
	PatientID string
	System    string
	Value     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Service stores, searches, lists, and removes patient identifiers with field-level encryption and blind indexing.
type Service struct {
	repo IdentifierRepository
	key  *crypto.MasterKey
	reg  *Registry
	log  *slog.Logger
}

// NewService is the Fx provider.
func NewService(repo IdentifierRepository, key *crypto.MasterKey, reg *Registry, log *slog.Logger) *Service {
	return &Service{repo: repo, key: key, reg: reg, log: log}
}

// AddIdentifier normalizes, validates, encrypts, and stores in.
func (s *Service) AddIdentifier(ctx context.Context, clinicID, createdBy string, in Input) (*Identifier, error) {
	strategy := s.resolve(in)
	normalized, err := strategy.Normalize(in.Value)
	if err != nil {
		return nil, err
	}

	blind, err := s.key.BlindIndex(strategy.System(), normalized)
	if err != nil {
		return nil, err
	}
	pURN := patientURN(in.PatientID)
	ciphertext, nonce, err := s.key.EncryptPatientData(ctx, pURN, []byte(strategy.System()), []byte(normalized))
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("identifier: generate id: %w", err)
	}

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	rec := IdentifierRecord{
		ID:              id,
		PatientID:       uuid.MustParse(in.PatientID),
		System:          strategy.System(),
		ValueCiphertext: ciphertext,
		Nonce:           nonce,
		BlindIndex:      blind,
		CreatedBy:       cb,
	}

	if _, err := s.repo.Add(ctx, rec); err != nil {
		return nil, err
	}
	return &Identifier{
		ID:        id.String(),
		PatientID: in.PatientID,
		System:    strategy.System(),
		Value:     normalized,
	}, nil
}

// FindByValue searches for the patient holding raw in clinicID.
func (s *Service) FindByValue(ctx context.Context, clinicID, raw string) ([]*Identifier, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, ErrValueRequired
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("identifier: invalid clinic id: %w", err)
	}
	var found []*Identifier
	for _, strategy := range s.reg.DetectCandidates(raw) {
		normalized, err := strategy.Normalize(raw)
		if err != nil {
			continue
		}
		blind, err := s.key.BlindIndex(strategy.System(), normalized)
		if err != nil {
			return nil, err
		}
		row, err := s.repo.FindByBlindIndex(ctx, cUUID, blind)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("identifier: find by value: %w", err)
		}
		pURN := patientURN(row.PatientID.String())
		value, err := s.key.DecryptPatientData(ctx, pURN, []byte(row.System), row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: blind index hit failed to decrypt", "system", row.System)
			continue
		}
		if string(value) != normalized {
			continue
		}
		found = append(found, &Identifier{
			ID:        row.ID.String(),
			PatientID: row.PatientID.String(),
			System:    row.System,
			Value:     string(value),
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return found, nil
}

// List returns the decrypted identifiers of a patient.
func (s *Service) List(ctx context.Context, clinicID, patientID string) ([]*Identifier, error) {
	pUUID, err := uuid.Parse(patientID)
	if err != nil {
		return nil, fmt.Errorf("identifier: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, fmt.Errorf("identifier: invalid clinic id: %w", err)
	}

	exists, err := s.repo.PatientExists(ctx, cUUID, pUUID)
	if err != nil {
		return nil, fmt.Errorf("identifier: patient check: %w", err)
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := s.repo.ListByPatient(ctx, pUUID)
	if err != nil {
		return nil, fmt.Errorf("identifier: list: %w", err)
	}
	pURN := patientURN(patientID)
	out := make([]*Identifier, 0, len(rows))
	for _, row := range rows {
		value, err := s.key.DecryptPatientData(ctx, pURN, []byte(row.System), row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: failed to decrypt", "id", row.ID, "system", row.System, "error", err)
			continue
		}
		out = append(out, &Identifier{
			ID:        row.ID.String(),
			PatientID: row.PatientID.String(),
			System:    row.System,
			Value:     string(value),
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return out, nil
}

// ListByPatients returns the decrypted values of a page of patients.
func (s *Service) ListByPatients(ctx context.Context, patientIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(patientIDs))
	if len(patientIDs) == 0 {
		return out, nil
	}
	var pUUIDs []uuid.UUID
	for _, pid := range patientIDs {
		if u, err := uuid.Parse(pid); err == nil {
			pUUIDs = append(pUUIDs, u)
		}
	}
	if len(pUUIDs) == 0 {
		return out, nil
	}

	rows, err := s.repo.ListByPatients(ctx, pUUIDs)
	if err != nil {
		return nil, fmt.Errorf("identifier: list by patients: %w", err)
	}
	for _, row := range rows {
		pURN := patientURN(row.PatientID.String())
		value, err := s.key.DecryptPatientData(ctx, pURN, []byte(row.System), row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: failed to decrypt", "patient_id", row.PatientID, "system", row.System, "error", err)
			continue
		}
		patientKey := row.PatientID.String()
		out[patientKey] = append(out[patientKey], string(value))
	}
	return out, nil
}

func patientURN(patientID string) string {
	return "urn:librevita:patient:" + patientID
}

// Remove deletes one identifier of the patient, scoped to the clinic.
func (s *Service) Remove(ctx context.Context, clinicID, patientID, identifierID string) error {
	pUUID, err := uuid.Parse(patientID)
	if err != nil {
		return fmt.Errorf("identifier: invalid patient id: %w", err)
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return fmt.Errorf("identifier: invalid clinic id: %w", err)
	}
	idUUID, err := uuid.Parse(identifierID)
	if err != nil {
		return fmt.Errorf("identifier: invalid identifier id: %w", err)
	}

	exists, err := s.repo.PatientExists(ctx, cUUID, pUUID)
	if err != nil {
		return fmt.Errorf("identifier: patient check: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	return s.repo.Remove(ctx, pUUID, idUUID)
}

func (s *Service) resolve(in Input) Strategy {
	if in.System != "" {
		return s.reg.ForSystem(in.System)
	}
	return s.reg.Detect(in.Value)
}

// ValidateValue normalizes raw under the given system and returns the system URN actually used.
func (s *Service) ValidateValue(system, raw string) (string, error) {
	strategy := s.resolve(Input{System: system, Value: raw})
	if _, err := strategy.Normalize(raw); err != nil {
		return "", err
	}
	return strategy.System(), nil
}
