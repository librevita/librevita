package usecase_test

import (
	"context"
	"errors"
	"testing"

	"librevita.org/internal/domain/user/usecase"
)

func TestCreateUser(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Dr. Lima", Email: "dr.lima@example.org", Password: "senha-segura", Role: "physician",
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Role != "physician" || user.DisplayName != "Dr. Lima" {
		t.Errorf("CreateUser = %+v, want physician/Dr. Lima", user)
	}
	if user.Active != 1 {
		t.Errorf("new account must start active, got %d", user.Active)
	}
	if user.PasswordHash == "" || user.PasswordHash == "senha-segura" {
		t.Errorf("password must be hashed")
	}

	// Duplicate email (case-insensitive) maps to ErrEmailTaken.
	_, err = svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Other", Email: "DR.LIMA@example.org", Password: "senha-segura", Role: "patient",
	})
	if !errors.Is(err, usecase.ErrEmailTaken) {
		t.Errorf("duplicate email err = %v, want ErrEmailTaken", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	cases := []usecase.CreateUserInput{
		{Name: "", Email: "a@b.org", Password: "senha-segura", Role: "physician"},
		{Name: "X", Email: "not-an-email", Password: "senha-segura", Role: "physician"},
		{Name: "X", Email: "a@b.org", Password: "short", Role: "physician"},
		{Name: "X", Email: "a@b.org", Password: "senha-segura", Role: "superuser"},
	}
	for _, in := range cases {
		if _, err := svc.CreateUser(ctx, in); err == nil {
			t.Errorf("CreateUser(%+v) should fail validation", in)
		}
	}
}

func TestUpdateUserRoleAndStatus(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	staff, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Nurse", Email: "nurse@example.org", Password: "senha-segura", Role: "receptionist",
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.UpdateUser(ctx, staff.ID, "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: false,
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if updated.DisplayName != "Nurse Chefe" || updated.Role != "physician" || updated.Active != 0 {
		t.Errorf("UpdateUser = %+v", updated)
	}

	// Reactivate.
	updated, err = svc.UpdateUser(ctx, staff.ID, "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: true,
	})
	if err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if updated.Active != 1 {
		t.Errorf("reactivated user must be active")
	}
}

func TestUpdateUserAntiLockout(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	// Only one admin exists (created by the test fixture below).
	admin, _, err := svc.Onboard(ctx, usecase.RegisterInput{
		Name: "Admin", Email: "admin@example.org", Password: "senha-segura",
	}, usecase.ClinicInput{Name: "Clinic", Timezone: "America/Sao_Paulo"})
	if err != nil {
		t.Fatalf("Onboard: %v", err)
	}

	// The last active admin cannot demote itself.
	_, err = svc.UpdateUser(ctx, admin.ID, admin.ID, usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "patient", Active: true,
	})
	if !errors.Is(err, usecase.ErrCannotDemoteSelf) {
		t.Errorf("self demote err = %v, want ErrCannotDemoteSelf", err)
	}
	_, err = svc.UpdateUser(ctx, admin.ID, admin.ID, usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "admin", Active: false,
	})
	if !errors.Is(err, usecase.ErrCannotDemoteSelf) {
		t.Errorf("self deactivate err = %v, want ErrCannotDemoteSelf", err)
	}

	// A second admin makes the first demotable.
	second, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Backup", Email: "backup@example.org", Password: "senha-segura", Role: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUser(ctx, admin.ID, second.ID, usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "receptionist", Active: true,
	}); err != nil {
		t.Errorf("demote with backup admin: %v", err)
	}

	// The second admin alone cannot be deactivated either.
	_, err = svc.UpdateUser(ctx, second.ID, second.ID, usecase.UpdateUserInput{
		Name: "Backup", Email: "backup@example.org", Role: "admin", Active: false,
	})
	if !errors.Is(err, usecase.ErrCannotDemoteSelf) {
		t.Errorf("last admin self deactivate err = %v, want ErrCannotDemoteSelf", err)
	}

	// Once the first admin was demoted, the second is the last active
	// admin: demoting it must be refused regardless of the actor.
	_, err = svc.UpdateUser(ctx, second.ID, admin.ID, usecase.UpdateUserInput{
		Name: "Backup", Email: "backup@example.org", Role: "patient", Active: true,
	})
	if !errors.Is(err, usecase.ErrLastActiveAdmin) {
		t.Errorf("demoting the last admin err = %v, want ErrLastActiveAdmin", err)
	}
}

func TestListUsersSearch(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	for _, in := range []usecase.CreateUserInput{
		{Name: "Ana Lima", Email: "ana@example.org", Password: "senha-segura", Role: "physician"},
		{Name: "Bia Reis", Email: "bia@example.org", Password: "senha-segura", Role: "receptionist"},
	} {
		if _, err := svc.CreateUser(ctx, in); err != nil {
			t.Fatal(err)
		}
	}

	rows, _, err := svc.ListUsersPage(ctx, "ANA", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Email != "ana@example.org" {
		t.Errorf("search 'ANA' = %+v", rows)
	}

	// Whole-word prefix: the term must start a word or the email value.
	rows, _, err = svc.ListUsersPage(ctx, "ana@example.org", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("search 'ana@example.org' = %d rows, want 1", len(rows))
	}
	rows, _, err = svc.ListUsersPage(ctx, "example.org", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("search 'example.org' = %d rows, want 0 (not a word prefix)", len(rows))
	}
}
