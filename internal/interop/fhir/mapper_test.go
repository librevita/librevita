package fhir

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	episodemodel "librevita.org/internal/domain/episode/model"
)

func TestSOAPBundleRoundTrip(t *testing.T) {
	t.Parallel()
	episodeID := ident.MustParseEpisode("01990000-0000-7000-8000-0000000000aa")
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000aa")
	patientID := ident.MustParsePatient("01990000-0000-7000-8000-0000000000bb")
	authorID := ident.MustParseUser("01990000-0000-7000-8000-0000000000cc")
	findingID := ident.MustParseFinding("01990000-0000-7000-8000-0000000000dd")
	problemID := ident.MustParseProblem("01990000-0000-7000-8000-0000000000ee")
	planID := ident.MustParsePlanItem("01990000-0000-7000-8000-0000000000ff")
	pred := ident.MustParseEpisode("01990000-0000-7000-8000-000000000099")
	occurred := time.Date(2026, 8, 28, 15, 0, 0, 0, time.UTC)
	qty := 120.0
	ep := episodemodel.Episode{
		ID:            episodeID,
		ClinicID:      clinicID,
		PatientID:     patientID,
		AuthorID:      authorID,
		PredecessorID: &pred,
		Type:          episodemodel.EpisodeTypeEvolution,
		Status:        episodemodel.EpisodeStatusDraft,
		Class:         episodemodel.CareSettingAmbulatory,
		OccurredAt:    occurred,
		SOAP: episodemodel.SOAP{
			Subjective: "Cefaleia há 2 dias",
			Objective:  "PA 120x80",
			Assessment: "Enxaqueca",
			Plan:       "Dipirona se dor",
		},
		Findings: []episodemodel.Finding{{
			ID:     findingID,
			Code:   episodemodel.Coding{System: SystemLOINC, Code: "8480-6", Display: "Systolic blood pressure"},
			Status: episodemodel.FindingStatusRecorded,
			Value: episodemodel.FindingValue{
				Kind:     episodemodel.FindingValueQuantity,
				Quantity: &episodemodel.Quantity{Value: qty, Unit: "mmHg", Code: "mm[Hg]", System: SystemUCUM},
			},
			EffectiveAt: occurred,
		}},
		Problems: []episodemodel.Problem{{
			ID:                 problemID,
			Code:               episodemodel.Coding{System: SystemICD10, Code: "G43.9", Display: "Migraine"},
			Text:               "Enxaqueca",
			ClinicalStatus:     episodemodel.ProblemClinicalActive,
			VerificationStatus: episodemodel.ProblemVerificationConfirmed,
			Category:           episodemodel.ProblemCategoryEncounter,
			Rank:               1,
		}},
		PlanItems: []episodemodel.PlanItem{{
			ID:          planID,
			Kind:        episodemodel.PlanItemKindMedication,
			Description: "Dipirona 500mg",
			Status:      episodemodel.PlanItemStatusActive,
		}},
	}

	bundle, err := ToDocumentBundle(ep, DocumentContext{PatientName: "Ana", AuthorName: "Dr. Silva"})
	require.NoError(t, err)
	require.Equal(t, "document", bundle.Type)
	require.Equal(t, "Composition/"+episodeID.String(), bundle.Entry[0].FullURL)
	assert.Equal(t, "Composition", PeekType(bundle.Entry[0].Resource))
	var obs Observation
	require.NoError(t, json.Unmarshal(bundle.Entry[2].Resource, &obs))
	assert.Equal(t, "final", obs.Status)
	var cond Condition
	require.NoError(t, json.Unmarshal(bundle.Entry[3].Resource, &cond))
	require.NotEmpty(t, cond.Category)
	assert.Equal(t, "encounter-diagnosis", firstCode(&cond.Category[0]))

	got, err := FromDocumentBundle(bundle)
	require.NoError(t, err)
	assert.Equal(t, ep.ID, got.ID)
	assert.Equal(t, ep.PatientID, got.PatientID)
	assert.Equal(t, ep.AuthorID, got.AuthorID)
	assert.Equal(t, ep.Type, got.Type)
	assert.Equal(t, ep.Status, got.Status)
	assert.Equal(t, ep.Class, got.Class)
	assert.Equal(t, ep.SOAP, got.SOAP)
	require.NotNil(t, got.PredecessorID)
	assert.Equal(t, pred, *got.PredecessorID)
	require.Len(t, got.Findings, 1)
	assert.Equal(t, findingID, got.Findings[0].ID)
	assert.Equal(t, "8480-6", got.Findings[0].Code.Code)
	assert.Equal(t, episodemodel.FindingStatusRecorded, got.Findings[0].Status)
	require.NotNil(t, got.Findings[0].Value.Quantity)
	assert.Equal(t, qty, got.Findings[0].Value.Quantity.Value)
	require.Len(t, got.Problems, 1)
	assert.Equal(t, "G43.9", got.Problems[0].Code.Code)
	assert.Equal(t, "Enxaqueca", got.Problems[0].Text)
	assert.Equal(t, episodemodel.ProblemCategoryEncounter, got.Problems[0].Category)
	require.Len(t, got.PlanItems, 1)
	assert.Equal(t, episodemodel.PlanItemKindMedication, got.PlanItems[0].Kind)
	assert.Equal(t, "Dipirona 500mg", got.PlanItems[0].Description)
	assert.False(t, WantFinalize(bundle))
}

