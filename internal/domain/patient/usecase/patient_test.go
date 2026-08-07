package usecase_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	_ "modernc.org/sqlite"

	"librevita.org/internal/core/database"
	"librevita.org/internal/domain/patient/usecase"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:patient-test?mode=memory&cache=shared")
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

func newService(t *testing.T, db *sql.DB) *usecase.Service {
	t.Helper()
	return usecase.NewService(db, slog.New(slog.DiscardHandler))
}

func validInput() usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: "Maria Oliveira",
		BirthDate:   "1985-03-14",
		Sex:         "female",
		Document:    "123.456.789-00",
		Phone:       "+55 11 99999-0000",
		Email:       "maria@example.org",
		City:        "São Paulo",
		State:       "SP",
	}
}

func TestCreateAndGet(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	pt, err := svc.Create(context.Background(), "clinic-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	if pt.ID == "" || pt.Status != "active" {
		t.Fatalf("created patient = %+v, want id and active status", pt)
	}

	got, err := svc.Get(context.Background(), pt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Maria Oliveira" || got.Document.String != "123.456.789-00" {
		t.Fatalf("Get = %+v", got)
	}
}

func TestCreateValidation(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	cases := []struct {
		name   string
		mutate func(*usecase.PatientInput)
	}{
		{"missing name", func(in *usecase.PatientInput) { in.DisplayName = " " }},
		{"bad sex", func(in *usecase.PatientInput) { in.Sex = "alien" }},
		{"bad birth date", func(in *usecase.PatientInput) { in.BirthDate = "14/03/1985" }},
		{"bad email", func(in *usecase.PatientInput) { in.Email = "not-an-email" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			_, err := svc.Create(context.Background(), "clinic-1", in)
			var v *usecase.ValidationError
			if !errors.As(err, &v) {
				t.Fatalf("Create = %v, want ValidationError", err)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	pt, err := svc.Create(context.Background(), "clinic-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	in := validInput()
	in.DisplayName = "Maria O. Lima"
	updated, err := svc.Update(context.Background(), pt.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Maria O. Lima" {
		t.Fatalf("Update = %+v", updated)
	}
	if _, err := svc.Update(context.Background(), "missing", in); !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("Update missing = %v, want ErrNotFound", err)
	}
}

func TestListAndSearch(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	for _, name := range []string{"Ana Souza", "Bruno Lima", "Carla Dias"} {
		in := validInput()
		in.DisplayName = name
		in.Document = name
		if _, err := svc.Create(context.Background(), "clinic-1", in); err != nil {
			t.Fatal(err)
		}
	}

	all, err := svc.List(context.Background(), "clinic-1", "", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("List = %d patients, want 3", len(all))
	}

	hit, err := svc.List(context.Background(), "clinic-1", "bruno", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 1 || hit[0].DisplayName != "Bruno Lima" {
		t.Fatalf("search 'bruno' = %+v, want only Bruno Lima", hit)
	}

	none, err := svc.List(context.Background(), "clinic-1", "zzz", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("search 'zzz' = %d, want 0", len(none))
	}

	// Status filter.
	pt, err := svc.Get(context.Background(), hit[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetStatus(context.Background(), pt.ID, usecase.StatusInactive); err != nil {
		t.Fatal(err)
	}
	active, err := svc.List(context.Background(), "clinic-1", "", usecase.StatusActive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("active = %d, want 2", len(active))
	}
	inactive, err := svc.List(context.Background(), "clinic-1", "", usecase.StatusInactive, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(inactive) != 1 {
		t.Fatalf("inactive = %d, want 1", len(inactive))
	}
}

func TestSetStatus(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	pt, err := svc.Create(context.Background(), "clinic-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetStatus(context.Background(), pt.ID, usecase.StatusInactive); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), pt.ID)
	if got.Status != usecase.StatusInactive {
		t.Fatalf("status = %q, want inactive", got.Status)
	}
	if err := svc.SetStatus(context.Background(), pt.ID, "banana"); err == nil {
		t.Fatal("invalid status accepted")
	}
}

func TestCount(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	if n, _ := svc.Count(context.Background(), "clinic-1"); n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}
	in := validInput()
	if _, err := svc.Create(context.Background(), "clinic-1", in); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Count(context.Background(), "clinic-1"); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}
