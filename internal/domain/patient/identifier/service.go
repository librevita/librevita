package identifier

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/types"
)

// ErrDuplicate reports that the same (system, value) already exists
// for another patient.
var ErrDuplicate = errors.New("identifier: duplicate value for system")

// ErrNotFound is returned when the identifier does not exist.
var ErrNotFound = errors.New("identifier: not found")

// Input is a document as typed at reception. System is optional: when
// empty the registry detects it from the value's shape. The detected
// system is what gets persisted, so search and registration are
// symmetric.
type Input struct {
	PatientID string
	System    string
	Value     string
}

// Identifier is a decrypted identifier. Value is the plaintext and
// must never be persisted, logged, or audited.
type Identifier struct {
	ID        string
	PatientID string
	System    string
	Value     string
	CreatedBy string
	CreatedAt types.DateTime
	UpdatedAt types.DateTime
}

// Service stores, searches, lists, and removes patient identifiers
// with field-level encryption and blind indexing.
type Service struct {
	db  *sql.DB
	q   *repository.Queries
	key *crypto.MasterKey
	reg *Registry
	log *slog.Logger
}

// NewService is the Fx provider.
func NewService(db *sql.DB, key *crypto.MasterKey, reg *Registry, log *slog.Logger) *Service {
	return &Service{db: db, q: repository.New(db), key: key, reg: reg, log: log}
}

// AddIdentifier normalizes, validates, encrypts, and stores in. The
// same (system, value) registered twice for different patients is
// rejected with ErrDuplicate: a CPF belongs to exactly one patient
// anywhere in the deployment.
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
	ciphertext, nonce, err := s.key.Seal([]byte(strategy.System()), []byte(normalized))
	if err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("identifier: generate id: %w", err)
	}
	if err := s.q.CreatePatientIdentifier(ctx, repository.CreatePatientIdentifierParams{
		ID:              id,
		PatientID:       uuid.MustParse(in.PatientID),
		System:          strategy.System(),
		ValueCiphertext: ciphertext,
		Nonce:           nonce,
		BlindIndex:      blind,
		CreatedBy:       uuidOrNil(createdBy),
	}); err != nil {
		if isUniqueViolation(err) {
			return nil, ErrDuplicate
		}
		return nil, fmt.Errorf("identifier: create: %w", err)
	}
	return &Identifier{
		ID:        id.String(),
		PatientID: in.PatientID,
		System:    strategy.System(),
		Value:     normalized,
	}, nil
}

