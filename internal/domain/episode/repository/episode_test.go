package repository_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/ent/enttest"
	"librevita.org/internal/core/crypto"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/repository"
)

const testKeyB64 = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=" // gitleaks:allow

func TestEpisodeRepository_SOAPAggregate(t *testing.T) {
	db, err := sql.Open("sqlite", "file:episode?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	key, err := base64.StdEncoding.DecodeString(testKeyB64)
	require.NoError(t, err)
	hasher, err := crypto.NewClinicIndexHasher(key)
	require.NoError(t, err)
	encryptor, err := crypto.NewPatientEncryptor(key)
	require.NoError(t, err)
	client.Use(ent.FLEMutationHook(hasher, encryptor))
	client.Intercept(ent.FLEDecryptionInterceptor(encryptor))

	ctx := context.Background()
	clinicID := ident.ClinicID(uuid.New())
	_, err = client.Clinic.Create().SetID(clinicID).SetSlug("soap").SetName("SOAP Clinic").Save(ctx)
	require.NoError(t, err)
	roleID := ident.RoleID(uuid.New())
	_, err = client.Role.Create().SetID(roleID).SetClinicID(clinicID).SetName("physician").SetSystem(true).Save(ctx)
	require.NoError(t, err)
	userID := ident.UserID(uuid.New())
	_, err = client.User.Create().SetID(userID).SetClinicID(clinicID).SetRoleID(roleID).
		SetEmail("dr@example.org").SetDisplayName("Dr").SetPasswordHash("x").Save(ctx)
	require.NoError(t, err)
	patientID := ident.PatientID(uuid.New())
	_, err = client.Patient.Create().SetID(patientID).SetClinicID(clinicID).
		SetDisplayName("Ana").SetStatus("active").SetPhone("1").SetEmail("a@b.c").Save(ctx)
	require.NoError(t, err)

	repo := repository.NewEpisodeRepository(client)
	now := time.Now().UTC().Truncate(time.Second)
	findingID := ident.FindingID(uuid.New())
	ep := episodemodel.Episode{
		ID: ident.EpisodeID(uuid.New()), ClinicID: clinicID, PatientID: patientID, AuthorID: userID,
		Type: episodemodel.EpisodeTypeEvolution, Status: episodemodel.EpisodeStatusDraft,
		Class: episodemodel.CareSettingAmbulatory, OccurredAt: now,
		SOAP: episodemodel.SOAP{Subjective: "queixa", Objective: "exame", Assessment: "dx", Plan: "rx"},
		Findings: []episodemodel.Finding{{
			ID: findingID, Status: episodemodel.FindingStatusRecorded,
			Code:        episodemodel.Coding{System: "http://loinc.org", Code: "8480-6", Display: "SBP"},
			Value:       episodemodel.FindingValue{Kind: episodemodel.FindingValueQuantity, Quantity: &episodemodel.Quantity{Value: 120, Unit: "mmHg"}},
			EffectiveAt: now,
		}},
		Problems: []episodemodel.Problem{{
			ID: ident.ProblemID(uuid.New()), ClinicalStatus: episodemodel.ProblemClinicalActive,
			VerificationStatus: episodemodel.ProblemVerificationConfirmed,
			Category:           episodemodel.ProblemCategoryEncounter, Rank: 1,
			Code: episodemodel.Coding{System: "http://hl7.org/fhir/sid/icd-10", Code: "G43.9"},
			Text: "enxaqueca",
		}},
		PlanItems: []episodemodel.PlanItem{{
			ID: ident.PlanItemID(uuid.New()), Kind: episodemodel.PlanItemKindInstruction,
			Status: episodemodel.PlanItemStatusActive, Description: "retorno",
		}},
	}

	saved, err := repo.Create(ctx, ep)
	require.NoError(t, err)
	assert.Equal(t, "queixa", saved.SOAP.Subjective)
	require.Len(t, saved.Findings, 1)
	assert.Equal(t, 120.0, saved.Findings[0].Value.Quantity.Value)
	require.Len(t, saved.Problems, 1)
	assert.Equal(t, "enxaqueca", saved.Problems[0].Text)
	require.Len(t, saved.PlanItems, 1)

	saved.SOAP.Plan = "ajuste"
	saved.Findings = nil
	updated, err := repo.UpdateDraft(ctx, *saved)
	require.NoError(t, err)
	assert.Equal(t, "ajuste", updated.SOAP.Plan)
	assert.Empty(t, updated.Findings)

	require.NoError(t, repo.SetStatus(ctx, clinicID, saved.ID, episodemodel.EpisodeStatusFinalized))
	got, err := repo.Get(ctx, clinicID, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, episodemodel.EpisodeStatusFinalized, got.Status)
	_, err = repo.UpdateDraft(ctx, *got)
	assert.ErrorIs(t, err, episodemodel.ErrNotDraft)

	list, err := repo.ListByPatient(ctx, clinicID, patientID, nil)
	require.NoError(t, err)
	assert.Len(t, list, 1)

	amendment := *got
	amendment.ID = ident.EpisodeID(uuid.New())
	amendment.Status = episodemodel.EpisodeStatusDraft
	amendment.PredecessorID = &got.ID
	amendment.Findings, amendment.Problems, amendment.PlanItems = nil, nil, nil
	child, err := repo.Create(ctx, amendment)
	require.NoError(t, err)
	require.NotNil(t, child.PredecessorID)
	assert.Equal(t, got.ID, *child.PredecessorID)

	dup := amendment
	dup.ID = ident.EpisodeID(uuid.New())
	_, err = repo.Create(ctx, dup)
	assert.ErrorIs(t, err, episodemodel.ErrAlreadyAmended)

	found, err := repo.GetByPredecessor(ctx, clinicID, got.ID)
	require.NoError(t, err)
	assert.Equal(t, child.ID, found.ID)

	parent, err := repo.Get(ctx, clinicID, got.ID)
	require.NoError(t, err)
	require.NotNil(t, parent.SuccessorID)
	assert.Equal(t, child.ID, *parent.SuccessorID)
}
