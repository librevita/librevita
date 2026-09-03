package repository

import (
	"context"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/cockroachdb/errors"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/appointment"
	"librevita.org/internal/database/record/episode"
	"librevita.org/internal/database/record/finding"
	"librevita.org/internal/database/record/patient"
	"librevita.org/internal/database/record/patientidentifier"
	"librevita.org/internal/database/record/planitem"
	"librevita.org/internal/database/record/problem"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/pkg/flow"
	"librevita.org/pkg/ident"
)

type patientRepository struct {
	client *record.Client
	engine *crypto.Engine
}

// NewPatientRepository creates a pure patient repository adapter.
func NewPatientRepository(client *record.Client) patientmodel.PatientRepository {
	return &patientRepository{
		client: client,
	}
}

// NewPatientRepositoryWithEngine creates the production repository with
// access to the request-scoped Patient DEK prefetcher.
func NewPatientRepositoryWithEngine(client *record.Client, engine *crypto.Engine) patientmodel.PatientRepository {
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
	applyOptionalCreate(create, p)

	saved, err := create.Save(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "patient repository: create")
	}
	return toDomainPatient(saved), nil
}

func (r *patientRepository) Get(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (*patientmodel.Patient, error) {
	p, err := r.client.Patient.Query().
		Where(
			patient.IDEQ(patientID),
			patient.ClinicIDEQ(clinicID),
		).
		Only(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, errors.WithSecondaryError(patientmodel.ErrNotFound, err)
		}
		return nil, errors.Wrap(err, "patient repository: get")
	}
	return toDomainPatient(p), nil
}

func (r *patientRepository) GetWithCreator(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (*patientmodel.GetPatientWithCreatorRow, error) {
	p, err := r.client.Patient.Query().
		Where(
			patient.IDEQ(patientID),
			patient.ClinicIDEQ(clinicID),
		).
		Only(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, errors.WithSecondaryError(patientmodel.ErrNotFound, err)
		}
		return nil, errors.Wrap(err, "patient repository: get with creator")
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
	applyOptionalUpdate(update, p)

	updated, err := update.Save(ctx)
	if err != nil {
		if record.IsNotFound(err) {
			return nil, errors.WithSecondaryError(patientmodel.ErrNotFound, err)
		}
		return nil, errors.Wrap(err, "patient repository: update")
	}
	return toDomainPatient(updated), nil
}

func applyOptionalCreate(create *record.PatientCreate, p patientmodel.Patient) {
	setPtr(p.BirthDate, create.SetBirthDate)
	if p.Sex != "" {
		create.SetSex(string(p.Sex))
	}
	setPtr(p.Phone, create.SetPhone)
	setPtr(p.Email, create.SetEmail)
	setPtr(p.Street, create.SetStreet)
	setPtr(p.City, create.SetCity)
	setPtr(p.State, create.SetState)
	setPtr(p.PostalCode, create.SetPostalCode)
	setPtr(p.Notes, create.SetNotes)
	if p.CreatedBy != nil {
		create.SetCreatedBy(*p.CreatedBy)
	}
}

func applyOptionalUpdate(update *record.PatientUpdateOne, p patientmodel.Patient) {
	setOrClear(p.BirthDate, update.SetBirthDate, update.ClearBirthDate)
	if p.Sex != "" {
		update.SetSex(string(p.Sex))
	} else {
		update.ClearSex()
	}
	setPtr(p.Phone, update.SetPhone)
	setPtr(p.Email, update.SetEmail)
	setOrClear(p.Street, update.SetStreet, update.ClearStreet)
	setOrClear(p.City, update.SetCity, update.ClearCity)
	setOrClear(p.State, update.SetState, update.ClearState)
	setOrClear(p.PostalCode, update.SetPostalCode, update.ClearPostalCode)
	setOrClear(p.Notes, update.SetNotes, update.ClearNotes)
}

func setPtr[T any](s *string, set func(string) T) {
	if s != nil && *s != "" {
		set(*s)
	}
}

func setOrClear[T any](s *string, set func(string) T, clear func() T) {
	if s != nil && *s != "" {
		set(*s)
		return
	}
	clear()
}

func (r *patientRepository) BulkSetStatus(ctx context.Context, clinicID ident.ClinicID, patientIDs []ident.PatientID, status patientmodel.PatientStatus) (int, error) {
	count, err := r.client.Patient.Update().
		Where(
			patient.ClinicIDEQ(clinicID),
			patient.IDIn(patientIDs...),
		).
		SetStatus(patient.Status(status)).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "patient repository: bulk set status")
	}
	return count, nil
}

