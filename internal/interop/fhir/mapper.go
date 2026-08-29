package fhir

import (
	"encoding/json"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"

	episodemodel "librevita.org/internal/domain/episode/model"
)

// DocumentContext is optional display data for outbound mapping.
type DocumentContext struct {
	PatientName string
	AuthorName  string
}

// ToDocumentBundle maps a SOAP episode to a FHIR R4 document Bundle.
func ToDocumentBundle(ep episodemodel.Episode, ctx DocumentContext) (*Bundle, error) {
	id := ep.ID.String()
	patientRef := &Reference{Reference: "Patient/" + ep.PatientID.String(), Display: ctx.PatientName}
	encRef := &Reference{Reference: "Encounter/" + id}
	authorRef := Reference{Reference: "Practitioner/" + ep.AuthorID.String(), Display: ctx.AuthorName}
	when := formatTime(ep.OccurredAt)

	enc := Encounter{
		ResourceType: "Encounter",
		ID:           id,
		Status:       encounterStatus(ep.Status),
		Class:        classCoding(ep.Class),
		Type:         []CodeableConcept{CodeableConceptFrom(SystemEpisodeType, ep.Type.String(), ep.Type.String())},
		Subject:      patientRef,
		Participant:  []EncounterParticipant{{Individual: &authorRef}},
		Period:       &Period{Start: when},
	}
	if ep.EndedAt != nil {
		enc.Period.End = formatTime(*ep.EndedAt)
	}

	var obsEntries []Reference
	var condEntries []Reference
	entries := make([]BundleEntry, 0, 8+len(ep.Findings)+len(ep.Problems))

	comp := Composition{
		ResourceType:    "Composition",
		ID:              id,
		Status:          compositionStatus(ep.Status),
		Type:            CodeableConceptFrom(SystemLOINC, LOINCProgressNote, "Progress note"),
		Subject:         patientRef,
		Encounter:       encRef,
		Date:            when,
		Author:          []Reference{authorRef},
		Title:           "SOAP note",
		Confidentiality: "R",
		Section: []CompositionSection{
			section(LOINCSubjective, "Subjective", ep.SOAP.Subjective, nil),
			section(LOINCObjective, "Objective", ep.SOAP.Objective, nil),
			section(LOINCAssessment, "Assessment", ep.SOAP.Assessment, nil),
			section(LOINCPlan, "Plan", ep.SOAP.Plan, nil),
		},
	}
	if ep.PredecessorID != nil {
		pred := ep.PredecessorID.String()
		comp.RelatesTo = []CompositionRelatesTo{{
			Code:   "replaces",
			Target: &Reference{Reference: "Composition/" + pred},
		}}
	}

	for _, f := range ep.Findings {
		obs := observationFromFinding(f, ep)
		raw, err := json.Marshal(obs)
		if err != nil {
			return nil, err
		}
		entries = append(entries, BundleEntry{FullURL: "Observation/" + f.ID.String(), Resource: raw})
		obsEntries = append(obsEntries, Reference{Reference: "Observation/" + f.ID.String()})
	}
	comp.Section[1].Entry = obsEntries

	for _, p := range ep.Problems {
		cond := conditionFromProblem(p, ep)
		raw, err := json.Marshal(cond)
		if err != nil {
			return nil, err
		}
		entries = append(entries, BundleEntry{FullURL: "Condition/" + p.ID.String(), Resource: raw})
		condEntries = append(condEntries, Reference{Reference: "Condition/" + p.ID.String()})
	}
	comp.Section[2].Entry = condEntries

	impression := ClinicalImpression{
		ResourceType: "ClinicalImpression",
		ID:           id,
		Status:       impressionStatus(ep.Status),
		Subject:      patientRef,
		Encounter:    encRef,
		Date:         when,
		Summary:      ep.SOAP.Assessment,
		Problem:      condEntries,
	}

	plan := CarePlan{
		ResourceType: "CarePlan",
		ID:           id,
		Status:       carePlanStatus(ep.Status),
		Intent:       "plan",
		Subject:      patientRef,
		Encounter:    encRef,
		Description:  ep.SOAP.Plan,
	}
	for _, item := range ep.PlanItems {
		plan.Activity = append(plan.Activity, carePlanActivity(item))
	}
	if len(plan.Activity) > 0 || ep.SOAP.Plan != "" {
		comp.Section[3].Entry = []Reference{{Reference: "CarePlan/" + id}}
	}

	compRaw, err := json.Marshal(comp)
	if err != nil {
		return nil, err
	}
	encRaw, err := json.Marshal(enc)
	if err != nil {
		return nil, err
	}
	impRaw, err := json.Marshal(impression)
	if err != nil {
		return nil, err
	}
	planRaw, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}

	out := &Bundle{
		ResourceType: "Bundle",
		ID:           id,
		Type:         "document",
		Timestamp:    when,
		Entry: []BundleEntry{
			{FullURL: "Composition/" + id, Resource: compRaw},
			{FullURL: "Encounter/" + id, Resource: encRaw},
		},
	}
	out.Entry = append(out.Entry, entries...)
	out.Entry = append(out.Entry,
		BundleEntry{FullURL: "ClinicalImpression/" + id, Resource: impRaw},
		BundleEntry{FullURL: "CarePlan/" + id, Resource: planRaw},
	)
	return out, nil
}

