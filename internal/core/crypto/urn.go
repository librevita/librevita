package crypto

import (
	"strings"

	"github.com/google/uuid"
)

// ClinicURN is the keystore key and FLE AAD for a clinic DEK.
func ClinicURN(clinicID uuid.UUID) string {
	return "urn:librevita:clinic:" + clinicID.String()
}

// PatientURN is the keystore key for a patient DEK wrapped by the clinic DEK.
func PatientURN(clinicID, patientID uuid.UUID) string {
	return "urn:librevita:clinic:" + clinicID.String() + ":patient:" + patientID.String()
}

// MetaURN is the key for installation metadata in the meta store.
func MetaURN(key string) string {
	return "urn:librevita:meta:" + key
}

// ClinicSessionURN is the revocation-index key for a clinic PASETO session.
func ClinicSessionURN(clinicID uuid.UUID, tokenHash string) string {
	return "urn:librevita:clinic:" + clinicID.String() + ":session:" + tokenHash
}

// PlatformSessionURN is the revocation-index key for an apex PASETO session.
func PlatformSessionURN(tokenHash string) string {
	return "urn:librevita:platform:session:" + tokenHash
}

// ClinicAAD is the request-scoped FLE AAD for a clinic.
func ClinicAAD(clinicID uuid.UUID) []byte {
	return []byte(ClinicURN(clinicID))
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

// ParseClinicURN extracts a clinic ID from a clinic-scoped key URN.
func ParseClinicURN(urn string) (clinicID uuid.UUID, ok bool) {
	const prefix = "urn:librevita:clinic:"
	if !strings.HasPrefix(urn, prefix) {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(strings.TrimPrefix(urn, prefix))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, false
	}
	return id, true
}
