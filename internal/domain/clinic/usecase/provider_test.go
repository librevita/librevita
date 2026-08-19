package usecase_test

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"librevita.org/ent"
	"librevita.org/internal/core/database"
	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/clinic/usecase"
)

func openClockDB(t *testing.T) (*sql.DB, *ent.Client) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:clinic-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	if err := database.Migrate(context.Background(), db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := ent.NewClient(ent.Driver(drv))
	t.Cleanup(func() { client.Close() })

	return db, client
}

func seedClinic(t *testing.T, client *ent.Client, zone string) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Clinic.Create().
		SetID(id).
		SetName("Test Clinic").
		SetCountry("BR").
		SetTimezone(zone).
		Save(context.Background()); err != nil {
		t.Fatalf("create clinic: %v", err)
	}
}

func TestClockProviderReadsClinicZone(t *testing.T) {
	_, client := openClockDB(t)
	seedClinic(t, client, "America/New_York")

	repo := repository.NewClinicRepository(client)
	clock, err := usecase.NewClockProvider(repo).Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 6, 21, 4, 5, 0, time.UTC)
	if got, want := clock.FormatUI(utc), "2026-08-06 17:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
}

func TestClockProviderFallsBackBeforeOnboarding(t *testing.T) {
	_, client := openClockDB(t)

	repo := repository.NewClinicRepository(client)
	clock, err := usecase.NewClockProvider(repo).Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	if got, want := clock.FormatUI(utc), "2026-08-06 15:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
}

func TestClockProviderRefreshesAfterTTL(t *testing.T) {
	_, client := openClockDB(t)
	seedClinic(t, client, "America/New_York")

	repo := repository.NewClinicRepository(client)
	provider := usecase.NewClockProvider(repo)
	if _, err := provider.Clock(context.Background()); err != nil {
		t.Fatal(err)
	}

	id := clinicID(t, client)
	if err := client.Clinic.UpdateOneID(uuid.MustParse(id)).SetTimezone("Asia/Tokyo").Exec(context.Background()); err != nil {
		t.Fatalf("update clinic: %v", err)
	}

	clock, err := provider.Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	if got, want := clock.FormatUI(utc), "2026-08-06 14:04"; got != want {
		t.Fatalf("cached FormatUI = %q, want %q", got, want)
	}

	// Expire cache manually by creating new provider
	providerNew := usecase.NewClockProvider(repo)
	clock, err = providerNew.Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := clock.FormatUI(utc), "2026-08-07 03:04"; got != want {
		t.Fatalf("refreshed FormatUI = %q, want %q", got, want)
	}
}

func clinicID(t *testing.T, client *ent.Client) string {
	t.Helper()
	row, err := client.Clinic.Query().First(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return row.ID.String()
}
