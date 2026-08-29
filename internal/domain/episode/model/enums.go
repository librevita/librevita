package model

// EpisodeType is the clinical purpose of the note. It mirrors the CHECK
// constraint on episodes.episode_type.
type EpisodeType string

const (
	EpisodeTypeConsultation EpisodeType = "consultation"
	EpisodeTypeAnamnesis    EpisodeType = "anamnesis"
	EpisodeTypeEvolution    EpisodeType = "evolution"
	EpisodeTypePrescription EpisodeType = "prescription"
	EpisodeTypeExamRequest  EpisodeType = "exam_request"
	EpisodeTypeDiagnostic   EpisodeType = "diagnostic"
)

// Valid reports whether t is one of the stored episode types.
func (t EpisodeType) Valid() bool {
	switch t {
	case EpisodeTypeConsultation, EpisodeTypeAnamnesis, EpisodeTypeEvolution,
		EpisodeTypePrescription, EpisodeTypeExamRequest, EpisodeTypeDiagnostic:
		return true
	}
	return false
}

func (t EpisodeType) String() string { return string(t) }

// ParseEpisodeType converts a stored value back to the enum.
func ParseEpisodeType(s string) (EpisodeType, bool) {
	t := EpisodeType(s)
	return t, t.Valid()
}

// EpisodeStatus is the lifecycle of the SOAP note. It mirrors the CHECK
// constraint on episodes.status.
type EpisodeStatus string

const (
	EpisodeStatusDraft     EpisodeStatus = "draft"
	EpisodeStatusFinalized EpisodeStatus = "finalized"
	EpisodeStatusArchived  EpisodeStatus = "archived"
)

func (s EpisodeStatus) Valid() bool {
	switch s {
	case EpisodeStatusDraft, EpisodeStatusFinalized, EpisodeStatusArchived:
		return true
	}
	return false
}

func (s EpisodeStatus) String() string { return string(s) }

// ParseEpisodeStatus converts a stored value back to the enum.
func ParseEpisodeStatus(s string) (EpisodeStatus, bool) {
	st := EpisodeStatus(s)
	return st, st.Valid()
}

// CareSetting is the setting of the visit. It mirrors episodes.class.
type CareSetting string

const (
	CareSettingAmbulatory CareSetting = "ambulatory"
	CareSettingEmergency  CareSetting = "emergency"
	CareSettingInpatient  CareSetting = "inpatient"
	CareSettingVirtual    CareSetting = "virtual"
)

func (c CareSetting) Valid() bool {
	switch c {
	case CareSettingAmbulatory, CareSettingEmergency, CareSettingInpatient, CareSettingVirtual:
		return true
	}
	return false
}

func (c CareSetting) String() string { return string(c) }

// ParseCareSetting converts a stored value back to the enum.
func ParseCareSetting(s string) (CareSetting, bool) {
	c := CareSetting(s)
	return c, c.Valid()
}

// FindingStatus is the clinical state of an objective finding.
// Stored values are domain tokens, not Observation.status.
type FindingStatus string

const (
	FindingStatusRecorded    FindingStatus = "recorded"
	FindingStatusProvisional FindingStatus = "provisional"
	FindingStatusCancelled   FindingStatus = "cancelled"
)

func (s FindingStatus) Valid() bool {
	switch s {
	case FindingStatusRecorded, FindingStatusProvisional, FindingStatusCancelled:
		return true
	}
	return false
}

func (s FindingStatus) String() string { return string(s) }

// ParseFindingStatus converts a stored value back to the enum.
func ParseFindingStatus(s string) (FindingStatus, bool) {
	st := FindingStatus(s)
	return st, st.Valid()
}

// FindingValueKind is how a finding's value is represented.
type FindingValueKind string

const (
	FindingValueQuantity FindingValueKind = "quantity"
	FindingValueString   FindingValueKind = "string"
	FindingValueBoolean  FindingValueKind = "boolean"
	FindingValueCoded    FindingValueKind = "coded"
)

func (k FindingValueKind) Valid() bool {
	switch k {
	case FindingValueQuantity, FindingValueString, FindingValueBoolean, FindingValueCoded:
		return true
	}
	return false
}

func (k FindingValueKind) String() string { return string(k) }

// ParseFindingValueKind converts a stored value back to the enum.
func ParseFindingValueKind(s string) (FindingValueKind, bool) {
	k := FindingValueKind(s)
	return k, k.Valid()
}

