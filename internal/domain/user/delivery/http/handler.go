// Package http exposes the authentication and admin web routes.
package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/ui/components"
	"librevita.org/internal/domain/clinic"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/domain/user/delivery/views"
	"librevita.org/internal/domain/user/repository"
	"librevita.org/internal/domain/user/usecase"
)

// Handler renders the auth pages and processes form submissions.
type Handler struct {
	svc      *usecase.Service
	patients *patientusecase.Service
	csrf     *auth.CSRF
	sessions *auth.SessionManager
	policies *policy.PolicyEngine
	audit    *audit.Logger
	clocks   *clinic.ClockProvider
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, patients *patientusecase.Service,
	csrf *auth.CSRF, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, clocks *clinic.ClockProvider) *Handler {
	return &Handler{svc: svc, patients: patients, csrf: csrf, sessions: sessions, policies: policies, audit: auditLogger, clocks: clocks}
}

// SetupGate redirects navigation to /setup while the system is not yet
// onboarded. The setup routes themselves are exempt.
func (h *Handler) SetupGate() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() == "/setup" {
				return next(c)
			}
			onboarded, err := h.svc.IsOnboarded(c.Request().Context())
			if err != nil {
				return err
			}
			if !onboarded {
				return server.HtmxRedirect(c, "/setup")
			}
			return next(c)
		}
	}
}

// SetupPage renders the onboarding form. Onboarded systems redirect to the
// login page.
func (h *Handler) SetupPage(c echo.Context) error {
	onboarded, err := h.svc.IsOnboarded(c.Request().Context())
	if err != nil {
		return err
	}
	if onboarded {
		return c.Redirect(http.StatusFound, "/auth/login")
	}
	return server.Render(c, http.StatusOK, views.Setup(server.CSRFToken(c, h.csrf), ""))
}

// Setup creates the initial admin account and the clinic profile, then
// starts a session.
func (h *Handler) Setup(c echo.Context) error {
	onboarded, err := h.svc.IsOnboarded(c.Request().Context())
	if err != nil {
		return err
	}
	if onboarded {
		return c.Redirect(http.StatusFound, "/auth/login")
	}

	_, token, err := h.svc.Onboard(c.Request().Context(),
		usecase.RegisterInput{
			Name:     c.FormValue("admin_name"),
			Email:    c.FormValue("admin_email"),
			Password: c.FormValue("admin_password"),
		},
		usecase.ClinicInput{
			Name:       c.FormValue("clinic_name"),
			TaxID:      c.FormValue("clinic_tax_id"),
			Phone:      c.FormValue("clinic_phone"),
			Email:      c.FormValue("clinic_email"),
			Street:     c.FormValue("clinic_street"),
			City:       c.FormValue("clinic_city"),
			State:      c.FormValue("clinic_state"),
			PostalCode: c.FormValue("clinic_postal_code"),
			Country:    c.FormValue("clinic_country"),
			Timezone:   c.FormValue("clinic_timezone"),
		})
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusBadRequest, views.Setup(server.CSRFToken(c, h.csrf), v.Msg))
		case errors.Is(err, usecase.ErrAlreadyOnboarded):
			return c.Redirect(http.StatusFound, "/auth/login")
		default:
			return err
		}
	}

	c.SetCookie(h.sessions.Cookie(token))
	return c.Redirect(http.StatusFound, "/")
}

// LoginPage renders the sign-in form.
func (h *Handler) LoginPage(c echo.Context) error {
	next := server.ValidNext(c.QueryParam("next"))
	return server.Render(c, http.StatusOK, views.Login(server.CSRFToken(c, h.csrf), next, ""))
}

// Login validates credentials and starts a session.
func (h *Handler) Login(c echo.Context) error {
	_, token, err := h.svc.Login(c.Request().Context(), usecase.Credentials{
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
	})
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			return server.Render(c, http.StatusUnauthorized,
				views.Login(server.CSRFToken(c, h.csrf), server.ValidNext(c.FormValue("next")), "Invalid email or password"))
		}
		return err
	}

	c.SetCookie(h.sessions.Cookie(token))
	next := server.ValidNext(c.FormValue("next"))
	if next == "" {
		next = "/"
	}
	return c.Redirect(http.StatusFound, next)
}

// RegisterPage renders the account creation form.
func (h *Handler) RegisterPage(c echo.Context) error {
	return server.Render(c, http.StatusOK, views.Register(server.CSRFToken(c, h.csrf), ""))
}

