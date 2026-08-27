package repository

import (
	"context"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"

	"librevita.org/ent"
	"librevita.org/ent/appointment"
	"librevita.org/ent/episode"
	"librevita.org/ent/patient"
	"librevita.org/ent/patientidentifier"
	"librevita.org/internal/core/crypto"
	patientmodel "librevita.org/internal/domain/patient/model"
)

type patientRepository struct {
	client *ent.Client
	engine *crypto.Engine
}

// NewPatientRepository creates a pure patient repository adapter.
func NewPatientRepository(client *ent.Client) patientmodel.PatientRepository {
	return &patientRepository{
		client: client,
	}
}

// NewPatientRepositoryWithEngine creates the production repository with
// access to the request-scoped Patient DEK prefetcher.
func NewPatientRepositoryWithEngine(client *ent.Client, engine *crypto.Engine) patientmodel.PatientRepository {
	return &patientRepository{
		client: client,
		engine: engine,
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
		SetClinicID(p.ClinicID).
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

// ListCandidates selects only identifiers, status, and timestamps so the
// database can paginate before any Patient DEK is loaded.
func (r *patientRepository) ListCandidates(ctx context.Context, clinicID uuid.UUID, status *patientmodel.PatientStatus, nameTokens []string, emailBlindIndex string, limit, offset int) ([]patientmodel.PatientCandidate, int, error) {
	query := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID))
	if status != nil {
		query = query.Where(patient.StatusEQ(patient.Status(*status)))
	}
	if len(nameTokens) > 0 {
		predicates := make([]*entsql.Predicate, 0, len(nameTokens))
		for _, token := range nameTokens {
			predicates = append(predicates, sqljson.ValueContains(patient.FieldDisplayNameTokenIndex, token))
		}
		query = query.Where(func(s *entsql.Selector) {
			s.Where(entsql.Or(predicates...))
		})
	}
	if emailBlindIndex != "" {
		query = query.Where(patient.EmailBlindIndexEQ(emailBlindIndex))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("patient repository: count candidates: %w", err)
	}
	rows, err := query.
		Select(patient.FieldID, patient.FieldClinicID, patient.FieldStatus, patient.FieldCreatedAt).
		Order(ent.Desc(patient.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("patient repository: list candidates: %w", err)
	}
	out := make([]patientmodel.PatientCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, patientmodel.PatientCandidate{
			ID:        row.ID,
			ClinicID:  row.ClinicID,
			Status:    patientmodel.PatientStatus(row.Status),
			CreatedAt: row.CreatedAt,
		})
	}
	return out, total, nil
}

// GetMany hydrates only the requested page of patients.
func (r *patientRepository) GetMany(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID) ([]patientmodel.Patient, error) {
	if len(patientIDs) == 0 {
		return nil, nil
	}
	queryCtx := ctx
	if r.engine != nil {
		if !crypto.HasRequestKeyCache(queryCtx) {
			queryCtx = crypto.WithRequestKeyCache(queryCtx)
			defer crypto.ClearRequestKeyCache(queryCtx)
		}
		deks, err := r.engine.GetPatientDEKsForClinic(queryCtx, clinicID, patientIDs)
		if err != nil {
			return nil, fmt.Errorf("patient repository: prefetch patient deks: %w", err)
		}
		for _, dek := range deks {
			crypto.ZeroBytes(dek)
		}
	}
	rows, err := r.client.Patient.Query().
		Where(patient.ClinicIDEQ(clinicID), patient.IDIn(patientIDs...)).
		All(queryCtx)
	if err != nil {
		return nil, fmt.Errorf("patient repository: get many: %w", err)
	}
	out := make([]patientmodel.Patient, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toDomainPatient(row))
	}
	return out, nil
}

// DeleteAggregate removes relational patient data in dependency order. The
// operation is idempotent so cleanup can be retried after a key has already
// been shredded.
func (r *patientRepository) DeleteAggregate(ctx context.Context, clinicID, patientID uuid.UUID) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("patient repository: begin aggregate delete: %w", err)
	}
	rollback := func(cause error) error {
		_ = tx.Rollback()
		return cause
	}

	if _, err := tx.PatientIdentifier.Delete().
		Where(
			patientidentifier.ClinicIDEQ(clinicID),
			patientidentifier.PatientIDEQ(patientID),
		).
		Exec(ctx); err != nil {
		return rollback(fmt.Errorf("patient repository: delete identifiers: %w", err))
	}
	if _, err := tx.Episode.Delete().
		Where(
			episode.ClinicIDEQ(clinicID),
			episode.PatientIDEQ(patientID),
		).
		Exec(ctx); err != nil {
		return rollback(fmt.Errorf("patient repository: delete episodes: %w", err))
	}
	if _, err := tx.Appointment.Delete().
		Where(
			appointment.ClinicIDEQ(clinicID),
			appointment.PatientIDEQ(patientID),
		).
		Exec(ctx); err != nil {
		return rollback(fmt.Errorf("patient repository: delete appointments: %w", err))
	}
	if _, err := tx.Patient.Delete().
		Where(
			patient.ClinicIDEQ(clinicID),
			patient.IDEQ(patientID),
		).
		Exec(ctx); err != nil {
		return rollback(fmt.Errorf("patient repository: delete patient: %w", err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("patient repository: commit aggregate delete: %w", err)
	}
	return nil
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

	sex := patientmodel.SexUnknown
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
