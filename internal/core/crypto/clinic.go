package crypto

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"

	"librevita.org/pkg/urn"
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

	key := urn.Clinic(clinicID)
	dek, created, err := e.createWrappedDEK(ctx, key, KeyScopeClinic, e.kek, []byte(key))
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
	key := urn.Clinic(clinicID)
	return e.cachedDEK(ctx, key, func() ([]byte, error) {
		encDEK, err := e.keystore.GetDEK(ctx, key)
		if err != nil {
			return nil, err
		}
		return unwrapKey(e.kek, encDEK, KeyScopeClinic, e.kid, []byte(key))
	})
}

// DeleteClinicDEK removes the clinic DEK from the keystore (crypto-shred of the
// clinic). Patient DEKs wrapped by it become unreadable.
func (e *Engine) DeleteClinicDEK(ctx context.Context, clinicID uuid.UUID) error {
	err := e.keystore.DeleteDEK(ctx, urn.Clinic(clinicID))
	if err == nil {
		ClearRequestKeyCache(ctx)
	}
	return err
}

func clinicIDFromURN(clinicURN string) (uuid.UUID, error) {
	clinicID, ok := urn.ParseClinic(clinicURN)
	if !ok {
		return uuid.Nil, errors.Newf("crypto: invalid clinic urn %q", clinicURN)
	}
	return clinicID, nil
}

// SetupClinicDEK creates the clinic-scoped DEK identified by a canonical
// clinic URN. Symmetric to SetupPatientDEK.
func (e *Engine) SetupClinicDEK(ctx context.Context, clinicURN string) ([]byte, error) {
	clinicID, err := clinicIDFromURN(clinicURN)
	if err != nil {
		return nil, err
	}
	return e.EnsureClinicDEK(ctx, clinicID)
}

// GetClinicDEKForURN unwraps the clinic DEK identified by a canonical clinic URN.
func (e *Engine) GetClinicDEKForURN(ctx context.Context, clinicURN string) ([]byte, error) {
	clinicID, err := clinicIDFromURN(clinicURN)
	if err != nil {
		return nil, err
	}
	return e.GetClinicDEK(ctx, clinicID)
}

// DeleteClinicDEKForURN removes the clinic DEK identified by a canonical clinic URN.
func (e *Engine) DeleteClinicDEKForURN(ctx context.Context, clinicURN string) error {
	clinicID, err := clinicIDFromURN(clinicURN)
	if err != nil {
		return err
	}
	return e.DeleteClinicDEK(ctx, clinicID)
}

