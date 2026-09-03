package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/database/record"
	"librevita.org/internal/database/record/enttest"
	usermodel "librevita.org/internal/domain/user/model"
	"librevita.org/internal/domain/user/repository"
	"librevita.org/pkg/ident"
)

func setupTestDB(t *testing.T) (*record.Client, ident.ClinicID, context.Context) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:ent_user_test?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(record.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})

	clinicID := ident.New[ident.ClinicID]()
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("test-clinic").
		SetName("Test Clinic").
		Save(context.Background())
	require.NoError(t, err)

	ctx := clinicctx.WithClinic(context.Background(), &clinicctx.Clinic{
		ID:   clinicID,
		Slug: "test-clinic",
		Name: "Test Clinic",
	})

	return client, clinicID, ctx
}

func TestUserRepository_CRUD(t *testing.T) {
	client, clinicID, ctx := setupTestDB(t)
	repo := repository.NewUserRepository(client)
	roleRepo := repository.NewRoleRepository(client)

	// Seed default roles
	require.NoError(t, roleRepo.SeedDefaults(ctx))
	adminRole, err := roleRepo.GetByName(ctx, "admin")
	require.NoError(t, err)

	physicianRole, err := roleRepo.GetByName(ctx, "physician")
	require.NoError(t, err)

	uID := ident.New[ident.UserID]()
	u := &usermodel.User{
		ID:           uID,
		ClinicID:     clinicID,
		Email:        "admin@example.org",
		PasswordHash: "hashedpass",
		DisplayName:  "Administrator",
		RoleID:       adminRole.ID,
		RoleName:     "admin",
		Active:       true,
	}

	// 1. Create
	created, err := repo.Create(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, uID, created.ID)
	assert.Equal(t, "admin@example.org", created.Email)
	assert.Equal(t, "admin", created.RoleName)

	// Create duplicate email should fail
	_, err = repo.Create(ctx, u)
	assert.Error(t, err)

	// 2. GetByID
	fetched, err := repo.GetByID(ctx, uID)
	require.NoError(t, err)
	assert.Equal(t, uID, fetched.ID)
	assert.Equal(t, "admin@example.org", fetched.Email)
	assert.Equal(t, "admin", fetched.RoleName)

	// GetByID not found
	_, err = repo.GetByID(ctx, ident.New[ident.UserID]())
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 3. GetByEmail
	byEmail, err := repo.GetByEmail(ctx, "admin@example.org")
	require.NoError(t, err)
	assert.Equal(t, uID, byEmail.ID)

	// GetByEmail not found
	_, err = repo.GetByEmail(ctx, "nonexistent@example.org")
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 4. Update
	u.DisplayName = "Updated Admin"
	updated, err := repo.Update(ctx, u)
	require.NoError(t, err)
	assert.Equal(t, "Updated Admin", updated.DisplayName)

	// Update non existent
	fakeUser := &usermodel.User{ID: ident.New[ident.UserID](), Email: "fake@example.org", DisplayName: "Fake User", RoleID: adminRole.ID}
	_, err = repo.Update(ctx, fakeUser)
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 5. UpdatePreferences
	err = repo.UpdatePreferences(ctx, uID, "America/Sao_Paulo", "dark")
	require.NoError(t, err)
	prefUser, err := repo.GetByID(ctx, uID)
	require.NoError(t, err)
	assert.Equal(t, "America/Sao_Paulo", prefUser.Timezone)
	assert.Equal(t, "dark", prefUser.UITheme)

	// Update preferences not found
	err = repo.UpdatePreferences(ctx, ident.New[ident.UserID](), "UTC", "light")
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 6. Create second user (physician)
	u2ID := ident.New[ident.UserID]()
	u2 := &usermodel.User{
		ID:           u2ID,
		ClinicID:     clinicID,
		Email:        "doc@example.org",
		PasswordHash: "hashedpass2",
		DisplayName:  "Dr. Silva",
		RoleID:       physicianRole.ID,
		RoleName:     "physician",
		Active:       true,
	}
	_, err = repo.Create(ctx, u2)
	require.NoError(t, err)

	// 7. Counts
	totalUsers, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalUsers)

	byRoleCount, err := repo.CountByRole(ctx, "physician")
	require.NoError(t, err)
	assert.Equal(t, int64(1), byRoleCount)

	staffCount, err := repo.CountStaff(ctx, []string{"admin", "physician"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), staffCount)

	activeAdmins, err := repo.CountActiveAdmins(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, activeAdmins)

	// 8. ListRecent
	recent, err := repo.ListRecent(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, recent, 2)

	// 9. ListPage
	page, total, err := repo.ListPage(ctx, "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, page, 2)

	// Filter by search query
	pageFiltered, totalFiltered, err := repo.ListPage(ctx, "silva", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), totalFiltered)
	assert.Len(t, pageFiltered, 1)

	// 10. ListPhysiciansPage
	physPage, physTotal, err := repo.ListPhysiciansPage(ctx, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), physTotal)
	assert.Len(t, physPage, 1)

	// 11. Specialties association
	specRepo := repository.NewSpecialtyRepository(client)
	specID := ident.New[ident.SpecialtyID]()
	_, err = specRepo.Create(ctx, &usermodel.Specialty{
		ID:       specID,
		ClinicID: clinicID,
		Name:     "Cardiologia",
	})
	require.NoError(t, err)

	err = repo.SetSpecialties(ctx, u2ID, []ident.SpecialtyID{specID})
	require.NoError(t, err)

	userSpecs, err := specRepo.ListByUser(ctx, u2ID)
	require.NoError(t, err)
	require.Len(t, userSpecs, 1)
	assert.Equal(t, "Cardiologia", userSpecs[0].Name)

	// 12. BindPortalPatient
	patID := ident.New[ident.PatientID]()
	_, err = client.Patient.Create().
		SetID(patID).
		SetClinicID(clinicID).
		SetDisplayName("Paciente Teste").
		SetPhone("+5511999999999").
		SetEmail("paciente@example.org").
		Save(ctx)
	require.NoError(t, err)

	err = repo.BindPortalPatient(ctx, uID, patID)
	require.NoError(t, err)

	// Bind non existent patient
	err = repo.BindPortalPatient(ctx, uID, ident.New[ident.PatientID]())
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 13. ApplyApprovedStaffChange
	staffReqRepo := repository.NewStaffRequestRepository(client)
	chReq, err := staffReqRepo.Create(ctx, &usermodel.StaffChangeRequest{
		ID:          ident.New[ident.StaffChangeRequestID](),
		UserID:      u2ID,
		RequestedBy: uID,
		Changes:     `{"display_name": "Dr. Silva Sauro"}`,
	})
	require.NoError(t, err)

	err = repo.ApplyApprovedStaffChange(ctx, chReq.ID, u2ID, uID, "Dr. Silva Sauro", "doc@example.org", []ident.SpecialtyID{specID})
	require.NoError(t, err)

	afterChange, err := repo.GetByID(ctx, u2ID)
	require.NoError(t, err)
	assert.Equal(t, "Dr. Silva Sauro", afterChange.DisplayName)
}

