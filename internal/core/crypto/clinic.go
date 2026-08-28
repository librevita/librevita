package crypto

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// EnsureClinicDEK returns the clinic DEK, creating and wrapping it with the
// installation KEK if it does not exist yet. A destroyed key is terminal and
// is never recreated.
func (e *Engine) EnsureClinicDEK(ctx context.Context, clinicID uuid.UUID) ([]byte, error) {
	dek, err := e.GetClinicDEK(ctx, clinicID)
	if err == nil {
		return dek, nil
	}
	if !errors.Is(err, ErrKeyNotFound) {
		return nil, err
	}

	urn := ClinicURN(clinicID)
	dek, created, err := e.createWrappedDEK(ctx, urn, keyScopeClinic, e.kek, []byte(urn))
	if err != nil {
		return nil, err
	}
	if !created {
		return e.GetClinicDEK(ctx, clinicID)
	}
	return dek, nil
}

// GetClinicDEK unwraps the clinic DEK with the installation KEK.
func (e *Engine) GetClinicDEK(ctx context.Context, clinicID uuid.UUID) ([]byte, error) {
	urn := ClinicURN(clinicID)
	return e.cachedDEK(ctx, urn, func() ([]byte, error) {
		encDEK, err := e.vault.GetDEK(ctx, urn)
		if err != nil {
			return nil, err
		}
		return unwrapKey(e.kek, encDEK, keyScopeClinic, []byte(urn))
	})
}

// DeleteClinicDEK removes the clinic DEK from the vault (crypto-shred of the
// clinic). Patient DEKs wrapped by it become unreadable.
func (e *Engine) DeleteClinicDEK(ctx context.Context, clinicID uuid.UUID) error {
	err := e.vault.DeleteDEK(ctx, ClinicURN(clinicID))
	if err == nil {
		ClearRequestKeyCache(ctx)
	}
	return err
}

// SetupPatientDEKForClinic generates a patient DEK and wraps it with the
// clinic DEK. The operation is idempotent when another request wins the
// create-if-absent race.
func (e *Engine) SetupPatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	return e.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
}

// GetPatientDEKForClinic unwraps the patient DEK with the clinic DEK.
func (e *Engine) GetPatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	clinicDEK, err := e.GetClinicDEK(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(clinicDEK)

	urn := PatientURN(clinicID, patientID)
	return e.cachedDEK(ctx, urn, func() ([]byte, error) {
		encDEK, err := e.vault.GetDEK(ctx, urn)
		if err != nil {
			return nil, err
		}
		return unwrapKey(clinicDEK, encDEK, keyScopePatient, []byte(urn))
	})
}

// GetPatientDEKsForClinic retrieves and unwraps each requested patient DEK
// once. Missing patient keys are omitted from the result; terminal or backend
// errors abort the batch.
func (e *Engine) GetPatientDEKsForClinic(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID) (map[uuid.UUID][]byte, error) {
	result := make(map[uuid.UUID][]byte, len(patientIDs))
	if len(patientIDs) == 0 {
		return result, nil
	}
	cleanup := true
	defer func() {
		if cleanup {
			zeroDEKMap(result)
		}
	}()

	clinicDEK, err := e.GetClinicDEK(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(clinicDEK)

	urns := make([]string, 0, len(patientIDs))
	idsByURN := make(map[string]uuid.UUID, len(patientIDs))
	for _, patientID := range patientIDs {
		urn := PatientURN(clinicID, patientID)
		if _, exists := idsByURN[urn]; exists {
			continue
		}
		idsByURN[urn] = patientID
		if cache := requestCacheFromContext(ctx); cache != nil {
			if dek, ok := cache.get(urn); ok {
				if e.metrics != nil {
					e.metrics.cacheHit.Add(1)
				}
				result[patientID] = dek
				continue
			}
			if e.metrics != nil {
				e.metrics.cacheMiss.Add(1)
			}
		}
		urns = append(urns, urn)
	}

	wrapped, err := e.getWrappedDEKs(ctx, urns)
	if err != nil {
		return nil, err
	}
	for _, urn := range urns {
		item := wrapped[urn]
		if errors.Is(item.Err, ErrKeyNotFound) {
			continue
		}
		if item.Err != nil {
			return nil, item.Err
		}
		dek, err := unwrapKey(clinicDEK, item.EncryptedDEK, keyScopePatient, []byte(urn))
		if err != nil {
			return nil, fmt.Errorf("crypto: unwrap patient dek %q: %w", urn, err)
		}
		if len(dek) != SizeDEK {
			ZeroBytes(dek)
			return nil, ErrInvalidDEK
		}
		cacheDEK(ctx, urn, dek)
		result[idsByURN[urn]] = dek
	}
	cleanup = false
	return result, nil
}

// EnsurePatientDEKForClinic returns the existing patient DEK or creates one.
// It never recreates a key that the vault has marked as destroyed.
func (e *Engine) EnsurePatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	clinicDEK, err := e.EnsureClinicDEK(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(clinicDEK)

	urn := PatientURN(clinicID, patientID)
	dek, err := e.cachedDEK(ctx, urn, func() ([]byte, error) {
		encDEK, err := e.vault.GetDEK(ctx, urn)
		if err != nil {
			return nil, err
		}
		return unwrapKey(clinicDEK, encDEK, keyScopePatient, []byte(urn))
	})
	if err == nil {
		return dek, nil
	}
	if !errors.Is(err, ErrKeyNotFound) {
		return nil, err
	}

	dek, created, err := e.createWrappedDEK(ctx, urn, keyScopePatient, clinicDEK, []byte(urn))
	if err != nil {
		return nil, err
	}
	if !created {
		return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	}
	return dek, nil
}

// DeletePatientDEKForClinic removes the patient DEK and leaves a terminal
// tombstone in the vault so future reads cannot recreate it.
func (e *Engine) DeletePatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) error {
	urn := PatientURN(clinicID, patientID)
	err := e.vault.DeleteDEK(ctx, urn)
	if err == nil {
		forgetDEK(ctx, urn)
	}
	return err
}