// Register creates the account and starts a session.
func (h *Handler) Register(c echo.Context) error {
	_, token, err := h.svc.Register(c.Request().Context(), usecase.RegisterInput{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
	})
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusBadRequest, views.Register(server.CSRFToken(c, h.csrf), v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return server.Render(c, http.StatusConflict, views.Register(server.CSRFToken(c, h.csrf), "That email is already registered"))
		default:
			return err
		}
	}

	c.SetCookie(h.sessions.Cookie(token))
	return c.Redirect(http.StatusFound, "/")
}

// Logout destroys the session and redirects to the login page. A failed
// revocation is returned as an error so the client knows the session may
// still be valid.
func (h *Handler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err != nil {
		c.SetCookie(h.sessions.ClearCookie())
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	if err := h.svc.Logout(c.Request().Context(), cookie.Value); err != nil {
		return err
	}
	c.SetCookie(h.sessions.ClearCookie())
	return c.Redirect(http.StatusFound, server.LoginPath)
}

// Home renders the authenticated dashboard with real counters, recent
// activity, latest users, and the clinic profile.
func (h *Handler) Home(c echo.Context) error {
	ctx := c.Request().Context()

	clinicRow, err := h.clocks.Profile(ctx)
	if err != nil {
		return err
	}
	patients, err := h.patients.Count(ctx, clinicRow.ID)
	if err != nil {
		return err
	}
	total, err := h.svc.UserCount(ctx)
	if err != nil {
		return err
	}
	staff, err := h.svc.CountStaff(ctx)
	if err != nil {
		return err
	}
	users, err := h.svc.ListRecentUsers(ctx, 8)
	if err != nil {
		return err
	}
	activity, err := h.audit.Recent(ctx, 8, 0)
	if err != nil {
		return err
	}
	policies, err := h.policies.List(ctx)
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}

	stats := views.DashboardStats{
		Patients: patients,
		Staff:    staff,
		Users:    total,
		Policies: int64(len(policies)),
		Clinic: views.ClinicInfo{
			Name:     clinicRow.Name,
			Timezone: clinicRow.Timezone,
			City:     orEmpty(clinicRow.City),
			State:    orEmpty(clinicRow.State),
			TaxID:    orEmpty(clinicRow.TaxID),
			Phone:    orEmpty(clinicRow.Phone),
			Email:    orEmpty(clinicRow.Email),
		},
	}
	for _, u := range users {
		stats.LatestUsers = append(stats.LatestUsers, views.UserRow{
			Name:   u.DisplayName,
			Email:  u.Email,
			Role:   u.RoleName,
			Joined: clock.FormatStored(u.CreatedAt),
		})
	}
	for _, pl := range policies {
		if len(stats.LatestPolicies) >= 5 {
			break
		}
		stats.LatestPolicies = append(stats.LatestPolicies, views.PolicyRow{
			Name: pl.Name, Expression: pl.Expression,
		})
	}
	stats.Activity = h.activityRows(ctx, activity, clock)
	if len(stats.Activity) == 8 {
		stats.ActivityCursor = stats.Activity[len(stats.Activity)-1].ID
	}

	return server.Render(c, http.StatusOK, views.Home(server.CSRFToken(c, h.csrf), server.Principal(c), stats))
}

// HomeActivity serves the dashboard activity feed fragment: the timeline
// refreshes itself every minute, and ?before= is the id cursor for the
// Load more button.
func (h *Handler) HomeActivity(c echo.Context) error {
	ctx := c.Request().Context()
	before, _ := strconv.ParseInt(c.QueryParam("before"), 10, 64)

	limit := 8
	if before > 0 {
		limit = 12
	}
	activity, err := h.audit.Recent(ctx, limit, before)
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	rows := h.activityRows(ctx, activity, clock)

	cursor := int64(0)
	if before > 0 {
		if len(rows) == 12 {
			cursor = rows[len(rows)-1].ID
		}
	} else if len(rows) == 8 {
		cursor = rows[len(rows)-1].ID
	}
	return server.Render(c, http.StatusOK, views.ActivityFeed(rows, cursor))
}