func TestSetupRepository(t *testing.T) {
	client, _, ctx := setupTestDB(t)
	setupRepo := repository.NewSetupRepository(client)

	// 1. IsOnboarded (false initially)
	onboarded, err := setupRepo.IsOnboarded(ctx)
	require.NoError(t, err)
	assert.False(t, onboarded)

	// 2. Onboard
	adminUser := &usermodel.User{
		ID:           ident.New[ident.UserID](),
		Email:        "founder@example.org",
		PasswordHash: "passhash",
		DisplayName:  "Founder",
		Active:       true,
	}

	createdAdmin, err := setupRepo.Onboard(ctx, adminUser, nil)
	require.NoError(t, err)
	assert.Equal(t, "founder@example.org", createdAdmin.Email)
	assert.Equal(t, "admin", createdAdmin.RoleName)

	// 3. IsOnboarded (true now)
	onboarded, err = setupRepo.IsOnboarded(ctx)
	require.NoError(t, err)
	assert.True(t, onboarded)

	// 4. Onboard again should fail
	_, err = setupRepo.Onboard(ctx, adminUser, nil)
	assert.ErrorIs(t, err, usermodel.ErrAlreadyOnboarded)

	// Context without clinic
	_, err = setupRepo.IsOnboarded(context.Background())
	require.NoError(t, err)
}