func TestWantFinalize(t *testing.T) {
	t.Parallel()
	raw := "01990000-0000-7000-8000-0000000000aa"
	ep := episodemodel.Episode{
		ID: ident.MustParseEpisode(raw), ClinicID: ident.MustParseClinic(raw), PatientID: ident.MustParsePatient(raw), AuthorID: ident.MustParseUser(raw),
		Type: episodemodel.EpisodeTypeConsultation, Status: episodemodel.EpisodeStatusFinalized,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: time.Now().UTC(),
	}
	b, err := ToDocumentBundle(ep, DocumentContext{})
	require.NoError(t, err)
	assert.True(t, WantFinalize(b))
}

func TestFromDocumentBundleRequiresCompositionAndEncounter(t *testing.T) {
	t.Parallel()
	_, err := FromDocumentBundle(&Bundle{ResourceType: "Bundle", Type: "document"})
	assert.ErrorIs(t, err, episodemodel.ErrInvalidSOAP)
}

func TestVocabularyMaps(t *testing.T) {
	t.Parallel()
	t.Run("finding status", func(t *testing.T) {
		assert.Equal(t, "final", fhirFindingStatus(episodemodel.FindingStatusRecorded))
		assert.Equal(t, "preliminary", fhirFindingStatus(episodemodel.FindingStatusProvisional))
		assert.Equal(t, "cancelled", fhirFindingStatus(episodemodel.FindingStatusCancelled))
		assert.Equal(t, episodemodel.FindingStatusRecorded, domainFindingStatus("final"))
		assert.Equal(t, episodemodel.FindingStatusRecorded, domainFindingStatus("amended"))
		assert.Equal(t, episodemodel.FindingStatusProvisional, domainFindingStatus("preliminary"))
		assert.Equal(t, episodemodel.FindingStatusProvisional, domainFindingStatus("registered"))
		assert.Equal(t, episodemodel.FindingStatusCancelled, domainFindingStatus("cancelled"))
		assert.Equal(t, episodemodel.FindingStatusCancelled, domainFindingStatus("entered-in-error"))
	})
	t.Run("problem clinical", func(t *testing.T) {
		assert.Equal(t, "active", fhirClinicalStatus(episodemodel.ProblemClinicalActive))
		assert.Equal(t, "inactive", fhirClinicalStatus(episodemodel.ProblemClinicalInactive))
		assert.Equal(t, "resolved", fhirClinicalStatus(episodemodel.ProblemClinicalResolved))
		assert.Equal(t, episodemodel.ProblemClinicalActive, domainClinicalStatus("relapse"))
		assert.Equal(t, episodemodel.ProblemClinicalActive, domainClinicalStatus("recurrence"))
		assert.Equal(t, episodemodel.ProblemClinicalInactive, domainClinicalStatus("remission"))
		assert.Equal(t, episodemodel.ProblemClinicalResolved, domainClinicalStatus("resolved"))
	})
	t.Run("problem verification", func(t *testing.T) {
		assert.Equal(t, "unconfirmed", fhirVerification(episodemodel.ProblemVerificationSuspected))
		assert.Equal(t, "entered-in-error", fhirVerification(episodemodel.ProblemVerificationError))
		assert.Equal(t, episodemodel.ProblemVerificationSuspected, domainVerification("differential"))
		assert.Equal(t, episodemodel.ProblemVerificationSuspected, domainVerification("provisional"))
		assert.Equal(t, episodemodel.ProblemVerificationError, domainVerification("entered-in-error"))
		assert.Equal(t, episodemodel.ProblemVerificationConfirmed, domainVerification("confirmed"))
	})
	t.Run("problem category", func(t *testing.T) {
		assert.Equal(t, "encounter-diagnosis", fhirProblemCategory(episodemodel.ProblemCategoryEncounter))
		assert.Equal(t, "problem-list-item", fhirProblemCategory(episodemodel.ProblemCategoryList))
		assert.Equal(t, episodemodel.ProblemCategoryList, domainProblemCategory("problem-list-item"))
		assert.Equal(t, episodemodel.ProblemCategoryEncounter, domainProblemCategory("encounter-diagnosis"))
	})
	t.Run("plan kind", func(t *testing.T) {
		assert.Equal(t, "ServiceRequest", fhirPlanKind(episodemodel.PlanItemKindExam))
		assert.Equal(t, "ServiceRequest", fhirPlanKind(episodemodel.PlanItemKindProcedure))
		assert.Equal(t, episodemodel.PlanItemKindProcedure, domainPlanKind("ServiceRequest"))
	})
}
