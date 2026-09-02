package usecase_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/auth"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/internal/domain/user/usecase"
)

var (
	testClinic  = ident.MustParseClinic("00000000-0000-0000-0000-000000000011")
	testAdminID = ident.MustParseUser("00000000-0000-0000-0000-000000000021")
)

func TestCreateUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	physicianRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000003")
	userID := ident.MustParseUser("01990000-0000-7000-8000-000000000010")

	env.roleRepo.EXPECT().GetByName(ctx, "physician").Return(&usermodel.Role{
		ID:   physicianRoleID,
		Name: "physician",
	}, nil).Once()

	var savedUser *usermodel.User
	env.userRepo.EXPECT().Create(ctx, mock.Anything).RunAndReturn(func(ctx context.Context, u *usermodel.User) (*usermodel.User, error) {
		u.ID = userID
		u.RoleName = "physician"
		u.CreatedAt = time.Now()
		u.UpdatedAt = time.Now()
		savedUser = u
		return u, nil
	}).Once()

	user, err := env.svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Dr. Lima", Email: "dr.lima@example.org", Password: "senha-segura", Role: "physician",
	})
	require.NoError(t, err)
	assert.Equal(t, "Dr. Lima", user.DisplayName)
	assert.True(t, user.Active)
	assert.NotEmpty(t, user.PasswordHash)
	assert.NotEqual(t, "senha-segura", user.PasswordHash)

	env.userRepo.EXPECT().GetByID(ctx, userID).Return(&usermodel.GetUserByIDRow{
		ID:          userID,
		DisplayName: savedUser.DisplayName,
		Email:       savedUser.Email,
		RoleName:    "physician",
		Active:      true,
	}, nil).Once()
	loaded, err := env.svc.GetUser(ctx, user.ID.String())
	require.NoError(t, err)
	assert.Equal(t, "physician", loaded.RoleName)

	// Duplicate email maps to ErrEmailTaken
	patientRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000001")
	env.roleRepo.EXPECT().GetByName(ctx, "patient").Return(&usermodel.Role{ID: patientRoleID, Name: "patient"}, nil).Once()
	env.userRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, usecase.ErrEmailTaken).Once()

	_, err = env.svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Other", Email: "DR.LIMA@example.org", Password: "senha-segura", Role: "patient",
	})
	require.ErrorIs(t, err, usecase.ErrEmailTaken)
}

func TestCreateUserValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	cases := []usecase.CreateUserInput{
		{Name: "", Email: "a@b.org", Password: "senha-segura", Role: "physician"},
		{Name: "X", Email: "not-an-email", Password: "senha-segura", Role: "physician"},
		{Name: "X", Email: "a@b.org", Password: "short", Role: "physician"},
		{Name: "X", Email: "a@b.org", Password: "senha-segura", Role: "superuser"},
	}
	for _, in := range cases {
		env.roleRepo.EXPECT().GetByName(ctx, in.Role).Return(nil, usecase.ErrRoleNotFound).Maybe()
		_, err := env.svc.CreateUser(ctx, in)
		require.Error(t, err)
	}
}

