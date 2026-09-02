package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"

	"librevita.org/ent"
	"librevita.org/ent/episode"
	"librevita.org/ent/finding"
	"librevita.org/ent/patient"
	"librevita.org/ent/planitem"
	"librevita.org/ent/problem"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/database/fle"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/pkg/ident"
)

type episodeRepository struct {
	client *ent.Client
}

// NewEpisodeRepository creates the Ent adapter for the SOAP aggregate.
func NewEpisodeRepository(client *ent.Client) episodemodel.EpisodeRepository {
	return &episodeRepository{client: client}
}

func (r *episodeRepository) Create(ctx context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	ctx = fle.WithPatientID(ctx, ep.PatientID)
	ctx = fle.WithClinicID(ctx, ep.ClinicID)
	var saved *ent.Episode
	err := database.WithTx(ctx, r.client, func(tx *ent.Tx) error {
		create := tx.Episode.Create().
			SetID(ep.ID).
			SetClinicID(ep.ClinicID).
			SetPatientID(ep.PatientID).
			SetUserID(ep.AuthorID).
			SetEpisodeType(episode.EpisodeType(ep.Type)).
			SetStatus(episode.StatusDraft).
			SetClass(episode.Class(ep.Class)).
			SetOccurredAt(ep.OccurredAt)
		if ep.AppointmentID != nil {
			create.SetAppointmentID(*ep.AppointmentID)
		}
		if ep.PredecessorID != nil {
			create.SetPredecessorID(*ep.PredecessorID)
		}
		if ep.EndedAt != nil {
			create.SetEndedAt(*ep.EndedAt)
		}
		setSOAP(create, ep.SOAP)
		row, err := create.Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) && ep.PredecessorID != nil {
				return errors.WithSecondaryError(episodemodel.ErrAlreadyAmended, err)
			}
			return errors.Wrap(err, "episode repository: create")
		}
		saved = row
		return replaceChildren(ctx, tx, ep)
	})
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, saved.ClinicID, saved.ID)
}

func (r *episodeRepository) UpdateDraft(ctx context.Context, ep episodemodel.Episode) (*episodemodel.Episode, error) {
	ctx = fle.WithPatientID(ctx, ep.PatientID)
	ctx = fle.WithClinicID(ctx, ep.ClinicID)
	err := database.WithTx(ctx, r.client, func(tx *ent.Tx) error {
		row, err := tx.Episode.Query().
			Where(episode.IDEQ(ep.ID), episode.ClinicIDEQ(ep.ClinicID)).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				return errors.WithSecondaryError(episodemodel.ErrNotFound, err)
			}
			return errors.Wrap(err, "episode repository: get for update")
		}
		if row.Status != episode.StatusDraft {
			return episodemodel.ErrNotDraft
		}
		upd := tx.Episode.UpdateOneID(ep.ID).
			SetUserID(ep.AuthorID).
			SetEpisodeType(episode.EpisodeType(ep.Type)).
			SetClass(episode.Class(ep.Class)).
			SetOccurredAt(ep.OccurredAt).
			SetUpdatedAt(time.Now())
		if ep.AppointmentID != nil {
			upd.SetAppointmentID(*ep.AppointmentID)
		} else {
			upd.ClearAppointmentID()
		}
		if ep.PredecessorID != nil {
			upd.SetPredecessorID(*ep.PredecessorID)
		} else {
			upd.ClearPredecessorID()
		}
		if ep.EndedAt != nil {
			upd.SetEndedAt(*ep.EndedAt)
		} else {
			upd.ClearEndedAt()
		}
		setSOAPUpdate(upd, ep.SOAP)
		if err := upd.Exec(ctx); err != nil {
			return errors.Wrap(err, "episode repository: update")
		}
		return replaceChildren(ctx, tx, ep)
	})
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, ep.ClinicID, ep.ID)
}

func (r *episodeRepository) Get(ctx context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID) (*episodemodel.Episode, error) {
	row, err := r.client.Episode.Query().
		Where(episode.IDEQ(episodeID), episode.ClinicIDEQ(clinicID)).
		WithFindings().
		WithProblems().
		WithPlanItems().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.WithSecondaryError(episodemodel.ErrNotFound, err)
		}
		return nil, errors.Wrap(err, "episode repository: get")
	}
	ep := toDomainEpisode(row)
	if err := r.attachSuccessor(ctx, ep); err != nil {
		return nil, err
	}
	return ep, nil
}

