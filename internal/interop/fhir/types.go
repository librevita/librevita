package fhir

// Meta is FHIR R4 Meta.
type Meta struct {
	VersionID   string   `json:"versionId,omitempty"`
	LastUpdated string   `json:"lastUpdated,omitempty"`
	Profile     []string `json:"profile,omitempty"`
}

// Coding is FHIR R4 Coding.
type Coding struct {
	System  string `json:"system,omitempty"`
	Code    string `json:"code,omitempty"`
	Display string `json:"display,omitempty"`
}

// CodeableConcept is FHIR R4 CodeableConcept.
type CodeableConcept struct {
	Coding []Coding `json:"coding,omitempty"`
	Text   string   `json:"text,omitempty"`
}

// Reference is FHIR R4 Reference.
type Reference struct {
	Reference string `json:"reference,omitempty"`
	Display   string `json:"display,omitempty"`
}

// Identifier is FHIR R4 Identifier.
type Identifier struct {
	System string `json:"system,omitempty"`
	Value  string `json:"value,omitempty"`
}

// Quantity is FHIR R4 Quantity.
type Quantity struct {
	Value  *float64 `json:"value,omitempty"`
	Unit   string   `json:"unit,omitempty"`
	System string   `json:"system,omitempty"`
	Code   string   `json:"code,omitempty"`
}

// Narrative is FHIR R4 Narrative.
type Narrative struct {
	Status string `json:"status,omitempty"`
	Div    string `json:"div,omitempty"`
}

// Period is FHIR R4 Period.
type Period struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// CodeableConceptFrom builds a concept with one coding.
func CodeableConceptFrom(system, code, display string) CodeableConcept {
	return CodeableConcept{
		Coding: []Coding{{System: system, Code: code, Display: display}},
		Text:   display,
	}
}