// FromDocumentBundle maps a FHIR document (or transaction) Bundle onto an Episode.
func FromDocumentBundle(b *Bundle) (*episodemodel.Episode, error) {
	if b == nil {
		return nil, episodemodel.ErrInvalidSOAP
	}
	var comp Composition
	var enc Encounter
	var plan CarePlan
	var observations []Observation
	var conditions []Condition
	var hasComp, hasEnc bool
	for _, e := range b.Entry {
		switch PeekType(e.Resource) {
		case "Composition":
			if err := json.Unmarshal(e.Resource, &comp); err != nil {
				return nil, episodemodel.ErrInvalidSOAP
			}
			hasComp = true
		case "Encounter":
			if err := json.Unmarshal(e.Resource, &enc); err != nil {
				return nil, episodemodel.ErrInvalidSOAP
			}
			hasEnc = true
		case "Observation":
			var o Observation
			if err := json.Unmarshal(e.Resource, &o); err != nil {
				return nil, episodemodel.ErrInvalidSOAP
			}
			observations = append(observations, o)
		case "Condition":
			var c Condition
			if err := json.Unmarshal(e.Resource, &c); err != nil {
				return nil, episodemodel.ErrInvalidSOAP
			}
			conditions = append(conditions, c)
		case "CarePlan":
			if err := json.Unmarshal(e.Resource, &plan); err != nil {
				return nil, episodemodel.ErrInvalidSOAP
			}
		}
	}
	if !hasComp || !hasEnc {
		return nil, episodemodel.ErrInvalidSOAP
	}

	ep := &episodemodel.Episode{
		Type:   episodemodel.EpisodeTypeConsultation,
		Status: statusFromComposition(comp.Status),
		Class:  classFromCoding(enc.Class),
		SOAP:   soapFromSections(comp.Section),
	}
	if id, ok := parseUUID(comp.ID); ok {
		ep.ID = id
	} else if id, ok := parseUUID(enc.ID); ok {
		ep.ID = id
	}
	if pid, ok := parseTypedRef(enc.Subject, "Patient"); ok {
		ep.PatientID = pid
	} else if pid, ok := parseTypedRef(comp.Subject, "Patient"); ok {
		ep.PatientID = pid
	}
	if aid, ok := parseTypedRef(firstAuthor(comp, enc), "Practitioner"); ok {
		ep.AuthorID = aid
	}
	if pred := predecessorFromRelatesTo(comp.RelatesTo); pred != nil {
		ep.PredecessorID = pred
	}
	if t, ok := parseTime(comp.Date); ok {
		ep.OccurredAt = t
	} else if enc.Period != nil {
		if t, ok := parseTime(enc.Period.Start); ok {
			ep.OccurredAt = t
		}
	}
	if enc.Period != nil {
		if t, ok := parseTime(enc.Period.End); ok {
			ep.EndedAt = &t
		}
	}
	if typ, ok := episodeTypeFrom(enc.Type); ok {
		ep.Type = typ
	}
	for _, o := range observations {
		ep.Findings = append(ep.Findings, findingFromObservation(o))
	}
	for i, c := range conditions {
		p := problemFromCondition(c)
		if p.Rank < 1 {
			p.Rank = i + 1
		}
		ep.Problems = append(ep.Problems, p)
	}
	for _, act := range plan.Activity {
		if act.Detail == nil {
			continue
		}
		ep.PlanItems = append(ep.PlanItems, planItemFromDetail(*act.Detail))
	}
	return ep, nil
}

