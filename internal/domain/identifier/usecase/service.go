package usecase

import (
	"context"
	"log/slog"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	identifiermodel "librevita.org/internal/domain/identifier/model"
)

type service struct {
	repo identifiermodel.IdentifierRepository
	key  *crypto.MasterKey
	reg  *identifiermodel.Registry
	log  *slog.Logger
}

// NewService creates a new Service implementation for identifier management.
func NewService(repo identifiermodel.IdentifierRepository, key *crypto.MasterKey, reg *identifiermodel.Registry, log *slog.Logger) Service {
	return &service{repo: repo, key: key, reg: reg, log: log}
}

func (s *service) blindIndex(ctx context.Context, system, value string) (string, error) {
	if h, ok := fle.HasherFromContext(ctx); ok {
		return h.BlindIndex(system, value)
	}
	return s.key.BlindIndex(system, value)
}

func (s *service) decryptValue(ctx context.Context, clinicID, patientID uuid.UUID, system string, ciphertext, nonce []byte) ([]byte, error) {
	pURN := crypto.PatientURN(clinicID, patientID)
	_ = system
	return s.key.DecryptPatientData(ctx, pURN, []byte(pURN), ciphertext, nonce)
}

func (s *service) decryptValueWithDEK(clinicID, patientID uuid.UUID, dek, ciphertext, nonce []byte) ([]byte, error) {
	pURN := crypto.PatientURN(clinicID, patientID)
	return s.key.DecryptPatientDataWithDEK(dek, []byte(pURN), ciphertext, nonce)
}

// AddIdentifier normalizes, validates, encrypts, and stores in.
func (s *service) AddIdentifier(ctx context.Context, clinicID, createdBy string, in Input) (*identifiermodel.Identifier, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid clinic id")
	}
	strategy := s.resolve(in)
	normalized, err := strategy.Normalize(in.Value)
	if err != nil {
		return nil, err
	}

	ok, err := s.repo.AllowsSystem(ctx, cUUID, strategy.System())
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, identifiermodel.ErrSystemNotAllowed
	}

	blind, err := s.blindIndex(ctx, strategy.System(), normalized)
	if err != nil {
		return nil, err
	}
	pUUID := uuid.MustParse(in.PatientID)
	pURN := crypto.PatientURN(cUUID, pUUID)
	ciphertext, nonce, err := s.key.EncryptPatientData(ctx, pURN, []byte(pURN), []byte(normalized))
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, errors.Wrap(err, "identifier: generate id")
	}

	var cb *uuid.UUID
	if createdBy != "" {
		parsed := uuid.MustParse(createdBy)
		cb = &parsed
	}

	rec := identifiermodel.IdentifierRecord{
		ID:              id,
		ClinicID:        cUUID,
		PatientID:       pUUID,
		System:          strategy.System(),
		ValueCiphertext: ciphertext,
		Nonce:           nonce,
		BlindIndex:      blind,
		CreatedBy:       cb,
	}

	if _, err := s.repo.Add(ctx, rec); err != nil {
		return nil, err
	}
	return &identifiermodel.Identifier{
		ID:        id.String(),
		PatientID: in.PatientID,
		System:    strategy.System(),
		Value:     normalized,
	}, nil
}