// ResolvePatientEncryptor creates an Encryptor backed by the patient's DEK.
// The unwrapped DEK is zeroed after the Encryptor has copied it.
func (e *Engine) ResolvePatientEncryptor(ctx context.Context, clinicID, patientID uuid.UUID) (Encryptor, error) {
	dek, err := e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(dek)
	return NewEncryptor(dek)
}

func (e *Engine) getWrappedDEKs(ctx context.Context, urns []string) (map[string]DEKResult, error) {
	urns = uniqueStrings(urns)
	if len(urns) == 0 {
		return map[string]DEKResult{}, nil
	}
	if batch, ok := e.vault.(BatchKeyVault); ok {
		if e.metrics != nil {
			e.metrics.vaultBatchGet.Add(1)
		}
		return batch.GetDEKs(ctx, urns)
	}
	if e.metrics != nil {
		e.metrics.vaultGet.Add(uint64(len(urns)))
	}
	results := make(map[string]DEKResult, len(urns))
	for _, urn := range urns {
		value, err := e.vault.GetDEK(ctx, urn)
		results[urn] = DEKResult{
			EncryptedDEK: value,
			Err:          err,
		}
	}
	return results, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func zeroDEKMap(deks map[uuid.UUID][]byte) {
	for _, dek := range deks {
		ZeroBytes(dek)
	}
}

func (e *Engine) createWrappedDEK(ctx context.Context, urn string, scope byte, wrappingKey, aad []byte) ([]byte, bool, error) {
	dek, err := RandomBytes(SizeDEK)
	if err != nil {
		return nil, false, fmt.Errorf("crypto: generate dek: %w", err)
	}
	encDEK, err := wrapKey(wrappingKey, dek, scope, aad)
	if err != nil {
		ZeroBytes(dek)
		return nil, false, fmt.Errorf("crypto: wrap dek: %w", err)
	}
	created, err := e.putIfAbsent(ctx, urn, encDEK)
	if err != nil {
		ZeroBytes(dek)
		return nil, false, fmt.Errorf("crypto: save dek to vault: %w", err)
	}
	if !created {
		ZeroBytes(dek)
		return nil, false, nil
	}
	cacheDEK(ctx, urn, dek)
	return dek, true, nil
}

func (e *Engine) putIfAbsent(ctx context.Context, urn string, encryptedDEK []byte) (bool, error) {
	if conditional, ok := e.vault.(ConditionalKeyVault); ok {
		return conditional.PutIfAbsent(ctx, urn, encryptedDEK)
	}
	if err := e.vault.PutDEK(ctx, urn, encryptedDEK); err != nil {
		return false, err
	}
	return true, nil
}