func section(code, title, text string, entry []Reference) CompositionSection {
	s := CompositionSection{
		Title: title,
		Code:  ptrConcept(CodeableConceptFrom(SystemLOINC, code, title)),
		Entry: entry,
	}
	if text != "" {
		escaped := html.EscapeString(text)
		s.Text = &Narrative{
			Status: "generated",
			Div:    `<div xmlns="http://www.w3.org/1999/xhtml"><p>` + escaped + `</p></div>`,
		}
	}
	return s
}

func soapFromSections(sections []CompositionSection) episodemodel.SOAP {
	var soap episodemodel.SOAP
	for _, s := range sections {
		code := firstCode(s.Code)
		text := textFromNarrative(s.Text)
		switch code {
		case LOINCSubjective:
			soap.Subjective = text
		case LOINCObjective:
			soap.Objective = text
		case LOINCAssessment:
			soap.Assessment = text
		case LOINCPlan:
			soap.Plan = text
		}
	}
	return soap
}

func observationFromFinding(f episodemodel.Finding, ep episodemodel.Episode) Observation {
	obs := Observation{
		ResourceType:      "Observation",
		ID:                f.ID.String(),
		Status:            fhirFindingStatus(f.Status),
		Code:              codingToConcept(f.Code),
		Subject:           &Reference{Reference: "Patient/" + ep.PatientID.String()},
		Encounter:         &Reference{Reference: "Encounter/" + ep.ID.String()},
		EffectiveDateTime: formatTime(f.EffectiveAt),
	}
	switch f.Value.Kind {
	case episodemodel.FindingValueQuantity:
		if f.Value.Quantity != nil {
			v := f.Value.Quantity.Value
			obs.ValueQuantity = &Quantity{
				Value:  &v,
				Unit:   f.Value.Quantity.Unit,
				System: f.Value.Quantity.System,
				Code:   f.Value.Quantity.Code,
			}
			if obs.ValueQuantity.System == "" {
				obs.ValueQuantity.System = SystemUCUM
			}
		}
	case episodemodel.FindingValueString:
		obs.ValueString = f.Value.String
	case episodemodel.FindingValueBoolean:
		obs.ValueBoolean = f.Value.Boolean
	case episodemodel.FindingValueCoded:
		if f.Value.Coded != nil {
			c := codingToConcept(*f.Value.Coded)
			obs.ValueCodeableConcept = &c
		}
	}
	return obs
}

func findingFromObservation(o Observation) episodemodel.Finding {
	f := episodemodel.Finding{
		Code:   conceptToCoding(o.Code),
		Status: domainFindingStatus(o.Status),
	}
	if id, ok := parseUUID(o.ID); ok {
		f.ID = id
	}
	if t, ok := parseTime(o.EffectiveDateTime); ok {
		f.EffectiveAt = t
	}
	switch {
	case o.ValueQuantity != nil:
		q := &episodemodel.Quantity{
			Unit:   o.ValueQuantity.Unit,
			Code:   o.ValueQuantity.Code,
			System: o.ValueQuantity.System,
		}
		if o.ValueQuantity.Value != nil {
			q.Value = *o.ValueQuantity.Value
		}
		f.Value = episodemodel.FindingValue{Kind: episodemodel.FindingValueQuantity, Quantity: q}
	case o.ValueBoolean != nil:
		f.Value = episodemodel.FindingValue{Kind: episodemodel.FindingValueBoolean, Boolean: o.ValueBoolean}
	case o.ValueCodeableConcept != nil:
		c := conceptToCoding(*o.ValueCodeableConcept)
		f.Value = episodemodel.FindingValue{Kind: episodemodel.FindingValueCoded, Coded: &c}
	default:
		f.Value = episodemodel.FindingValue{Kind: episodemodel.FindingValueString, String: o.ValueString}
	}
	return f
}