func (h *Handler) activityRows(ctx context.Context, activity []audit.EventRow, clock *clinic.Clock) []views.ActivityRow {
	out := make([]views.ActivityRow, 0, len(activity))
	for _, ev := range activity {
		row := views.ActivityRow{
			ID:       ev.ID,
			When:     clock.FormatStored(ev.CreatedAt),
			Action:   ev.Action,
			Resource: ev.Resource,
			Result:   ev.Result,
		}
		if ev.ActorEmail != nil {
			row.Actor = *ev.ActorEmail
		}
		if ev.Detail != nil {
			row.Detail = *ev.Detail
		}
		out = append(out, row)
	}
	return out
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ProfilePage renders the signed-in user's profile with the color scheme
// preference.
func (h *Handler) ProfilePage(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	user, err := h.svc.UserByID(ctx, p.ID)
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.Profile(server.CSRFToken(c, h.csrf), p, clock.FormatStored(user.CreatedAt)))
}

// Admin renders the admin-only area.
// AdminPoliciesPage lists the dynamic CEL policies for editing, each with
// its recent change history.
func (h *Handler) AdminPoliciesPage(c echo.Context) error {
	return h.policiesPage(c, "")
}

// AdminPolicySave validates and persists a policy expression. Invalid
// expressions are rejected and the previous policy stays active.
func (h *Handler) AdminPolicySave(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	expression := c.FormValue("expression")

	actor := policy.Actor{}
	if p := server.Principal(c); p != nil {
		actor = policy.Actor{ID: p.ID, Email: p.Email}
	}

	err := h.policies.Set(c.Request().Context(), name, expression, actor)
	result := audit.ResultSuccess
	detail := ""
	if err != nil {
		result = audit.ResultFailure
		detail = err.Error()
	}

	h.audit.Record(c.Request().Context(), audit.Event{
		ActorID: actor.ID, ActorMail: actor.Email,
		Action: "policy.update", Resource: "policy:" + name,
		Result: result, Detail: detail,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})

	if server.IsHtmx(c) {
		return h.policyCardFragment(c, name, detail)
	}
	return h.policiesPage(c, "")
}

// AdminPolicyReset restores the default expression of a policy. htmx
// requests receive the refreshed card; others navigate back to the list.
func (h *Handler) AdminPolicyReset(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	expression, ok := policy.DefaultPolicies[name]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "unknown policy")
	}
	actor := policy.Actor{}
	if p := server.Principal(c); p != nil {
		actor = policy.Actor{ID: p.ID, Email: p.Email}
	}
	err := h.policies.Set(c.Request().Context(), name, expression, actor)
	h.audit.Record(c.Request().Context(), audit.Event{
		ActorID: actor.ID, ActorMail: actor.Email,
		Action: "policy.update", Resource: "policy:" + name,
		Result: audit.ResultSuccess, Detail: "reset to default",
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	if err != nil {
		return err
	}
	if server.IsHtmx(c) {
		return h.policyCardFragment(c, name, "")
	}
	return h.policiesPage(c, "")
}

// policyCardFragment renders a single policy card, the swap target of
// the inline save and reset forms.
func (h *Handler) policyCardFragment(c echo.Context, name, errMsg string) error {
	viewsList, err := h.policyViews(c, c.Request().Context())
	if err != nil {
		return err
	}
	for _, pv := range viewsList {
		if pv.Name == name {
			return server.Render(c, http.StatusOK, views.PolicyCard(server.CSRFToken(c, h.csrf), pv, errMsg))
		}
	}
	return echo.NewHTTPError(http.StatusNotFound, "unknown policy")
}

// policiesPage renders the full policies page.
func (h *Handler) policiesPage(c echo.Context, errMsg string) error {
	viewsList, err := h.policyViews(c, c.Request().Context())
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.AdminPolicies(server.CSRFToken(c, h.csrf), server.Principal(c), viewsList, errMsg))
}

// policyViews decorates the stored policies with their change history,
// rendering timestamps in the clinic's timezone.
func (h *Handler) policyViews(c echo.Context, ctx context.Context) ([]views.PolicyView, error) {
	policies, err := h.policies.List(ctx)
	if err != nil {
		return nil, err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]views.PolicyView, 0, len(policies))
	for _, p := range policies {
		history, err := h.policies.History(ctx, p.Name, 5)
		if err != nil {
			return nil, err
		}
		rows := make([]views.PolicyHistoryRow, 0, len(history))
		for _, v := range history {
			rows = append(rows, views.PolicyHistoryRow{
				Expression:     v.Expression,
				ChangedBy:      v.ChangedBy,
				ChangedByEmail: v.ChangedByEmail,
				Origin:         v.Origin,
				When:           clock.FormatStored(v.CreatedAt),
			})
		}
		out = append(out, views.PolicyView{Name: p.Name, Expression: p.Expression, History: rows})
	}
	return out, nil
}

