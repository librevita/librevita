package types

// Sex is the patient sex value. It mirrors the CHECK constraint on
// patients.sex (see db/migrations/00006_patients.sql).
type Sex string

const (
	SexFemale  Sex = "female"
	SexMale    Sex = "male"
	SexOther   Sex = "other"
	SexUnknown Sex = "unknown"
)

// Valid reports whether s is one of the options the database CHECK
// constraint accepts.
func (s Sex) Valid() bool {
	switch s {
	case SexFemale, SexMale, SexOther, SexUnknown:
		return true
	}
	return false
}

// String returns the stored representation of s.
func (s Sex) String() string {
	return string(s)
}

// ParseSex converts a stored value back to the enum. ok is false when
// the value is not one of the CHECK options.
func ParseSex(s string) (Sex, bool) {
	sex := Sex(s)
	return sex, sex.Valid()
}

// PatientStatus is the patient record status. It mirrors the CHECK
// constraint on patients.status (see db/migrations/00006_patients.sql).
type PatientStatus string

const (
	PatientStatusActive   PatientStatus = "active"
	PatientStatusInactive PatientStatus = "inactive"
)

// Valid reports whether s is one of the options the database CHECK
// constraint accepts.
func (s PatientStatus) Valid() bool {
	return s == PatientStatusActive || s == PatientStatusInactive
}

// String returns the stored representation of s.
func (s PatientStatus) String() string {
	return string(s)
}

// ParsePatientStatus converts a stored value back to the enum. ok is
// false when the value is not one of the CHECK options.
func ParsePatientStatus(s string) (PatientStatus, bool) {
	status := PatientStatus(s)
	return status, status.Valid()
}