func TestUpdateUserRoleAndStatus(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	staffID := ident.MustParseUser("01990000-0000-7000-8000-000000000010")
	physicianRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000003")

	env.roleRepo.EXPECT().GetByName(ctx, "physician").Return(&usermodel.Role{ID: physicianRoleID, Name: "physician"}, nil).Maybe()

	staffUserRow := &usermodel.GetUserByIDRow{
		ID:          staffID,
		DisplayName: "Nurse Chefe",
		Email:       "nurse@example.org",
		RoleName:    "physician",
		Active:      false,
	}

	staffUser := &usermodel.User{
		ID:          staffID,
		DisplayName: "Nurse Chefe",
		Email:       "nurse@example.org",
		RoleID:      physicianRoleID,
		RoleName:    "physician",
		Active:      false,
	}

	env.userRepo.EXPECT().GetByID(ctx, staffID).Return(staffUserRow, nil).Maybe()
	env.userRepo.EXPECT().Update(ctx, mock.Anything).Return(staffUser, nil).Once()

	updated, err := env.svc.UpdateUser(ctx, staffID.String(), "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "Nurse Chefe", updated.DisplayName)
	assert.False(t, updated.Active)

	// Reactivate
	staffUserActive := &usermodel.User{
		ID:          staffID,
		DisplayName: "Nurse Chefe",
		Email:       "nurse@example.org",
		RoleID:      physicianRoleID,
		RoleName:    "physician",
		Active:      true,
	}
	env.userRepo.EXPECT().Update(ctx, mock.Anything).Return(staffUserActive, nil).Once()

	updated, err = env.svc.UpdateUser(ctx, staffID.String(), "someone-else", usecase.UpdateUserInput{
		Name: "Nurse Chefe", Email: "nurse@example.org", Role: "physician", Active: true,
	})
	require.NoError(t, err)
	assert.True(t, updated.Active)
}

func TestUpdateUserAntiLockout(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	adminID := ident.MustParseUser("01990000-0000-7000-8000-000000000001")
	adminRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000002")
	patientRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000001")

	adminUserRow := &usermodel.GetUserByIDRow{
		ID:          adminID,
		DisplayName: "Admin",
		Email:       "admin@example.org",
		RoleName:    "admin",
		Active:      true,
	}

	env.userRepo.EXPECT().GetByID(ctx, adminID).Return(adminUserRow, nil).Maybe()
	env.roleRepo.EXPECT().GetByName(ctx, "patient").Return(&usermodel.Role{ID: patientRoleID, Name: "patient"}, nil).Maybe()
	env.roleRepo.EXPECT().GetByName(ctx, "admin").Return(&usermodel.Role{ID: adminRoleID, Name: "admin"}, nil).Maybe()
	env.userRepo.EXPECT().CountActiveAdmins(ctx).Return(1, nil).Maybe()

	// The last active admin cannot demote or deactivate itself
	_, err := env.svc.UpdateUser(ctx, adminID.String(), adminID.String(), usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "patient", Active: true,
	})
	require.ErrorIs(t, err, usecase.ErrCannotDemoteSelf)

	_, err = env.svc.UpdateUser(ctx, adminID.String(), adminID.String(), usecase.UpdateUserInput{
		Name: "Admin", Email: "admin@example.org", Role: "admin", Active: false,
	})
	require.ErrorIs(t, err, usecase.ErrCannotDemoteSelf)
}

func TestListUsersSearch(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	users := []usermodel.ListUsersRow{
		{
			ID:          ident.UserID(uuid.New()),
			DisplayName: "Ana Lima",
			Email:       "ana@example.org",
			RoleName:    "physician",
			Active:      true,
		},
	}

	env.userRepo.EXPECT().ListPage(ctx, "ANA", 10, 0).Return(users, int64(1), nil).Once()
	rows, total, err := env.svc.ListUsersPage(ctx, "ANA", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "ana@example.org", rows[0].Email)

	env.userRepo.EXPECT().ListPage(ctx, "example.org", 10, 0).Return(nil, int64(0), nil).Once()
	rows, total, err = env.svc.ListUsersPage(ctx, "example.org", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, rows)
}

func TestSpecialties(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	psyID := ident.MustParseSpecialty("01990000-0000-7000-8000-000000000031")
	physioID := ident.MustParseSpecialty("01990000-0000-7000-8000-000000000032")
	staffID := ident.MustParseUser("01990000-0000-7000-8000-000000000010")

	psy := &usermodel.Specialty{ID: psyID, ClinicID: testClinic, Name: "Psychologist"}
	physio := &usermodel.Specialty{ID: physioID, ClinicID: testClinic, Name: "Physiotherapist"}

	env.specialtyRepo.EXPECT().Create(ctx, mock.MatchedBy(func(s *usermodel.Specialty) bool {
		return s.Name == "Psychologist"
	})).Return(psy, nil).Once()
	createdPsy, err := env.svc.CreateSpecialty(ctx, testClinic.String(), "Psychologist")
	require.NoError(t, err)
	assert.Equal(t, "Psychologist", createdPsy.Name)

	env.specialtyRepo.EXPECT().Create(ctx, mock.MatchedBy(func(s *usermodel.Specialty) bool {
		return s.Name == "Physiotherapist"
	})).Return(physio, nil).Once()
	createdPhysio, err := env.svc.CreateSpecialty(ctx, testClinic.String(), "Physiotherapist")
	require.NoError(t, err)
	assert.Equal(t, "Physiotherapist", createdPhysio.Name)

	env.specialtyRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, usecase.ErrDuplicateSpecialty).Once()
	_, err = env.svc.CreateSpecialty(ctx, testClinic.String(), " psychologist ")
	require.ErrorIs(t, err, usecase.ErrDuplicateSpecialty)

	// List
	env.specialtyRepo.EXPECT().ListByClinic(ctx, testClinic).Return([]usermodel.Specialty{*psy, *physio}, nil).Once()
	rows, err := env.svc.ListSpecialties(ctx, testClinic.String())
	require.NoError(t, err)
	assert.Len(t, rows, 2)

	// SetUserSpecialties
	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, testClinic, []ident.SpecialtyID{psyID, physioID}).Return(true, nil).Once()
	env.userRepo.EXPECT().SetSpecialties(ctx, staffID, []ident.SpecialtyID{psyID, physioID}).Return(nil).Once()
	err = env.svc.SetUserSpecialties(ctx, testClinic.String(), staffID.String(), []string{psyID.String(), physioID.String()})
	require.NoError(t, err)

	// UserSpecialties
	env.specialtyRepo.EXPECT().ListByUser(ctx, staffID).Return([]usermodel.Specialty{*psy, *physio}, nil).Once()
	assigned, err := env.svc.UserSpecialties(ctx, staffID.String())
	require.NoError(t, err)
	assert.Len(t, assigned, 2)

	// DeleteSpecialty
	env.specialtyRepo.EXPECT().Delete(ctx, testClinic, physioID).Return(nil).Once()
	err = env.svc.DeleteSpecialty(ctx, testClinic.String(), physioID.String())
	require.NoError(t, err)
}

