package fhir

// Encounter is FHIR R4 Encounter.
type Encounter struct {
	ResourceType string                 `json:"resourceType"`
	ID           string                 `json:"id,omitempty"`
	Meta         *Meta                  `json:"meta,omitempty"`
	Status       string                 `json:"status"`
	Class        Coding                 `json:"class"`
	Type         []CodeableConcept      `json:"type,omitempty"`
	Subject      *Reference             `json:"subject,omitempty"`
	Participant  []EncounterParticipant `json:"participant,omitempty"`
	Period       *Period                `json:"period,omitempty"`
}

// EncounterParticipant is Encounter.participant.
type EncounterParticipant struct {
	Individual *Reference `json:"individual,omitempty"`
}

// Composition is FHIR R4 Composition (SOAP document).
type Composition struct {
	ResourceType    string                 `json:"resourceType"`
	ID              string                 `json:"id,omitempty"`
	Meta            *Meta                  `json:"meta,omitempty"`
	Status          string                 `json:"status"`
	Type            CodeableConcept        `json:"type"`
	Subject         *Reference             `json:"subject,omitempty"`
	Encounter       *Reference             `json:"encounter,omitempty"`
	Date            string                 `json:"date,omitempty"`
	Author          []Reference            `json:"author,omitempty"`
	Title           string                 `json:"title,omitempty"`
	Confidentiality string                 `json:"confidentiality,omitempty"`
	RelatesTo       []CompositionRelatesTo `json:"relatesTo,omitempty"`
	Section         []CompositionSection   `json:"section,omitempty"`
}

// CompositionRelatesTo is Composition.relatesTo (amendment chain).
type CompositionRelatesTo struct {
	Code   string     `json:"code,omitempty"`
	Target *Reference `json:"targetReference,omitempty"`
}

// CompositionSection is Composition.section.
type CompositionSection struct {
	Title string           `json:"title,omitempty"`
	Code  *CodeableConcept `json:"code,omitempty"`
	Text  *Narrative       `json:"text,omitempty"`
	Entry []Reference      `json:"entry,omitempty"`
}

// Observation is FHIR R4 Observation.
type Observation struct {
	ResourceType         string           `json:"resourceType"`
	ID                   string           `json:"id,omitempty"`
	Status               string           `json:"status"`
	Code                 CodeableConcept  `json:"code"`
	Subject              *Reference       `json:"subject,omitempty"`
	Encounter            *Reference       `json:"encounter,omitempty"`
	EffectiveDateTime    string           `json:"effectiveDateTime,omitempty"`
	ValueQuantity        *Quantity        `json:"valueQuantity,omitempty"`
	ValueString          string           `json:"valueString,omitempty"`
	ValueBoolean         *bool            `json:"valueBoolean,omitempty"`
	ValueCodeableConcept *CodeableConcept `json:"valueCodeableConcept,omitempty"`
}

// Condition is FHIR R4 Condition.
type Condition struct {
	ResourceType       string            `json:"resourceType"`
	ID                 string            `json:"id,omitempty"`
	ClinicalStatus     *CodeableConcept  `json:"clinicalStatus,omitempty"`
	VerificationStatus *CodeableConcept  `json:"verificationStatus,omitempty"`
	Category           []CodeableConcept `json:"category,omitempty"`
	Code               *CodeableConcept  `json:"code,omitempty"`
	Subject            *Reference        `json:"subject,omitempty"`
	Encounter          *Reference        `json:"encounter,omitempty"`
}

// ClinicalImpression is FHIR R4 ClinicalImpression.
type ClinicalImpression struct {
	ResourceType string      `json:"resourceType"`
	ID           string      `json:"id,omitempty"`
	Status       string      `json:"status"`
	Subject      *Reference  `json:"subject,omitempty"`
	Encounter    *Reference  `json:"encounter,omitempty"`
	Date         string      `json:"date,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	Problem      []Reference `json:"problem,omitempty"`
}

// CarePlan is FHIR R4 CarePlan.
type CarePlan struct {
	ResourceType string             `json:"resourceType"`
	ID           string             `json:"id,omitempty"`
	Status       string             `json:"status"`
	Intent       string             `json:"intent"`
	Subject      *Reference         `json:"subject,omitempty"`
	Encounter    *Reference         `json:"encounter,omitempty"`
	Description  string             `json:"description,omitempty"`
	Activity     []CarePlanActivity `json:"activity,omitempty"`
}

// CarePlanActivity is CarePlan.activity.
type CarePlanActivity struct {
	Detail *CarePlanActivityDetail `json:"detail,omitempty"`
}

// CarePlanActivityDetail is CarePlan.activity.detail.
type CarePlanActivityDetail struct {
	Kind            string           `json:"kind,omitempty"`
	Code            *CodeableConcept `json:"code,omitempty"`
	Status          string           `json:"status"`
	Description     string           `json:"description,omitempty"`
	ScheduledString string           `json:"scheduledString,omitempty"`
}
