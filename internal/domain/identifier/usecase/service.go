package usecase

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/pkg/flow"
	"librevita.org/pkg/log"
	"librevita.org/pkg/urn"
)

type service struct {
	repo identifiermodel.IdentifierRepository
	key  *crypto.MasterKey
	reg  *identifiermodel.Registry
	log  log.Logger
}

// NewService creates a new Service implementation for identifier management.
func NewService(repo identifiermodel.IdentifierRepository, key *crypto.MasterKey, reg *identifiermodel.Registry, logger log.Logger) Service {
	return &service{repo: repo, key: key, reg: reg, log: logger}
}

func (s *service) blindIndex(ctx context.Context, system, value string) (string, error) {
	if h, ok := fle.HasherFromContext(ctx); ok {
		return h.BlindIndex(system, value)
	}
	return s.key.BlindIndex(system, value)
}

func (s *service) decryptValue(ctx context.Context, clinicID, patientID uuid.UUID, system string, ciphertext, nonce []byte) ([]byte, error) {
	pURN := urn.Patient(clinicID, patientID)
	_ = system
	return s.key.DecryptPatientData(ctx, pURN, []byte(pURN), ciphertext, nonce)
}

func (s *service) decryptValueWithDEK(clinicID, patientID uuid.UUID, dek, ciphertext, nonce []byte) ([]byte, error) {
	pURN := urn.Patient(clinicID, patientID)
	return s.key.DecryptPatientDataWithDEK(dek, []byte(pURN), ciphertext, nonce)
}

// AddIdentifier normalizes, validates, encrypts, and stores in.
func (s *service) AddIdentifier(ctx context.Context, clinicID, createdBy string, in Input) (*identifiermodel.Identifier, error) {
	cUUID, err := uuid.Parse(clinicID)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: invalid clinic id")
	}

	strategy := s.resolve(in)
	var normalized string
	var blind string
	var ciphertext []byte
	var nonce []byte
	var id uuid.UUID
	pUUID := uuid.MustParse(in.PatientID)
	pURN := urn.Patient(cUUID, pUUID)

	err = flow.New().
		Step("normalize input", func() error {
			var nerr error
			normalized, nerr = strategy.Normalize(in.Value)
			return nerr
		}).
		Step("check system allowed", func() error {
			ok, aerr := s.repo.AllowsSystem(ctx, cUUID, strategy.System())
			if aerr != nil {
				return aerr
			}
			if !ok {
				return identifiermodel.ErrSystemNotAllowed
			}
			return nil
		}).
		Step("compute blind index", func() error {
			var berr error
			blind, berr = s.blindIndex(ctx, strategy.System(), normalized)
			return berr
		}).
		Step("encrypt data", func() error {
			var eerr error
			ciphertext, nonce, eerr = s.key.EncryptPatientData(ctx, pURN, []byte(pURN), []byte(normalized))
			return eerr
		}).
		Step("generate id", func() error {
			var gerr error
			id, gerr = uuid.NewV7()
			if gerr != nil {
				return errors.Wrap(gerr, "identifier: generate id")
			}
			return nil
		}).
		Step("persist identifier", func() error {
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
			_, perr := s.repo.Add(ctx, rec)
			return perr
		}).
		Err()

	if err != nil {
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
			s.log.ErrorContext(ctx, "identifier: blind index hit failed to decrypt",
				log.String("system", row.System),
				log.Stringer("patient_id", row.PatientID),
				log.Error(err),
			)
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
			s.log.ErrorContext(ctx, "identifier: failed to decrypt",
				log.Stringer("id", row.ID),
				log.String("system", row.System),
				log.Error(err),
			)
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
	pUUIDs := parsePatientUUIDs(patientIDs)
	if len(pUUIDs) == 0 {
		return out, nil
	}

	rows, err := s.repo.ListByPatients(ctx, pUUIDs)
	if err != nil {
		return nil, errors.Wrap(err, "identifier: list by patients")
	}
	keysByClinic, cleanup, err := s.loadDEKsByClinic(ctx, rows)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	s.decryptIdentifierRows(ctx, rows, keysByClinic, out)
	return out, nil
}

func parsePatientUUIDs(patientIDs []string) []uuid.UUID {
	var pUUIDs []uuid.UUID
	for _, pid := range patientIDs {
		if u, err := uuid.Parse(pid); err == nil {
			pUUIDs = append(pUUIDs, u)
		}
	}
	return pUUIDs
}

func recordClinicID(ctx context.Context, row identifiermodel.IdentifierRecord) uuid.UUID {
	if row.ClinicID != uuid.Nil {
		return row.ClinicID
	}
	cid, _ := clinicctx.ClinicID(ctx)
	return cid
}

func (s *service) loadDEKsByClinic(ctx context.Context, rows []identifiermodel.IdentifierRecord) (map[uuid.UUID]map[uuid.UUID][]byte, func(), error) {
	keysByClinic := make(map[uuid.UUID]map[uuid.UUID][]byte)
	cleanup := func() {
		for _, deks := range keysByClinic {
			for _, dek := range deks {
				crypto.ZeroBytes(dek)
			}
		}
	}
	idsByClinic := make(map[uuid.UUID][]uuid.UUID)
	for _, row := range rows {
		cid := recordClinicID(ctx, row)
		if cid == uuid.Nil {
			continue
		}
		idsByClinic[cid] = append(idsByClinic[cid], row.PatientID)
	}
	for cid, ids := range idsByClinic {
		deks, err := s.key.GetPatientDEKsForClinic(ctx, cid, ids)
		if err != nil {
			cleanup()
			return nil, func() {}, errors.Wrap(err, "identifier: load patient deks")
		}
		keysByClinic[cid] = deks
	}
	return keysByClinic, cleanup, nil
}

func (s *service) decryptIdentifierRows(ctx context.Context, rows []identifiermodel.IdentifierRecord, keysByClinic map[uuid.UUID]map[uuid.UUID][]byte, out map[string][]string) {
	for _, row := range rows {
		cid := recordClinicID(ctx, row)
		dek, ok := keysByClinic[cid][row.PatientID]
		if !ok {
			s.log.ErrorContext(ctx, "identifier: patient DEK unavailable",
				log.Stringer("patient_id", row.PatientID),
			)
			continue
		}
		value, err := s.decryptValueWithDEK(cid, row.PatientID, dek, row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.ErrorContext(ctx, "identifier: failed to decrypt",
				log.Stringer("patient_id", row.PatientID),
				log.String("system", row.System),
				log.Error(err),
			)
			continue
		}
		patientKey := row.PatientID.String()
		out[patientKey] = append(out[patientKey], string(value))
	}
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
