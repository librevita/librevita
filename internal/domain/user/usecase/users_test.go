package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/types"
)

var (
	testClinic  = uuid.MustParse("00000000-0000-0000-0000-000000000011")
	testClinic2 = uuid.MustParse("00000000-0000-0000-0000-000000000012")
	testAdminID = uuid.MustParse("00000000-0000-0000-0000-000000000021")
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
	loaded, err := svc.GetUser(ctx, user.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RoleName != "physician" || user.DisplayName != "Dr. Lima" {
		t.Errorf("CreateUser = %+v, want physician/Dr. Lima", user)
	}
	if !user.Active {
		t.Errorf("new account must start active, got %v", user.Active)
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

	updated, err := svc.UpdateUser(ctx, staff.ID.String(), "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: false,
	})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	loaded, err := svc.GetUser(ctx, staff.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Nurse Chefe" || loaded.RoleName != "physician" || updated.Active {
		t.Errorf("UpdateUser = %+v", updated)
	}

	// Reactivate.
	updated, err = svc.UpdateUser(ctx, staff.ID.String(), "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: true,
	})
	if err != nil {
		t.Fatalf("reactivate: %v", err)
	}
	if !updated.Active {
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
	if _, err := svc.UpdateUser(ctx, admin.ID, second.ID.String(), usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "receptionist", Active: true,
	}); err != nil {
		t.Errorf("demote with backup admin: %v", err)
	}

	// The second admin alone cannot be deactivated either.
	_, err = svc.UpdateUser(ctx, second.ID.String(), second.ID.String(), usecase.UpdateUserInput{
		Name: "Backup", Email: "backup@example.org", Role: "admin", Active: false,
	})
	if !errors.Is(err, usecase.ErrCannotDemoteSelf) {
		t.Errorf("last admin self deactivate err = %v, want ErrCannotDemoteSelf", err)
	}

	// Once the first admin was demoted, the second is the last active
	// admin: demoting it must be refused regardless of the actor.
	_, err = svc.UpdateUser(ctx, second.ID.String(), admin.ID, usecase.UpdateUserInput{
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

func TestSpecialties(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	psy, err := svc.CreateSpecialty(ctx, testClinic.String(), "Psychologist")
	if err != nil {
		t.Fatal(err)
	}
	physio, err := svc.CreateSpecialty(ctx, testClinic.String(), "Physiotherapist")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSpecialty(ctx, testClinic.String(), " psychologist "); !errors.Is(err, usecase.ErrDuplicateSpecialty) {
		t.Errorf("duplicate specialty err = %v, want ErrDuplicateSpecialty", err)
	}
	if _, err := svc.CreateSpecialty(ctx, testClinic.String(), ""); err == nil {
		t.Errorf("empty specialty name must fail")
	}
	// The other clinic has its own catalog.
	if _, err := svc.CreateSpecialty(ctx, testClinic2.String(), "Psychologist"); err != nil {
		t.Errorf("same name in another clinic must be allowed: %v", err)
	}

	rows, err := svc.ListSpecialties(ctx, testClinic.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("ListSpecialties = %d, want 2", len(rows))
	}

	// Assign more than one specialty to a user.
	staff, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Dr. Ana", Email: "ana.sp@example.org", Password: "senha-segura", Role: "physician",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserSpecialties(ctx, staff.ID.String(), []string{psy.ID.String(), physio.ID.String()}); err != nil {
		t.Fatal(err)
	}
	assigned, err := svc.UserSpecialties(ctx, staff.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 2 {
		t.Fatalf("UserSpecialties = %d, want 2", len(assigned))
	}

	// Replacing the set drops the first assignment.
	if err := svc.SetUserSpecialties(ctx, staff.ID.String(), []string{physio.ID.String()}); err != nil {
		t.Fatal(err)
	}
	assigned, err = svc.UserSpecialties(ctx, staff.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 1 || assigned[0].ID.String() != physio.ID.String() {
		t.Fatalf("after replace = %+v, want only physiotherapy", assigned)
	}

	// Deleting the specialty removes the mapping too.
	if err := svc.DeleteSpecialty(ctx, testClinic.String(), physio.ID.String()); err != nil {
		t.Fatal(err)
	}
	assigned, err = svc.UserSpecialties(ctx, staff.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 0 {
		t.Fatalf("after specialty delete = %d, want 0", len(assigned))
	}
}

func TestStaffChangeRequests(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	phys, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Dr. Lima", Email: "dr.lima@example.org", Password: "senha-segura", Role: "physician",
	})
	if err != nil {
		t.Fatal(err)
	}
	receptionist, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Nurse", Email: "nurse@example.org", Password: "senha-segura", Role: "receptionist",
	})
	if err != nil {
		t.Fatal(err)
	}
	psy, err := svc.CreateSpecialty(ctx, testClinic.String(), "Psychologist")
	if err != nil {
		t.Fatal(err)
	}

	// Receptionist proposes changes.
	req, err := svc.CreateStaffChangeRequest(ctx, phys.ID.String(), receptionist.ID.String(), usecase.StaffChange{
		Name: "Dr. Lima Jr", Email: "dr.lima@example.org", Specialties: []string{psy.ID.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	// An email that belongs to another account is caught at request time.
	other, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Other", Email: "other@example.org", Password: "senha-segura", Role: "receptionist",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateStaffChangeRequest(ctx, phys.ID.String(), receptionist.ID.String(), usecase.StaffChange{
		Name: "Dr. Lima", Email: "other@example.org", Specialties: nil,
	}); !errors.Is(err, usecase.ErrEmailInUse) {
		t.Errorf("email collision err = %v, want ErrEmailInUse", err)
	}
	_ = other

	pend, total, err := svc.ListStaffChangeRequestsFiltered(ctx, types.StaffRequestPending.String(), "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pend) != 1 || total != 1 {
		t.Fatalf("pending = %d (total %d), want 1", len(pend), total)
	}
	all, totalAll, err := svc.ListStaffChangeRequestsFiltered(ctx, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if totalAll != 1 {
		t.Fatalf("all statuses total = %d, want 1", totalAll)
	}
	_ = all

	// Approving applies the changes.
	if err := svc.ApproveStaffChangeRequest(ctx, req.ID.String(), testAdminID.String()); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.GetUser(ctx, phys.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Dr. Lima Jr" {
		t.Errorf("approved name = %q, want Dr. Lima Jr", updated.DisplayName)
	}
	assigned, err := svc.UserSpecialties(ctx, phys.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(assigned) != 1 || assigned[0].ID != psy.ID {
		t.Errorf("approved specialties = %+v, want the psychologist", assigned)
	}
	// A second approval of the same request is refused.
	if err := svc.ApproveStaffChangeRequest(ctx, req.ID.String(), testAdminID.String()); !errors.Is(err, usecase.ErrRequestNotPending) {
		t.Errorf("double approve err = %v, want ErrRequestNotPending", err)
	}

	// Rejection keeps the profile untouched.
	req2, err := svc.CreateStaffChangeRequest(ctx, phys.ID.String(), receptionist.ID.String(), usecase.StaffChange{
		Name: "Dr. Nope", Email: "dr.lima@example.org", Specialties: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.RejectStaffChangeRequest(ctx, req2.ID.String(), testAdminID.String(), "not needed"); err != nil {
		t.Fatal(err)
	}
	updated, err = svc.GetUser(ctx, phys.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Dr. Lima Jr" {
		t.Errorf("after reject name = %q, want unchanged", updated.DisplayName)
	}
}

func TestListPhysicians(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	phys, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Dr. Ana", Email: "dr.ana@example.org", Password: "senha-segura", Role: "physician",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Nurse", Email: "nurse@example.org", Password: "senha-segura", Role: "receptionist",
	}); err != nil {
		t.Fatal(err)
	}
	psy, err := svc.CreateSpecialty(ctx, testClinic.String(), "Psychologist")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserSpecialties(ctx, phys.ID.String(), []string{psy.ID.String()}); err != nil {
		t.Fatal(err)
	}

	rows, total, err := svc.ListPhysiciansPage(ctx, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || total != 1 {
		t.Fatalf("ListPhysiciansPage = %d (total %d), want 1", len(rows), total)
	}
	if rows[0].Specialties != "Psychologist" {
		t.Errorf("joined specialties = %q, want Psychologist", rows[0].Specialties)
	}
}

func TestRolesCRUD(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	// The four system roles are seeded by the migration.
	rows, err := svc.ListRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("ListRoles = %d, want 4 seeded roles", len(rows))
	}

	psy, err := svc.CreateRole(ctx, "psychologist", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRole(ctx, " psychologist ", true); !errors.Is(err, usecase.ErrDuplicateRole) {
		t.Errorf("duplicate role err = %v, want ErrDuplicateRole", err)
	}

	// System roles cannot be renamed or deleted.
	if _, err := svc.RenameRole(ctx, rows[0].ID.String(), "director"); !errors.Is(err, usecase.ErrSystemRole) {
		t.Errorf("rename system err = %v, want ErrSystemRole", err)
	}
	if err := svc.DeleteRole(ctx, rows[0].ID.String()); !errors.Is(err, usecase.ErrSystemRole) {
		t.Errorf("delete system err = %v, want ErrSystemRole", err)
	}

	// A role in use cannot be deleted.
	staff, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Psy", Email: "psy@example.org", Password: "senha-segura", Role: "psychologist",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = staff
	if err := svc.DeleteRole(ctx, psy.ID.String()); !errors.Is(err, usecase.ErrRoleInUse) {
		t.Errorf("delete in-use err = %v, want ErrRoleInUse", err)
	}

	// Rename works on custom roles and the account reflects it.
	renamed, err := svc.RenameRole(ctx, psy.ID.String(), "psychotherapist")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "psychotherapist" {
		t.Errorf("renamed = %q, want psychotherapist", renamed.Name)
	}
	loaded, err := svc.GetUser(ctx, staff.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RoleName != "psychotherapist" {
		t.Errorf("account role after rename = %q, want psychotherapist", loaded.RoleName)
	}

	// The custom role works in account creation, so it stays in use and
	// cannot be deleted; an unused role can.
	spare, err := svc.CreateRole(ctx, "spare", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteRole(ctx, spare.ID.String()); err != nil {
		t.Fatalf("delete unused role: %v", err)
	}
}

func TestUpdatePreferences(t *testing.T) {
	db := openAuthDB(t)
	svc := newService(t, db)
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Ana", Email: "ana.prefs@example.org", Password: "senha-segura", Role: "physician",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Invalid theme and timezone are rejected before touching the row.
	if err := svc.UpdatePreferences(ctx, user.ID.String(), "America/Sao_Paulo", types.UITheme("sepia")); err == nil {
		t.Error("invalid theme must be rejected")
	}
	if err := svc.UpdatePreferences(ctx, user.ID.String(), "Mars/Olympus", types.UIThemeDark); err == nil {
		t.Error("unknown timezone must be rejected")
	}

	if err := svc.UpdatePreferences(ctx, user.ID.String(), "Asia/Tokyo", types.UIThemeDark); err != nil {
		t.Fatalf("UpdatePreferences: %v", err)
	}
	loaded, err := svc.GetUser(ctx, user.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Timezone != "Asia/Tokyo" || loaded.UiTheme != types.UIThemeDark {
		t.Errorf("preferences = %q/%q, want Asia/Tokyo/dark", loaded.Timezone, loaded.UiTheme)
	}

	// Empty timezone means "inherit the clinic timezone".
	if err := svc.UpdatePreferences(ctx, user.ID.String(), "", types.UIThemeSystem); err != nil {
		t.Fatalf("reset preferences: %v", err)
	}
	loaded, err = svc.GetUser(ctx, user.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Timezone != "" || loaded.UiTheme != types.UIThemeSystem {
		t.Errorf("reset preferences = %q/%q, want empty/system", loaded.Timezone, loaded.UiTheme)
	}
}
