//go:build dqlite

package database

import (
	"context"
	"librevita.org/pkg/log"
	"path/filepath"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/ent"
	"librevita.org/ent/clinic"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/keystore"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierrepo "librevita.org/internal/domain/identifier/repository"
	identifierusecase "librevita.org/internal/domain/identifier/usecase"
	patientrepo "librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/pkg/ident"
)

// TestDqliteSpike connects to the local dqlite cluster (see
// /tmp/opencode/dqlite-node) through the pure-Go driver and exercises
// the full stack: goose migrations, a real transaction, and the FLE
// round trip. Skips when the cluster is not reachable.
func TestDqliteSpike(t *testing.T) {
	db, err := openDqlite("127.0.0.1:9001,127.0.0.1:9002,127.0.0.1:9003", "librevita")
	if err != nil {
		t.Skipf("no dqlite cluster: %v", err)
	}
	defer db.Close()

	err = Migrate(context.Background(), db, log.Nop())
	require.NoError(t, err)

	for _, table := range []string{"patient_identifiers", "patients", "users", "clinics"} {
		_, err := db.Exec("DELETE FROM " + table)
		require.NoError(t, err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	defer client.Close()

	// A real transaction: BEGIN/COMMIT through Ent.
	tx, err := client.Tx(context.Background())
	require.NoError(t, err)

	_, err = tx.Clinic.Create().
		SetID(ident.MustParseClinic("00000000-0000-0000-0000-0000000000d1")).
		SetSlug("dqlite").
		SetName("Dqlite").
		SetTaxID("1").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	// Rollback must not persist.
	tx, err = client.Tx(context.Background())
	require.NoError(t, err)

	_, err = tx.Clinic.Create().
		SetID(ident.MustParseClinic("00000000-0000-0000-0000-0000000000d2")).
		SetSlug("rolled").
		SetName("Rolled").
		SetTaxID("2").
		SetCountry("BR").
		SetTimezone("UTC").
		Save(context.Background())
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())

	count, err := client.Clinic.Query().Where(clinic.IDEQ(ident.MustParseClinic("00000000-0000-0000-0000-0000000000d2"))).Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// FLE round trip: encrypted identifier + blind index + duplicate.
	clinicID := "00000000-0000-0000-0000-0000000000d1"
	adminID := "00000000-0000-0000-0000-0000000000d5"
	adminRole, err := client.Role.Create().
		SetClinicID(ident.MustParseClinic(clinicID)).
		SetName("admin").
		SetSystem(true).
		Save(context.Background())
	require.NoError(t, err)

	_, err = client.User.Create().
		SetID(ident.MustParseUser(adminID)).
		SetClinicID(ident.MustParseClinic(clinicID)).
		SetEmail("admin@dqlite.test").
		SetPasswordHash("x").
		SetDisplayName("Admin").
		SetRoleID(adminRole.ID).
		Save(context.Background())
	require.NoError(t, err)

	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = v.Close() })

	engine, err := crypto.NewEngine("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	patientSvc := usecase.NewService(patientrepo.NewPatientRepository(client), nil, engine)
	createdPt, err := patientSvc.Create(context.Background(), clinicID, adminID, usecase.PatientInput{
		DisplayName: "P",
		Sex:         "unknown",
	})
	require.NoError(t, err)
	patientID := createdPt.ID

	reg := identifiermodel.NewRegistry()
	sysRepo := identifierrepo.NewSystemRepository(client)
	rows, err := sysRepo.ListActive(context.Background())
	require.NoError(t, err)
	require.NoError(t, reg.Reload(rows))

	clinicUUID := ident.MustParseClinic(clinicID)
	for _, row := range rows {
		_, err = client.ClinicIdentifierSystem.Create().
			SetClinicID(clinicUUID).
			SetIdentifierSystemID(row.ID).
			Save(context.Background())
		require.NoError(t, err)
	}

	masterKey, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	require.NoError(t, err)

	idRepo := identifierrepo.NewIdentifierRepository(client)
	svc := identifierusecase.NewService(idRepo, masterKey, reg, log.Nop())
	_, err = svc.AddIdentifier(context.Background(), clinicID, adminID, identifierusecase.Input{
		PatientID: patientID.String(), Value: "123.456.789-09",
	})
	require.NoError(t, err)

	found, err := svc.FindByValue(context.Background(), clinicID, "12345678909")
	require.NoError(t, err)
	assert.Len(t, found, 1)

	otherPt, err := patientSvc.Create(context.Background(), clinicID, adminID, usecase.PatientInput{
		DisplayName: "O",
		Sex:         "unknown",
	})
	require.NoError(t, err)
	other := otherPt.ID

	_, err = svc.AddIdentifier(context.Background(), clinicID, adminID, identifierusecase.Input{
		PatientID: other.String(), Value: "12345678909",
	})
	assert.ErrorIs(t, err, identifierusecase.ErrDuplicate)
	t.Log("dqlite spike OK: migrations, transactions, FLE")
}