func TestStaffChangeRequests(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	physID := ident.MustParseUser("01990000-0000-7000-8000-000000000010")
	receptionistID := ident.MustParseUser("01990000-0000-7000-8000-000000000011")
	reqID := ident.MustParseStaffChangeRequest("01990000-0000-7000-8000-000000000050")
	psyID := ident.MustParseSpecialty("01990000-0000-7000-8000-000000000031")

	physUserRow := &usermodel.GetUserByIDRow{
		ID:          physID,
		DisplayName: "Dr. Lima",
		Email:       "dr.lima@example.org",
	}

	// Email collision check returns ErrEmailInUse
	env.userRepo.EXPECT().GetByID(ctx, physID).Return(physUserRow, nil).Once()
	env.specialtyRepo.EXPECT().ListByUser(ctx, physID).Return(nil, nil).Once()
	env.userRepo.EXPECT().GetByEmail(ctx, "other@example.org").Return(&usermodel.GetUserByIDRow{ID: ident.UserID(uuid.New()), Email: "other@example.org"}, nil).Once()
	_, err := env.svc.CreateStaffChangeRequest(ctx, physID.String(), receptionistID.String(), usecase.StaffChange{
		Name: "Dr. Lima", Email: "other@example.org", Specialties: nil,
	})
	require.ErrorIs(t, err, usecase.ErrEmailInUse)

	// Valid proposal
	env.userRepo.EXPECT().GetByID(ctx, physID).Return(physUserRow, nil).Once()
	env.specialtyRepo.EXPECT().ListByUser(ctx, physID).Return(nil, nil).Once()

	reqObj := &usermodel.StaffChangeRequest{
		ID:          reqID,
		UserID:      physID,
		RequestedBy: receptionistID,
		Status:      "pending",
		Changes:     `{"name":"Dr. Lima Jr","email":"dr.lima@example.org","specialties":["` + psyID.String() + `"]}`,
		CreatedAt:   time.Now(),
	}
	env.staffReqRepo.EXPECT().Create(ctx, mock.Anything).Return(reqObj, nil).Once()

	createdReq, err := env.svc.CreateStaffChangeRequest(ctx, physID.String(), receptionistID.String(), usecase.StaffChange{
		Name: "Dr. Lima Jr", Email: "dr.lima@example.org", Specialties: []string{psyID.String()},
	})
	require.NoError(t, err)
	assert.Equal(t, reqID, createdReq.ID)

	// List pending
	filteredRow := usermodel.ListStaffChangeRequestsFilteredRow{
		ID:     reqID,
		Status: "pending",
	}
	env.staffReqRepo.EXPECT().ListFiltered(ctx, "pending", "", 50, 0).Return([]usermodel.ListStaffChangeRequestsFilteredRow{filteredRow}, int64(1), nil).Once()
	pend, total, err := env.svc.ListStaffChangeRequestsFiltered(ctx, usecase.StaffRequestPending.String(), "", 50, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, pend, 1)

	// Approve
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(reqObj, nil).Once()
	env.userRepo.EXPECT().ApplyApprovedStaffChange(ctx, reqID, physID, testAdminID, "Dr. Lima Jr", "dr.lima@example.org", []ident.SpecialtyID{psyID}).Return(nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), testAdminID.String())
	require.NoError(t, err)

	// Second approval returns ErrRequestNotPending
	approvedReqObj := *reqObj
	approvedReqObj.Status = "approved"
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(&approvedReqObj, nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), testAdminID.String())
	require.ErrorIs(t, err, usecase.ErrRequestNotPending)

	// Reject
	req2ID := ident.MustParseStaffChangeRequest("01990000-0000-7000-8000-000000000051")
	req2Obj := &usermodel.StaffChangeRequest{
		ID:          req2ID,
		UserID:      physID,
		RequestedBy: receptionistID,
		Status:      "pending",
	}
	env.staffReqRepo.EXPECT().GetByID(ctx, req2ID).Return(req2Obj, nil).Once()
	env.staffReqRepo.EXPECT().Reject(ctx, req2ID, testAdminID, "not needed").Return(nil).Once()
	err = env.svc.RejectStaffChangeRequest(ctx, req2ID.String(), testAdminID.String(), "not needed")
	require.NoError(t, err)
}