// EnsureClinicDEKForURN returns the existing clinic DEK or creates one for a
// canonical clinic URN if it does not exist yet.
func (e *Engine) EnsureClinicDEKForURN(ctx context.Context, clinicURN string) ([]byte, error) {
	clinicID, err := clinicIDFromURN(clinicURN)
	if err != nil {
		return nil, err
	}
	return e.EnsureClinicDEK(ctx, clinicID)
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

	key := urn.Patient(clinicID, patientID)
	return e.cachedDEK(ctx, key, func() ([]byte, error) {
		encDEK, err := e.keystore.GetDEK(ctx, key)
		if err != nil {
			return nil, err
		}
		return unwrapKey(clinicDEK, encDEK, KeyScopePatient, e.kid, []byte(key))
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

	urns, idsByURN := e.collectCachedPatientDEKs(ctx, clinicID, patientIDs, result)
	wrapped, err := e.getWrappedDEKs(ctx, urns)
	if err != nil {
		return nil, err
	}
	if err := e.unwrapMissingPatientDEKs(ctx, clinicDEK, urns, idsByURN, wrapped, result); err != nil {
		return nil, err
	}
	cleanup = false
	return result, nil
}

func (e *Engine) collectCachedPatientDEKs(ctx context.Context, clinicID uuid.UUID, patientIDs []uuid.UUID, result map[uuid.UUID][]byte) ([]string, map[string]uuid.UUID) {
	urns := make([]string, 0, len(patientIDs))
	idsByURN := make(map[string]uuid.UUID, len(patientIDs))
	for _, patientID := range patientIDs {
		key := urn.Patient(clinicID, patientID)
		if _, exists := idsByURN[key]; exists {
			continue
		}
		idsByURN[key] = patientID
		if e.takeCachedPatientDEK(ctx, key, patientID, result) {
			continue
		}
		urns = append(urns, key)
	}
	return urns, idsByURN
}

func (e *Engine) takeCachedPatientDEK(ctx context.Context, urn string, patientID uuid.UUID, result map[uuid.UUID][]byte) bool {
	cache := requestCacheFromContext(ctx)
	if cache == nil {
		return false
	}
	dek, ok := cache.get(urn)
	if ok {
		e.recordCacheHit()
		result[patientID] = dek
		return true
	}
	e.recordCacheMiss()
	return false
}

func (e *Engine) unwrapMissingPatientDEKs(ctx context.Context, clinicDEK []byte, urns []string, idsByURN map[string]uuid.UUID, wrapped map[string]DEKResult, result map[uuid.UUID][]byte) error {
	for _, urn := range urns {
		item := wrapped[urn]
		if errors.Is(item.Err, ErrKeyNotFound) {
			continue
		}
		if item.Err != nil {
			return item.Err
		}
		dek, err := unwrapKey(clinicDEK, item.EncryptedDEK, KeyScopePatient, e.kid, []byte(urn))
		if err != nil {
			return errors.Wrapf(err, "crypto: unwrap patient dek %q", urn)
		}
		if len(dek) != SizeDEK {
			ZeroBytes(dek)
			return ErrInvalidDEK
		}
		cacheDEK(ctx, urn, dek)
		result[idsByURN[urn]] = dek
	}
	return nil
}

// EnsurePatientDEKForClinic returns the existing patient DEK or creates one.
// It never recreates a key that the keystore has marked as destroyed.
func (e *Engine) EnsurePatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) ([]byte, error) {
	clinicDEK, err := e.EnsureClinicDEK(ctx, clinicID)
	if err != nil {
		return nil, err
	}
	defer ZeroBytes(clinicDEK)

	key := urn.Patient(clinicID, patientID)
	dek, err := e.cachedDEK(ctx, key, func() ([]byte, error) {
		encDEK, err := e.keystore.GetDEK(ctx, key)
		if err != nil {
			return nil, err
		}
		return unwrapKey(clinicDEK, encDEK, KeyScopePatient, e.kid, []byte(key))
	})
	if err == nil {
		return dek, nil
	}
	if !errors.Is(err, ErrKeyNotFound) {
		return nil, err
	}

	dek, created, err := e.createWrappedDEK(ctx, key, KeyScopePatient, clinicDEK, []byte(key))
	if err != nil {
		return nil, err
	}
	if !created {
		return e.GetPatientDEKForClinic(ctx, clinicID, patientID)
	}
	return dek, nil
}

// DeletePatientDEKForClinic removes the patient DEK and leaves a terminal
// tombstone in the keystore so future reads cannot recreate it.
func (e *Engine) DeletePatientDEKForClinic(ctx context.Context, clinicID, patientID uuid.UUID) error {
	key := urn.Patient(clinicID, patientID)
	err := e.keystore.DeleteDEK(ctx, key)
	if err == nil {
		forgetDEK(ctx, key)
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
	return newEncryptor(dek, KeyScopePatient, e.kid)
}

func (e *Engine) getWrappedDEKs(ctx context.Context, urns []string) (map[string]DEKResult, error) {
	urns = uniqueStrings(urns)
	if len(urns) == 0 {
		return map[string]DEKResult{}, nil
	}
	if batch, ok := e.keystore.(BatchKeyStore); ok {
		if e.metrics != nil {
			e.metrics.keyStoreBatchGet.Add(1)
		}
		return batch.GetDEKs(ctx, urns)
	}
	if e.metrics != nil {
		e.metrics.keyStoreGet.Add(uint64(len(urns)))
	}
	results := make(map[string]DEKResult, len(urns))
	for _, urn := range urns {
		value, err := e.keystore.GetDEK(ctx, urn)
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
		return nil, false, errors.Wrap(err, "crypto: generate dek")
	}
	encDEK, err := wrapKey(wrappingKey, dek, scope, e.kid, aad)
	if err != nil {
		ZeroBytes(dek)
		return nil, false, errors.Wrap(err, "crypto: wrap dek")
	}
	created, err := e.putIfAbsent(ctx, urn, encDEK)
	if err != nil {
		ZeroBytes(dek)
		return nil, false, errors.Wrap(err, "crypto: save dek to keystore")
	}
	if !created {
		ZeroBytes(dek)
		return nil, false, nil
	}
	cacheDEK(ctx, urn, dek)
	return dek, true, nil
}

func (e *Engine) putIfAbsent(ctx context.Context, urn string, encryptedDEK []byte) (bool, error) {
	if conditional, ok := e.keystore.(ConditionalKeyStore); ok {
		return conditional.PutIfAbsent(ctx, urn, encryptedDEK)
	}
	if err := e.keystore.PutDEK(ctx, urn, encryptedDEK); err != nil {
		return false, err
	}
	return true, nil
}
