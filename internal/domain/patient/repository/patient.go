package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/patient"
	patientmodel "librevita.org/internal/domain/patient/model"
)

type patientRepository struct {
	client *ent.Client
}

// NewPatientRepository creates a pure patient repository adapter.
func NewPatientRepository(client *ent.Client) patientmodel.PatientRepository {
	return &patientRepository{
		client: client,
	}
}

func (r *patientRepository) Create(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
	create := r.client.Patient.Create().
		SetID(p.ID).
		SetClinicID(p.ClinicID).
		SetDisplayName(p.DisplayName).
		SetStatus(patient.Status(p.Status))

	if p.BirthDate != nil && *p.BirthDate != "" {
		create.SetBirthDate(*p.BirthDate)
	}
	if p.Sex != "" {
		create.SetSex(string(p.Sex))
	}
	if p.Phone != nil && *p.Phone != "" {
		create.SetPhone(*p.Phone)
	}
	if p.Email != nil && *p.Email != "" {
		create.SetEmail(*p.Email)
	}
	if p.Street != nil && *p.Street != "" {
		create.SetStreet(*p.Street)
	}
	if p.City != nil && *p.City != "" {
		create.SetCity(*p.City)
	}
	if p.State != nil && *p.State != "" {
		create.SetState(*p.State)
	}
	if p.PostalCode != nil && *p.PostalCode != "" {
		create.SetPostalCode(*p.PostalCode)
	}
	if p.Notes != nil && *p.Notes != "" {
		create.SetNotes(*p.Notes)
	}
	if p.CreatedBy != nil {
		create.SetCreatedBy(*p.CreatedBy)
	}

	saved, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("patient repository: create: %w", err)
	}
	return toDomainPatient(saved), nil
}

func (r *patientRepository) Get(ctx context.Context, clinicID, patientID uuid.UUID) (*patientmodel.Patient, error) {
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
	return toDomainPatient(p), nil
}

func (r *patientRepository) GetWithCreator(ctx context.Context, clinicID, patientID uuid.UUID) (*patientmodel.GetPatientWithCreatorRow, error) {
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

	pt := toDomainPatient(p)
	var creatorName, creatorEmail *string
	if pt.CreatedBy != nil {
		if u, err := r.client.User.Get(ctx, *pt.CreatedBy); err == nil && u != nil {
			creatorName = &u.DisplayName
			creatorEmail = &u.Email
		}
	}

	return &patientmodel.GetPatientWithCreatorRow{
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
		CreatorEmail: creatorEmail,
		CreatorName:  creatorName,
	}, nil
}

func (r *patientRepository) Update(ctx context.Context, p patientmodel.Patient) (*patientmodel.Patient, error) {
	update := r.client.Patient.UpdateOneID(p.ID).
		SetDisplayName(p.DisplayName).
		SetStatus(patient.Status(p.Status)).
		SetUpdatedAt(time.Now())

	if p.BirthDate != nil && *p.BirthDate != "" {
		update.SetBirthDate(*p.BirthDate)
	} else {
		update.ClearBirthDate()
	}

	if p.Sex != "" {
		update.SetSex(string(p.Sex))
	} else {
		update.ClearSex()
	}

	if p.Phone != nil && *p.Phone != "" {
		update.SetPhone(*p.Phone)
	}

	if p.Email != nil && *p.Email != "" {
		update.SetEmail(*p.Email)
	}

	if p.Street != nil && *p.Street != "" {
		update.SetStreet(*p.Street)
	} else {
		update.ClearStreet()
	}

	if p.City != nil && *p.City != "" {
		update.SetCity(*p.City)
	} else {
		update.ClearCity()
	}

	if p.State != nil && *p.State != "" {
		update.SetState(*p.State)
	} else {
		update.ClearState()
	}

	if p.PostalCode != nil && *p.PostalCode != "" {
		update.SetPostalCode(*p.PostalCode)
	} else {
		update.ClearPostalCode()
	}

	if p.Notes != nil && *p.Notes != "" {
		update.SetNotes(*p.Notes)
	} else {
		update.ClearNotes()
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, patientmodel.ErrNotFound
		}
		return nil, fmt.Errorf("patient repository: update: %w", err)
	}
	return toDomainPatient(updated), nil
}

func (r *patientRepository) BulkSetStatus(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID, status patientmodel.PatientStatus) (int, error) {
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

func (r *patientRepository) ListByClinicAndStatus(ctx context.Context, clinicID uuid.UUID, status *patientmodel.PatientStatus) ([]patientmodel.Patient, error) {
	query := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID))
	if status != nil {
		query = query.Where(patient.StatusEQ(patient.Status(*status)))
	}
	rows, err := query.Order(ent.Desc(patient.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("patient repository: list: %w", err)
	}

	out := make([]patientmodel.Patient, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toDomainPatient(row))
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

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toDomainPatient(p *ent.Patient) *patientmodel.Patient {
	if p == nil {
		return nil
	}

	var sex patientmodel.Sex = patientmodel.SexUnknown
	if p.Sex != "" {
		sex = patientmodel.Sex(p.Sex)
	}

	return &patientmodel.Patient{
		ID:          p.ID,
		ClinicID:    p.ClinicID,
		DisplayName: p.DisplayName,
		BirthDate:   stringPtr(p.BirthDate),
		Sex:         sex,
		Phone:       stringPtr(p.Phone),
		Email:       stringPtr(p.Email),
		Street:      stringPtr(p.Street),
		City:        stringPtr(p.City),
		State:       stringPtr(p.State),
		PostalCode:  stringPtr(p.PostalCode),
		Notes:       stringPtr(p.Notes),
		Status:      patientmodel.PatientStatus(p.Status),
		CreatedBy:   p.CreatedBy,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}