func TestListPhysicians(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	physicians := []usermodel.ListPhysiciansPageRow{
		{
			ID:          ident.UserID(uuid.New()),
			DisplayName: "Dr. Ana",
			Email:       "dr.ana@example.org",
			Specialties: "Psychologist",
		},
	}

	env.userRepo.EXPECT().ListPhysiciansPage(ctx, 50, 0).Return(physicians, int64(1), nil).Once()
	rows, total, err := env.svc.ListPhysiciansPage(ctx, 50, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, rows, 1)
	assert.Equal(t, "Psychologist", rows[0].Specialties)
}

func TestMalformedSpecialtyIDsFailValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	physID := ident.UserID(uuid.New())
	receptionistID := ident.UserID(uuid.New())

	// Direct assignment rejects malformed UUID
	err := env.svc.SetUserSpecialties(ctx, testClinic.String(), physID.String(), []string{"not-a-uuid"})
	require.Error(t, err)

	// Proposal rejects malformed UUID
	_, err = env.svc.CreateStaffChangeRequest(ctx, physID.String(), receptionistID.String(), usecase.StaffChange{
		Name: "Dr. Lima", Email: "dr.lima@example.org", Specialties: []string{"not-a-uuid"},
	})
	require.Error(t, err)

	// Poisoned request fails on approve
	badReqID := ident.StaffChangeRequestID(uuid.New())
	badReq := &usermodel.StaffChangeRequest{
		ID:          badReqID,
		UserID:      physID,
		RequestedBy: receptionistID,
		Status:      "pending",
		Changes:     `{"name":"Dr. Lima","email":"dr.lima@example.org","specialties":["not-a-uuid"]}`,
	}
	env.staffReqRepo.EXPECT().GetByID(ctx, badReqID).Return(badReq, nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, badReqID.String(), testAdminID.String())
	require.Error(t, err)
}

func TestRolesCRUD(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	systemRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000001")
	customRoleID := ident.MustParseRole("01990000-0000-7000-8000-000000000005")

	roles := []usermodel.Role{
		{ID: systemRoleID, Name: "admin", System: true},
		{ID: ident.RoleID(uuid.New()), Name: "physician", System: true},
		{ID: ident.RoleID(uuid.New()), Name: "receptionist", System: true},
		{ID: ident.RoleID(uuid.New()), Name: "patient", System: true},
	}

	env.roleRepo.EXPECT().List(ctx).Return(roles, nil).Once()
	rows, err := env.svc.ListRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, rows, 4)

	// Create custom role
	customRole := &usermodel.Role{ID: customRoleID, Name: "psychologist", IsClinical: true}
	env.roleRepo.EXPECT().Create(ctx, mock.MatchedBy(func(r *usermodel.Role) bool {
		return r.Name == "psychologist" && r.IsClinical
	})).Return(customRole, nil).Once()
	created, err := env.svc.CreateRole(ctx, "psychologist", true)
	require.NoError(t, err)
	assert.Equal(t, "psychologist", created.Name)

	// Duplicate role
	env.roleRepo.EXPECT().Create(ctx, mock.Anything).Return(nil, usecase.ErrDuplicateRole).Once()
	_, err = env.svc.CreateRole(ctx, " psychologist ", true)
	require.ErrorIs(t, err, usecase.ErrDuplicateRole)

	// Rename and delete system role forbidden
	env.roleRepo.EXPECT().GetByID(ctx, systemRoleID).Return(&roles[0], nil).Twice()
	_, err = env.svc.RenameRole(ctx, systemRoleID.String(), "director")
	require.ErrorIs(t, err, usecase.ErrSystemRole)

	err = env.svc.DeleteRole(ctx, systemRoleID.String())
	require.ErrorIs(t, err, usecase.ErrSystemRole)

	// Delete custom role in use
	env.roleRepo.EXPECT().GetByID(ctx, customRoleID).Return(customRole, nil).Twice()
	env.roleRepo.EXPECT().CountUsersWithRole(ctx, customRoleID).Return(1, nil).Once()
	err = env.svc.DeleteRole(ctx, customRoleID.String())
	require.ErrorIs(t, err, usecase.ErrRoleInUse)

	// Rename custom role
	renamedRole := &usermodel.Role{ID: customRoleID, Name: "psychotherapist", IsClinical: true}
	env.roleRepo.EXPECT().Update(ctx, mock.MatchedBy(func(r *usermodel.Role) bool {
		return r.ID == customRoleID && r.Name == "psychotherapist"
	})).Return(renamedRole, nil).Once()
	renamed, err := env.svc.RenameRole(ctx, customRoleID.String(), "psychotherapist")
	require.NoError(t, err)
	assert.Equal(t, "psychotherapist", renamed.Name)

	// Delete unused custom role
	spareID := ident.MustParseRole("01990000-0000-7000-8000-000000000006")
	spareRole := &usermodel.Role{ID: spareID, Name: "spare", System: false}
	env.roleRepo.EXPECT().GetByID(ctx, spareID).Return(spareRole, nil).Once()
	env.roleRepo.EXPECT().CountUsersWithRole(ctx, spareID).Return(0, nil).Once()
	env.roleRepo.EXPECT().Delete(ctx, spareID).Return(nil).Once()
	err = env.svc.DeleteRole(ctx, spareID.String())
	require.NoError(t, err)
}