// FindByValue searches for the patient holding raw in clinicID.
func (s *service) FindByValue(ctx context.Context, clinicID, raw string) ([]*identifiermodel.Identifier, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, ErrValueRequired
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid clinic id")
	}
	var found []*identifiermodel.Identifier
	for _, strategy := range s.reg.DetectCandidates(raw) {
		normalized, err := strategy.Normalize(raw)
		if err != nil {
			continue
		}
		blind, err := s.blindIndex(ctx, strategy.System(), normalized)
		if err != nil {
			return nil, err
		}
		row, err := s.repo.FindByBlindIndex(ctx, cUUID, blind)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Wrap(err, "identifier: find by value")
		}
		value, err := s.decryptValue(ctx, cUUID, row.PatientID, row.System, row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: blind index hit failed to decrypt", "system", row.System)
			continue
		}
		if string(value) != normalized {
			continue
		}
		found = append(found, &identifiermodel.Identifier{
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
func (s *service) List(ctx context.Context, clinicID, patientID string) ([]*identifiermodel.Identifier, error) {
	pUUID, err := uuid.Parse(patientID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid patient id")
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid clinic id")
	}

	exists, err := s.repo.PatientExists(ctx, cUUID, pUUID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: patient check")
	}
	if !exists {
		return nil, ErrNotFound
	}

	rows, err := s.repo.ListByPatient(ctx, pUUID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: list")
	}
	out := make([]*identifiermodel.Identifier, 0, len(rows))
	deks, err := s.key.GetPatientDEKsForClinic(ctx, cUUID, []uuid.UUID{pUUID})
	if err != nil {
		if errors.Is(err, crypto.ErrKeyNotFound) || errors.Is(err, crypto.ErrKeyDestroyed) {
			return out, nil
		}
		return nil, errors.Wrap(err, "identifier: load patient dek")
	}
	defer func() {
		for _, dek := range deks {
			crypto.ZeroBytes(dek)
		}
	}()
	dek, ok := deks[pUUID]
	if !ok {
		return out, nil
	}
	for _, row := range rows {
		cid := row.ClinicID
		if cid == uuid.Nil {
			cid = cUUID
		}
		value, err := s.decryptValueWithDEK(cid, row.PatientID, dek, row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: failed to decrypt", "id", row.ID, "system", row.System, "error", err)
			continue
		}
		out = append(out, &identifiermodel.Identifier{
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
func (s *service) ListByPatients(ctx context.Context, patientIDs []string) (map[string][]string, error) {
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
		return nil, errors.Wrap(err, "identifier: list by patients")
	}
	keysByClinic := make(map[uuid.UUID]map[uuid.UUID][]byte)
	idsByClinic := make(map[uuid.UUID][]uuid.UUID)
	for _, row := range rows {
		cid := row.ClinicID
		if cid == uuid.Nil {
			cid, _ = clinicctx.ClinicID(ctx)
		}
		if cid == uuid.Nil {
			continue
		}
		idsByClinic[cid] = append(idsByClinic[cid], row.PatientID)
	}
	for cid, ids := range idsByClinic {
		deks, err := s.key.GetPatientDEKsForClinic(ctx, cid, ids)
		if err != nil {
			return nil, errors.Wrap(err, "identifier: load patient deks")
		}
		keysByClinic[cid] = deks
		defer func(deks map[uuid.UUID][]byte) {
			for _, dek := range deks {
				crypto.ZeroBytes(dek)
			}
		}(deks)
	}
	for _, row := range rows {
		cid := row.ClinicID
		if cid == uuid.Nil {
			cid, _ = clinicctx.ClinicID(ctx)
		}
		dek, ok := keysByClinic[cid][row.PatientID]
		if !ok {
			s.log.Error("identifier: patient DEK unavailable", "patient_id", row.PatientID)
			continue
		}
		value, err := s.decryptValueWithDEK(cid, row.PatientID, dek, row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: failed to decrypt", "patient_id", row.PatientID, "system", row.System, "error", err)
			continue
		}
		patientKey := row.PatientID.String()
		out[patientKey] = append(out[patientKey], string(value))
	}
	return out, nil
}

// Remove deletes one identifier of the patient, scoped to the clinic.
func (s *service) Remove(ctx context.Context, clinicID, patientID, identifierID string) error {
	pUUID, err := uuid.Parse(patientID)
	if err != nil {
		return errors.Wrap(err, "identifier: invalid patient id")
	}
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return errors.Wrap(err, "identifier: invalid clinic id")
	}
	idUUID, err := uuid.Parse(identifierID)
	if err != nil {
		return errors.Wrap(err, "identifier: invalid identifier id")
	}

	exists, err := s.repo.PatientExists(ctx, cUUID, pUUID)
	if err != nil {
		return errors.Wrap(err, "identifier: patient check")
	}
	if !exists {
		return ErrNotFound
	}

	return s.repo.Remove(ctx, pUUID, idUUID)
}

func (s *service) resolve(in Input) identifiermodel.Strategy {
	if in.System != "" {
		return s.reg.ForSystem(in.System)
	}
	return s.reg.Detect(in.Value)
}

// ValidateValue normalizes raw under the given system and returns the system URN actually used.
func (s *service) ValidateValue(system, raw string) (string, error) {
	strategy := s.resolve(Input{System: system, Value: raw})
	if _, err := strategy.Normalize(raw); err != nil {
		return "", err
	}
	return strategy.System(), nil
}
