package crypto

import (
	"strings"

	"github.com/google/uuid"
)

// ClinicURN is the vault key and FLE AAD for a clinic DEK.
func ClinicURN(clinicID uuid.UUID) string {
	return "urn:librevita:clinic:" + clinicID.String()
}

// PatientURN is the vault key for a patient DEK wrapped by the clinic DEK.
func PatientURN(clinicID, patientID uuid.UUID) string {
	return "urn:librevita:clinic:" + clinicID.String() + ":patient:" + patientID.String()
}

// LegacyPatientURN is the pre-multi-clinic vault key (wrapped by the installation KEK).
func LegacyPatientURN(patientID uuid.UUID) string {
	return "urn:librevita:patient:" + patientID.String()
}

// ClinicAAD is the request-scoped FLE AAD for a clinic.
func ClinicAAD(clinicID uuid.UUID) []byte {
	return []byte(ClinicURN(clinicID))
}

// LegacyAAD is the installation-wide FLE AAD used before clinic-scoped keys.
func LegacyAAD() []byte {
	return []byte("urn:librevita")
}

// ParsePatientURN extracts clinic and patient IDs from a clinic-scoped patient URN.
func ParsePatientURN(urn string) (clinicID, patientID uuid.UUID, ok bool) {
	const prefix = "urn:librevita:clinic:"
	if !strings.HasPrefix(urn, prefix) {
		return uuid.Nil, uuid.Nil, false
	}
	rest := strings.TrimPrefix(urn, prefix)
	clinicStr, patientStr, found := strings.Cut(rest, ":patient:")
	if !found {
		return uuid.Nil, uuid.Nil, false
	}
	cid, err := uuid.Parse(clinicStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	pid, err := uuid.Parse(patientStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	return cid, pid, true
}