func TestUpdatePreferences(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	userID := ident.MustParseUser("01990000-0000-7000-8000-000000000010")

	// Invalid preferences rejected before touching repo
	err := env.svc.UpdatePreferences(ctx, userID.String(), "America/Sao_Paulo", auth.UITheme("sepia"))
	require.Error(t, err)

	err = env.svc.UpdatePreferences(ctx, userID.String(), "Mars/Olympus", auth.UIThemeDark)
	require.Error(t, err)

	// Valid update
	env.userRepo.EXPECT().UpdatePreferences(ctx, userID, "Asia/Tokyo", "dark").Return(nil).Once()
	err = env.svc.UpdatePreferences(ctx, userID.String(), "Asia/Tokyo", auth.UIThemeDark)
	require.NoError(t, err)

	// Reset
	env.userRepo.EXPECT().UpdatePreferences(ctx, userID, "", "system").Return(nil).Once()
	err = env.svc.UpdatePreferences(ctx, userID.String(), "", auth.UIThemeSystem)
	require.NoError(t, err)
}

func TestSetUserSpecialtiesRejectsCrossClinic(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	staffID := ident.UserID(uuid.New())
	otherSpecialtyID := ident.SpecialtyID(uuid.New())

	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, testClinic, []ident.SpecialtyID{otherSpecialtyID}).Return(false, nil).Once()
	err := env.svc.SetUserSpecialties(ctx, testClinic.String(), staffID.String(), []string{otherSpecialtyID.String()})
	require.ErrorIs(t, err, usecase.ErrSpecialtyScope)
}

