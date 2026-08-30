package usecase

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	episodemodel "librevita.org/internal/domain/episode/model"
)

// Re-export domain types for delivery adapters.
type (
	Episode           = episodemodel.Episode
	EpisodeStatus     = episodemodel.EpisodeStatus
	EpisodeRepository = episodemodel.EpisodeRepository
)

var (
	ErrNotFound       = episodemodel.ErrNotFound
	ErrForbidden      = episodemodel.ErrForbidden
	ErrNotDraft       = episodemodel.ErrNotDraft
	ErrNotFinalized   = episodemodel.ErrNotFinalized
	ErrAlreadyAmended = episodemodel.ErrAlreadyAmended
	ErrInvalidSOAP    = episodemodel.ErrInvalidSOAP
	ErrPatientGone    = episodemodel.ErrPatientGone
)

// Service coordinates SOAP chart persistence and authorization.
type Service struct {
	repo     EpisodeRepository
	policies *policy.PolicyEngine
}

// NewService is the Fx provider.
func NewService(repo EpisodeRepository, policies *policy.PolicyEngine) *Service {
	return &Service{repo: repo, policies: policies}
}

// Create inserts a draft SOAP episode.
func (s *Service) Create(ctx context.Context, principal *auth.Principal, ep Episode) (*Episode, error) {
	if err := s.authorizeWrite(ctx, principal, ep.PatientID); err != nil {
		return nil, err
	}
	if err := prepareNew(&ep); err != nil {
		return nil, err
	}
	ok, err := s.repo.PatientExists(ctx, ep.ClinicID, ep.PatientID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrPatientGone
	}
	saved, err := s.repo.Create(ctx, ep)
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// UpdateDraft replaces a draft SOAP episode.
func (s *Service) UpdateDraft(ctx context.Context, principal *auth.Principal, ep Episode) (*Episode, error) {
	if err := s.authorizeWrite(ctx, principal, ep.PatientID); err != nil {
		return nil, err
	}
	existing, err := s.repo.Get(ctx, ep.ClinicID, ep.ID)
	if err != nil {
		return nil, err
	}
	if existing.Status != episodemodel.EpisodeStatusDraft {
		return nil, ErrNotDraft
	}
	if err := prepareUpdate(&ep, existing); err != nil {
		return nil, err
	}
	return s.repo.UpdateDraft(ctx, ep)
}

// Finalize locks a draft episode.
func (s *Service) Finalize(ctx context.Context, principal *auth.Principal, clinicID, episodeID uuid.UUID) (*Episode, error) {
	existing, err := s.repo.Get(ctx, clinicID, episodeID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWrite(ctx, principal, existing.PatientID); err != nil {
		return nil, err
	}
	if existing.Status != episodemodel.EpisodeStatusDraft {
		return nil, ErrNotDraft
	}
	if err := s.repo.SetStatus(ctx, clinicID, episodeID, episodemodel.EpisodeStatusFinalized); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, clinicID, episodeID)
}

// Amend returns the open successor draft of a finalized note (linear replaces chain).
// A second call while that draft is open returns the same episode. After the
// successor is finalized, Amend of the original note is ErrAlreadyAmended;
// Amend of the successor starts the next link.
func (s *Service) Amend(ctx context.Context, principal *auth.Principal, clinicID, episodeID uuid.UUID) (*Episode, error) {
	existing, err := s.repo.Get(ctx, clinicID, episodeID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWrite(ctx, principal, existing.PatientID); err != nil {
		return nil, err
	}
	if existing.Status != episodemodel.EpisodeStatusFinalized {
		return nil, ErrNotFinalized
	}
	if child, err := s.successorDraft(ctx, clinicID, existing.ID); err != nil {
		return nil, err
	} else if child != nil {
		return child, nil
	}
	authorID, err := uuid.Parse(principal.ID)
	if err != nil {
		return nil, errors.Wrap(err, "usecase: amend author")
	}
	pred := existing.ID
	draft := Episode{
		ClinicID:      existing.ClinicID,
		PatientID:     existing.PatientID,
		AuthorID:      authorID,
		PredecessorID: &pred,
		Type:          existing.Type,
		Class:         existing.Class,
		OccurredAt:    time.Now().UTC(),
		SOAP:          existing.SOAP,
		Findings:      copyFindings(existing.Findings),
		Problems:      copyProblems(existing.Problems),
		PlanItems:     copyPlanItems(existing.PlanItems),
	}
	saved, err := s.Create(ctx, principal, draft)
	if err == nil {
		return saved, nil
	}
	return s.amendAfterCreateConflict(ctx, clinicID, existing.ID, err)
}

func (s *Service) amendAfterCreateConflict(ctx context.Context, clinicID, predecessorID uuid.UUID, err error) (*Episode, error) {
	if !errors.Is(err, ErrAlreadyAmended) {
		return nil, err
	}
	child, rerr := s.successorDraft(ctx, clinicID, predecessorID)
	if rerr != nil {
		return nil, rerr
	}
	if child != nil {
		return child, nil
	}
	return nil, ErrAlreadyAmended
}

func (s *Service) successorDraft(ctx context.Context, clinicID, predecessorID uuid.UUID) (*Episode, error) {
	child, err := s.repo.GetByPredecessor(ctx, clinicID, predecessorID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if child.Status != episodemodel.EpisodeStatusDraft {
		return nil, ErrAlreadyAmended
	}
	return child, nil
}

// Get returns one episode after chart.view authorization.
func (s *Service) Get(ctx context.Context, principal *auth.Principal, clinicID, episodeID uuid.UUID) (*Episode, error) {
	ep, err := s.repo.Get(ctx, clinicID, episodeID)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeView(ctx, principal, ep); err != nil {
		return nil, err
	}
	return ep, nil
}

// ListByPatient returns episodes for a patient (staff: all; portal: finalized own).
func (s *Service) ListByPatient(ctx context.Context, principal *auth.Principal, clinicID, patientID uuid.UUID) ([]Episode, error) {
	dummy := &Episode{PatientID: patientID, Status: episodemodel.EpisodeStatusFinalized}
	if err := s.authorizeView(ctx, principal, dummy); err != nil {
		return nil, err
	}
	var status *episodemodel.EpisodeStatus
	if principal != nil && principal.Role == auth.RolePatient {
		final := episodemodel.EpisodeStatusFinalized
		status = &final
	}
	return s.repo.ListByPatient(ctx, clinicID, patientID, status)
}

func (s *Service) authorizeWrite(ctx context.Context, principal *auth.Principal, patientID uuid.UUID) error {
	if principal == nil {
		return ErrForbidden
	}
	resource := map[string]any{"patient_id": patientID.String()}
	allowed, err := s.policies.AllowedResource(ctx, "chart.write", principal,
		policy.RequestInfo{Method: "POST", Path: "/episodes"}, resource, nil)
	if err != nil {
		return errors.Wrap(err, "usecase: authorize chart write")
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *Service) authorizeView(ctx context.Context, principal *auth.Principal, ep *Episode) error {
	if principal == nil {
		return ErrForbidden
	}
	resource := map[string]any{"patient_id": ep.PatientID.String()}
	allowed, err := s.policies.AllowedResource(ctx, "chart.view", principal,
		policy.RequestInfo{Method: "GET", Path: "/episodes/" + ep.ID.String()}, resource, nil)
	if err != nil {
		return errors.Wrap(err, "usecase: authorize chart view")
	}
	if !allowed {
		return ErrForbidden
	}
	if principal.Role == auth.RolePatient {
		if principal.PatientID != ep.PatientID.String() {
			return ErrForbidden
		}
		if ep.Status != episodemodel.EpisodeStatusFinalized && ep.ID != uuid.Nil {
			return ErrForbidden
		}
	}
	return nil
}

func prepareNew(ep *Episode) error {
	if ep.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return errors.Wrap(err, "usecase: episode id")
		}
		ep.ID = id
	}
	ep.Status = episodemodel.EpisodeStatusDraft
	if ep.Type == "" {
		ep.Type = episodemodel.EpisodeTypeConsultation
	}
	if ep.Class == "" {
		ep.Class = episodemodel.CareSettingAmbulatory
	}
	if err := assignChildIDs(ep); err != nil {
		return err
	}
	return ep.Validate()
}

func prepareUpdate(ep, existing *Episode) error {
	ep.ClinicID = existing.ClinicID
	ep.PatientID = existing.PatientID
	if ep.AuthorID == uuid.Nil {
		ep.AuthorID = existing.AuthorID
	}
	if ep.Type == "" {
		ep.Type = existing.Type
	}
	if ep.Class == "" {
		ep.Class = existing.Class
	}
	if ep.OccurredAt.IsZero() {
		ep.OccurredAt = existing.OccurredAt
	}
	ep.PredecessorID = existing.PredecessorID
	ep.Status = episodemodel.EpisodeStatusDraft
	if err := assignChildIDs(ep); err != nil {
		return err
	}
	return ep.Validate()
}

func assignChildIDs(ep *Episode) error {
	for i := range ep.Findings {
		if ep.Findings[i].ID == uuid.Nil {
			id, err := uuid.NewV7()
			if err != nil {
				return errors.Wrap(err, "usecase: finding id")
			}
			ep.Findings[i].ID = id
		}
		ep.Findings[i].ClinicID = ep.ClinicID
		ep.Findings[i].PatientID = ep.PatientID
		ep.Findings[i].EpisodeID = ep.ID
		if ep.Findings[i].Status == "" {
			ep.Findings[i].Status = episodemodel.FindingStatusRecorded
		}
		if ep.Findings[i].EffectiveAt.IsZero() {
			ep.Findings[i].EffectiveAt = ep.OccurredAt
		}
	}
	for i := range ep.Problems {
		if ep.Problems[i].ID == uuid.Nil {
			id, err := uuid.NewV7()
			if err != nil {
				return errors.Wrap(err, "usecase: problem id")
			}
			ep.Problems[i].ID = id
		}
		ep.Problems[i].ClinicID = ep.ClinicID
		ep.Problems[i].PatientID = ep.PatientID
		ep.Problems[i].EpisodeID = ep.ID
		if ep.Problems[i].ClinicalStatus == "" {
			ep.Problems[i].ClinicalStatus = episodemodel.ProblemClinicalActive
		}
		if ep.Problems[i].VerificationStatus == "" {
			ep.Problems[i].VerificationStatus = episodemodel.ProblemVerificationConfirmed
		}
		if ep.Problems[i].Category == "" {
			ep.Problems[i].Category = episodemodel.ProblemCategoryEncounter
		}
		if ep.Problems[i].Rank < 1 {
			ep.Problems[i].Rank = i + 1
		}
	}
	for i := range ep.PlanItems {
		if ep.PlanItems[i].ID == uuid.Nil {
			id, err := uuid.NewV7()
			if err != nil {
				return errors.Wrap(err, "usecase: plan item id")
			}
			ep.PlanItems[i].ID = id
		}
		ep.PlanItems[i].ClinicID = ep.ClinicID
		ep.PlanItems[i].PatientID = ep.PatientID
		ep.PlanItems[i].EpisodeID = ep.ID
		if ep.PlanItems[i].Kind == "" {
			ep.PlanItems[i].Kind = episodemodel.PlanItemKindInstruction
		}
		if ep.PlanItems[i].Status == "" {
			ep.PlanItems[i].Status = episodemodel.PlanItemStatusActive
		}
	}
	return nil
}

func copyFindings(in []episodemodel.Finding) []episodemodel.Finding {
	out := make([]episodemodel.Finding, len(in))
	for i, f := range in {
		out[i] = episodemodel.Finding{Code: f.Code, Value: f.Value, Status: f.Status, EffectiveAt: f.EffectiveAt}
	}
	return out
}

func copyProblems(in []episodemodel.Problem) []episodemodel.Problem {
	out := make([]episodemodel.Problem, len(in))
	for i, p := range in {
		out[i] = episodemodel.Problem{
			Code: p.Code, Text: p.Text, ClinicalStatus: p.ClinicalStatus,
			VerificationStatus: p.VerificationStatus, Category: p.Category, Rank: p.Rank,
		}
	}
	return out
}

func copyPlanItems(in []episodemodel.PlanItem) []episodemodel.PlanItem {
	out := make([]episodemodel.PlanItem, len(in))
	for i, item := range in {
		out[i] = episodemodel.PlanItem{
			Kind: item.Kind, Code: item.Code, Description: item.Description,
			Status: item.Status, ScheduledAt: item.ScheduledAt,
		}
	}
	return out
}