// User management (users.manage policy).

const userListLimit = 20

// UsersPage lists one page of accounts with an optional name/email
// search, flowbite-style: breadcrumb, toolbar, and paged table.
func (h *Handler) UsersPage(c echo.Context) error {
	ctx := c.Request().Context()
	q := strings.TrimSpace(c.QueryParam("q"))
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	rows, total, err := h.svc.ListUsersPage(ctx, q, userListLimit, (page-1)*userListLimit)
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	// The search input and the pager request fragments; boosted
	// navigation (sidebar links) also arrives with HX-Request but must
	// render the full page.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.UsersListTable(
			h.userRows(rows, clock), q, page, total, ""))
	}
	return server.Render(c, http.StatusOK, views.UsersListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, page, total,
		h.userRows(rows, clock), ""))
}

func createFormValues(in usecase.CreateUserInput) views.UserFormValues {
	return views.UserFormValues{Name: in.Name, Email: in.Email, Password: in.Password, Role: in.Role}
}

// userRows renders the list rows with the clinic timezone.
func (h *Handler) userRows(rows []repository.ListUsersRow, clock *clinic.Clock) []views.UserListRow {
	out := make([]views.UserListRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, views.UserListRow{
			ID: r.ID, Name: r.DisplayName, Email: r.Email, Role: r.RoleName,
			Active: r.Active, CreatedAt: clock.FormatStored(r.CreatedAt),
		})
	}
	return out
}

// UserNewPage renders the staff account creation form.
func (h *Handler) UserNewPage(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	specialties, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return err
	}
	roles, err := h.formRoles(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.UserFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), "", views.UserFormValues{},
		roles, h.specialtyViews(specialties, nil), ""))
}

// UserCreate creates a staff account.
func (h *Handler) UserCreate(c echo.Context) error {
	ctx := c.Request().Context()
	in := usecase.CreateUserInput{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
		Role:     c.FormValue("role"),
	}
	user, err := h.svc.CreateUser(ctx, in)
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), "", createFormValues(in), nil, nil, v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return server.Render(c, http.StatusConflict, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), "", createFormValues(in), nil, nil, "That email is already registered"))
		default:
			return err
		}
	}
	if err := h.setFormSpecialties(c, user.ID); err != nil {
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "user.create", Resource: "user:" + user.ID,
		Result: audit.ResultSuccess, Detail: "role: " + in.Role,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/users")
}

// UserEditPage renders the account edit form with the assigned
// specialties pre-selected.
func (h *Handler) UserEditPage(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := h.svc.GetUser(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	specialties, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return err
	}
	selected, err := h.svc.UserSpecialties(ctx, user.ID)
	if err != nil {
		return err
	}
	roles, err := h.formRoles(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.UserFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), user.ID, views.UserFormValues{
			Name: user.DisplayName, Email: user.Email, Role: user.RoleName, Active: user.Active == 1,
		}, roles, h.specialtyViews(specialties, selected), ""))
}

// UserUpdate applies the account changes.
func (h *Handler) UserUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	in := usecase.UpdateUserInput{
		Name:   c.FormValue("name"),
		Email:  c.FormValue("email"),
		Role:   c.FormValue("role"),
		Active: c.FormValue("active") == "on",
	}
	before, err := h.svc.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	user, err := h.svc.UpdateUser(ctx, id, server.ActorID(c), in)
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, nil, v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return server.Render(c, http.StatusConflict, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, nil, "That email is already registered"))
		case errors.Is(err, usecase.ErrCannotDemoteSelf):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, nil, "You cannot change your own role or status"))
		case errors.Is(err, usecase.ErrLastActiveAdmin):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, nil, "The system needs at least one active admin"))
		default:
			return err
		}
	}
	if err := h.setFormSpecialties(c, id); err != nil {
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "user.update", Resource: "user:" + id,
		Result: audit.ResultSuccess, Detail: h.userChanges(before, user, in),
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/users")
}

// userChanges renders the changed fields for the audit detail. The
// before row carries the role name; the submitted input carries the new
// role name.
func (h *Handler) userChanges(before *repository.GetUserByIDRow, after *repository.User, in usecase.UpdateUserInput) string {
	parts := make([]string, 0, 4)
	if before.DisplayName != after.DisplayName {
		parts = append(parts, "name: "+before.DisplayName+" -> "+after.DisplayName)
	}
	if before.Email != after.Email {
		parts = append(parts, "email: "+before.Email+" -> "+after.Email)
	}
	if before.RoleName != in.Role {
		parts = append(parts, "role: "+before.RoleName+" -> "+in.Role)
	}
	if before.Active != after.Active {
		if after.Active == 1 {
			parts = append(parts, "status: inactive -> active")
		} else {
			parts = append(parts, "status: active -> inactive")
		}
	}
	return strings.Join(parts, ", ")
}