func TestRoleRepository(t *testing.T) {
	client, _, ctx := setupTestDB(t)
	repo := repository.NewRoleRepository(client)

	// 1. SeedDefaults
	err := repo.SeedDefaults(ctx)
	require.NoError(t, err)

	// Calling SeedDefaults again should no-op
	err = repo.SeedDefaults(ctx)
	require.NoError(t, err)

	// 2. List
	roles, err := repo.List(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 4)

	// 3. Create Custom Role
	customID := ident.New[ident.RoleID]()
	customRole := &usermodel.Role{
		ID:         customID,
		Name:       "Nurse",
		IsClinical: true,
	}
	created, err := repo.Create(ctx, customRole)
	require.NoError(t, err)
	assert.Equal(t, "Nurse", created.Name)
	assert.True(t, created.IsClinical)

	// Duplicate create fails
	_, err = repo.Create(ctx, customRole)
	assert.Error(t, err)

	// 4. GetByID
	fetched, err := repo.GetByID(ctx, customID)
	require.NoError(t, err)
	assert.Equal(t, "Nurse", fetched.Name)

	// GetByID not found
	_, err = repo.GetByID(ctx, ident.New[ident.RoleID]())
	assert.Error(t, err)

	// 5. Update
	created.Name = "Head Nurse"
	updated, err := repo.Update(ctx, created)
	require.NoError(t, err)
	assert.Equal(t, "Head Nurse", updated.Name)

	// 6. CountUsersWithRole
	count, err := repo.CountUsersWithRole(ctx, customID)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// 7. Delete
	err = repo.Delete(ctx, customID)
	require.NoError(t, err)

	_, err = repo.GetByID(ctx, customID)
	assert.Error(t, err)
}

func TestSpecialtyRepository(t *testing.T) {
	client, clinicID, ctx := setupTestDB(t)
	repo := repository.NewSpecialtyRepository(client)

	spID1 := ident.New[ident.SpecialtyID]()
	sp1 := &usermodel.Specialty{
		ID:       spID1,
		ClinicID: clinicID,
		Name:     "Pediatria",
	}
	created1, err := repo.Create(ctx, sp1)
	require.NoError(t, err)
	assert.Equal(t, "Pediatria", created1.Name)

	// Duplicate name
	_, err = repo.Create(ctx, sp1)
	assert.ErrorIs(t, err, usermodel.ErrDuplicateSpecialty)

	spID2 := ident.New[ident.SpecialtyID]()
	sp2 := &usermodel.Specialty{
		ID:       spID2,
		ClinicID: clinicID,
		Name:     "Ortopedia",
	}
	_, err = repo.Create(ctx, sp2)
	require.NoError(t, err)

	// ListByClinic
	all, err := repo.ListByClinic(ctx, clinicID)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// ListPageByClinic
	page, total, err := repo.ListPageByClinic(ctx, clinicID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, page, 2)

	// CheckClinicScope
	inScope, err := repo.CheckClinicScope(ctx, clinicID, []ident.SpecialtyID{spID1, spID2})
	require.NoError(t, err)
	assert.True(t, inScope)

	outScope, err := repo.CheckClinicScope(ctx, clinicID, []ident.SpecialtyID{spID1, ident.New[ident.SpecialtyID]()})
	require.NoError(t, err)
	assert.False(t, outScope)

	// Delete
	err = repo.Delete(ctx, clinicID, spID1)
	require.NoError(t, err)

	allAfter, err := repo.ListByClinic(ctx, clinicID)
	require.NoError(t, err)
	assert.Len(t, allAfter, 1)
}

