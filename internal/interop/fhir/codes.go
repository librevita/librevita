package fhir

const (
	ContentType     = "application/fhir+json"
	ContentTypeUTF8 = "application/fhir+json; charset=utf-8"
	FHIRVersion     = "4.0.1"

	SystemLOINC         = "http://loinc.org"
	SystemICD10         = "http://hl7.org/fhir/sid/icd-10"
	SystemUCUM          = "http://unitsofmeasure.org"
	SystemActCode       = "http://terminology.hl7.org/CodeSystem/v3-ActCode"
	SystemEpisodeType   = "urn:librevita:episode-type"
	SystemConditionCat  = "http://terminology.hl7.org/CodeSystem/condition-category"
	SystemConditionClin = "http://terminology.hl7.org/CodeSystem/condition-clinical"
	SystemConditionVer  = "http://terminology.hl7.org/CodeSystem/condition-ver-status"
	SystemCarePlanKind  = "http://hl7.org/fhir/resource-types"

	LOINCProgressNote = "11506-3"
	LOINCSubjective   = "61150-9"
	LOINCObjective    = "61149-1"
	LOINCAssessment   = "51848-0"
	LOINCPlan         = "18776-5"

	ActCodeAmbulatory = "AMB"
	ActCodeEmergency  = "EMER"
	ActCodeInpatient  = "IMP"
	ActCodeVirtual    = "VR"
)