func conditionFromProblem(p episodemodel.Problem, ep episodemodel.Episode) Condition {
	clin := CodeableConceptFrom(SystemConditionClin, fhirClinicalStatus(p.ClinicalStatus), p.ClinicalStatus.String())
	ver := CodeableConceptFrom(SystemConditionVer, fhirVerification(p.VerificationStatus), p.VerificationStatus.String())
	cat := fhirProblemCategory(p.Category)
	code := codingToConcept(p.Code)
	if p.Text != "" {
		code.Text = p.Text
	}
	return Condition{
		ResourceType:       "Condition",
		ID:                 p.ID.String(),
		ClinicalStatus:     &clin,
		VerificationStatus: &ver,
		Category:           []CodeableConcept{CodeableConceptFrom(SystemConditionCat, cat, cat)},
		Code:               &code,
		Subject:            &Reference{Reference: "Patient/" + ep.PatientID.String()},
		Encounter:          &Reference{Reference: "Encounter/" + ep.ID.String()},
	}
}

func problemFromCondition(c Condition) episodemodel.Problem {
	p := episodemodel.Problem{
		ClinicalStatus:     episodemodel.ProblemClinicalActive,
		VerificationStatus: episodemodel.ProblemVerificationConfirmed,
		Category:           episodemodel.ProblemCategoryEncounter,
		Rank:               1,
	}
	if id, ok := parseUUID(c.ID); ok {
		p.ID = id
	}
	if c.Code != nil {
		p.Code = conceptToCoding(*c.Code)
		p.Text = c.Code.Text
	}
	if c.ClinicalStatus != nil {
		p.ClinicalStatus = domainClinicalStatus(firstCode(c.ClinicalStatus))
	}
	if c.VerificationStatus != nil {
		p.VerificationStatus = domainVerification(firstCode(c.VerificationStatus))
	}
	if len(c.Category) > 0 {
		p.Category = domainProblemCategory(firstCode(&c.Category[0]))
	}
	return p
}

func carePlanActivity(item episodemodel.PlanItem) CarePlanActivity {
	d := &CarePlanActivityDetail{
		Kind:        fhirPlanKind(item.Kind),
		Status:      fhirPlanStatus(item.Status),
		Description: item.Description,
	}
	if !item.Code.Empty() {
		c := codingToConcept(item.Code)
		d.Code = &c
	}
	if item.ScheduledAt != nil {
		d.ScheduledString = formatTime(*item.ScheduledAt)
	}
	return CarePlanActivity{Detail: d}
}

func planItemFromDetail(d CarePlanActivityDetail) episodemodel.PlanItem {
	item := episodemodel.PlanItem{
		Kind:        domainPlanKind(d.Kind),
		Status:      domainPlanStatus(d.Status),
		Description: d.Description,
	}
	if d.Code != nil {
		item.Code = conceptToCoding(*d.Code)
	}
	if t, ok := parseTime(d.ScheduledString); ok {
		item.ScheduledAt = &t
	}
	return item
}

func compositionStatus(s episodemodel.EpisodeStatus) string {
	if s == episodemodel.EpisodeStatusDraft {
		return "preliminary"
	}
	return "final"
}

func encounterStatus(s episodemodel.EpisodeStatus) string {
	if s == episodemodel.EpisodeStatusDraft {
		return "in-progress"
	}
	return "finished"
}

func impressionStatus(s episodemodel.EpisodeStatus) string {
	if s == episodemodel.EpisodeStatusDraft {
		return "in-progress"
	}
	return "completed"
}

func carePlanStatus(s episodemodel.EpisodeStatus) string {
	if s == episodemodel.EpisodeStatusDraft {
		return "draft"
	}
	return "active"
}