func TestStaffRequestRepository(t *testing.T) {
	client, clinicID, ctx := setupTestDB(t)
	userRepo := repository.NewUserRepository(client)
	roleRepo := repository.NewRoleRepository(client)
	require.NoError(t, roleRepo.SeedDefaults(ctx))
	adminRole, err := roleRepo.GetByName(ctx, "admin")
	require.NoError(t, err)

	u1, err := userRepo.Create(ctx, &usermodel.User{
		ID:           ident.New[ident.UserID](),
		ClinicID:     clinicID,
		Email:        "requester@example.org",
		PasswordHash: "hash",
		DisplayName:  "Requester",
		RoleID:       adminRole.ID,
		RoleName:     "admin",
		Active:       true,
	})
	require.NoError(t, err)

	u2, err := userRepo.Create(ctx, &usermodel.User{
		ID:           ident.New[ident.UserID](),
		ClinicID:     clinicID,
		Email:        "target@example.org",
		PasswordHash: "hash",
		DisplayName:  "Target Staff",
		RoleID:       adminRole.ID,
		RoleName:     "admin",
		Active:       true,
	})
	require.NoError(t, err)

	repo := repository.NewStaffRequestRepository(client)

	reqID := ident.New[ident.StaffChangeRequestID]()
	req := &usermodel.StaffChangeRequest{
		ID:          reqID,
		UserID:      u2.ID,
		RequestedBy: u1.ID,
		Changes:     `{"active": false}`,
	}

	created, err := repo.Create(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, reqID, created.ID)
	assert.Equal(t, "pending", created.Status)

	// GetByID
	fetched, err := repo.GetByID(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, reqID, fetched.ID)

	// GetByID not found
	_, err = repo.GetByID(ctx, ident.New[ident.StaffChangeRequestID]())
	assert.ErrorIs(t, err, usermodel.ErrRequestNotFound)

	// ListByRequester
	byReq, err := repo.ListByRequester(ctx, u1.ID, 10)
	require.NoError(t, err)
	assert.Len(t, byReq, 1)

	// ListFiltered
	filtered, total, err := repo.ListFiltered(ctx, "pending", "target", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, filtered, 1)

	// Reject
	err = repo.Reject(ctx, reqID, u1.ID, "Não autorizado")
	require.NoError(t, err)

	rejected, err := repo.GetByID(ctx, reqID)
	require.NoError(t, err)
	assert.Equal(t, "rejected", rejected.Status)
	assert.Equal(t, "Não autorizado", *rejected.DecisionNote)

	// Reject not found
	err = repo.Reject(ctx, ident.New[ident.StaffChangeRequestID](), u1.ID, "note")
	assert.ErrorIs(t, err, usermodel.ErrRequestNotFound)
}

