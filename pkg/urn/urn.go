// Package urn builds and parses LibreVita resource identifiers.
//
// Resource keys (KeyStore, meta, sessions, FLE AAD) and identifier-system
// catalog URNs live here. Seed rows call Identifier.
package urn

import (
	"strings"

	"librevita.org/pkg/ident"
)

const (
	// Namespace is the LibreVita URN namespace.
	Namespace = "urn:librevita"

	// ClinicPrefix is the prefix of clinic-scoped resource keys.
	ClinicPrefix = Namespace + ":clinic:"

	// MetaPrefix is the prefix of installation metadata keys.
	MetaPrefix = Namespace + ":meta:"

	// PlatformSessionPrefix is the prefix of apex PASETO session keys.
	PlatformSessionPrefix = Namespace + ":platform:session:"

	// IdentifierPrefix is the prefix of identifier-system catalog URNs.
	IdentifierPrefix = Namespace + ":id:"

	// IdentifierRaw is the reserved catalog URN of the built-in fallback
	// identifier strategy. Administrators cannot register this system.
	IdentifierRaw = IdentifierPrefix + "raw"
)

const patientSegment = ":patient:"

// Clinic is the keystore key and FLE AAD for a clinic DEK.
func Clinic(clinicID ident.ClinicID) string {
	return ClinicPrefix + clinicID.String()
}

// Patient is the keystore key and FLE AAD for a patient DEK wrapped by the clinic DEK.
func Patient(clinicID ident.ClinicID, patientID ident.PatientID) string {
	return Clinic(clinicID) + patientSegment + patientID.String()
}

// Meta is the key for installation metadata in the meta store.
func Meta(key string) string {
	return MetaPrefix + key
}

// ClinicSession is the revocation-index key for a clinic PASETO session.
func ClinicSession(clinicID ident.ClinicID, tokenHash string) string {
	return Clinic(clinicID) + ":session:" + tokenHash
}

// PlatformSession is the revocation-index key for an apex PASETO session.
func PlatformSession(tokenHash string) string {
	return PlatformSessionPrefix + tokenHash
}

// Identifier is the catalog URN of an identifier system (a document
// kind such as CPF or NIF). The package joins parts with ":".
// Identifier("br", "cpf") is "urn:librevita:id:br:cpf".
func Identifier(parts ...string) string {
	return IdentifierPrefix + strings.Join(parts, ":")
}

// ParseIdentifier extracts the catalog parts from an identifier-system URN.
// ParseIdentifier("urn:librevita:id:br:cpf") is []string{"br", "cpf"}.
func ParseIdentifier(s string) (parts []string, ok bool) {
	if !strings.HasPrefix(s, IdentifierPrefix) {
		return nil, false
	}
	rest := strings.TrimPrefix(s, IdentifierPrefix)
	if rest == "" {
		return nil, false
	}
	parts = strings.Split(rest, ":")
	for _, p := range parts {
		if p == "" {
			return nil, false
		}
	}
	return parts, true
}

// ParsePatient extracts clinic and patient IDs from a clinic-scoped patient URN.
func ParsePatient(s string) (clinicID ident.ClinicID, patientID ident.PatientID, ok bool) {
	if !strings.HasPrefix(s, ClinicPrefix) {
		return ident.ClinicID{}, ident.PatientID{}, false
	}
	rest := strings.TrimPrefix(s, ClinicPrefix)
	clinicStr, patientStr, found := strings.Cut(rest, patientSegment)
	if !found {
		return ident.ClinicID{}, ident.PatientID{}, false
	}
	cid, err := ident.ParseClinic(clinicStr)
	if err != nil {
		return ident.ClinicID{}, ident.PatientID{}, false
	}
	pid, err := ident.ParsePatient(patientStr)
	if err != nil {
		return ident.ClinicID{}, ident.PatientID{}, false
	}
	return cid, pid, true
}

// ParseClinic extracts a clinic ID from a clinic-scoped key URN.
func ParseClinic(s string) (clinicID ident.ClinicID, ok bool) {
	if !strings.HasPrefix(s, ClinicPrefix) {
		return ident.ClinicID{}, false
	}
	parsed, err := ident.ParseClinic(strings.TrimPrefix(s, ClinicPrefix))
	if err != nil || parsed.IsZero() {
		return ident.ClinicID{}, false
	}
	return parsed, true
}
