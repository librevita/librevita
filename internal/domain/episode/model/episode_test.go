package model

import (
	"testing"
	"time"

	"librevita.org/pkg/ident"
)

func TestEpisodeValidate(t *testing.T) {
	t.Parallel()
	raw := "01990000-0000-7000-8000-000000000001"
	valid := Episode{
		ID:         ident.MustParseEpisode(raw),
		ClinicID:   ident.MustParseClinic(raw),
		PatientID:  ident.MustParsePatient(raw),
		AuthorID:   ident.MustParseUser(raw),
		Type:       EpisodeTypeConsultation,
		Status:     EpisodeStatusDraft,
		Class:      CareSettingAmbulatory,
		OccurredAt: time.Now(),
		Findings: []Finding{{
			Code:   Coding{System: "http://loinc.org", Code: "8480-6"},
			Status: FindingStatusRecorded,
			Value:  FindingValue{Kind: FindingValueQuantity},
		}},
		Problems: []Problem{{
			ClinicalStatus:     ProblemClinicalActive,
			VerificationStatus: ProblemVerificationConfirmed,
			Category:           ProblemCategoryEncounter,
			Rank:               1,
		}},
		PlanItems: []PlanItem{{
			Kind:   PlanItemKindInstruction,
			Status: PlanItemStatusActive,
		}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid episode: %v", err)
	}

	bad := valid
	bad.Class = CareSetting("ward")
	if err := bad.Validate(); err != ErrInvalidSOAP {
		t.Fatalf("got %v want ErrInvalidSOAP", err)
	}

	badRank := valid
	badRank.Problems = []Problem{{
		ClinicalStatus:     ProblemClinicalActive,
		VerificationStatus: ProblemVerificationConfirmed,
		Category:           ProblemCategoryEncounter,
		Rank:               0,
	}}
	if err := badRank.Validate(); err != ErrInvalidSOAP {
		t.Fatalf("rank 0: got %v want ErrInvalidSOAP", err)
	}
}
