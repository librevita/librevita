package crypto_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/keystore"
)

func BenchmarkBatchPatientDEKResolution(b *testing.B) {
	for _, size := range []int{1, 20, 50} {
		b.Run(fmt.Sprintf("patients_%d", size), func(b *testing.B) {
			v, err := keystore.OpenBBolt(filepath.Join(b.TempDir(), "keystore.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = v.Close() })

			master := make([]byte, crypto.SizeDEK)
			if _, err := rand.Read(master); err != nil {
				b.Fatal(err)
			}
			engine, err := crypto.NewEngine(base64.StdEncoding.EncodeToString(master), v)
			if err != nil {
				b.Fatal(err)
			}
			crypto.ZeroBytes(master)
			clinicID := ident.ClinicID(uuid.New())
			setupCtx := context.Background()
			patientIDs := make([]ident.PatientID, 0, size)
			for i := 0; i < size; i++ {
				pid := ident.PatientID(uuid.New())
				patientIDs = append(patientIDs, pid)
				if _, err := engine.EnsurePatientDEKForClinic(setupCtx, clinicID, pid); err != nil {
					b.Fatal(err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := crypto.WithRequestKeyCache(context.Background())
				deks, err := engine.GetPatientDEKsForClinic(ctx, clinicID, patientIDs)
				if err != nil {
					b.Fatal(err)
				}
				for _, dek := range deks {
					crypto.ZeroBytes(dek)
				}
				crypto.ClearRequestKeyCache(ctx)
			}
		})
	}
}
