package usecase_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"

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
	policies, err := policy.NewPolicyEngine(db, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := policies.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return usecase.NewService(db, slog.New(slog.DiscardHandler), policies)
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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

	pt, err := svc.Create(context.Background(), "clinic-1", "user-1", validInput())
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
	if got.DisplayName != "Maria Oliveira" || orEmpty(got.Document) != "123.456.789-00" {
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
			_, err := svc.Create(context.Background(), "clinic-1", "user-1", in)
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

	pt, err := svc.Create(context.Background(), "clinic-1", "user-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	in := validInput()
	in.DisplayName = "Maria O. Lima"
	updated, err := svc.Update(context.Background(), "clinic-1", pt.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Maria O. Lima" {
		t.Fatalf("Update = %+v", updated)
	}
	if _, err := svc.Update(context.Background(), "clinic-1", "missing", in); !errors.Is(err, usecase.ErrNotFound) {
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
		if _, err := svc.Create(context.Background(), "clinic-1", "user-1", in); err != nil {
			t.Fatal(err)
		}
	}

	all, total, err := svc.ListPage(context.Background(), "clinic-1", "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || total != 3 {
		t.Fatalf("ListPage = %d patients (total %d), want 3", len(all), total)
	}

	// Whole-word prefix: 're' must not match 'Moreno'.
	hit, _, err := svc.ListPage(context.Background(), "clinic-1", "bruno", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hit) != 1 || hit[0].DisplayName != "Bruno Lima" {
		t.Fatalf("search 'bruno' = %+v, want only Bruno Lima", hit)
	}
	moreno, _, err := svc.ListPage(context.Background(), "clinic-1", "re", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range moreno {
		if p.DisplayName == "Gustavo Moreno da Veiga" {
			t.Errorf("search 're' matched 'Moreno' via substring")
		}
	}

	none, _, err := svc.ListPage(context.Background(), "clinic-1", "zzz", "", 50, 0)
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
	if err := svc.SetStatus(context.Background(), "clinic-1", pt.ID, usecase.StatusInactive); err != nil {
		t.Fatal(err)
	}
	active, _, err := svc.ListPage(context.Background(), "clinic-1", "", usecase.StatusActive, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("active = %d, want 2", len(active))
	}
	inactive, _, err := svc.ListPage(context.Background(), "clinic-1", "", usecase.StatusInactive, 50, 0)
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

	pt, err := svc.Create(context.Background(), "clinic-1", "user-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetStatus(context.Background(), "clinic-1", pt.ID, usecase.StatusInactive); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(context.Background(), pt.ID)
	if got.Status != usecase.StatusInactive {
		t.Fatalf("status = %q, want inactive", got.Status)
	}
	if err := svc.SetStatus(context.Background(), "clinic-1", pt.ID, "banana"); err == nil {
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
	if _, err := svc.Create(context.Background(), "clinic-1", "user-1", in); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.Count(context.Background(), "clinic-1"); n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestCreateRecordsRegistrar(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	if _, err := db.Exec(`INSERT INTO users (id, email, password_hash, display_name, role)
		VALUES ('user-1', 'ana@example.org', 'x', 'Ana Souza', 'admin')`); err != nil {
		t.Fatal(err)
	}
	pt, err := svc.Create(context.Background(), "clinic-1", "user-1", validInput())
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.GetWithCreator(context.Background(), pt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if orEmpty(row.CreatedBy) != "user-1" {
		t.Fatalf("CreatedBy = %q, want user-1", orEmpty(row.CreatedBy))
	}
	if orEmpty(row.CreatedByEmail) != "ana@example.org" {
		t.Fatalf("CreatedByEmail = %q, want ana@example.org", orEmpty(row.CreatedByEmail))
	}
}

func TestGetWithCreatorMissingUser(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)

	pt, err := svc.Create(context.Background(), "clinic-1", "ghost", validInput())
	if err != nil {
		t.Fatal(err)
	}
	row, err := svc.GetWithCreator(context.Background(), pt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.CreatedByEmail != nil {
		t.Fatalf("CreatedByEmail = %q, want empty for unknown user", *row.CreatedByEmail)
	}
}

func TestAuthorizePatientEdit(t *testing.T) {
	db := openDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	admin := &auth.Principal{ID: "01990000-0000-7000-8000-000000000001", Email: "admin@c.org", Name: "Admin", Role: auth.RoleAdmin}
	owner := &auth.Principal{ID: "01990000-0000-7000-8000-000000000002", Email: "owner@c.org", Name: "Owner", Role: auth.RolePhysician}
	other := &auth.Principal{ID: "01990000-0000-7000-8000-000000000003", Email: "other@c.org", Name: "Other", Role: auth.RolePhysician}

	pt, err := svc.Create(ctx, "clinic-1", owner.ID, validInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AuthorizePatientEdit(ctx, admin, pt.ID, pt.CreatedBy, pt.Status); err != nil {
		t.Errorf("admin edit denied: %v", err)
	}
	if err := svc.AuthorizePatientEdit(ctx, owner, pt.ID, pt.CreatedBy, pt.Status); err != nil {
		t.Errorf("owner edit denied: %v", err)
	}
	if err := svc.AuthorizePatientEdit(ctx, other, pt.ID, pt.CreatedBy, pt.Status); !errors.Is(err, usecase.ErrForbidden) {
		t.Errorf("other physician err = %v, want ErrForbidden", err)
	}
}