func (r *episodeRepository) GetByPredecessor(ctx context.Context, clinicID ident.ClinicID, predecessorID ident.EpisodeID) (*episodemodel.Episode, error) {
	row, err := r.client.Episode.Query().
		Where(episode.PredecessorIDEQ(predecessorID), episode.ClinicIDEQ(clinicID)).
		WithFindings().
		WithProblems().
		WithPlanItems().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.WithSecondaryError(episodemodel.ErrNotFound, err)
		}
		return nil, errors.Wrap(err, "episode repository: get by predecessor")
	}
	ep := toDomainEpisode(row)
	if err := r.attachSuccessor(ctx, ep); err != nil {
		return nil, err
	}
	return ep, nil
}

func (r *episodeRepository) ListByPatient(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID, status *episodemodel.EpisodeStatus) ([]episodemodel.Episode, error) {
	q := r.client.Episode.Query().
		Where(episode.ClinicIDEQ(clinicID), episode.PatientIDEQ(patientID)).
		Order(ent.Desc(episode.FieldOccurredAt))
	if status != nil {
		q = q.Where(episode.StatusEQ(episode.Status(*status)))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "episode repository: list by patient")
	}
	out := make([]episodemodel.Episode, 0, len(rows))
	for _, row := range rows {
		out = append(out, *toDomainEpisode(row))
	}
	attachSuccessors(out)
	return out, nil
}

func (r *episodeRepository) attachSuccessor(ctx context.Context, ep *episodemodel.Episode) error {
	id, err := r.client.Episode.Query().
		Where(episode.PredecessorIDEQ(ep.ID), episode.ClinicIDEQ(ep.ClinicID)).
		OnlyID(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return errors.Wrap(err, "episode repository: successor")
	}
	ep.SuccessorID = &id
	return nil
}

func attachSuccessors(out []episodemodel.Episode) {
	byPred := make(map[ident.EpisodeID]ident.EpisodeID, len(out))
	for i := range out {
		if out[i].PredecessorID != nil {
			byPred[*out[i].PredecessorID] = out[i].ID
		}
	}
	for i := range out {
		if id, ok := byPred[out[i].ID]; ok {
			out[i].SuccessorID = &id
		}
	}
}

func (r *episodeRepository) SetStatus(ctx context.Context, clinicID ident.ClinicID, episodeID ident.EpisodeID, status episodemodel.EpisodeStatus) error {
	n, err := r.client.Episode.Update().
		Where(episode.IDEQ(episodeID), episode.ClinicIDEQ(clinicID)).
		SetStatus(episode.Status(status)).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return errors.Wrap(err, "episode repository: set status")
	}
	if n == 0 {
		return episodemodel.ErrNotFound
	}
	return nil
}

