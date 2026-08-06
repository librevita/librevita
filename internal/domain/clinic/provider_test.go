package clinic

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/google/uuid"

	"librevita.org/internal/core/database"
	"librevita.org/internal/domain/clinic/repository"
)

func openClockDB(t *testing.T) *sql.DB {
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
	return db
}

func seedClinic(t *testing.T, db *sql.DB, zone string) {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.New(db).CreateClinic(context.Background(), repository.CreateClinicParams{
		ID: id.String(), Name: "Test Clinic", Country: "BR", Timezone: zone,
	}); err != nil {
		t.Fatalf("create clinic: %v", err)
	}
}

func TestClockProviderReadsClinicZone(t *testing.T) {
	db := openClockDB(t)
	seedClinic(t, db, "America/New_York")

	clock, err := NewClockProvider(db).Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 6, 21, 4, 5, 0, time.UTC)
	if got, want := clock.FormatUI(utc), "2026-08-06 17:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
}

func TestClockProviderFallsBackBeforeOnboarding(t *testing.T) {
	db := openClockDB(t)

	clock, err := NewClockProvider(db).Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	utc := time.Date(2026, 8, 6, 18, 4, 5, 0, time.UTC)
	if got, want := clock.FormatUI(utc), "2026-08-06 15:04"; got != want {
		t.Fatalf("FormatUI = %q, want %q", got, want)
	}
}

func TestClockProviderRefreshesAfterTTL(t *testing.T) {
	db := openClockDB(t)
	seedClinic(t, db, "America/New_York")

	provider := NewClockProvider(db)
	if _, err := provider.Clock(context.Background()); err != nil {
		t.Fatal(err)
	}

	id := clinicID(t, db)
	if err := repository.New(db).UpdateClinicTimezone(context.Background(),
		repository.UpdateClinicTimezoneParams{ID: id, Timezone: "Asia/Tokyo"}); err != nil {
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

	provider.exp = time.Now().Add(-time.Second)
	clock, err = provider.Clock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := clock.FormatUI(utc), "2026-08-07 03:04"; got != want {
		t.Fatalf("refreshed FormatUI = %q, want %q", got, want)
	}
}

func clinicID(t *testing.T, db *sql.DB) string {
	t.Helper()
	row, err := repository.New(db).GetClinic(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return row.ID
}
