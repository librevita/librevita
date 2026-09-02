package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"

	clinicmodel "librevita.org/internal/domain/clinic/model"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/pkg/ident"
)

func TestFormParsingAndDynamics(t *testing.T) {
	e := echo.New()

	form := url.Values{
		"episode_type":     {"consultation"},
		"class":            {"ambulatory"},
		"occurred_at":      {"2026-08-28T15:00"},
		"subjective":       {"Patient reports headache"},
		"objective":        {"BP 120/80"},
		"assessment":       {"Tension headache"},
		"plan":             {"Rest and hydration"},
		"finding_code":     {"8480-6"},
		"finding_display":  {"Systolic BP"},
		"finding_value":    {"120"},
		"problem_code":     {"G44.2"},
		"problem_display":  {"Tension headache"},
		"problem_text":     {"Primary complaint"},
		"plan_kind":        {"instruction"},
		"plan_description": {"Drink 2L water daily"},
	}

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c := e.NewContext(req, httptest.NewRecorder())

	v := parseForm(c)
	assert.Equal(t, "consultation", v.Type)
	assert.Equal(t, "ambulatory", v.Class)
	assert.Equal(t, "Patient reports headache", v.Subjective)
	assert.Len(t, v.Findings, 1)
	assert.Equal(t, "8480-6", v.Findings[0].Code)
	assert.Len(t, v.Problems, 1)
	assert.Equal(t, "G44.2", v.Problems[0].Code)
	assert.Len(t, v.PlanItems, 1)
	assert.Equal(t, "instruction", v.PlanItems[0].Kind)

	// Test applyAdd
	v2 := applyAdd(v, "finding")
	assert.Len(t, v2.Findings, 2)
	v3 := applyAdd(v, "problem")
	assert.Len(t, v3.Problems, 2)
	v4 := applyAdd(v, "plan")
	assert.Len(t, v4.PlanItems, 2)

	// Test episodeFromForm
	clinicID := ident.New[ident.ClinicID]()
	patientID := ident.New[ident.PatientID]()
	authorID := ident.New[ident.UserID]()
	clock := clinicmodel.NewClock("America/Sao_Paulo")

	ep := episodeFromForm(v, clinicID, patientID, authorID, clock)
	assert.Equal(t, episodemodel.EpisodeTypeConsultation, ep.Type)
	assert.Equal(t, episodemodel.CareSettingAmbulatory, ep.Class)
	assert.Equal(t, "Patient reports headache", ep.SOAP.Subjective)
	assert.Len(t, ep.Findings, 1)
	assert.Len(t, ep.Problems, 1)
	assert.Len(t, ep.PlanItems, 1)

	// Test valuesFromEpisode
	fv := valuesFromEpisode(ep, clock)
	assert.Equal(t, "consultation", fv.Type)
	assert.Equal(t, "ambulatory", fv.Class)
	assert.Len(t, fv.Findings, 1)
	assert.Len(t, fv.Problems, 1)
	assert.Len(t, fv.PlanItems, 1)

	// Test findingValueText for Quantity, Boolean, Coded, and String
	qty := 120.5
	fQty := episodemodel.Finding{
		Value: episodemodel.FindingValue{
			Kind:     episodemodel.FindingValueQuantity,
			Quantity: &episodemodel.Quantity{Value: qty, Unit: "mmHg"},
		},
	}
	assert.Equal(t, "120.5 mmHg", findingValueText(fQty))

	bTrue := true
	fBool := episodemodel.Finding{
		Value: episodemodel.FindingValue{
			Kind:    episodemodel.FindingValueBoolean,
			Boolean: &bTrue,
		},
	}
	assert.Equal(t, "true", findingValueText(fBool))

	bFalse := false
	fBoolFalse := episodemodel.Finding{
		Value: episodemodel.FindingValue{
			Kind:    episodemodel.FindingValueBoolean,
			Boolean: &bFalse,
		},
	}
	assert.Equal(t, "false", findingValueText(fBoolFalse))

	fCoded := episodemodel.Finding{
		Value: episodemodel.FindingValue{
			Kind:  episodemodel.FindingValueCoded,
			Coded: &episodemodel.Coding{Code: "POSITIVE"},
		},
	}
	assert.Equal(t, "POSITIVE", findingValueText(fCoded))
}
