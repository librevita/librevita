package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/patient"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/internal/types"
)

type patientRepository struct {
	client *ent.Client
}

// NewPatientRepository creates a patient repository adapter.
func NewPatientRepository(client *ent.Client) patientmodel.PatientRepository {
	return &patientRepository{client: client}
}

func (r *patientRepository) Create(ctx context.Context, rec patientmodel.PatientRecord) (*patientmodel.PatientRecord, error) {
	create := r.client.Patient.Create().
		SetID(rec.ID).
		SetClinicID(rec.ClinicID).
		SetBlindIndex(rec.BlindIndex).
		SetEncryptedPayload(rec.EncryptedPayload).
		SetNonce(rec.Nonce).
		SetStatus(patient.Status(rec.Status))

	if rec.CreatedBy != nil {
		create.SetCreatedBy(*rec.CreatedBy)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("patient repository: create: %w", err)
	}
	return toPatientRecord(saved), nil
}

func (r *patientRepository) Get(ctx context.Context, clinicID, patientID uuid.UUID) (*patientmodel.PatientRecord, error) {
	p, err := r.client.Patient.Query().
		Where(
			patient.IDEQ(patientID),
			patient.ClinicIDEQ(clinicID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, patientmodel.ErrNotFound
		}
		return nil, fmt.Errorf("patient repository: get: %w", err)
	}
	return toPatientRecord(p), nil
}

func (r *patientRepository) GetWithCreator(ctx context.Context, clinicID, patientID uuid.UUID) (*patientmodel.PatientRecordWithCreator, error) {
	p, err := r.client.Patient.Query().
		Where(
			patient.IDEQ(patientID),
			patient.ClinicIDEQ(clinicID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, patientmodel.ErrNotFound
		}
		return nil, fmt.Errorf("patient repository: get with creator: %w", err)
	}

	rec := toPatientRecord(p)
	var creatorName, creatorEmail *string
	if rec.CreatedBy != nil {
		if u, err := r.client.User.Get(ctx, *rec.CreatedBy); err == nil && u != nil {
			creatorName = &u.DisplayName
			creatorEmail = &u.Email
		}
	}

	return &patientmodel.PatientRecordWithCreator{
		Record:       *rec,
		CreatorName:  creatorName,
		CreatorEmail: creatorEmail,
	}, nil
}

func (r *patientRepository) Update(ctx context.Context, rec patientmodel.PatientRecord) (*patientmodel.PatientRecord, error) {
	update := r.client.Patient.UpdateOneID(rec.ID).
		SetBlindIndex(rec.BlindIndex).
		SetEncryptedPayload(rec.EncryptedPayload).
		SetNonce(rec.Nonce).
		SetStatus(patient.Status(rec.Status)).
		SetUpdatedAt(time.Now())

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, patientmodel.ErrNotFound
		}
		return nil, fmt.Errorf("patient repository: update: %w", err)
	}
	return toPatientRecord(updated), nil
}

func (r *patientRepository) BulkSetStatus(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID, status types.PatientStatus) (int, error) {
	count, err := r.client.Patient.Update().
		Where(
			patient.ClinicIDEQ(clinicID),
			patient.IDIn(patientIDs...),
		).
		SetStatus(patient.Status(status)).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("patient repository: bulk set status: %w", err)
	}
	return count, nil
}

func (r *patientRepository) ListByClinicAndStatus(ctx context.Context, clinicID uuid.UUID, status *types.PatientStatus) ([]patientmodel.PatientRecord, error) {
	query := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID))
	if status != nil {
		query = query.Where(patient.StatusEQ(patient.Status(*status)))
	}
	rows, err := query.Order(ent.Desc(patient.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("patient repository: list: %w", err)
	}

	out := make([]patientmodel.PatientRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toPatientRecord(row))
	}
	return out, nil
}

func (r *patientRepository) Count(ctx context.Context, clinicID uuid.UUID) (int, error) {
	count, err := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID)).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("patient repository: count: %w", err)
	}
	return count, nil
}

func toPatientRecord(p *ent.Patient) *patientmodel.PatientRecord {
	if p == nil {
		return nil
	}
	var createdBy *uuid.UUID
	if p.CreatedBy != nil {
		cb := *p.CreatedBy
		createdBy = &cb
	}
	return &patientmodel.PatientRecord{
		ID:               p.ID,
		ClinicID:         p.ClinicID,
		BlindIndex:       p.BlindIndex,
		EncryptedPayload: p.EncryptedPayload,
		Nonce:            p.Nonce,
		Status:           types.PatientStatus(p.Status),
		CreatedBy:        createdBy,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}