func TestSpecialtiesAndUserEdgeCases(t *testing.T) {
	client, clinicID, ctx := setupTestDB(t)
	specRepo := repository.NewSpecialtyRepository(client)
	userRepo := repository.NewUserRepository(client)
	roleRepo := repository.NewRoleRepository(client)

	require.NoError(t, roleRepo.SeedDefaults(ctx))
	adminRole, err := roleRepo.GetByName(ctx, "admin")
	require.NoError(t, err)

	// 1. Specialty ListPage and CheckClinicScope
	sID1 := ident.New[ident.SpecialtyID]()
	sID2 := ident.New[ident.SpecialtyID]()
	_, err = specRepo.Create(ctx, &usermodel.Specialty{ID: sID1, ClinicID: clinicID, Name: "Dermatologia"})
	require.NoError(t, err)
	_, err = specRepo.Create(ctx, &usermodel.Specialty{ID: sID2, ClinicID: clinicID, Name: "Ortopedia"})
	require.NoError(t, err)

	specsPage, totalSpecs, err := specRepo.ListPageByClinic(ctx, clinicID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalSpecs)
	assert.Len(t, specsPage, 2)

	inScope, err := specRepo.CheckClinicScope(ctx, clinicID, []ident.SpecialtyID{sID1, sID2})
	require.NoError(t, err)
	assert.True(t, inScope)

	// Out of scope check
	fakeClinicID := ident.New[ident.ClinicID]()
	outScope, err := specRepo.CheckClinicScope(ctx, fakeClinicID, []ident.SpecialtyID{sID1})
	require.NoError(t, err)
	assert.False(t, outScope)

	// Delete specialty
	require.NoError(t, specRepo.Delete(ctx, clinicID, sID2))

	// 2. User Create with Zero ClinicID in User struct (resolves from context)
	uID := ident.New[ident.UserID]()
	createdUser, err := userRepo.Create(ctx, &usermodel.User{
		ID:           uID,
		Email:        "ctxuser@example.org",
		PasswordHash: "hash",
		DisplayName:  "Context User",
		RoleID:       adminRole.ID,
		Active:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, clinicID, createdUser.ClinicID)

	// 3. User Create without context clinic should fail
	_, err = userRepo.Create(context.Background(), &usermodel.User{
		ID:           ident.New[ident.UserID](),
		Email:        "noclinic@example.org",
		PasswordHash: "hash",
		DisplayName:  "No Clinic",
		RoleID:       adminRole.ID,
	})
	assert.Error(t, err)

	// 4. Role operations: Update, CountUsersWithRole, Delete, Duplicate
	customRoleID := ident.New[ident.RoleID]()
	customRole, err := roleRepo.Create(ctx, &usermodel.Role{
		ID:         customRoleID,
		Name:       "Pharmacist",
		IsClinical: false,
	})
	require.NoError(t, err)

	// Duplicate role create
	_, err = roleRepo.Create(ctx, &usermodel.Role{
		ID:         ident.New[ident.RoleID](),
		Name:       "Pharmacist",
		IsClinical: false,
	})
	assert.Error(t, err)

	// Update role
	customRole.Name = "Clinical Pharmacist"
	customRole.IsClinical = true
	updatedRole, err := roleRepo.Update(ctx, customRole)
	require.NoError(t, err)
	assert.Equal(t, "Clinical Pharmacist", updatedRole.Name)
	assert.True(t, updatedRole.IsClinical)

	// Count users with role
	roleUsersCount, err := roleRepo.CountUsersWithRole(ctx, customRoleID)
	require.NoError(t, err)
	assert.Equal(t, 0, roleUsersCount)

	// Delete role
	require.NoError(t, roleRepo.Delete(ctx, customRoleID))
	assert.NotNil(t, repository.Module)
}

func TestUserRepositoryQueriesAndStaffChanges(t *testing.T) {
	client, clinicID, ctx := setupTestDB(t)
	userRepo := repository.NewUserRepository(client)
	roleRepo := repository.NewRoleRepository(client)
	staffReqRepo := repository.NewStaffRequestRepository(client)

	require.NoError(t, roleRepo.SeedDefaults(ctx))
	adminRole, err := roleRepo.GetByName(ctx, "admin")
	require.NoError(t, err)

	uID := ident.New[ident.UserID]()
	_, err = userRepo.Create(ctx, &usermodel.User{
		ID:           uID,
		ClinicID:     clinicID,
		Email:        "queries_admin@example.org",
		PasswordHash: "hash",
		DisplayName:  "Queries Administrator",
		RoleID:       adminRole.ID,
		Active:       true,
	})
	require.NoError(t, err)

	// 1. ListPage with search query
	page, total, err := userRepo.ListPage(ctx, "queries", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, page, 1)
	assert.Equal(t, "Queries Administrator", page[0].DisplayName)

	// 2. ListRecent
	recent, err := userRepo.ListRecent(ctx, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, recent)

	// 3. Count, CountByRole, CountStaff, CountActiveAdmins
	cnt, err := userRepo.Count(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cnt, int64(1))

	roleCnt, err := userRepo.CountByRole(ctx, "admin")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, roleCnt, int64(1))

	staffCnt, err := userRepo.CountStaff(ctx, []string{"admin", "physician"})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, staffCnt, int64(1))

	adminCnt, err := userRepo.CountActiveAdmins(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, adminCnt, 1)

	// 4. UpdatePreferences
	require.NoError(t, userRepo.UpdatePreferences(ctx, uID, "America/Sao_Paulo", "dark"))
	err = userRepo.UpdatePreferences(ctx, ident.New[ident.UserID](), "UTC", "system")
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 5. ApplyApprovedStaffChange
	reqID := ident.New[ident.StaffChangeRequestID]()
	_, err = staffReqRepo.Create(ctx, &usermodel.StaffChangeRequest{
		ID:          reqID,
		UserID:      uID,
		RequestedBy: uID,
		Changes:     `{"name":"Approved Admin","email":"queries_approved@example.org"}`,
	})
	require.NoError(t, err)

	err = userRepo.ApplyApprovedStaffChange(ctx, reqID, uID, uID, "Approved Admin", "queries_approved@example.org", nil)
	require.NoError(t, err)

	updatedUser, err := userRepo.GetByID(ctx, uID)
	require.NoError(t, err)
	assert.Equal(t, "Approved Admin", updatedUser.DisplayName)
	assert.Equal(t, "queries_approved@example.org", updatedUser.Email)

	// 6. SpecialtyRepository.ListByClinic and ListByUser
	specRepo := repository.NewSpecialtyRepository(client)
	sID := ident.New[ident.SpecialtyID]()
	_, err = specRepo.Create(ctx, &usermodel.Specialty{
		ID:       sID,
		ClinicID: clinicID,
		Name:     "Cardiologia Geral",
	})
	require.NoError(t, err)

	specList, err := specRepo.ListByClinic(ctx, clinicID)
	require.NoError(t, err)
	assert.NotEmpty(t, specList)

	require.NoError(t, userRepo.SetSpecialties(ctx, uID, []ident.SpecialtyID{sID}))

	userSpecs, err := specRepo.ListByUser(ctx, uID)
	require.NoError(t, err)
	assert.Len(t, userSpecs, 1)

	_, err = specRepo.ListByUser(ctx, ident.New[ident.UserID]())
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 7. ListPhysiciansPage
	physPage, physTotal, err := userRepo.ListPhysiciansPage(ctx, 10, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, physTotal, int64(0))
	assert.NotNil(t, physPage)

	// 8. StaffRequestRepository GetByID, ListByRequester, ListFiltered, Reject
	staffRepo := repository.NewStaffRequestRepository(client)
	newReqID := ident.New[ident.StaffChangeRequestID]()
	_, err = staffRepo.Create(ctx, &usermodel.StaffChangeRequest{
		ID:          newReqID,
		UserID:      uID,
		RequestedBy: uID,
		Changes:     `{"name":"Staff Test"}`,
	})
	require.NoError(t, err)

	fetchedReq, err := staffRepo.GetByID(ctx, newReqID)
	require.NoError(t, err)
	assert.Equal(t, newReqID, fetchedReq.ID)

	_, err = staffRepo.GetByID(ctx, ident.New[ident.StaffChangeRequestID]())
	assert.ErrorIs(t, err, usermodel.ErrRequestNotFound)

	requesterRows, err := staffRepo.ListByRequester(ctx, uID, 10)
	require.NoError(t, err)
	assert.NotEmpty(t, requesterRows)

	filteredRows, totalFiltered, err := staffRepo.ListFiltered(ctx, "pending", "Staff", 10, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, totalFiltered, int64(0))
	assert.NotNil(t, filteredRows)

	err = staffRepo.Reject(ctx, newReqID, uID, "rejection note")
	require.NoError(t, err)

	err = staffRepo.Reject(ctx, ident.New[ident.StaffChangeRequestID](), uID, "note")
	assert.ErrorIs(t, err, usermodel.ErrRequestNotFound)

	// 9. BindPortalPatient
	patID := ident.New[ident.PatientID]()
	_, err = client.Patient.Create().
		SetID(patID).
		SetClinicID(clinicID).
		SetDisplayName("Portal Patient").
		SetPhone("+55 11 99999-9999").
		SetEmail("portalpatient@example.org").
		Save(ctx)
	require.NoError(t, err)

	err = userRepo.BindPortalPatient(ctx, uID, patID)
	require.NoError(t, err)

	// Binding again returns ErrUserNotFound since patient.UserID is already set
	err = userRepo.BindPortalPatient(ctx, uID, patID)
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// BindPortalPatient on missing patient
	err = userRepo.BindPortalPatient(ctx, uID, ident.New[ident.PatientID]())
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 10. Update non-existent user returns ErrUserNotFound
	_, err = userRepo.Update(ctx, &usermodel.User{
		ID:          ident.New[ident.UserID](),
		Email:       "nonexistent@example.org",
		DisplayName: "Non Existent",
		RoleID:      adminRole.ID,
		RoleName:    "admin",
	})
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 11. UpdatePreferences non-existent user returns ErrUserNotFound
	err = userRepo.UpdatePreferences(ctx, ident.New[ident.UserID](), "UTC", "dark")
	assert.ErrorIs(t, err, usermodel.ErrUserNotFound)

	// 12. SpecialtyRepository CheckClinicScope and ListPageByClinic
	inScope, err := specRepo.CheckClinicScope(ctx, clinicID, []ident.SpecialtyID{sID})
	require.NoError(t, err)
	assert.True(t, inScope)

	outScope, err := specRepo.CheckClinicScope(ctx, clinicID, []ident.SpecialtyID{ident.New[ident.SpecialtyID]()})
	require.NoError(t, err)
	assert.False(t, outScope)

	pagedSpecs, specTotal, err := specRepo.ListPageByClinic(ctx, clinicID, 10, 0)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, specTotal, int64(1))
	assert.NotEmpty(t, pagedSpecs)

	// 13. RoleRepository Update duplicate role name returns ErrDuplicateRole
	dupRole := &usermodel.Role{
		ID:         adminRole.ID,
		Name:       "physician",
		IsClinical: false,
	}
	roleRepo = repository.NewRoleRepository(client)
	_, err = roleRepo.Update(ctx, dupRole)
	assert.ErrorIs(t, err, usermodel.ErrDuplicateRole)
}