func updateFormValues(in usecase.UpdateUserInput) views.UserFormValues {
	return views.UserFormValues{Name: in.Name, Email: in.Email, Role: in.Role, Active: in.Active}
}

// UserStatus toggles an account between active and inactive from the
// list row. The response is the refreshed row fragment, or an OOB alert
// when the anti-lockout rules refuse the change.
func (h *Handler) UserStatus(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	user, err := h.svc.GetUser(ctx, id)
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	updated, err := h.svc.UpdateUser(ctx, id, server.ActorID(c), usecase.UpdateUserInput{
		Name:   user.DisplayName,
		Email:  user.Email,
		Role:   user.RoleName,
		Active: user.Active != 1,
	})
	if err != nil {
		msg := "Could not update the user"
		switch {
		case errors.Is(err, usecase.ErrCannotDemoteSelf):
			msg = "You cannot change your own status"
		case errors.Is(err, usecase.ErrLastActiveAdmin):
			msg = "The system needs at least one active admin"
		}
		h.audit.Record(ctx, audit.Event{
			ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
			Action: "user.update", Resource: "user:" + id, Result: audit.ResultFailure,
			Detail: msg,
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
		// htmx does not swap 4xx responses, so errors return 200 with an
		// OOB alert that lands in the shell's #app-alert container.
		return server.Render(c, http.StatusOK, components.Alert(msg, true))
	}
	roleName := user.RoleName
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "user.update", Resource: "user:" + id,
		Result: audit.ResultSuccess, Detail: h.userChanges(user, updated, usecase.UpdateUserInput{Active: updated.Active == 1, Role: roleName}),
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.Render(c, http.StatusOK, views.UserRowOnly([]views.UserListRow{{
		ID: updated.ID, Name: updated.DisplayName, Email: updated.Email,
		Role: roleName, Active: updated.Active, CreatedAt: clock.FormatStored(user.CreatedAt),
	}}))
}

// setFormSpecialties applies the submitted specialty checkboxes to the
// account.
func (h *Handler) setFormSpecialties(c echo.Context, userID string) error {
	ids := c.Request().PostForm["specialties"]
	return h.svc.SetUserSpecialties(c.Request().Context(), userID, ids)
}

// specialtyViews joins the catalog with the user's selection.
func (h *Handler) specialtyViews(all, selected []repository.Specialty) []views.SpecialtyView {
	sel := make(map[string]bool, len(selected))
	for _, sp := range selected {
		sel[sp.ID] = true
	}
	out := make([]views.SpecialtyView, 0, len(all))
	for _, sp := range all {
		out = append(out, views.SpecialtyView{ID: sp.ID, Name: sp.Name, Selected: sel[sp.ID]})
	}
	return out
}

// SpecialtiesPage lists the clinic's specialty catalog.
func (h *Handler) SpecialtiesPage(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	rows, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.SpecialtiesPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), h.specialtyRows(rows), ""))
}

func (h *Handler) specialtyRows(rows []repository.Specialty) []views.SpecialtyRow {
	out := make([]views.SpecialtyRow, 0, len(rows))
	for _, sp := range rows {
		out = append(out, views.SpecialtyRow{ID: sp.ID, Name: sp.Name})
	}
	return out
}

// SpecialtyCreate adds a specialty to the clinic catalog.
func (h *Handler) SpecialtyCreate(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	specialty, err := h.svc.CreateSpecialty(ctx, clinicID, c.FormValue("name"))
	if err != nil {
		msg := "Could not create the specialty"
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			msg = v.Msg
		case errors.Is(err, usecase.ErrDuplicateSpecialty):
			msg = "A specialty with this name already exists"
		}
		rows, lerr := h.svc.ListSpecialties(ctx, clinicID)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.SpecialtiesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.specialtyRows(rows), msg))
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "specialty.create", Resource: "specialty:" + specialty.ID,
		Result: audit.ResultSuccess, Detail: specialty.Name,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/specialties")
}

