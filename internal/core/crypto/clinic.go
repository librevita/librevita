package crypto

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// EnsureClinicDEK returns the clinic DEK, creating and wrapping it with the
// installation KEK if it does not exist yet.
func (e *Engine) EnsureClinicDEK(ctx context.Context, clinicID uuid.UUID) ([]byte, error) {
	dek, err := e.GetClinicDEK(ctx, clinicID)
	if errors.Is(err, ErrKeyNotFound) {
		return e.setupDEK(ctx, ClinicURN(clinicID), e.encryptWithKEK)
	}
	return dek, err
}

// GetClinicDEK unwraps the clinic DEK with the installation KEK.
func (e *Engine) GetClinicDEK(ctx context.Context, clinicID uuid.UUID) ([]byte, error) {
	encDEK, err := e.vault.GetDEK(ctx, ClinicURN(clinicID))
	if err != nil {
		return nil, err
	}
	return e.decryptWithKEK(encDEK)
}

// DeleteClinicDEK removes the clinic DEK from the vault (crypto-shred of the
// clinic). Patient DEKs wrapped by it become unreadable.
func (e *Engine) DeleteClinicDEK(ctx context.Context, clinicID uuid.UUID) error {
	return e.vault.DeleteDEK(ctx, ClinicURN(clinicID))
}

// SetupPatientDEKForClinic generates a patient DEK and wraps it with the clinic DEK.
func (e *Engine) SetupPatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	clinicDEK, err := e.EnsureClinicDEK(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(clinicDEK)

	urn := PatientURN(clinicID, patientID)
	aad := []byte(urn)
	return e.setupDEK(ctx, urn, func(plaintext []byte) ([]byte, error) {
		return sealWithDEK(clinicDEK, plaintext, aad)
	})
}

// GetPatientDEKForClinic unwraps the patient DEK with the clinic DEK.
// Legacy vault keys wrapped by the installation KEK are still accepted.
func (e *Engine) GetPatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	clinicDEK, err := e.GetClinicDEK(ctx, clinicID)
	if err != nil && !errors.Is(err, ErrKeyNotFound) {
		return nil, err
	}
	if clinicDEK != nil {
		defer ZeroBytes(clinicDEK)
		urn := PatientURN(clinicID, patientID)
		encDEK, err := e.vault.GetDEK(ctx, urn)
		if err == nil {
			return openWithDEK(clinicDEK, encDEK, []byte(urn))
		}
		if !errors.Is(err, ErrKeyNotFound) {
			return nil, err
		}
	}

	legacy, err := e.vault.GetDEK(ctx, LegacyPatientURN(patientID))
	if err != nil {
		return nil, err
	}
	return e.decryptWithKEK(legacy)
}

// EnsurePatientDEKForClinic returns the existing patient DEK or creates one.
func (e *Engine) EnsurePatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	dek, err := e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	if errors.Is(err, ErrKeyNotFound) {
		return e.SetupPatientDEKForClinic(ctx, clinicID, patientID)
	}
	return dek, err
}

// ReenvelopePatientDEK wraps a legacy (KEK-wrapped) patient DEK with the clinic DEK.
func (e *Engine) ReenvelopePatientDEK(ctx context.Context, clinicID, patientID uuid.UUID) error {
	clinicDEK, err := e.EnsureClinicDEK(ctx, clinicID)
	if err != nil {
		return err
	}
	defer ZeroBytes(clinicDEK)

	urn := PatientURN(clinicID, patientID)
	if _, err := e.vault.GetDEK(ctx, urn); err == nil {
		return nil
	} else if !errors.Is(err, ErrKeyNotFound) {
		return err
	}

	legacy, err := e.vault.GetDEK(ctx, LegacyPatientURN(patientID))
	if err != nil {
		return err
	}
	dek, err := e.decryptWithKEK(legacy)
	if err != nil {
		return err
	}
	defer ZeroBytes(dek)

	wrapped, err := sealWithDEK(clinicDEK, dek, []byte(urn))
	if err != nil {
		return err
	}
	return e.vault.PutDEK(ctx, urn, wrapped)
}

func (e *Engine) setupDEK(ctx context.Context, urn string, wrap func([]byte) ([]byte, error)) ([]byte, error) {
	dek := make([]byte, SizeDEK)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("crypto: generate dek: %w", err)
	}
	encDEK, err := wrap(dek)
	if err != nil {
		ZeroBytes(dek)
		return nil, fmt.Errorf("crypto: wrap dek: %w", err)
	}
	if err := e.vault.PutDEK(ctx, urn, encDEK); err != nil {
		ZeroBytes(dek)
		return nil, fmt.Errorf("crypto: save dek to vault: %w", err)
	}
	return dek, nil
}

func sealWithDEK(dek, plaintext, aad []byte) ([]byte, error) {
	enc, err := NewEncryptor(dek)
	if err != nil {
		return nil, err
	}
	return enc.Encrypt(plaintext, aad)
}

func openWithDEK(dek, ciphertext, aad []byte) ([]byte, error) {
	enc, err := NewEncryptor(dek)
	if err != nil {
		return nil, err
	}
	return enc.Decrypt(ciphertext, aad)
}