func TestSpecialtiesAndStaffQueries(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// 1. Specialties
	specID := ident.New[ident.SpecialtyID]()
	env.specialtyRepo.EXPECT().ListByClinic(ctx, testClinic).Return([]usermodel.Specialty{
		{ID: specID, Name: "Cardiologia"},
	}, nil).Once()
	specs, err := env.svc.ListSpecialties(ctx, testClinic.String())
	require.NoError(t, err)
	assert.Len(t, specs, 1)

	env.specialtyRepo.EXPECT().Create(ctx, mock.Anything).Return(&usermodel.Specialty{
		ID: specID, Name: "Dermatologia",
	}, nil).Once()
	createdSpec, err := env.svc.CreateSpecialty(ctx, testClinic.String(), "Dermatologia")
	require.NoError(t, err)
	assert.Equal(t, "Dermatologia", createdSpec.Name)

	// Specialty name required / too long
	_, err = env.svc.CreateSpecialty(ctx, testClinic.String(), "")
	assert.Error(t, err)
	_, err = env.svc.CreateSpecialty(ctx, testClinic.String(), strings.Repeat("a", 100))
	assert.Error(t, err)

	env.specialtyRepo.EXPECT().Delete(ctx, testClinic, specID).Return(nil).Once()
	require.NoError(t, env.svc.DeleteSpecialty(ctx, testClinic.String(), specID.String()))

	// 2. CountStaff
	env.userRepo.EXPECT().CountStaff(ctx, mock.Anything).Return(int64(5), nil).Once()
	count, err := env.svc.CountStaff(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// 3. ListUsersPage
	env.userRepo.EXPECT().ListPage(ctx, "Dr", 10, 0).Return([]usermodel.ListUsersRow{
		{DisplayName: "Dr. Lima", Email: "lima@example.org"},
	}, int64(1), nil).Once()
	users, total, err := env.svc.ListUsersPage(ctx, "Dr", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, users, 1)

	// 4. SetRoleClinical
	roleID := ident.New[ident.RoleID]()
	env.roleRepo.EXPECT().GetByID(ctx, roleID).Return(&usermodel.Role{ID: roleID, Name: "Nurse", System: false}, nil).Once()
	env.roleRepo.EXPECT().Update(ctx, mock.MatchedBy(func(r *usermodel.Role) bool {
		return r.ID == roleID && r.IsClinical == true
	})).Return(&usermodel.Role{ID: roleID, Name: "Nurse", IsClinical: true}, nil).Once()
	require.NoError(t, env.svc.SetRoleClinical(ctx, roleID.String(), true))
}

func TestStaffChangeRequestsUsecase(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	physicianID := ident.New[ident.UserID]()
	requesterID := ident.New[ident.UserID]()
	reqID := ident.New[ident.StaffChangeRequestID]()
	specID := ident.New[ident.SpecialtyID]()

	// 1. ListPhysiciansPage
	env.userRepo.EXPECT().ListPhysiciansPage(ctx, 10, 0).Return([]usermodel.ListPhysiciansPageRow{
		{ID: physicianID, DisplayName: "Dr. House", Email: "house@example.org"},
	}, int64(1), nil).Once()
	physicians, total, err := env.svc.ListPhysiciansPage(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, physicians, 1)

	// 2. CreateStaffChangeRequest
	env.userRepo.EXPECT().GetByID(ctx, physicianID).Return(&usermodel.GetUserByIDRow{
		ID: physicianID, DisplayName: "Dr. House", Email: "house@example.org",
	}, nil).Once()
	env.specialtyRepo.EXPECT().ListByUser(ctx, physicianID).Return([]usermodel.Specialty{
		{ID: specID, Name: "Infectologia"},
	}, nil).Once()
	env.userRepo.EXPECT().GetByEmail(ctx, "gregory.house@example.org").Return(nil, usecase.ErrUserNotFound).Once()
	env.staffReqRepo.EXPECT().Create(ctx, mock.Anything).Return(&usermodel.StaffChangeRequest{
		ID: reqID, UserID: physicianID, RequestedBy: requesterID, Status: string(usecase.StaffRequestPending),
	}, nil).Once()

	change := usecase.StaffChange{
		Name:        "Dr. Gregory House",
		Email:       "gregory.house@example.org",
		Specialties: []string{specID.String()},
	}
	createdReq, err := env.svc.CreateStaffChangeRequest(ctx, physicianID.String(), requesterID.String(), change)
	require.NoError(t, err)
	assert.Equal(t, reqID, createdReq.ID)

	// Validation errors on CreateStaffChangeRequest
	_, err = env.svc.CreateStaffChangeRequest(ctx, physicianID.String(), requesterID.String(), usecase.StaffChange{Name: ""})
	assert.Error(t, err)
	_, err = env.svc.CreateStaffChangeRequest(ctx, physicianID.String(), requesterID.String(), usecase.StaffChange{Name: "House", Email: "bad-email"})
	assert.Error(t, err)

	// 3. ListMyStaffChangeRequests & ListStaffChangeRequestsFiltered
	env.staffReqRepo.EXPECT().ListByRequester(ctx, requesterID, 50).Return([]usermodel.ListStaffChangeRequestsByRequesterRow{
		{ID: reqID, StaffName: "Dr. Gregory House"},
	}, nil).Once()
	myReqs, err := env.svc.ListMyStaffChangeRequests(ctx, requesterID.String())
	require.NoError(t, err)
	assert.Len(t, myReqs, 1)

	env.staffReqRepo.EXPECT().ListFiltered(ctx, "pending", "", 10, 0).Return([]usermodel.ListStaffChangeRequestsFilteredRow{
		{ID: reqID, StaffName: "Dr. Gregory House"},
	}, int64(1), nil).Once()
	filteredReqs, count, err := env.svc.ListStaffChangeRequestsFiltered(ctx, "pending", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	assert.Len(t, filteredReqs, 1)

	// 4. ApproveStaffChangeRequest
	adminID := ident.New[ident.UserID]()
	payload := `{"name":"Dr. Gregory House","email":"gregory.house@example.org","specialties":["` + specID.String() + `"]}`
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(&usermodel.StaffChangeRequest{
		ID: reqID, UserID: physicianID, Changes: payload, Status: string(usecase.StaffRequestPending),
	}, nil).Once()
	env.userRepo.EXPECT().ApplyApprovedStaffChange(ctx, reqID, physicianID, adminID, "Dr. Gregory House", "gregory.house@example.org", []ident.SpecialtyID{specID}).Return(nil).Once()

	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), adminID.String())
	require.NoError(t, err)

	// Approve non-pending fails
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(&usermodel.StaffChangeRequest{
		ID: reqID, Status: string(usecase.StaffRequestApproved),
	}, nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), adminID.String())
	assert.ErrorIs(t, err, usecase.ErrRequestNotPending)

	// 5. RejectStaffChangeRequest
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(&usermodel.StaffChangeRequest{
		ID: reqID, Status: string(usecase.StaffRequestPending),
	}, nil).Once()
	env.staffReqRepo.EXPECT().Reject(ctx, reqID, adminID, "rejeitado").Return(nil).Once()
	err = env.svc.RejectStaffChangeRequest(ctx, reqID.String(), adminID.String(), "rejeitado")
	require.NoError(t, err)

	// 6. RenameRole
	roleID := ident.New[ident.RoleID]()
	env.roleRepo.EXPECT().GetByID(ctx, roleID).Return(&usermodel.Role{
		ID: roleID, Name: "Custom Role", System: false,
	}, nil).Once()
	env.roleRepo.EXPECT().Update(ctx, mock.Anything).Return(&usermodel.Role{
		ID: roleID, Name: "Renamed Role", System: false,
	}, nil).Once()
	renamed, err := env.svc.RenameRole(ctx, roleID.String(), "Renamed Role")
	require.NoError(t, err)
	assert.Equal(t, "Renamed Role", renamed.Name)

	// Rename system role fails
	env.roleRepo.EXPECT().GetByID(ctx, roleID).Return(&usermodel.Role{
		ID: roleID, Name: "Admin", System: true,
	}, nil).Once()
	_, err = env.svc.RenameRole(ctx, roleID.String(), "Super Admin")
	assert.ErrorIs(t, err, usecase.ErrSystemRole)
}