// SpecialtyDelete removes a specialty from the catalog.
func (h *Handler) SpecialtyDelete(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	id := c.Param("id")
	if err := h.svc.DeleteSpecialty(ctx, clinicID, id); err != nil {
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "specialty.delete", Resource: "specialty:" + id,
		Result: audit.ResultSuccess,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/specialties")
}

// RolesPage lists the role catalog for the administrator.
func (h *Handler) RolesPage(c echo.Context) error {
	ctx := c.Request().Context()
	rows, err := h.svc.ListRoles(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.RolesPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows), ""))
}

func (h *Handler) roleViews(rows []repository.Role) []views.RoleView {
	out := make([]views.RoleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, views.RoleView{ID: r.ID, Name: r.Name, System: r.System == 1, IsClinical: r.IsClinical == 1})
	}
	return out
}

// RoleCreate adds a role to the catalog.
func (h *Handler) RoleCreate(c echo.Context) error {
	ctx := c.Request().Context()
	role, err := h.svc.CreateRole(ctx, c.FormValue("name"), c.FormValue("clinical") == "on")
	if err != nil {
		msg := "Could not create the role"
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			msg = v.Msg
		case errors.Is(err, usecase.ErrDuplicateRole):
			msg = "A role with this name already exists"
		}
		rows, lerr := h.svc.ListRoles(ctx)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.RolesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows), msg))
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "role.create", Resource: "role:" + role.ID,
		Result: audit.ResultSuccess, Detail: role.Name,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/roles")
}

// RoleRename renames a non-system role, refusing names still referenced
// by active CEL policies so a policy never silently stops matching.
func (h *Handler) RoleRename(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	current, err := h.svc.RoleByID(ctx, id)
	if err != nil {
		return err
	}
	if policies := h.policiesReferencing(ctx, current.Name); len(policies) > 0 {
		rows, lerr := h.svc.ListRoles(ctx)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.RolesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows),
			"This role is referenced by active policies ("+strings.Join(policies, ", ")+"). Update them first."))
	}
	role, err := h.svc.RenameRole(ctx, id, c.FormValue("name"))
	if err != nil {
		msg := "Could not rename the role"
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			msg = v.Msg
		case errors.Is(err, usecase.ErrDuplicateRole):
			msg = "A role with this name already exists"
		case errors.Is(err, usecase.ErrSystemRole):
			msg = "System roles cannot be renamed"
		}

		rows, lerr := h.svc.ListRoles(ctx)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.RolesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows), msg))
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "role.rename", Resource: "role:" + id,
		Result: audit.ResultSuccess, Detail: role.Name,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/roles")
}

// RoleClinical toggles a non-system role's clinical flag.
func (h *Handler) RoleClinical(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	err := h.svc.SetRoleClinical(ctx, id, c.FormValue("clinical") == "on")
	if err != nil {
		msg := "Could not update the role"
		if errors.Is(err, usecase.ErrSystemRole) {
			msg = "System roles cannot be changed"
		}
		rows, lerr := h.svc.ListRoles(ctx)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.RolesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows), msg))
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "role.update", Resource: "role:" + id,
		Result: audit.ResultSuccess, Detail: "clinical flag changed",
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/roles")
}

// RoleDelete removes a non-system role that no account uses.
func (h *Handler) RoleDelete(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	err := h.svc.DeleteRole(ctx, id)
	if err != nil {
		msg := "Could not delete the role"
		switch {
		case errors.Is(err, usecase.ErrSystemRole):
			msg = "System roles cannot be deleted"
		case errors.Is(err, usecase.ErrRoleInUse):
			msg = "This role is assigned to accounts and cannot be deleted"
		}
		rows, lerr := h.svc.ListRoles(ctx)
		if lerr != nil {
			return lerr
		}
		return server.Render(c, http.StatusBadRequest, views.RolesPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), h.roleViews(rows), msg))
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "role.delete", Resource: "role:" + id,
		Result: audit.ResultSuccess,
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/roles")
}


// formRoles loads the role catalog for the user form select.
func (h *Handler) formRoles(ctx context.Context) ([]views.RoleView, error) {
	rows, err := h.svc.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	return h.roleViews(rows), nil
}


// policiesReferencing returns the policy names whose expressions mention
// the role name.
func (h *Handler) policiesReferencing(ctx context.Context, roleName string) []string {
	rows, err := h.policies.List(ctx)
	if err != nil {
		return nil
	}
	needle := "'" + roleName + "'"
	var out []string
	for _, r := range rows {
		if strings.Contains(r.Expression, needle) {
			out = append(out, r.Name)
		}
	}
	return out
}
