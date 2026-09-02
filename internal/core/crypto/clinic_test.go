package crypto_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/keystore"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/urn"
)

func TestClinicAndPatientDEKLifecycle(t *testing.T) {
	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	defer func() { _ = v.Close() }()

	engine, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	require.NoError(t, err)

	ctx := crypto.WithRequestKeyCache(context.Background())
	clinicID := ident.New[ident.ClinicID]()
	patientID := ident.New[ident.PatientID]()
	clinicURN := urn.Clinic(clinicID)

	// 1. EnsureClinicDEK (creates)
	clinicDEK, err := engine.EnsureClinicDEK(ctx, clinicID)
	require.NoError(t, err)
	assert.Len(t, clinicDEK, crypto.SizeDEK)

	// 2. GetClinicDEK (cache hit or read)
	gotDEK, err := engine.GetClinicDEK(ctx, clinicID)
	require.NoError(t, err)
	assert.Equal(t, clinicDEK, gotDEK)

	// 3. URN-based clinic methods
	gotURN, err := engine.GetClinicDEKForURN(ctx, clinicURN)
	require.NoError(t, err)
	assert.Equal(t, clinicDEK, gotURN)

	ensuredURN, err := engine.EnsureClinicDEKForURN(ctx, clinicURN)
	require.NoError(t, err)
	assert.Equal(t, clinicDEK, ensuredURN)

	// 4. Patient DEK lifecycle
	patDEK, err := engine.EnsurePatientDEKForClinic(ctx, clinicID, patientID)
	require.NoError(t, err)
	assert.Len(t, patDEK, crypto.SizeDEK)

	gotPatDEK, err := engine.GetPatientDEKForClinic(ctx, clinicID, patientID)
	require.NoError(t, err)
	assert.Equal(t, patDEK, gotPatDEK)

	// 5. Batch patient DEKs
	patID2 := ident.New[ident.PatientID]()
	patDEK2, err := engine.EnsurePatientDEKForClinic(ctx, clinicID, patID2)
	require.NoError(t, err)

	batchMap, err := engine.GetPatientDEKsForClinic(ctx, clinicID, []ident.PatientID{patientID, patID2})
	require.NoError(t, err)
	assert.Len(t, batchMap, 2)
	assert.Equal(t, patDEK, batchMap[patientID])
	assert.Equal(t, patDEK2, batchMap[patID2])

	// 6. ResolvePatientEncryptor
	enc, err := engine.ResolvePatientEncryptor(ctx, clinicID, patientID)
	require.NoError(t, err)
	require.NotNil(t, enc)

	ciphertext, err := enc.Encrypt([]byte("patient-secret-record"), []byte("aad"))
	require.NoError(t, err)
	plaintext, err := enc.Decrypt(ciphertext, []byte("aad"))
	require.NoError(t, err)
	assert.Equal(t, []byte("patient-secret-record"), plaintext)

	// 7. Delete patient DEK (shred)
	require.NoError(t, engine.DeletePatientDEKForClinic(ctx, clinicID, patientID))
	_, err = engine.GetPatientDEKForClinic(ctx, clinicID, patientID)
	assert.Error(t, err)

	// 8. Delete clinic DEK
	require.NoError(t, engine.DeleteClinicDEK(ctx, clinicID))
	_, err = engine.GetClinicDEK(ctx, clinicID)
	assert.Error(t, err)

	// 9. Error handling on invalid URN
	_, err = engine.GetClinicDEKForURN(ctx, "invalid-urn")
	assert.Error(t, err)
	_, err = engine.EnsureClinicDEKForURN(ctx, "invalid-urn")
	assert.Error(t, err)
	assert.Error(t, engine.DeleteClinicDEKForURN(ctx, "invalid-urn"))
	_, err = engine.SetupClinicDEK(ctx, "invalid-urn")
	assert.Error(t, err)
}
