package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	clinicmodel "librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/episode/delivery/views"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/pkg/ident"
)

const occurredLayout = "2006-01-02T15:04"

func parseForm(c echo.Context) views.EpisodeFormValues {
	_ = c.Request().ParseForm()
	v := views.EpisodeFormValues{
		Type:       c.FormValue("episode_type"),
		Class:      c.FormValue("class"),
		OccurredAt: c.FormValue("occurred_at"),
		Subjective: c.FormValue("subjective"),
		Objective:  c.FormValue("objective"),
		Assessment: c.FormValue("assessment"),
		Plan:       c.FormValue("plan"),
	}
	codes := formList(c, "finding_code")
	displays := formList(c, "finding_display")
	vals := formList(c, "finding_value")
	n := maxLen(len(codes), len(displays), len(vals))
	for i := 0; i < n; i++ {
		row := views.FindingForm{
			Code:    at(codes, i),
			Display: at(displays, i),
			Value:   at(vals, i),
		}
		if row.Code == "" && row.Display == "" && row.Value == "" {
			continue
		}
		v.Findings = append(v.Findings, row)
	}
	pCodes := formList(c, "problem_code")
	pDisp := formList(c, "problem_display")
	pText := formList(c, "problem_text")
	n = maxLen(len(pCodes), len(pDisp), len(pText))
	for i := 0; i < n; i++ {
		row := views.ProblemForm{
			Code:    at(pCodes, i),
			Display: at(pDisp, i),
			Text:    at(pText, i),
		}
		if row.Code == "" && row.Display == "" && row.Text == "" {
			continue
		}
		v.Problems = append(v.Problems, row)
	}
	kinds := formList(c, "plan_kind")
	descs := formList(c, "plan_description")
	n = maxLen(len(kinds), len(descs))
	for i := 0; i < n; i++ {
		row := views.PlanForm{Kind: at(kinds, i), Description: at(descs, i)}
		if row.Kind == "" && row.Description == "" {
			continue
		}
		v.PlanItems = append(v.PlanItems, row)
	}
	return v
}

func formList(c echo.Context, name string) []string {
	return c.Request().PostForm[name]
}

func at(ss []string, i int) string {
	if i >= len(ss) {
		return ""
	}
	return strings.TrimSpace(ss[i])
}

func maxLen(ns ...int) int {
	m := 0
	for _, n := range ns {
		if n > m {
			m = n
		}
	}
	return m
}

func applyAdd(v views.EpisodeFormValues, add string) views.EpisodeFormValues {
	switch add {
	case "finding":
		v.Findings = append(v.Findings, views.FindingForm{})
	case "problem":
		v.Problems = append(v.Problems, views.ProblemForm{})
	case "plan":
		v.PlanItems = append(v.PlanItems, views.PlanForm{Kind: string(episodemodel.PlanItemKindInstruction)})
	}
	return v
}

func valuesFromEpisode(ep episodemodel.Episode, clock *clinicmodel.Clock) views.EpisodeFormValues {
	v := views.EpisodeFormValues{
		Type:       ep.Type.String(),
		Class:      ep.Class.String(),
		OccurredAt: clock.Format(ep.OccurredAt, occurredLayout),
		Subjective: ep.SOAP.Subjective,
		Objective:  ep.SOAP.Objective,
		Assessment: ep.SOAP.Assessment,
		Plan:       ep.SOAP.Plan,
	}
	for _, f := range ep.Findings {
		v.Findings = append(v.Findings, views.FindingForm{
			Code:    f.Code.Code,
			Display: f.Code.Display,
			Value:   findingValueText(f),
		})
	}
	for _, p := range ep.Problems {
		v.Problems = append(v.Problems, views.ProblemForm{
			Code:    p.Code.Code,
			Display: p.Code.Display,
			Text:    p.Text,
		})
	}
	for _, item := range ep.PlanItems {
		v.PlanItems = append(v.PlanItems, views.PlanForm{
			Kind:        item.Kind.String(),
			Description: item.Description,
		})
	}
	return v
}

func findingValueText(f episodemodel.Finding) string {
	switch f.Value.Kind {
	case episodemodel.FindingValueQuantity:
		if f.Value.Quantity != nil {
			s := strconv.FormatFloat(f.Value.Quantity.Value, 'G', -1, 64)
			if f.Value.Quantity.Unit != "" {
				return s + " " + f.Value.Quantity.Unit
			}
			return s
		}
	case episodemodel.FindingValueBoolean:
		if f.Value.Boolean != nil && *f.Value.Boolean {
			return "true"
		}
		if f.Value.Boolean != nil {
			return "false"
		}
	case episodemodel.FindingValueCoded:
		if f.Value.Coded != nil {
			return f.Value.Coded.Code
		}
	}
	return f.Value.String
}

func episodeFromForm(v views.EpisodeFormValues, clinicID ident.ClinicID, patientID ident.PatientID, authorID ident.UserID, clock *clinicmodel.Clock) episodemodel.Episode {
	occurred := clock.Now().UTC()
	if t, err := time.ParseInLocation(occurredLayout, v.OccurredAt, clock.Zone()); err == nil {
		occurred = t.UTC()
	}
	typ, ok := episodemodel.ParseEpisodeType(v.Type)
	if !ok {
		typ = episodemodel.EpisodeTypeConsultation
	}
	class, ok := episodemodel.ParseCareSetting(v.Class)
	if !ok {
		class = episodemodel.CareSettingAmbulatory
	}
	ep := episodemodel.Episode{
		ClinicID:   clinicID,
		PatientID:  patientID,
		AuthorID:   authorID,
		Type:       typ,
		Class:      class,
		OccurredAt: occurred,
		SOAP: episodemodel.SOAP{
			Subjective: v.Subjective,
			Objective:  v.Objective,
			Assessment: v.Assessment,
			Plan:       v.Plan,
		},
	}
	for _, row := range v.Findings {
		if strings.TrimSpace(row.Code) == "" {
			continue
		}
		ep.Findings = append(ep.Findings, episodemodel.Finding{
			Code:   episodemodel.Coding{Code: row.Code, Display: row.Display},
			Status: episodemodel.FindingStatusRecorded,
			Value:  episodemodel.FindingValue{Kind: episodemodel.FindingValueString, String: row.Value},
		})
	}
	for i, row := range v.Problems {
		ep.Problems = append(ep.Problems, episodemodel.Problem{
			Code:               episodemodel.Coding{Code: row.Code, Display: row.Display},
			Text:               row.Text,
			ClinicalStatus:     episodemodel.ProblemClinicalActive,
			VerificationStatus: episodemodel.ProblemVerificationConfirmed,
			Category:           episodemodel.ProblemCategoryEncounter,
			Rank:               i + 1,
		})
	}
	for _, row := range v.PlanItems {
		kind, ok := episodemodel.ParsePlanItemKind(row.Kind)
		if !ok {
			kind = episodemodel.PlanItemKindInstruction
		}
		if kind == episodemodel.PlanItemKindInstruction && strings.TrimSpace(row.Description) == "" {
			continue
		}
		ep.PlanItems = append(ep.PlanItems, episodemodel.PlanItem{
			Kind:        kind,
			Description: row.Description,
			Status:      episodemodel.PlanItemStatusActive,
		})
	}
	return ep
}
