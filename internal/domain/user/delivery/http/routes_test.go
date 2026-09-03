package http_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/keystore"
	"librevita.org/internal/core/kv"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/database/record"
	clinicrepo "librevita.org/internal/domain/clinic/repository"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	identrepo "librevita.org/internal/domain/identifier/repository"
	patientrepo "librevita.org/internal/domain/patient/repository"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	userrepo "librevita.org/internal/domain/user/repository"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/test"
	"librevita.org/pkg/ident"
	"librevita.org/pkg/log"
)

type userHttpTestEnv struct {
	echo        *echo.Echo
	handler     *httphandler.Handler
	svc         *usecase.Service
	sessions    *auth.SessionManager
	client      *record.Client
	adminToken  string
	adminUser   *usecase.GetUserByIDRow
	adminCookie *http.Cookie
	clinicID    ident.ClinicID
}

func (e *userHttpTestEnv) newContext(req *http.Request, rec *httptest.ResponseRecorder) echo.Context {
	ctx := clinicctx.WithTestClinic(req.Context())
	ctx = fle.WithClinicID(ctx, e.clinicID)
	req = req.WithContext(ctx)
	c := e.echo.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID: e.adminUser.ID.String(), Email: e.adminUser.Email, Name: e.adminUser.DisplayName, Role: auth.RoleAdmin,
	})
	return c
}

func setupUserHttpEnv(t *testing.T) *userHttpTestEnv {
	t.Helper()
	logger := log.Nop()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "user_http.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, database.Migrate(context.Background(), db, logger))

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := record.NewClient(record.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	sessKV, err := kv.OpenBBolt(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sessKV.Close() })

	sessions, err := auth.NewSessionManager(auth.NewSessionRepository(sessKV, client), &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)
	sessions.SetPlatformRepository(auth.NewPlatformSessionRepository(sessKV, client))

	auditLogger, err := audit.NewLogger(audit.NewAuditRepository(client), logger)
	require.NoError(t, err)

	policies, err := policy.NewPolicyEngine(policy.NewPolicyRepository(client), logger)
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	uRepo := userrepo.NewUserRepository(client)
	roleRepo := userrepo.NewRoleRepository(client)
	specRepo := userrepo.NewSpecialtyRepository(client)
	staffReqRepo := userrepo.NewStaffRequestRepository(client)
	setupRepo := userrepo.NewSetupRepository(client)
	svc := usecase.NewService(uRepo, roleRepo, specRepo, staffReqRepo, setupRepo, sessions, auditLogger, logger)

	const testMasterKey = "nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=" // gitleaks:allow
	ks := keystore.Wrap(sessKV)
	cryptoEngine, err := crypto.NewEngine(testMasterKey, ks)
	require.NoError(t, err)

	patRepo := patientrepo.NewPatientRepository(client)
	patSvc := patientusecase.NewService(patRepo, policies, cryptoEngine)

	clinicRepo := clinicrepo.NewClinicRepository(client)
	platformRepo := clinicrepo.NewPlatformUserRepository(client)
	platformSvc := clinicusecase.NewPlatformService(platformRepo, clinicRepo, cryptoEngine)
	systemsRepo := identrepo.NewSystemRepository(client)

	clocks := clinicusecase.NewClockProvider(clinicRepo)
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})

	s, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	fileMgr, err := storage.NewFileManager(storage.NewIndexRepository(client), s, logger)
	require.NoError(t, err)

	h := httphandler.NewHandler(svc, patSvc, platformSvc, systemsRepo, csrf, sessions, policies, auditLogger, clocks, fileMgr, &config.Config{Mode: "development"}, logger)

	// Seed clinic and admin
	clinicID := clinicctx.TestClinicID
	now := time.Now().UTC()
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("test").
		SetName("Test Clinic").
		SetOnboardedAt(now).
		Save(context.Background())
	require.NoError(t, err)

	ctx := clinicctx.WithTestClinic(context.Background())
	require.NoError(t, roleRepo.SeedDefaults(ctx))

	// Hash password properly for login test
	hash, err := auth.HashPassword("AdminPassword123!")
	require.NoError(t, err)
	require.NoError(t, test.User(ctx, client, "01990000-0000-7000-8000-00000000000a", "admin@example.org", "admin", hash))

	adminUser, err := svc.GetUser(ctx, "01990000-0000-7000-8000-00000000000a")
	require.NoError(t, err)

	token, err := sessions.Create(ctx, auth.Principal{
		ID: adminUser.ID.String(), Email: adminUser.Email, Name: adminUser.DisplayName, Role: auth.RoleAdmin,
	})
	require.NoError(t, err)
	cookie := sessions.Cookie(token)

	e := echo.New()

	return &userHttpTestEnv{
		echo:        e,
		handler:     h,
		svc:         svc,
		sessions:    sessions,
		client:      client,
		adminToken:  token,
		adminUser:   adminUser,
		adminCookie: cookie,
		clinicID:    clinicID,
	}
}

