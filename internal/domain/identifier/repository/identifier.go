package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/patient"
	"librevita.org/ent/patientidentifier"
	identifiermodel "librevita.org/internal/domain/identifier/model"
)

type identifierRepository struct {
	client *ent.Client
}

// NewIdentifierRepository creates an identifier repository adapter.
func NewIdentifierRepository(client *ent.Client) identifiermodel.IdentifierRepository {
	return &identifierRepository{client: client}
}

func (r *identifierRepository) Add(ctx context.Context, rec identifiermodel.IdentifierRecord) (*identifiermodel.IdentifierRecord, error) {
	create := r.client.PatientIdentifier.Create().
		SetID(rec.ID).
		SetPatientID(rec.PatientID).
		SetSystem(rec.System).
		SetValueCiphertext(rec.ValueCiphertext).
		SetNonce(rec.Nonce).
		SetBlindIndex(rec.BlindIndex)
	if rec.CreatedBy != nil {
		create.SetCreatedBy(*rec.CreatedBy)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, identifiermodel.ErrDuplicate
		}
		return nil, fmt.Errorf("identifier repository: add: %w", err)
	}
	return toIdentifierRecordDomain(saved), nil
}

func (r *identifierRepository) FindByBlindIndex(ctx context.Context, clinicID uuid.UUID, blindIndex string) (*identifiermodel.IdentifierRecord, error) {
	row, err := r.client.PatientIdentifier.Query().
		Where(
			patientidentifier.BlindIndexEQ(blindIndex),
			patientidentifier.HasPatientWith(patient.ClinicIDEQ(clinicID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, identifiermodel.ErrNotFound
		}
		return nil, fmt.Errorf("identifier repository: find by blind index: %w", err)
	}
	return toIdentifierRecordDomain(row), nil
}

func (r *identifierRepository) ListByPatient(ctx context.Context, patientID uuid.UUID) ([]identifiermodel.IdentifierRecord, error) {
	rows, err := r.client.PatientIdentifier.Query().
		Where(patientidentifier.PatientIDEQ(patientID)).
		Order(ent.Asc(patientidentifier.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("identifier repository: list by patient: %w", err)
	}
	out := make([]identifiermodel.IdentifierRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toIdentifierRecordDomain(row))
	}
	return out, nil
}

func (r *identifierRepository) ListByPatients(ctx context.Context, patientIDs []uuid.UUID) ([]identifiermodel.IdentifierRecord, error) {
	if len(patientIDs) == 0 {
		return nil, nil
	}
	rows, err := r.client.PatientIdentifier.Query().
		Where(patientidentifier.PatientIDIn(patientIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("identifier repository: list by patients: %w", err)
	}
	out := make([]identifiermodel.IdentifierRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toIdentifierRecordDomain(row))
	}
	return out, nil
}

func (r *identifierRepository) Remove(ctx context.Context, patientID, identifierID uuid.UUID) error {
	count, err := r.client.PatientIdentifier.Delete().
		Where(
			patientidentifier.IDEQ(identifierID),
			patientidentifier.PatientIDEQ(patientID),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("identifier repository: remove: %w", err)
	}
	if count == 0 {
		return identifiermodel.ErrNotFound
	}
	return nil
}

func (r *identifierRepository) PatientExists(ctx context.Context, clinicID, patientID uuid.UUID) (bool, error) {
	return r.client.Patient.Query().
		Where(
			patient.IDEQ(patientID),
			patient.ClinicIDEQ(clinicID),
		).
		Exist(ctx)
}

func toIdentifierRecordDomain(row *ent.PatientIdentifier) *identifiermodel.IdentifierRecord {
	if row == nil {
		return nil
	}
	return &identifiermodel.IdentifierRecord{
		ID:              row.ID,
		PatientID:       row.PatientID,
		System:          row.System,
		ValueCiphertext: row.ValueCiphertext,
		Nonce:           row.Nonce,
		BlindIndex:      row.BlindIndex,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