func (r *episodeRepository) PatientExists(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (bool, error) {
	ok, err := r.client.Patient.Query().
		Where(patient.IDEQ(patientID), patient.ClinicIDEQ(clinicID)).
		Exist(ctx)
	if err != nil {
		return false, errors.Wrap(err, "episode repository: patient exists")
	}
	return ok, nil
}

func setSOAP(create *ent.EpisodeCreate, soap episodemodel.SOAP) {
	if soap.Subjective != "" {
		create.SetSubjective(soap.Subjective)
	}
	if soap.Objective != "" {
		create.SetObjective(soap.Objective)
	}
	if soap.Assessment != "" {
		create.SetAssessment(soap.Assessment)
	}
	if soap.Plan != "" {
		create.SetPlan(soap.Plan)
	}
}

func setSOAPUpdate(upd *ent.EpisodeUpdateOne, soap episodemodel.SOAP) {
	if soap.Subjective != "" {
		upd.SetSubjective(soap.Subjective)
	} else {
		upd.ClearSubjective()
	}
	if soap.Objective != "" {
		upd.SetObjective(soap.Objective)
	} else {
		upd.ClearObjective()
	}
	if soap.Assessment != "" {
		upd.SetAssessment(soap.Assessment)
	} else {
		upd.ClearAssessment()
	}
	if soap.Plan != "" {
		upd.SetPlan(soap.Plan)
	} else {
		upd.ClearPlan()
	}
}

func replaceChildren(ctx context.Context, tx *ent.Tx, ep episodemodel.Episode) error {
	if _, err := tx.Finding.Delete().Where(finding.EpisodeIDEQ(ep.ID)).Exec(ctx); err != nil {
		return errors.Wrap(err, "episode repository: clear findings")
	}
	if _, err := tx.Problem.Delete().Where(problem.EpisodeIDEQ(ep.ID)).Exec(ctx); err != nil {
		return errors.Wrap(err, "episode repository: clear problems")
	}
	if _, err := tx.PlanItem.Delete().Where(planitem.EpisodeIDEQ(ep.ID)).Exec(ctx); err != nil {
		return errors.Wrap(err, "episode repository: clear plan items")
	}
	for _, f := range ep.Findings {
		if err := insertFinding(ctx, tx, ep, f); err != nil {
			return err
		}
	}
	for _, p := range ep.Problems {
		if err := insertProblem(ctx, tx, ep, p); err != nil {
			return err
		}
	}
	for _, item := range ep.PlanItems {
		if err := insertPlanItem(ctx, tx, ep, item); err != nil {
			return err
		}
	}
	return nil
}

func insertFinding(ctx context.Context, tx *ent.Tx, ep episodemodel.Episode, f episodemodel.Finding) error {
	c := tx.Finding.Create().
		SetID(f.ID).
		SetClinicID(ep.ClinicID).
		SetPatientID(ep.PatientID).
		SetEpisodeID(ep.ID).
		SetStatus(finding.Status(f.Status)).
		SetValueKind(finding.ValueKind(f.Value.Kind)).
		SetEffectiveAt(f.EffectiveAt).
		SetCodeSystem(f.Code.System).
		SetCode(f.Code.Code).
		SetDisplay(f.Code.Display)
	applyFindingValue(c, f)
	if _, err := c.Save(ctx); err != nil {
		return errors.Wrap(err, "episode repository: create finding")
	}
	return nil
}

func applyFindingValue(c *ent.FindingCreate, f episodemodel.Finding) {
	switch f.Value.Kind {
	case episodemodel.FindingValueQuantity:
		if f.Value.Quantity != nil {
			c.SetValueNumber(strconv.FormatFloat(f.Value.Quantity.Value, 'G', -1, 64))
			c.SetValueUnit(f.Value.Quantity.Unit)
			c.SetValueUcum(f.Value.Quantity.Code)
		}
	case episodemodel.FindingValueString:
		c.SetValueText(f.Value.String)
	case episodemodel.FindingValueBoolean:
		if f.Value.Boolean != nil && *f.Value.Boolean {
			c.SetValueBool("true")
		} else {
			c.SetValueBool("false")
		}
	case episodemodel.FindingValueCoded:
		if f.Value.Coded != nil {
			c.SetValueCodedSystem(f.Value.Coded.System)
			c.SetValueCodedCode(f.Value.Coded.Code)
			c.SetValueCodedDisplay(f.Value.Coded.Display)
		}
	}
}

func insertProblem(ctx context.Context, tx *ent.Tx, ep episodemodel.Episode, p episodemodel.Problem) error {
	c := tx.Problem.Create().
		SetID(p.ID).
		SetClinicID(ep.ClinicID).
		SetPatientID(ep.PatientID).
		SetEpisodeID(ep.ID).
		SetClinicalStatus(problem.ClinicalStatus(p.ClinicalStatus)).
		SetVerificationStatus(problem.VerificationStatus(p.VerificationStatus)).
		SetCategory(problem.Category(p.Category)).
		SetRank(p.Rank).
		SetCodeSystem(p.Code.System).
		SetCode(p.Code.Code).
		SetDisplay(p.Code.Display)
	if p.Text != "" {
		c.SetText(p.Text)
	}
	if _, err := c.Save(ctx); err != nil {
		return errors.Wrap(err, "episode repository: create problem")
	}
	return nil
}

func insertPlanItem(ctx context.Context, tx *ent.Tx, ep episodemodel.Episode, item episodemodel.PlanItem) error {
	c := tx.PlanItem.Create().
		SetID(item.ID).
		SetClinicID(ep.ClinicID).
		SetPatientID(ep.PatientID).
		SetEpisodeID(ep.ID).
		SetKind(planitem.Kind(item.Kind)).
		SetStatus(planitem.Status(item.Status)).
		SetCodeSystem(item.Code.System).
		SetCode(item.Code.Code).
		SetDisplay(item.Code.Display)
	if item.Description != "" {
		c.SetDescription(item.Description)
	}
	if item.ScheduledAt != nil {
		c.SetScheduledAt(*item.ScheduledAt)
	}
	if _, err := c.Save(ctx); err != nil {
		return errors.Wrap(err, "episode repository: create plan item")
	}
	return nil
}

func toDomainEpisode(row *ent.Episode) *episodemodel.Episode {
	ep := &episodemodel.Episode{
		ID:            row.ID,
		ClinicID:      row.ClinicID,
		PatientID:     row.PatientID,
		AuthorID:      row.UserID,
		AppointmentID: row.AppointmentID,
		PredecessorID: row.PredecessorID,
		Type:          episodemodel.EpisodeType(row.EpisodeType),
		Status:        episodemodel.EpisodeStatus(row.Status),
		Class:         episodemodel.CareSetting(row.Class),
		OccurredAt:    row.OccurredAt,
		EndedAt:       row.EndedAt,
		SOAP: episodemodel.SOAP{
			Subjective: row.Subjective,
			Objective:  row.Objective,
			Assessment: row.Assessment,
			Plan:       row.Plan,
		},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	for _, f := range row.Edges.Findings {
		ep.Findings = append(ep.Findings, toDomainFinding(f))
	}
	for _, p := range row.Edges.Problems {
		ep.Problems = append(ep.Problems, toDomainProblem(p))
	}
	for _, item := range row.Edges.PlanItems {
		ep.PlanItems = append(ep.PlanItems, toDomainPlanItem(item))
	}
	return ep
}

func toDomainFinding(f *ent.Finding) episodemodel.Finding {
	val := episodemodel.FindingValue{Kind: episodemodel.FindingValueKind(f.ValueKind)}
	switch val.Kind {
	case episodemodel.FindingValueQuantity:
		q := &episodemodel.Quantity{Unit: f.ValueUnit, Code: f.ValueUcum, System: "http://unitsofmeasure.org"}
		if f.ValueNumber != "" {
			if n, err := strconv.ParseFloat(f.ValueNumber, 64); err == nil {
				q.Value = n
			}
		}
		val.Quantity = q
	case episodemodel.FindingValueString:
		val.String = f.ValueText
	case episodemodel.FindingValueBoolean:
		b := f.ValueBool == "true"
		val.Boolean = &b
	case episodemodel.FindingValueCoded:
		val.Coded = &episodemodel.Coding{
			System:  f.ValueCodedSystem,
			Code:    f.ValueCodedCode,
			Display: f.ValueCodedDisplay,
		}
	}
	return episodemodel.Finding{
		ID:          f.ID,
		ClinicID:    f.ClinicID,
		PatientID:   f.PatientID,
		EpisodeID:   f.EpisodeID,
		Code:        episodemodel.Coding{System: f.CodeSystem, Code: f.Code, Display: f.Display},
		Value:       val,
		Status:      episodemodel.FindingStatus(f.Status),
		EffectiveAt: f.EffectiveAt,
		CreatedAt:   f.CreatedAt,
		UpdatedAt:   f.UpdatedAt,
	}
}

func toDomainProblem(p *ent.Problem) episodemodel.Problem {
	return episodemodel.Problem{
		ID:                 p.ID,
		ClinicID:           p.ClinicID,
		PatientID:          p.PatientID,
		EpisodeID:          p.EpisodeID,
		Code:               episodemodel.Coding{System: p.CodeSystem, Code: p.Code, Display: p.Display},
		Text:               p.Text,
		ClinicalStatus:     episodemodel.ProblemClinicalStatus(p.ClinicalStatus),
		VerificationStatus: episodemodel.ProblemVerificationStatus(p.VerificationStatus),
		Category:           episodemodel.ProblemCategory(p.Category),
		Rank:               p.Rank,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	}
}

func toDomainPlanItem(item *ent.PlanItem) episodemodel.PlanItem {
	return episodemodel.PlanItem{
		ID:          item.ID,
		ClinicID:    item.ClinicID,
		PatientID:   item.PatientID,
		EpisodeID:   item.EpisodeID,
		Kind:        episodemodel.PlanItemKind(item.Kind),
		Code:        episodemodel.Coding{System: item.CodeSystem, Code: item.Code, Display: item.Display},
		Description: item.Description,
		Status:      episodemodel.PlanItemStatus(item.Status),
		ScheduledAt: item.ScheduledAt,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}
