package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/domain/episode/model"
	"librevita.org/pkg/ident"
)

func TestEpisodeValidate(t *testing.T) {
	raw := "01990000-0000-7000-8000-000000000001"
	valid := model.Episode{
		ID:         ident.MustParseEpisode(raw),
		ClinicID:   ident.MustParseClinic(raw),
		PatientID:  ident.MustParsePatient(raw),
		AuthorID:   ident.MustParseUser(raw),
		Type:       model.EpisodeTypeConsultation,
		Status:     model.EpisodeStatusDraft,
		Class:      model.CareSettingAmbulatory,
		OccurredAt: time.Now(),
		Findings: []model.Finding{{
			Code:   model.Coding{System: "http://loinc.org", Code: "8480-6"},
			Status: model.FindingStatusRecorded,
			Value:  model.FindingValue{Kind: model.FindingValueQuantity},
		}},
		Problems: []model.Problem{{
			ClinicalStatus:     model.ProblemClinicalActive,
			VerificationStatus: model.ProblemVerificationConfirmed,
			Category:           model.ProblemCategoryEncounter,
			Rank:               1,
		}},
		PlanItems: []model.PlanItem{{
			Kind:   model.PlanItemKindInstruction,
			Status: model.PlanItemStatusActive,
		}},
	}
	assert.NoError(t, valid.Validate())

	// 1. CanAmend
	assert.False(t, valid.CanAmend()) // is draft
	valid.Status = model.EpisodeStatusFinalized
	assert.True(t, valid.CanAmend()) // finalized, no successor
	succID := ident.New[ident.EpisodeID]()
	valid.SuccessorID = &succID
	assert.False(t, valid.CanAmend()) // finalized, has successor
	valid.SuccessorID = nil
	valid.Status = model.EpisodeStatusDraft

	// 2. Header validation failures
	badClinic := valid
	badClinic.ClinicID = ident.ClinicID{}
	assert.ErrorIs(t, badClinic.Validate(), model.ErrInvalidSOAP)

	badPatient := valid
	badPatient.PatientID = ident.PatientID{}
	assert.ErrorIs(t, badPatient.Validate(), model.ErrInvalidSOAP)

	badAuthor := valid
	badAuthor.AuthorID = ident.UserID{}
	assert.ErrorIs(t, badAuthor.Validate(), model.ErrInvalidSOAP)

	badType := valid
	badType.Type = model.EpisodeType("invalid")
	assert.ErrorIs(t, badType.Validate(), model.ErrInvalidSOAP)

	badStatus := valid
	badStatus.Status = model.EpisodeStatus("invalid")
	assert.ErrorIs(t, badStatus.Validate(), model.ErrInvalidSOAP)

	badClass := valid
	badClass.Class = model.CareSetting("invalid")
	assert.ErrorIs(t, badClass.Validate(), model.ErrInvalidSOAP)

	badTime := valid
	badTime.OccurredAt = time.Time{}
	assert.ErrorIs(t, badTime.Validate(), model.ErrInvalidSOAP)

	// 3. Finding validation failures
	badFindingStatus := valid
	badFindingStatus.Findings = []model.Finding{{
		Code:   model.Coding{Code: "123"},
		Status: model.FindingStatus("invalid"),
		Value:  model.FindingValue{Kind: model.FindingValueString},
	}}
	assert.ErrorIs(t, badFindingStatus.Validate(), model.ErrInvalidSOAP)

	badFindingValue := valid
	badFindingValue.Findings = []model.Finding{{
		Code:   model.Coding{Code: "123"},
		Status: model.FindingStatusRecorded,
		Value:  model.FindingValue{Kind: model.FindingValueKind("invalid")},
	}}
	assert.ErrorIs(t, badFindingValue.Validate(), model.ErrInvalidSOAP)

	badFindingCode := valid
	badFindingCode.Findings = []model.Finding{{
		Code:   model.Coding{},
		Status: model.FindingStatusRecorded,
		Value:  model.FindingValue{Kind: model.FindingValueString},
	}}
	assert.ErrorIs(t, badFindingCode.Validate(), model.ErrInvalidSOAP)

	// 4. Problem validation failures
	badProbClinical := valid
	badProbClinical.Problems = []model.Problem{{
		ClinicalStatus:     model.ProblemClinicalStatus("invalid"),
		VerificationStatus: model.ProblemVerificationConfirmed,
		Category:           model.ProblemCategoryEncounter,
		Rank:               1,
	}}
	assert.ErrorIs(t, badProbClinical.Validate(), model.ErrInvalidSOAP)

	badProbVerification := valid
	badProbVerification.Problems = []model.Problem{{
		ClinicalStatus:     model.ProblemClinicalActive,
		VerificationStatus: model.ProblemVerificationStatus("invalid"),
		Category:           model.ProblemCategoryEncounter,
		Rank:               1,
	}}
	assert.ErrorIs(t, badProbVerification.Validate(), model.ErrInvalidSOAP)

	badProbCategory := valid
	badProbCategory.Problems = []model.Problem{{
		ClinicalStatus:     model.ProblemClinicalActive,
		VerificationStatus: model.ProblemVerificationConfirmed,
		Category:           model.ProblemCategory("invalid"),
		Rank:               1,
	}}
	assert.ErrorIs(t, badProbCategory.Validate(), model.ErrInvalidSOAP)

	badProbRank := valid
	badProbRank.Problems = []model.Problem{{
		ClinicalStatus:     model.ProblemClinicalActive,
		VerificationStatus: model.ProblemVerificationConfirmed,
		Category:           model.ProblemCategoryEncounter,
		Rank:               0,
	}}
	assert.ErrorIs(t, badProbRank.Validate(), model.ErrInvalidSOAP)

	// 5. PlanItem validation failures
	badPlanKind := valid
	badPlanKind.PlanItems = []model.PlanItem{{
		Kind:   model.PlanItemKind("invalid"),
		Status: model.PlanItemStatusActive,
	}}
	assert.ErrorIs(t, badPlanKind.Validate(), model.ErrInvalidSOAP)

	badPlanStatus := valid
	badPlanStatus.PlanItems = []model.PlanItem{{
		Kind:   model.PlanItemKindInstruction,
		Status: model.PlanItemStatus("invalid"),
	}}
	assert.ErrorIs(t, badPlanStatus.Validate(), model.ErrInvalidSOAP)
}

func TestCoding(t *testing.T) {
	c1 := model.Coding{}
	assert.True(t, c1.Empty())

	c2 := model.Coding{Code: "ABC"}
	assert.False(t, c2.Empty())

	c3 := model.Coding{System: "http://example.org"}
	assert.False(t, c3.Empty())
}