func statusFromComposition(s string) episodemodel.EpisodeStatus {
	if s == "preliminary" {
		return episodemodel.EpisodeStatusDraft
	}
	return episodemodel.EpisodeStatusFinalized
}

func classCoding(c episodemodel.CareSetting) Coding {
	code := ActCodeAmbulatory
	switch c {
	case episodemodel.CareSettingEmergency:
		code = ActCodeEmergency
	case episodemodel.CareSettingInpatient:
		code = ActCodeInpatient
	case episodemodel.CareSettingVirtual:
		code = ActCodeVirtual
	}
	return Coding{System: SystemActCode, Code: code, Display: c.String()}
}

func classFromCoding(c Coding) episodemodel.CareSetting {
	switch c.Code {
	case ActCodeEmergency:
		return episodemodel.CareSettingEmergency
	case ActCodeInpatient:
		return episodemodel.CareSettingInpatient
	case ActCodeVirtual:
		return episodemodel.CareSettingVirtual
	default:
		return episodemodel.CareSettingAmbulatory
	}
}

func fhirFindingStatus(s episodemodel.FindingStatus) string {
	switch s {
	case episodemodel.FindingStatusProvisional:
		return "preliminary"
	case episodemodel.FindingStatusCancelled:
		return "cancelled"
	default:
		return "final"
	}
}

func domainFindingStatus(code string) episodemodel.FindingStatus {
	switch code {
	case "registered", "preliminary":
		return episodemodel.FindingStatusProvisional
	case "cancelled", "entered-in-error":
		return episodemodel.FindingStatusCancelled
	default:
		// final, amended, corrected, unknown
		return episodemodel.FindingStatusRecorded
	}
}

func fhirClinicalStatus(s episodemodel.ProblemClinicalStatus) string {
	switch s {
	case episodemodel.ProblemClinicalInactive:
		return "inactive"
	case episodemodel.ProblemClinicalResolved:
		return "resolved"
	default:
		return "active"
	}
}

func domainClinicalStatus(code string) episodemodel.ProblemClinicalStatus {
	switch code {
	case "inactive", "remission":
		return episodemodel.ProblemClinicalInactive
	case "resolved":
		return episodemodel.ProblemClinicalResolved
	default:
		// active, recurrence, relapse, or unknown
		return episodemodel.ProblemClinicalActive
	}
}

func fhirVerification(s episodemodel.ProblemVerificationStatus) string {
	switch s {
	case episodemodel.ProblemVerificationSuspected:
		return "unconfirmed"
	case episodemodel.ProblemVerificationRefuted:
		return "refuted"
	case episodemodel.ProblemVerificationError:
		return "entered-in-error"
	default:
		return "confirmed"
	}
}

func domainVerification(code string) episodemodel.ProblemVerificationStatus {
	switch code {
	case "unconfirmed", "provisional", "differential":
		return episodemodel.ProblemVerificationSuspected
	case "refuted":
		return episodemodel.ProblemVerificationRefuted
	case "entered-in-error":
		return episodemodel.ProblemVerificationError
	default:
		return episodemodel.ProblemVerificationConfirmed
	}
}

func fhirProblemCategory(c episodemodel.ProblemCategory) string {
	if c == episodemodel.ProblemCategoryList {
		return "problem-list-item"
	}
	return "encounter-diagnosis"
}

func domainProblemCategory(code string) episodemodel.ProblemCategory {
	if code == "problem-list-item" {
		return episodemodel.ProblemCategoryList
	}
	return episodemodel.ProblemCategoryEncounter
}

func fhirPlanKind(k episodemodel.PlanItemKind) string {
	switch k {
	case episodemodel.PlanItemKindMedication:
		return "MedicationRequest"
	case episodemodel.PlanItemKindProcedure, episodemodel.PlanItemKindExam:
		return "ServiceRequest"
	case episodemodel.PlanItemKindAppointment:
		return "Appointment"
	default:
		return ""
	}
}

