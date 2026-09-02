package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/ent/role"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/testutil"
	"librevita.org/pkg/ident"
)

func TestStaffRoutes(t *testing.T) {
	env := setupUserHttpEnv(t)
	ctx := clinicctx.WithTestClinic(context.Background())

	// Seed a physician user with clinical role
	physID := "01990000-0000-7000-8000-00000000000b"
	require.NoError(t, testutil.User(ctx, env.client, physID, "dr.phys@example.org", "physician", "PhysPass123!"))

	// Ensure role is marked as clinical
	r, err := env.client.Role.Query().Where(role.NameEQ("physician")).Only(ctx)
	require.NoError(t, err)
	_, err = env.client.Role.UpdateOneID(r.ID).SetIsClinical(true).Save(ctx)
	require.NoError(t, err)

	// 1. GET /staff (StaffPage)
	req := httptest.NewRequest(http.MethodGet, "/staff", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err = env.handler.StaffPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. GET /staff/:id/edit (StaffEditPage)
	req = httptest.NewRequest(http.MethodGet, "/staff/"+physID+"/edit", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(physID)
	err = env.handler.StaffEditPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. POST /staff/:id (StaffUpdate - direct admin edit)
	updateForm := url.Values{
		"name":  {"Dr. Phys Updated"},
		"email": {"dr.phys@example.org"},
	}
	req = httptest.NewRequest(http.MethodPost, "/staff/"+physID, strings.NewReader(updateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(physID)
	err = env.handler.StaffUpdate(c)
	require.NoError(t, err)

	// 4. POST /staff/:id/request (StaffRequestChange)
	reqForm := url.Values{
		"name":  {"Dr. Phys Proposal"},
		"email": {"dr.phys.proposal@example.org"},
	}
	req = httptest.NewRequest(http.MethodPost, "/staff/"+physID+"/request", strings.NewReader(reqForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(physID)
	c.Set("server.principal", &auth.Principal{
		ID: env.adminUser.ID.String(), Email: env.adminUser.Email, Name: env.adminUser.DisplayName, Role: auth.RoleReceptionist,
	})
	err = env.handler.StaffRequestChange(c)
	require.NoError(t, err)

	// 5. GET /staff/requests (StaffRequestsPage)
	req = httptest.NewRequest(http.MethodGet, "/staff/requests", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.StaffRequestsPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 6. GET /staff/my-requests (MyStaffRequestsPage)
	req = httptest.NewRequest(http.MethodGet, "/staff/my-requests", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID: env.adminUser.ID.String(), Email: env.adminUser.Email, Name: env.adminUser.DisplayName, Role: auth.RoleReceptionist,
	})
	err = env.handler.MyStaffRequestsPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Fetch pending request
	pendingReqs, err := env.client.StaffChangeRequest.Query().All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pendingReqs)
	targetReqID := pendingReqs[0].ID.String()

	// 7. POST /staff/requests/:id/reject (StaffRequestReject)
	rejectForm := url.Values{
		"note": {"Rejeitado para teste"},
	}
	req = httptest.NewRequest(http.MethodPost, "/staff/requests/"+targetReqID+"/reject", strings.NewReader(rejectForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(targetReqID)
	err = env.handler.StaffRequestReject(c)
	require.NoError(t, err)

	// 8. Create another request and approve it
	chReq2, err := env.client.StaffChangeRequest.Create().
		SetID(ident.New[ident.StaffChangeRequestID]()).
		SetClinicID(env.clinicID).
		SetUserID(ident.MustParseUser(physID)).
		SetRequestedBy(env.adminUser.ID).
		SetChanges(`{"name": "Dr. Approved Name", "email": "dr.approved@example.org"}`).
		Save(ctx)
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodPost, "/staff/requests/"+chReq2.ID.String()+"/approve", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(chReq2.ID.String())
	err = env.handler.StaffRequestApprove(c)
	require.NoError(t, err)
}

func TestStaffCreateAndErrors(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. GET /staff/create (StaffCreatePage)
	req := httptest.NewRequest(http.MethodGet, "/staff/create", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.StaffCreatePage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. POST /staff/create (StaffCreate)
	createForm := url.Values{
		"name":     {"Dr. Novo Medico"},
		"email":    {"dr.novo@example.org"},
		"password": {"SenhaForte123!"},
	}
	req = httptest.NewRequest(http.MethodPost, "/staff/create", strings.NewReader(createForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.StaffCreate(c)
	require.NoError(t, err)

	// Validation error on StaffCreate
	badForm := url.Values{"name": {""}}
	req = httptest.NewRequest(http.MethodPost, "/staff/create", strings.NewReader(badForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.StaffCreate(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRoleAndSpecialtiesHandlers(t *testing.T) {
	env := setupUserHttpEnv(t)

	// 1. RolesPage
	req := httptest.NewRequest(http.MethodGet, "/admin/roles", nil)
	req.AddCookie(env.adminCookie)
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	err := env.handler.RolesPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. RoleCreate
	roleForm := url.Values{
		"name":     {"Enfermeiro"},
		"clinical": {"true"},
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/roles", strings.NewReader(roleForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.RoleCreate(c)
	require.NoError(t, err)

	// 3. SpecialtiesPage
	req = httptest.NewRequest(http.MethodGet, "/admin/specialties", nil)
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.SpecialtiesPage(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 4. SpecialtyCreate
	specForm := url.Values{"name": {"Ortopedia"}}
	req = httptest.NewRequest(http.MethodPost, "/admin/specialties", strings.NewReader(specForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(env.adminCookie)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	err = env.handler.SpecialtyCreate(c)
	require.NoError(t, err)
}

func TestStaffHTMXAndErrorBranches(t *testing.T) {
	env := setupUserHttpEnv(t)
	ctx := clinicctx.WithTestClinic(context.Background())

	physID := "01990000-0000-7000-8000-00000000000c"
	require.NoError(t, testutil.User(ctx, env.client, physID, "dr.htmx@example.org", "physician", "PhysPass123!"))
	r, err := env.client.Role.Query().Where(role.NameEQ("physician")).Only(ctx)
	require.NoError(t, err)
	_, err = env.client.Role.UpdateOneID(r.ID).SetIsClinical(true).Save(ctx)
	require.NoError(t, err)

	// 1. StaffPage HTMX fragment
	req := httptest.NewRequest(http.MethodGet, "/staff?page=1", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := env.newContext(req, rec)
	require.NoError(t, env.handler.StaffPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 2. StaffRequestsPage HTMX fragment
	req = httptest.NewRequest(http.MethodGet, "/staff/requests?page=1&status=pending", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	require.NoError(t, env.handler.StaffRequestsPage(c))
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. StaffEditPage for non-existent user returns 404
	req = httptest.NewRequest(http.MethodGet, "/staff/01990000-0000-7000-8000-000000000099/edit", nil)
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("01990000-0000-7000-8000-000000000099")
	assert.Error(t, env.handler.StaffEditPage(c))

	// 4. StaffUpdate validation error
	badUpdateForm := url.Values{"name": {""}, "email": {"invalid"}}
	req = httptest.NewRequest(http.MethodPost, "/staff/"+physID, strings.NewReader(badUpdateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(physID)
	require.NoError(t, env.handler.StaffUpdate(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 5. StaffRequestChange validation error
	req = httptest.NewRequest(http.MethodPost, "/staff/"+physID+"/request", strings.NewReader(badUpdateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	c = env.newContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(physID)
	require.NoError(t, env.handler.StaffRequestChange(c))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