func TestLoginPageAndSubmit(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /auth/login
	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.LoginPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /auth/login - valid credentials
	form := url.Values{
		"email":    {"admin@example.org"},
		"password": {"AdminPassword123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.Login(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, rec.Code)

	// 3. POST /auth/login - wrong password
	form = url.Values{
		"email":    {"admin@example.org"},
		"password": {"WrongPassword!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.Login(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	// 4. POST /auth/login - invalid form
	form = url.Values{
		"email":    {"invalid-email"},
		"password": {""},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.Login(c)
	require.NoError(t, err)
	assert.True(t, rec.Code == http.StatusBadRequest || rec.Code == http.StatusUnauthorized)
}

func TestUsersManagementRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /users (UsersPage)
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.UsersPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /users/new (UserNewPage)
	req = httptest.NewRequest(http.MethodGet, "/users/new", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.UserNewPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. POST /users (UserCreate)
	form := url.Values{
		"name":     {"Dr. Novo Medico"},
		"email":    {"novomedico@example.org"},
		"password": {"SecretPass123!"},
		"role":     {"physician"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.UserCreate(c)
	require.NoError(t, err)

	// Fetch created user
	users, err := env.client.User.Query().All(context.Background())
	require.NoError(t, err)
	var createdID string
	for _, u := range users {
		if u.Email == "novomedico@example.org" {
			createdID = u.ID.String()
		}
	}
	require.NotEmpty(t, createdID)

	// 4. GET /users/:id/edit (UserEditPage)
	req = httptest.NewRequest(http.MethodGet, "/users/"+createdID+"/edit", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(createdID)
	err = env.handler.UserEditPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. POST /users/:id (UserUpdate)
	updateForm := url.Values{
		"name":   {"Dr. Medico Atualizado"},
		"email":  {"novomedico@example.org"},
		"role":   {"physician"},
		"active": {"on"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users/"+createdID, strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(createdID)
	err = env.handler.UserUpdate(c)
	require.NoError(t, err)

	// 6. POST /users/:id/status (UserStatus)
	statusForm := url.Values{
		"active": {"false"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users/"+createdID+"/status", strings.NewReader(statusForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(createdID)
	err = env.handler.UserStatus(c)
	require.NoError(t, err)
}

func TestProfileAndPreferencesRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /profile (ProfilePage)
	req := httptest.NewRequest(http.MethodGet, "/profile", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.ProfilePage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /profile (ProfileUpdate - action=profile)
	form := url.Values{
		"action": {"profile"},
		"name":   {"Admin Renamed"},
		"email":  {"admin@example.org"},
	}
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.ProfileUpdate(c)
	require.NoError(t, err)

	// 3. POST /profile (ProfileUpdate - action=password)
	pwForm := url.Values{
		"action":           {"password"},
		"current_password": {"AdminPassword123!"},
		"new_password":     {"NewAdminPass456!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(pwForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.ProfileUpdate(c)
	require.NoError(t, err)

	// 4. POST /profile (ProfileUpdate - action=preferences)
	prefForm := url.Values{
		"action":   {"preferences"},
		"timezone": {"America/Sao_Paulo"},
		"theme":    {"dark"},
	}
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(prefForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.ProfileUpdate(c)
	require.NoError(t, err)
}

func TestRolesAndSpecialtiesRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /roles (RolesPage)
	req := httptest.NewRequest(http.MethodGet, "/roles", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.RolesPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /roles (RoleCreate)
	form := url.Values{
		"name":        {"Biomedico"},
		"is_clinical": {"true"},
	}
	req = httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.RoleCreate(c)
	require.NoError(t, err)

	// 3. GET /specialties (SpecialtiesPage)
	req = httptest.NewRequest(http.MethodGet, "/specialties", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.SpecialtiesPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. POST /specialties (SpecialtyCreate)
	specCreateForm := url.Values{
		"name": {"Dermatologia"},
	}
	req = httptest.NewRequest(http.MethodPost, "/specialties", strings.NewReader(specCreateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.SpecialtyCreate(c)
	require.NoError(t, err)

	// Fetch created specialty
	allSpecs, err := env.client.Specialty.Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, allSpecs)
	targetSpecID := allSpecs[0].ID.String()

	// 5. POST /specialties/:id/delete (SpecialtyDelete)
	req = httptest.NewRequest(http.MethodPost, "/specialties/"+targetSpecID+"/delete", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(targetSpecID)
	err = env.handler.SpecialtyDelete(c)
	require.NoError(t, err)
}

func TestAdminPoliciesRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /admin/policies
	req := httptest.NewRequest(http.MethodGet, "/admin/policies", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.AdminPoliciesPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /admin/policies (Save Policy)
	form := url.Values{
		"name":       {"patient.view"},
		"expression": {"principal.role in ['admin', 'physician', 'receptionist']"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.AdminPolicySave(c)
	require.NoError(t, err)

	// 3. POST /admin/policies/reset (Reset Policy)
	resetForm := url.Values{
		"name": {"patient.view"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(resetForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.AdminPolicyReset(c)
	require.NoError(t, err)
}

func TestHomeAndActivityRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET / (Home)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.Home(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /home/activity (HomeActivity)
	req = httptest.NewRequest(http.MethodGet, "/home/activity", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.HomeActivity(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoleRenameClinicalDelete(t *testing.T) {
	env := setupUserHttpEnv(t)
	ctx := clinicctx.WithTestClinic(context.Background())

	// 1. Create a custom non-system role
	r, err := env.client.Role.Create().
		SetClinicID(env.clinicID).
		SetName("Enfermeiro").
		SetIsClinical(false).
		SetSystem(false).
		Save(ctx)
	require.NoError(t, err)
	roleID := r.ID.String()

	// 2. POST /roles/:id/rename (RoleRename)
	renameForm := url.Values{
		"name": {"Enfermeiro Chefe"},
	}
	req := httptest.NewRequest(http.MethodPost, "/roles/"+roleID+"/rename", strings.NewReader(renameForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(roleID)
	err = env.handler.RoleRename(c)
	require.NoError(t, err)

	// 3. POST /roles/:id/clinical (RoleClinical)
	clinicalForm := url.Values{
		"clinical": {"on"},
	}
	req = httptest.NewRequest(http.MethodPost, "/roles/"+roleID+"/clinical", strings.NewReader(clinicalForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(roleID)
	err = env.handler.RoleClinical(c)
	require.NoError(t, err)

	// 4. POST /roles/:id/delete (RoleDelete)
	req = httptest.NewRequest(http.MethodPost, "/roles/"+roleID+"/delete", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(roleID)
	err = env.handler.RoleDelete(c)
	require.NoError(t, err)
}

func TestPlatformBootstrapAndProvision(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /setup on Apex (SetupPage -> platformSetupPage)
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec := httptest.NewRecorder()
	c := env.echo.NewContext(req, rec)
	err := env.handler.SetupPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /setup on Apex (Setup -> platformBootstrap)
	bootForm := url.Values{
		"admin_name":     {"Platform Master"},
		"admin_email":    {"master@librevita.org"},
		"admin_password": {"MasterSecretPass123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(bootForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec = httptest.NewRecorder()
	c = env.echo.NewContext(req, rec)
	err = env.handler.Setup(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, rec.Code)

	// 3. GET /provision (ProvisionPage with platform operator)
	req = httptest.NewRequest(http.MethodGet, "/provision", nil)
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec = httptest.NewRecorder()
	c = env.echo.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:       "01990000-0000-7000-8000-000000000099",
		Email:    "master@librevita.org",
		Name:     "Platform Master",
		Role:     auth.RoleAdmin,
		Platform: true,
	})
	err = env.handler.ProvisionPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. POST /provision (Provision)
	provForm := url.Values{
		"clinic_name":        {"Nova Clinica Sao Paulo"},
		"clinic_slug":        {"sp-clinic"},
		"clinic_tax_id":      {"12.345.678/0001-90"},
		"clinic_phone":       {"+55 11 91234-5678"},
		"clinic_email":       {"contato@spclinic.org"},
		"clinic_street":      {"Av. Paulista, 1000"},
		"clinic_city":        {"Sao Paulo"},
		"clinic_state":       {"SP"},
		"clinic_postal_code": {"01310-100"},
		"clinic_timezone":    {"America/Sao_Paulo"},
	}
	req = httptest.NewRequest(http.MethodPost, "/provision", strings.NewReader(provForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec = httptest.NewRecorder()
	c = env.echo.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:       "01990000-0000-7000-8000-000000000099",
		Email:    "master@librevita.org",
		Name:     "Platform Master",
		Role:     auth.RoleAdmin,
		Platform: true,
	})
	err = env.handler.Provision(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestDashboardProfilePoliciesAuditAndSpecialties(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. Home & HomeActivity
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.Home(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/activity", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.HomeActivity(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. ProfilePage & ProfileUpdate
	req = httptest.NewRequest(http.MethodGet, "/profile", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.ProfilePage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	profForm := url.Values{
		"ui_theme": {"dark"},
		"timezone": {"America/Sao_Paulo"},
	}
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(profForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.ProfileUpdate(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// ProfileUpdate with invalid timezone
	badProfForm := url.Values{
		"ui_theme": {"dark"},
		"timezone": {"Invalid/Timezone"},
	}
	req = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(badProfForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.ProfileUpdate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. AdminPoliciesPage, Save and Reset
	req = httptest.NewRequest(http.MethodGet, "/admin/policies", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPoliciesPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	policyForm := url.Values{
		"name":       {"dashboard.view"},
		"expression": {"principal.role in ['admin', 'physician']"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies", strings.NewReader(policyForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPolicySave(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	resetForm := url.Values{"name": {"dashboard.view"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(resetForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPolicyReset(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. AuditIntegrity
	req = httptest.NewRequest(http.MethodGet, "/admin/audit/integrity", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AuditIntegrity(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. SpecialtiesPage, SpecialtyCreate, SpecialtyDelete
	req = httptest.NewRequest(http.MethodGet, "/admin/specialties", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtiesPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	specForm := url.Values{"name": {"Cardiologia"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/specialties", strings.NewReader(specForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUserManagementErrorPathsAndFragments(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. UserStatus demoting self returns OOB alert
	req := httptest.NewRequest(http.MethodPost, "/users/"+env.adminUser.ID.String()+"/status", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserStatus(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "You cannot change your own status")

	// 2. SpecialtyCreate with empty name
	badSpecForm := url.Values{"name": {""}}
	req = httptest.NewRequest(http.MethodPost, "/admin/specialties", strings.NewReader(badSpecForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. SpecialtiesPage HTMX table
	req = httptest.NewRequest(http.MethodGet, "/admin/specialties?page=1", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtiesPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. RoleCreate with empty name
	badRoleForm := url.Values{"name": {""}}
	req = httptest.NewRequest(http.MethodPost, "/admin/roles", strings.NewReader(badRoleForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.RoleCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. UserCreate validation error
	badUserForm := url.Values{
		"name":     {""},
		"email":    {"invalid"},
		"password": {"short"},
		"role":     {"physician"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(badUserForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.UserCreate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 6. UserUpdate validation error
	badUpdateForm := url.Values{
		"name":  {""},
		"email": {"invalid"},
		"role":  {"physician"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users/"+env.adminUser.ID.String(), strings.NewReader(badUpdateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserUpdate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuditIntegrityAndAdminHandlerEdgeCases(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. AuditIntegrity endpoint
	req := httptest.NewRequest(http.MethodGet, "/admin/audit/integrity", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.AuditIntegrity(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)

	// 2. SpecialtyCreate and SpecialtyDelete
	specForm := url.Values{"name": {"Ortopedia Pediatrica"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/specialties", strings.NewReader(specForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound)

	specs, err := env.client.Specialty.Query().All(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, specs)

	req = httptest.NewRequest(http.MethodPost, "/admin/specialties/"+specs[0].ID.String()+"/delete", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(specs[0].ID.String())
	require.NoError(t, env.handler.SpecialtyDelete(c))
	assert.True(t, rec.Code == http.StatusOK || rec.Code == http.StatusFound)

	// 3. AdminPolicyReset
	resetForm := url.Values{"name": {"patient.view"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(resetForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPolicyReset(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. UserNewPage and UserEditPage
	req = httptest.NewRequest(http.MethodGet, "/users/new", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.UserNewPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/users/"+env.adminUser.ID.String()+"/edit", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserEditPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. HomeActivity fragment
	req = httptest.NewRequest(http.MethodGet, "/activity?before=10", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.HomeActivity(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRegisterAndLogout(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. RegisterPage
	req := httptest.NewRequest(http.MethodGet, "/auth/register", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.RegisterPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "csrf")

	// 2. Register with missing name (validation error)
	form2 := url.Values{
		"name":     {""},
		"email":    {"bad@example.org"},
		"password": {"SecurePassword123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form2.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.Register(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Register with valid data but missing patient_id (validation error)
	form3 := url.Values{
		"name":     {"Valid Name"},
		"email":    {"valid@example.org"},
		"password": {"SecurePassword123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(form3.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.Register(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 4. Logout with session cookie
	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.Request().AddCookie(env.adminCookie)
	require.NoError(t, env.handler.Logout(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 5. Logout without session cookie
	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.Logout(c))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestSetupGateMiddleware(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. Clinic with onboarded_at set: gate should pass through
	nextCalled := false
	gate := env.handler.SetupGate()
	handler := gate(func(c echo.Context) error {
		nextCalled = true
		return c.String(http.StatusOK, "passed")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, handler(c))
	assert.True(t, nextCalled)

	// 2. Static path is not gated
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, handler(c))
	assert.True(t, nextCalled)

	// 3. /setup path is not gated
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, handler(c))
	assert.True(t, nextCalled)

	// 4. /healthz path is not gated
	nextCalled = false
	req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, handler(c))
	assert.True(t, nextCalled)
}

func TestUserStatusToggle(t *testing.T) {
	env := setupUserHttpEnv(t)
	ctx := clinicctx.WithTestClinic(context.Background())

	// Create a second user to toggle
	hash, err := auth.HashPassword("Password123!")
	require.NoError(t, err)
	require.NoError(t, test.User(ctx, env.client, "01990000-0000-7000-8000-00000000000b", "toggle@example.org", "physician", hash))

	// Toggle user status (deactivate)
	req := httptest.NewRequest(http.MethodPost, "/users/01990000-0000-7000-8000-00000000000b/status", nil)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-00000000000b")
	require.NoError(t, env.handler.UserStatus(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Toggle again (reactivate)
	req = httptest.NewRequest(http.MethodPost, "/users/01990000-0000-7000-8000-00000000000b/status", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-00000000000b")
	require.NoError(t, env.handler.UserStatus(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Toggle admin's own status (should get error message)
	req = httptest.NewRequest(http.MethodPost, "/users/"+env.adminUser.ID.String()+"/status", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserStatus(c))
	assert.Equal(t, http.StatusOK, rec.Code) // Returns 200 with OOB alert

	// UserStatus for non-existent user
	req = httptest.NewRequest(http.MethodPost, "/users/01990000-0000-7000-8000-ffffffffffff/status", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-ffffffffffff")
	err = env.handler.UserStatus(c)
	assert.Error(t, err)
}

func TestSetupPageAndSubmit(t *testing.T) {
	// Use a fresh DB without onboarding for setup flow
	logger := log.Nop()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "setup.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, database.Migrate(context.Background(), db, logger))

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := record.NewClient(record.Driver(drv))
	t.Cleanup(func() { _ = client.Close() })

	sessKV, err := kv.OpenBBolt(filepath.Join(t.TempDir(), "sessions.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sessKV.Close() })

	sessions, err := auth.NewSessionManager(auth.NewSessionRepository(sessKV, client), &config.Config{Mode: "development"}, logger)
	require.NoError(t, err)
	auditLogger, err := audit.NewLogger(audit.NewAuditRepository(client), logger)
	require.NoError(t, err)
	policies, err := policy.NewPolicyEngine(policy.NewPolicyRepository(client), logger)
	require.NoError(t, err)
	require.NoError(t, policies.Load(context.Background()))

	uRepo := userrepo.NewUserRepository(client)
	roleRepo := userrepo.NewRoleRepository(client)
	specRepo := userrepo.NewSpecialtyRepository(client)
	staffReqRepo := userrepo.NewStaffRequestRepository(client)
	setupRepo := userrepo.NewSetupRepository(client)
	svc := usecase.NewService(uRepo, roleRepo, specRepo, staffReqRepo, setupRepo, sessions, auditLogger, logger)

	clinicRepo := clinicrepo.NewClinicRepository(client)
	systemsRepo := identrepo.NewSystemRepository(client)
	clocks := clinicusecase.NewClockProvider(clinicRepo)
	csrf := auth.NewCSRF(&config.Config{Mode: "development"})

	s, err := storage.NewLocal(t.TempDir())
	require.NoError(t, err)
	fileMgr, err := storage.NewFileManager(storage.NewIndexRepository(client), s, logger)
	require.NoError(t, err)

	h := httphandler.NewHandler(svc, nil, nil, systemsRepo, csrf, sessions, policies, auditLogger, clocks, fileMgr, &config.Config{Mode: "development"}, logger)

	// Create an un-onboarded clinic
	clinicID := clinicctx.TestClinicID
	_, err = client.Clinic.Create().
		SetID(clinicID).
		SetSlug("setup-test").
		SetName("Setup Test Clinic").
		Save(context.Background())
	require.NoError(t, err)

	e := echo.New()
	unonboardedClinic := &clinicctx.Clinic{
		ID:       clinicID,
		Slug:     "setup-test",
		Name:     "Setup Test Clinic",
		Timezone: "America/Sao_Paulo",
	}
	newCtx := func(req *http.Request, rec *httptest.ResponseRecorder) echo.Context {
		ctx := clinicctx.WithClinic(req.Context(), unonboardedClinic)
		req = req.WithContext(ctx)
		return e.NewContext(req, rec)
	}

	// 1. SetupPage renders the onboarding form
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	c := newCtx(req, rec)
	require.NoError(t, h.SetupPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. SetupGate redirects un-onboarded clinic to /setup
	gate := h.SetupGate()
	gatedHandler := gate(func(c echo.Context) error {
		return c.String(http.StatusOK, "should not reach")
	})

	req = httptest.NewRequest(http.MethodGet, "/patients", nil)
	rec = httptest.NewRecorder()
	c = newCtx(req, rec)
	require.NoError(t, gatedHandler(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 3. Setup form submission creates admin and onboards
	form := url.Values{
		"admin_name":     {"Setup Admin"},
		"admin_email":    {"setup@example.org"},
		"admin_password": {"SecurePassword123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = newCtx(req, rec)
	require.NoError(t, h.Setup(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 4. SetupPage after onboarding redirects to login
	req = httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec = httptest.NewRecorder()
	c = newCtx(req, rec)
	require.NoError(t, h.SetupPage(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 5. Setup form after onboarding redirects to login
	req = httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = newCtx(req, rec)
	require.NoError(t, h.Setup(c))
	assert.Equal(t, http.StatusFound, rec.Code)
}

func TestUsersPageHTMXFragmentAndPagination(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. UsersPage with HTMX header returns UsersListTable fragment
	req := httptest.NewRequest(http.MethodGet, "/users?q=admin&page=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.UsersPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "admin@example.org")

	// 2. UsersPage with boosted HX-Request renders full page
	req = httptest.NewRequest(http.MethodGet, "/users?page=2", nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Boosted", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.UsersPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. AdminPolicySave with HTMX header returns policy card fragment
	policyForm := url.Values{
		"name":       {"chart.view"},
		"expression": {"true"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies", strings.NewReader(policyForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPolicySave(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. AdminPolicyReset with HTMX header returns refreshed card fragment
	resetForm := url.Values{
		"name": {"chart.view"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(resetForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AdminPolicyReset(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestApexPlatformLoginAndProvisionErrors(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. Provision on apex without platform operator principal -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/provision", nil)
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec := httptest.NewRecorder()
	c := env.echo.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:       "01990000-0000-7000-8000-000000000099",
		Role:     auth.RoleAdmin,
		Platform: false, // not a platform operator
	})
	err := env.handler.ProvisionPage(c)
	assert.Error(t, err)

	// 2. Provision with invalid slug -> 400 Bad Request
	provForm := url.Values{
		"clinic_name": {"Bad Clinic"},
		"clinic_slug": {"INVALID SLUG WITH SPACES!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/provision", strings.NewReader(provForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec = httptest.NewRecorder()
	c = env.echo.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:       "01990000-0000-7000-8000-000000000099",
		Role:     auth.RoleAdmin,
		Platform: true,
	})
	require.NoError(t, env.handler.Provision(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 3. Platform login with wrong credentials -> 401 Unauthorized
	loginForm := url.Values{
		"email":    {"nonexistent@librevita.org"},
		"password": {"WrongPassword123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(loginForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(clinicctx.WithApex(req.Context()))
	rec = httptest.NewRecorder()
	c = env.echo.NewContext(req, rec)
	require.NoError(t, env.handler.Login(c))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestSpecialtyAndRoleCreateErrorBranches(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. SpecialtiesPage HTMX table fragment
	req := httptest.NewRequest(http.MethodGet, "/specialties?page=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtiesPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. SpecialtyCreate with empty name (validation error, returns 200 with error form)
	form := url.Values{"name": {""}}
	req = httptest.NewRequest(http.MethodPost, "/specialties", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. SpecialtyCreate successful
	form = url.Values{"name": {"Dermatologia Especial"}}
	req = httptest.NewRequest(http.MethodPost, "/specialties", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.Equal(t, http.StatusFound, rec.Code)

	// 4. SpecialtyCreate duplicate name returns 200 with error form
	req = httptest.NewRequest(http.MethodPost, "/specialties", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.SpecialtyCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 5. RoleCreate duplicate name (e.g. "admin") returns 200 with error form
	roleForm := url.Values{
		"name":     {"admin"},
		"clinical": {""},
	}
	req = httptest.NewRequest(http.MethodPost, "/roles", strings.NewReader(roleForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.RoleCreate(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 6. RoleRename on system role (e.g. admin role) returns 400 with error message
	allRoles, err := env.client.Role.Query().All(context.Background())
	require.NoError(t, err)
	var adminRoleID string
	for _, r := range allRoles {
		if r.Name == "admin" {
			adminRoleID = r.ID.String()
			break
		}
	}
	require.NotEmpty(t, adminRoleID)

	renameForm := url.Values{"name": {"SuperAdmin"}}
	req = httptest.NewRequest(http.MethodPost, "/roles/"+adminRoleID+"/rename", strings.NewReader(renameForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(adminRoleID)
	require.NoError(t, env.handler.RoleRename(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 7. RoleClinical on system role returns 400 with error message
	clinForm := url.Values{"clinical": {"on"}}
	req = httptest.NewRequest(http.MethodPost, "/roles/"+adminRoleID+"/clinical", strings.NewReader(clinForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(adminRoleID)
	require.NoError(t, env.handler.RoleClinical(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 8. RoleDelete on system role returns 400 with error message
	req = httptest.NewRequest(http.MethodPost, "/roles/"+adminRoleID+"/delete", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(adminRoleID)
	require.NoError(t, env.handler.RoleDelete(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestUserCreateAndUpdateErrorBranches(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. UserCreate validation error (missing name)
	createForm := url.Values{
		"name":     {""},
		"email":    {"valid@example.org"},
		"password": {"Secret123!"},
		"role":     {"physician"},
	}
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(createForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.UserCreate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 2. UserCreate email taken
	createForm = url.Values{
		"name":     {"Another Admin"},
		"email":    {"admin@example.org"}, // already exists
		"password": {"Secret123!"},
		"role":     {"physician"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(createForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.UserCreate(c))
	assert.Equal(t, http.StatusConflict, rec.Code)

	// 3. UserUpdate validation error (missing name)
	updateForm := url.Values{
		"name":   {""},
		"email":  {"admin@example.org"},
		"role":   {"admin"},
		"active": {"on"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users/"+env.adminUser.ID.String(), strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserUpdate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 4. UserUpdate cannot demote self
	updateForm = url.Values{
		"name":   {"Admin Self Demote"},
		"email":  {"admin@example.org"},
		"role":   {"physician"}, // changing own role from admin
		"active": {"on"},
	}
	req = httptest.NewRequest(http.MethodPost, "/users/"+env.adminUser.ID.String(), strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(env.adminUser.ID.String())
	require.NoError(t, env.handler.UserUpdate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 5. UserEditPage on non-existent user returns 404
	req = httptest.NewRequest(http.MethodGet, "/users/01990000-0000-7000-8000-ffffffffffff/edit", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-ffffffffffff")
	err := env.handler.UserEditPage(c)
	assert.Error(t, err)

	// 6. UserUpdate on non-existent user returns 404
	req = httptest.NewRequest(http.MethodPost, "/users/01990000-0000-7000-8000-ffffffffffff", strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-ffffffffffff")
	err = env.handler.UserUpdate(c)
	assert.Error(t, err)

	// 7. AuditIntegrity endpoint
	req = httptest.NewRequest(http.MethodGet, "/audit/integrity", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.AuditIntegrity(c))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ok":true`)

	// 8. SpecialtyDelete endpoint
	delReq := httptest.NewRequest(http.MethodPost, "/specialties/01990000-0000-7000-8000-ffffffffffff/delete", nil)
	delRec := httptest.NewRecorder()
	delCtx := env.newContext(delReq, delRec)
	delCtx.SetParamNames("id")
	delCtx.SetParamValues("01990000-0000-7000-8000-ffffffffffff")
	require.NoError(t, env.handler.SpecialtyDelete(delCtx))
	assert.Equal(t, http.StatusFound, delRec.Code)

	// 9. ProfilePage without principal redirects to login
	profReq := httptest.NewRequest(http.MethodGet, "/profile", nil)
	profRec := httptest.NewRecorder()
	profCtx := env.echo.NewContext(profReq, profRec)
	require.NoError(t, env.handler.ProfilePage(profCtx))
	assert.Equal(t, http.StatusFound, profRec.Code)

	// 10. ProfileUpdate without principal redirects to login
	profUpReq := httptest.NewRequest(http.MethodPost, "/profile", nil)
	profUpRec := httptest.NewRecorder()
	profUpCtx := env.echo.NewContext(profUpReq, profUpRec)
	require.NoError(t, env.handler.ProfileUpdate(profUpCtx))
	assert.Equal(t, http.StatusFound, profUpRec.Code)

	// 11. ProfileUpdate with invalid timezone returns 400
	badTzForm := url.Values{
		"timezone": {"Invalid/Timezone/Bad"},
		"ui_theme": {"dark"},
	}
	profUpReq = httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(badTzForm.Encode()))
	profUpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	profUpRec = httptest.NewRecorder()
	profUpCtx = env.newContext(profUpReq, profUpRec)
	require.NoError(t, env.handler.ProfileUpdate(profUpCtx))
	assert.Equal(t, http.StatusBadRequest, profUpRec.Code)

	// 12. UserNewPage renders user creation form
	newUserReq := httptest.NewRequest(http.MethodGet, "/users/new", nil)
	newUserRec := httptest.NewRecorder()
	newUserCtx := env.newContext(newUserReq, newUserRec)
	require.NoError(t, env.handler.UserNewPage(newUserCtx))
	assert.Equal(t, http.StatusOK, newUserRec.Code)

	// 13. UserUpdate with all fields changed (to cover userChanges name, email, role, status branches)
	targetUser, err := env.svc.CreateUser(clinicctx.WithTestClinic(context.Background()), usecase.CreateUserInput{
		Name:     "Target Old Name",
		Email:    "targetuseruniq99@example.org",
		Password: "SecurePassword123!",
		Role:     "physician",
	})
	require.NoError(t, err)

	upAllForm := url.Values{
		"name":   {"Target New Name"},
		"email":  {"targetnew@example.org"},
		"role":   {"receptionist"},
		"active": {""}, // deactivate: status active -> inactive
	}
	upAllReq := httptest.NewRequest(http.MethodPost, "/users/"+targetUser.ID.String(), strings.NewReader(upAllForm.Encode()))
	upAllReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	upAllRec := httptest.NewRecorder()
	upAllCtx := env.newContext(upAllReq, upAllRec)
	upAllCtx.SetParamNames("id")
	upAllCtx.SetParamValues(targetUser.ID.String())
	require.NoError(t, env.handler.UserUpdate(upAllCtx))
	assert.Equal(t, http.StatusFound, upAllRec.Code)

	// 14. AdminPolicyReset on unknown policy returns 404
	unkForm := url.Values{"name": {"unknown.policy.name"}}
	unkReq := httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(unkForm.Encode()))
	unkReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	unkRec := httptest.NewRecorder()
	unkCtx := env.newContext(unkReq, unkRec)
	err = env.handler.AdminPolicyReset(unkCtx)
	assert.Error(t, err)

	// 15. AdminPolicySave without HTMX header renders full policies page (non-HTMX branch)
	saveNonHtmxForm := url.Values{
		"name":       {"chart.view"},
		"expression": {"true"},
	}
	snhReq := httptest.NewRequest(http.MethodPost, "/admin/policies", strings.NewReader(saveNonHtmxForm.Encode()))
	snhReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	snhRec := httptest.NewRecorder()
	snhCtx := env.newContext(snhReq, snhRec)
	require.NoError(t, env.handler.AdminPolicySave(snhCtx))
	assert.Equal(t, http.StatusOK, snhRec.Code)

	// 16. AdminPolicyReset without HTMX header renders full policies page (non-HTMX branch)
	rnhReq := httptest.NewRequest(http.MethodPost, "/admin/policies/reset", strings.NewReader(url.Values{"name": {"chart.view"}}.Encode()))
	rnhReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rnhRec := httptest.NewRecorder()
	rnhCtx := env.newContext(rnhReq, rnhRec)
	require.NoError(t, env.handler.AdminPolicyReset(rnhCtx))
	assert.Equal(t, http.StatusOK, rnhRec.Code)
}
