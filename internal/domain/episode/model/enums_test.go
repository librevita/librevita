package model

import "testing"

func TestEnumParseRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ok   bool
		fn   func() bool
	}{
		{"episode type", true, func() bool {
			v, ok := ParseEpisodeType("evolution")
			return ok && v == EpisodeTypeEvolution && v.Valid()
		}},
		{"episode type bad", false, func() bool {
			_, ok := ParseEpisodeType("soap")
			return ok
		}},
		{"status", true, func() bool {
			v, ok := ParseEpisodeStatus("draft")
			return ok && v.Valid()
		}},
		{"class", true, func() bool {
			v, ok := ParseCareSetting("ambulatory")
			return ok && v.Valid()
		}},
		{"finding status", true, func() bool {
			v, ok := ParseFindingStatus("recorded")
			return ok && v.Valid()
		}},
		{"finding status fhir token", false, func() bool {
			_, ok := ParseFindingStatus("final")
			return ok
		}},
		{"finding value kind", true, func() bool {
			v, ok := ParseFindingValueKind("quantity")
			return ok && v.Valid()
		}},
		{"problem clinical", true, func() bool {
			v, ok := ParseProblemClinicalStatus("active")
			return ok && v.Valid()
		}},
		{"problem clinical fhir extra", false, func() bool {
			_, ok := ParseProblemClinicalStatus("relapse")
			return ok
		}},
		{"problem verification", true, func() bool {
			v, ok := ParseProblemVerificationStatus("error")
			return ok && v.Valid()
		}},
		{"problem category", true, func() bool {
			v, ok := ParseProblemCategory("encounter")
			return ok && v.Valid()
		}},
		{"plan kind", true, func() bool {
			v, ok := ParsePlanItemKind("medication")
			return ok && v.Valid()
		}},
		{"plan status", true, func() bool {
			v, ok := ParsePlanItemStatus("active")
			return ok && v.Valid()
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fn()
			if got != tc.ok {
				t.Fatalf("got %v want %v", got, tc.ok)
			}
		})
	}
}

func TestCodingEmpty(t *testing.T) {
	t.Parallel()
	if !(Coding{}).Empty() {
		t.Fatal("zero coding should be empty")
	}
	if (Coding{System: "http://loinc.org", Code: "8480-6"}).Empty() {
		t.Fatal("populated coding should not be empty")
	}
}
