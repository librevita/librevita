package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/auth"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/internal/domain/user/usecase"
)

var (
	testClinic  = uuid.MustParse("00000000-0000-0000-0000-000000000011")
	testAdminID = uuid.MustParse("00000000-0000-0000-0000-000000000021")
)

func TestCreateUser(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	physicianRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000003")
	userID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

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
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
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
	staffID := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	physicianRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000003")

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
	adminID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	adminRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000002")
	patientRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")

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
			ID:          uuid.New(),
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
	psyID := uuid.MustParse("01990000-0000-7000-8000-000000000031")
	physioID := uuid.MustParse("01990000-0000-7000-8000-000000000032")
	staffID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

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
	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, testClinic, []uuid.UUID{psyID, physioID}).Return(true, nil).Once()
	env.userRepo.EXPECT().SetSpecialties(ctx, staffID, []uuid.UUID{psyID, physioID}).Return(nil).Once()
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

	physID := uuid.MustParse("01990000-0000-7000-8000-000000000010")
	receptionistID := uuid.MustParse("01990000-0000-7000-8000-000000000011")
	reqID := uuid.MustParse("01990000-0000-7000-8000-000000000050")
	psyID := uuid.MustParse("01990000-0000-7000-8000-000000000031")

	physUserRow := &usermodel.GetUserByIDRow{
		ID:          physID,
		DisplayName: "Dr. Lima",
		Email:       "dr.lima@example.org",
	}

	// Email collision check returns ErrEmailInUse
	env.userRepo.EXPECT().GetByID(ctx, physID).Return(physUserRow, nil).Once()
	env.specialtyRepo.EXPECT().ListByUser(ctx, physID).Return(nil, nil).Once()
	env.userRepo.EXPECT().GetByEmail(ctx, "other@example.org").Return(&usermodel.GetUserByIDRow{ID: uuid.New(), Email: "other@example.org"}, nil).Once()
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
	env.userRepo.EXPECT().ApplyApprovedStaffChange(ctx, reqID, physID, testAdminID, "Dr. Lima Jr", "dr.lima@example.org", []uuid.UUID{psyID}).Return(nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), testAdminID.String())
	require.NoError(t, err)

	// Second approval returns ErrRequestNotPending
	approvedReqObj := *reqObj
	approvedReqObj.Status = "approved"
	env.staffReqRepo.EXPECT().GetByID(ctx, reqID).Return(&approvedReqObj, nil).Once()
	err = env.svc.ApproveStaffChangeRequest(ctx, reqID.String(), testAdminID.String())
	require.ErrorIs(t, err, usecase.ErrRequestNotPending)

	// Reject
	req2ID := uuid.MustParse("01990000-0000-7000-8000-000000000051")
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
			ID:          uuid.New(),
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
	physID := uuid.New()
	receptionistID := uuid.New()

	// Direct assignment rejects malformed UUID
	err := env.svc.SetUserSpecialties(ctx, testClinic.String(), physID.String(), []string{"not-a-uuid"})
	require.Error(t, err)

	// Proposal rejects malformed UUID
	_, err = env.svc.CreateStaffChangeRequest(ctx, physID.String(), receptionistID.String(), usecase.StaffChange{
		Name: "Dr. Lima", Email: "dr.lima@example.org", Specialties: []string{"not-a-uuid"},
	})
	require.Error(t, err)

	// Poisoned request fails on approve
	badReqID := uuid.New()
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
	systemRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000001")
	customRoleID := uuid.MustParse("01990000-0000-7000-8000-000000000005")

	roles := []usermodel.Role{
		{ID: systemRoleID, Name: "admin", System: true},
		{ID: uuid.New(), Name: "physician", System: true},
		{ID: uuid.New(), Name: "receptionist", System: true},
		{ID: uuid.New(), Name: "patient", System: true},
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
	spareID := uuid.MustParse("01990000-0000-7000-8000-000000000006")
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
	userID := uuid.MustParse("01990000-0000-7000-8000-000000000010")

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
	staffID := uuid.New()
	otherSpecialtyID := uuid.New()

	env.specialtyRepo.EXPECT().CheckClinicScope(ctx, testClinic, []uuid.UUID{otherSpecialtyID}).Return(false, nil).Once()
	err := env.svc.SetUserSpecialties(ctx, testClinic.String(), staffID.String(), []string{otherSpecialtyID.String()})
	require.ErrorIs(t, err, usecase.ErrSpecialtyScope)
}