func domainPlanKind(kind string) episodemodel.PlanItemKind {
	switch kind {
	case "MedicationRequest":
		return episodemodel.PlanItemKindMedication
	case "ServiceRequest":
		return episodemodel.PlanItemKindProcedure
	case "Appointment":
		return episodemodel.PlanItemKindAppointment
	default:
		return episodemodel.PlanItemKindInstruction
	}
}

func fhirPlanStatus(s episodemodel.PlanItemStatus) string {
	switch s {
	case episodemodel.PlanItemStatusDraft:
		return "draft"
	case episodemodel.PlanItemStatusCompleted:
		return "completed"
	case episodemodel.PlanItemStatusCancelled:
		return "cancelled"
	default:
		return "in-progress"
	}
}

func domainPlanStatus(s string) episodemodel.PlanItemStatus {
	switch s {
	case "draft":
		return episodemodel.PlanItemStatusDraft
	case "completed":
		return episodemodel.PlanItemStatusCompleted
	case "cancelled":
		return episodemodel.PlanItemStatusCancelled
	default:
		return episodemodel.PlanItemStatusActive
	}
}

func codingToConcept(c episodemodel.Coding) CodeableConcept {
	return CodeableConceptFrom(c.System, c.Code, c.Display)
}

func conceptToCoding(c CodeableConcept) episodemodel.Coding {
	if len(c.Coding) == 0 {
		return episodemodel.Coding{Display: c.Text}
	}
	return episodemodel.Coding{System: c.Coding[0].System, Code: c.Coding[0].Code, Display: c.Coding[0].Display}
}

func firstCode(c *CodeableConcept) string {
	if c == nil || len(c.Coding) == 0 {
		return ""
	}
	return c.Coding[0].Code
}

func ptrConcept(c CodeableConcept) *CodeableConcept { return &c }

func textFromNarrative(n *Narrative) string {
	if n == nil {
		return ""
	}
	s := n.Div
	s = strings.ReplaceAll(s, `<div xmlns="http://www.w3.org/1999/xhtml">`, "")
	s = strings.ReplaceAll(s, "</div>", "")
	s = strings.ReplaceAll(s, "<p>", "")
	s = strings.ReplaceAll(s, "</p>", "")
	return html.UnescapeString(strings.TrimSpace(s))
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseUUID(s string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(s))
	return id, err == nil
}

func parseTypedRef(ref *Reference, resourceType string) (uuid.UUID, bool) {
	if ref == nil {
		return uuid.Nil, false
	}
	r := strings.TrimSpace(ref.Reference)
	if strings.HasPrefix(r, "urn:uuid:") {
		return parseUUID(strings.TrimPrefix(r, "urn:uuid:"))
	}
	prefix := resourceType + "/"
	if i := strings.LastIndex(r, prefix); i >= 0 {
		return parseUUID(r[i+len(prefix):])
	}
	return uuid.Nil, false
}

func firstAuthor(comp Composition, enc Encounter) *Reference {
	if len(comp.Author) > 0 {
		return &comp.Author[0]
	}
	if len(enc.Participant) > 0 {
		return enc.Participant[0].Individual
	}
	return nil
}

func episodeTypeFrom(types []CodeableConcept) (episodemodel.EpisodeType, bool) {
	for _, t := range types {
		if st, ok := episodemodel.ParseEpisodeType(firstCode(&t)); ok {
			return st, true
		}
	}
	return "", false
}

func predecessorFromRelatesTo(rels []CompositionRelatesTo) *uuid.UUID {
	for _, r := range rels {
		if r.Code != "replaces" {
			continue
		}
		if id, ok := parseTypedRef(r.Target, "Composition"); ok {
			return &id
		}
		if id, ok := parseTypedRef(r.Target, "Encounter"); ok {
			return &id
		}
	}
	return nil
}

// WantFinalize reports whether the inbound Composition asks to lock the note.
func WantFinalize(b *Bundle) bool {
	if b == nil {
		return false
	}
	for _, e := range b.Entry {
		if PeekType(e.Resource) != "Composition" {
			continue
		}
		var c Composition
		if err := json.Unmarshal(e.Resource, &c); err != nil {
			return false
		}
		return c.Status == "final"
	}
	return false
}