func TestPreferencesAndCountStaff(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	uID := ident.New[ident.UserID]()

	// 1. CountStaff
	env.userRepo.EXPECT().CountStaff(ctx, []string{"admin", "physician", "receptionist"}).Return(int64(5), nil).Once()
	count, err := env.svc.CountStaff(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// 2. UpdatePreferences success
	env.userRepo.EXPECT().UpdatePreferences(ctx, uID, "America/Sao_Paulo", "dark").Return(nil).Once()
	require.NoError(t, env.svc.UpdatePreferences(ctx, uID.String(), "America/Sao_Paulo", "dark"))

	// 3. UpdatePreferences invalid timezone
	err = env.svc.UpdatePreferences(ctx, uID.String(), "Invalid/Timezone", "dark")
	assert.Error(t, err)

	// 4. UpdatePreferences invalid user ID
	err = env.svc.UpdatePreferences(ctx, "invalid-user-uuid", "UTC", "system")
	assert.Error(t, err)
}

func TestUsersEdgeCasesAndValidation(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// 1. CreateUser invalid input
	_, err := env.svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "", Email: "invalid", Password: "short", Role: "physician",
	})
	assert.Error(t, err)

	// 2. CreateUser unsupported role
	env.roleRepo.EXPECT().GetByName(ctx, "non-existent-role").Return(nil, errors.New("not found")).Once()
	_, err = env.svc.CreateUser(ctx, usecase.CreateUserInput{
		Name: "Valid Name", Email: "valid@example.org", Password: "SecurePassword123!", Role: "non-existent-role",
	})
	assert.Error(t, err)

	// 3. UpdateUser invalid input
	_, err = env.svc.UpdateUser(ctx, "some-id", "actor-id", usecase.UpdateUserInput{
		Name: "", Email: "invalid-email", Role: "physician", Active: true,
	})
	assert.Error(t, err)

	// 4. UpdateUser unsupported role
	env.roleRepo.EXPECT().GetByName(ctx, "fake-role").Return(nil, errors.New("not found")).Once()
	_, err = env.svc.UpdateUser(ctx, "some-id", "actor-id", usecase.UpdateUserInput{
		Name: "Valid Name", Email: "valid@example.org", Role: "fake-role", Active: true,
	})
	assert.Error(t, err)

	// 5. UpdateUser invalid user ID
	env.roleRepo.EXPECT().GetByName(ctx, "physician").Return(&usermodel.Role{Name: "physician"}, nil).Once()
	_, err = env.svc.UpdateUser(ctx, "invalid-user-id", "actor-id", usecase.UpdateUserInput{
		Name: "Valid Name", Email: "valid@example.org", Role: "physician", Active: true,
	})
	assert.Error(t, err)

	// 6. GetUser
	uID := ident.New[ident.UserID]()
	env.userRepo.EXPECT().GetByID(ctx, uID).Return(&usermodel.GetUserByIDRow{
		ID: uID, DisplayName: "Found User", Email: "found@example.org",
	}, nil).Once()
	u, err := env.svc.GetUser(ctx, uID.String())
	require.NoError(t, err)
	assert.Equal(t, "Found User", u.DisplayName)

	// 7. Specialties usecase methods
	cID := ident.New[ident.ClinicID]()
	spID := ident.New[ident.SpecialtyID]()

	// ListSpecialties invalid clinic
	_, err = env.svc.ListSpecialties(ctx, "invalid-clinic-id")
	assert.Error(t, err)

	// ListSpecialties success
	env.specialtyRepo.EXPECT().ListByClinic(ctx, cID).Return([]usermodel.Specialty{
		{ID: spID, ClinicID: cID, Name: "Cardiology"},
	}, nil).Once()
	specs, err := env.svc.ListSpecialties(ctx, cID.String())
	require.NoError(t, err)
	assert.Len(t, specs, 1)

	// ListSpecialtiesPage invalid clinic
	_, _, err = env.svc.ListSpecialtiesPage(ctx, "invalid-clinic-id", 10, 0)
	assert.Error(t, err)

	// ListSpecialtiesPage success
	env.specialtyRepo.EXPECT().ListPageByClinic(ctx, cID, 10, 0).Return([]usermodel.Specialty{
		{ID: spID, ClinicID: cID, Name: "Cardiology"},
	}, int64(1), nil).Once()
	pagedSpecs, total, err := env.svc.ListSpecialtiesPage(ctx, cID.String(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, pagedSpecs, 1)

	// CreateSpecialty validation errors
	_, err = env.svc.CreateSpecialty(ctx, cID.String(), "")
	assert.Error(t, err)
	_, err = env.svc.CreateSpecialty(ctx, cID.String(), strings.Repeat("A", 65))
	assert.Error(t, err)
	_, err = env.svc.CreateSpecialty(ctx, "invalid-clinic", "Neurology")
	assert.Error(t, err)

	// CreateSpecialty success
	env.specialtyRepo.EXPECT().Create(ctx, mock.MatchedBy(func(s *usermodel.Specialty) bool {
		return s.Name == "Neurology" && s.ClinicID == cID
	})).Return(&usermodel.Specialty{ID: spID, ClinicID: cID, Name: "Neurology"}, nil).Once()
	createdSpec, err := env.svc.CreateSpecialty(ctx, cID.String(), "Neurology")
	require.NoError(t, err)
	assert.Equal(t, "Neurology", createdSpec.Name)

	// DeleteSpecialty invalid IDs
	err = env.svc.DeleteSpecialty(ctx, "invalid-clinic", spID.String())
	assert.Error(t, err)
	err = env.svc.DeleteSpecialty(ctx, cID.String(), "invalid-specialty")
	assert.Error(t, err)

	// DeleteSpecialty success
	env.specialtyRepo.EXPECT().Delete(ctx, cID, spID).Return(nil).Once()
	require.NoError(t, env.svc.DeleteSpecialty(ctx, cID.String(), spID.String()))

	// UserSpecialties
	_, err = env.svc.UserSpecialties(ctx, "invalid-user-id")
	assert.Error(t, err)

	env.specialtyRepo.EXPECT().ListByUser(ctx, uID).Return([]usermodel.Specialty{
		{ID: spID, ClinicID: cID, Name: "Cardiology"},
	}, nil).Once()
	userSpecs, err := env.svc.UserSpecialties(ctx, uID.String())
	require.NoError(t, err)
	assert.Len(t, userSpecs, 1)

	// SetUserSpecialties validation errors and success
	err = env.svc.SetUserSpecialties(ctx, cID.String(), uID.String(), []string{"invalid-uuid"})
	assert.Error(t, err)

	err = env.svc.SetUserSpecialties(ctx, "invalid-clinic", uID.String(), []string{spID.String()})
	assert.Error(t, err)

	err = env.svc.SetUserSpecialties(ctx, cID.String(), "invalid-user", []string{spID.String()})
	assert.Error(t, err)

	// SetUserSpecialties scope check failure
	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, cID, []ident.SpecialtyID{spID}).Return(false, nil).Once()
	err = env.svc.SetUserSpecialties(ctx, cID.String(), uID.String(), []string{spID.String()})
	assert.ErrorIs(t, err, usecase.ErrSpecialtyScope)

	// SetUserSpecialties success
	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, cID, []ident.SpecialtyID{spID}).Return(true, nil).Once()
	env.userRepo.EXPECT().SetSpecialties(ctx, uID, []ident.SpecialtyID{spID}).Return(nil).Once()
	require.NoError(t, env.svc.SetUserSpecialties(ctx, cID.String(), uID.String(), []string{spID.String()}))
}