// FindByValue searches for the patient holding raw in clinicID. Every
// active system whose shape matches raw is tried, most specific first;
// the raw fallback runs last, so a value that matches no configured
// system still finds its document. Each candidate decrypts and
// verifies the plaintext before accepting the match.
func (s *Service) FindByValue(ctx context.Context, clinicID, raw string) ([]*Identifier, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, ErrValueRequired
	}
	var found []*Identifier
	for _, strategy := range s.reg.DetectCandidates(raw) {
		normalized, err := strategy.Normalize(raw)
		if err != nil {
			// The shape matched but the value failed the system's own
			// rules; the value may still belong to another system.
			continue
		}
		blind, err := s.key.BlindIndex(strategy.System(), normalized)
		if err != nil {
			return nil, err
		}
		row, err := s.q.FindPatientByBlindIndex(ctx, repository.FindPatientByBlindIndexParams{
			BlindIndex: blind, ClinicID: uuid.MustParse(clinicID),
		})
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("identifier: find by value: %w", err)
		}
		value, err := s.key.Open([]byte(row.System), row.ValueCiphertext, row.Nonce)
		if err != nil {
			// Defense in depth: a blind index hit must decrypt back to
			// the expected value, otherwise the row is corrupted.
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

// List returns the decrypted identifiers of a patient, ordered by
// system. The patient is scoped to the clinic.
func (s *Service) List(ctx context.Context, clinicID, patientID string) ([]*Identifier, error) {
	if _, err := s.q.GetPatientByID(ctx, repository.GetPatientByIDParams{
		ID: uuid.MustParse(patientID), ClinicID: uuid.MustParse(clinicID),
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("identifier: patient: %w", err)
	}
	rows, err := s.q.FindIdentifiersByPatient(ctx, uuid.MustParse(patientID))
	if err != nil {
		return nil, fmt.Errorf("identifier: list: %w", err)
	}
	out := make([]*Identifier, 0, len(rows))
	for _, row := range rows {
		value, err := s.key.Open([]byte(row.System), row.ValueCiphertext, row.Nonce)
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

// ListByPatients returns the decrypted values of a page of patients,
// keyed by patient id, in a single query (see
// ListIdentifiersByPatients). Used by the registry list to render the
// documents column (masked) without per-row round trips. The patient
// ids come from the already clinic-scoped page query.
func (s *Service) ListByPatients(ctx context.Context, patientIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(patientIDs))
	if len(patientIDs) == 0 {
		return out, nil
	}
	idsJSON, err := json.Marshal(patientIDs)
	if err != nil {
		return nil, fmt.Errorf("identifier: list by patients: %w", err)
	}
	rows, err := s.q.ListIdentifiersByPatients(ctx, string(idsJSON))
	if err != nil {
		return nil, fmt.Errorf("identifier: list by patients: %w", err)
	}
	for _, row := range rows {
		value, err := s.key.Open([]byte(row.System), row.ValueCiphertext, row.Nonce)
		if err != nil {
			s.log.Error("identifier: failed to decrypt", "patient_id", row.PatientID, "system", row.System, "error", err)
			continue
		}
		patient := row.PatientID.String()
		out[patient] = append(out[patient], string(value))
	}
	return out, nil
}

// Remove deletes one identifier of the patient, scoped to the clinic.
// An id that does not belong to the patient is ErrNotFound, never a
// silent no-op.
func (s *Service) Remove(ctx context.Context, clinicID, patientID, identifierID string) error {
	if _, err := s.q.GetPatientByID(ctx, repository.GetPatientByIDParams{
		ID: uuid.MustParse(patientID), ClinicID: uuid.MustParse(clinicID),
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("identifier: patient: %w", err)
	}
	res, err := s.q.DeletePatientIdentifier(ctx, repository.DeletePatientIdentifierParams{
		ID: uuid.MustParse(identifierID), PatientID: uuid.MustParse(patientID),
	})
	if err != nil {
		return fmt.Errorf("identifier: delete: %w", err)
	}
	if affected, err := res.RowsAffected(); err != nil || affected == 0 {
		return ErrNotFound
	}
	return nil
}

// resolve picks the strategy for in: the configured system when given
// (unknown systems fall back to raw), otherwise the detected one.
func (s *Service) resolve(in Input) Strategy {
	if in.System != "" {
		return s.reg.ForSystem(in.System)
	}
	return s.reg.Detect(in.Value)
}

// ValidateValue normalizes raw under the given system (empty detects
// it by shape) and returns the system URN actually used. It does not
// touch the database; callers use it to validate a document before
// creating the patient that will hold it.
func (s *Service) ValidateValue(system, raw string) (string, error) {
	strategy := s.resolve(Input{System: system, Value: raw})
	if _, err := strategy.Normalize(raw); err != nil {
		return "", err
	}
	return strategy.System(), nil
}

// isUniqueViolation reports whether err is a UNIQUE constraint
// failure (the blind_index collision that maps to ErrDuplicate). It
// recognizes the backends the application can run on:
//
//   - the SQLite driver (modernc) returns an error with Code() 2067
//     (SQLITE_CONSTRAINT_UNIQUE);
//   - the dqlite driver reports the extended code 2067 on the write
//     path and a plain "UNIQUE constraint failed" message otherwise;
//   - the rqlite driver normalizes to the same Code() interface.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	type coded interface{ Code() int }
	var withCode coded
	if errors.As(err, &withCode) {
		return withCode.Code() == 2067
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