func (r *patientRepository) ListByClinicAndStatus(ctx context.Context, clinicID ident.ClinicID, status *patientmodel.PatientStatus) ([]patientmodel.Patient, error) {
	query := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID))
	if status != nil {
		query = query.Where(patient.StatusEQ(patient.Status(*status)))
	}
	rows, err := query.Order(record.Desc(patient.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "patient repository: list")
	}

	out := make([]patientmodel.Patient, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toDomainPatient(row))
	}
	return out, nil
}

// ListCandidates selects only identifiers, status, and timestamps so the
// database can paginate before any Patient DEK is loaded.
func (r *patientRepository) ListCandidates(ctx context.Context, clinicID ident.ClinicID, status *patientmodel.PatientStatus, nameTokens []string, emailBlindIndex string, limit, offset int) ([]patientmodel.PatientCandidate, int, error) {
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
		return nil, 0, errors.Wrap(err, "patient repository: count candidates")
	}
	rows, err := query.
		Select(patient.FieldID, patient.FieldClinicID, patient.FieldStatus, patient.FieldCreatedAt).
		Order(record.Desc(patient.FieldCreatedAt)).
		Limit(limit).
		Offset(offset).
		All(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "patient repository: list candidates")
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
func (r *patientRepository) GetMany(ctx context.Context, clinicID ident.ClinicID, patientIDs []ident.PatientID) ([]patientmodel.Patient, error) {
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
			return nil, errors.Wrap(err, "patient repository: prefetch patient deks")
		}
		for _, dek := range deks {
			crypto.ZeroBytes(dek)
		}
	}
	rows, err := r.client.Patient.Query().
		Where(patient.ClinicIDEQ(clinicID), patient.IDIn(patientIDs...)).
		All(queryCtx)
	if err != nil {
		return nil, errors.Wrap(err, "patient repository: get many")
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
func (r *patientRepository) DeleteAggregate(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) error {
	return database.WithTx(ctx, r.client, func(tx *record.Tx) error {
		return flow.Exec(
			func() error {
				_, err := tx.PatientIdentifier.Delete().
					Where(
						patientidentifier.ClinicIDEQ(clinicID),
						patientidentifier.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete identifiers")
			},
			func() error {
				_, err := tx.Finding.Delete().
					Where(
						finding.ClinicIDEQ(clinicID),
						finding.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete findings")
			},
			func() error {
				_, err := tx.Problem.Delete().
					Where(
						problem.ClinicIDEQ(clinicID),
						problem.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete problems")
			},
			func() error {
				_, err := tx.PlanItem.Delete().
					Where(
						planitem.ClinicIDEQ(clinicID),
						planitem.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete plan items")
			},
			func() error {
				_, err := tx.Episode.Delete().
					Where(
						episode.ClinicIDEQ(clinicID),
						episode.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete episodes")
			},
			func() error {
				_, err := tx.Appointment.Delete().
					Where(
						appointment.ClinicIDEQ(clinicID),
						appointment.PatientIDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete appointments")
			},
			func() error {
				_, err := tx.Patient.Delete().
					Where(
						patient.ClinicIDEQ(clinicID),
						patient.IDEQ(patientID),
					).
					Exec(ctx)
				return errors.Wrap(err, "patient repository: delete patient")
			},
		)
	})
}

func (r *patientRepository) Count(ctx context.Context, clinicID ident.ClinicID) (int, error) {
	count, err := r.client.Patient.Query().Where(patient.ClinicIDEQ(clinicID)).Count(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "patient repository: count")
	}
	return count, nil
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toDomainPatient(p *record.Patient) *patientmodel.Patient {
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