// ProblemClinicalStatus is whether the problem is still in play.
// Stored values are domain tokens, not Condition.clinicalStatus.
type ProblemClinicalStatus string

const (
	ProblemClinicalActive   ProblemClinicalStatus = "active"
	ProblemClinicalInactive ProblemClinicalStatus = "inactive"
	ProblemClinicalResolved ProblemClinicalStatus = "resolved"
)

func (s ProblemClinicalStatus) Valid() bool {
	switch s {
	case ProblemClinicalActive, ProblemClinicalInactive, ProblemClinicalResolved:
		return true
	}
	return false
}

func (s ProblemClinicalStatus) String() string { return string(s) }

// ParseProblemClinicalStatus converts a stored value back to the enum.
func ParseProblemClinicalStatus(s string) (ProblemClinicalStatus, bool) {
	st := ProblemClinicalStatus(s)
	return st, st.Valid()
}

// ProblemVerificationStatus is how sure the clinician is of the problem.
// Stored values are domain tokens, not Condition.verificationStatus.
type ProblemVerificationStatus string

const (
	ProblemVerificationConfirmed ProblemVerificationStatus = "confirmed"
	ProblemVerificationSuspected ProblemVerificationStatus = "suspected"
	ProblemVerificationRefuted   ProblemVerificationStatus = "refuted"
	ProblemVerificationError     ProblemVerificationStatus = "error"
)

func (s ProblemVerificationStatus) Valid() bool {
	switch s {
	case ProblemVerificationConfirmed, ProblemVerificationSuspected,
		ProblemVerificationRefuted, ProblemVerificationError:
		return true
	}
	return false
}

func (s ProblemVerificationStatus) String() string { return string(s) }

// ParseProblemVerificationStatus converts a stored value back to the enum.
func ParseProblemVerificationStatus(s string) (ProblemVerificationStatus, bool) {
	st := ProblemVerificationStatus(s)
	return st, st.Valid()
}

// ProblemCategory is whether the problem belongs to this visit or the list.
type ProblemCategory string

const (
	ProblemCategoryEncounter ProblemCategory = "encounter"
	ProblemCategoryList      ProblemCategory = "list"
)

func (c ProblemCategory) Valid() bool {
	switch c {
	case ProblemCategoryEncounter, ProblemCategoryList:
		return true
	}
	return false
}

func (c ProblemCategory) String() string { return string(c) }

// ParseProblemCategory converts a stored value back to the enum.
func ParseProblemCategory(s string) (ProblemCategory, bool) {
	c := ProblemCategory(s)
	return c, c.Valid()
}

// PlanItemKind is the kind of planned activity.
type PlanItemKind string

const (
	PlanItemKindMedication  PlanItemKind = "medication"
	PlanItemKindProcedure   PlanItemKind = "procedure"
	PlanItemKindExam        PlanItemKind = "exam"
	PlanItemKindAppointment PlanItemKind = "appointment"
	PlanItemKindInstruction PlanItemKind = "instruction"
)

func (k PlanItemKind) Valid() bool {
	switch k {
	case PlanItemKindMedication, PlanItemKindProcedure, PlanItemKindExam,
		PlanItemKindAppointment, PlanItemKindInstruction:
		return true
	}
	return false
}

func (k PlanItemKind) String() string { return string(k) }

// ParsePlanItemKind converts a stored value back to the enum.
func ParsePlanItemKind(s string) (PlanItemKind, bool) {
	k := PlanItemKind(s)
	return k, k.Valid()
}

// PlanItemStatus mirrors plan_items.status.
type PlanItemStatus string

const (
	PlanItemStatusDraft     PlanItemStatus = "draft"
	PlanItemStatusActive    PlanItemStatus = "active"
	PlanItemStatusCompleted PlanItemStatus = "completed"
	PlanItemStatusCancelled PlanItemStatus = "cancelled"
)

func (s PlanItemStatus) Valid() bool {
	switch s {
	case PlanItemStatusDraft, PlanItemStatusActive, PlanItemStatusCompleted, PlanItemStatusCancelled:
		return true
	}
	return false
}

func (s PlanItemStatus) String() string { return string(s) }

// ParsePlanItemStatus converts a stored value back to the enum.
func ParsePlanItemStatus(s string) (PlanItemStatus, bool) {
	st := PlanItemStatus(s)
	return st, st.Valid()
}
