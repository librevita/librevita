// Package http exposes the authentication and admin web routes.
package http

import (
	"context"
	"librevita.org/pkg/log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	clinicmodel "librevita.org/internal/domain/clinic/model"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/domain/user/delivery/views"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/ui/components"
	"librevita.org/pkg/ident"
)

// Handler renders the auth pages and processes form submissions.
type Handler struct {
	svc      *usecase.Service
	patients *patientusecase.Service
	platform *clinicusecase.PlatformService
	systems  identifiermodel.SystemRepository
	csrf     *auth.CSRF
	sessions *auth.SessionManager
	policies *policy.PolicyEngine
	audit    *audit.Logger
	clocks   *clinicusecase.ClockProvider
	files    *storage.FileManager
	cfg      *config.Config
	log      log.Logger
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, patients *patientusecase.Service,
	platform *clinicusecase.PlatformService, systems identifiermodel.SystemRepository,
	csrf *auth.CSRF, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, clocks *clinicusecase.ClockProvider,
	files *storage.FileManager, cfg *config.Config, logger log.Logger) *Handler {
	return &Handler{svc: svc, patients: patients, platform: platform, systems: systems,
		csrf: csrf, sessions: sessions, policies: policies, audit: auditLogger,
		clocks: clocks, files: files, cfg: cfg, log: logger}
}

// SetupGate redirects clinic hosts to /setup while onboarded_at is null.
// Apex platform routes are not gated.
func (h *Handler) SetupGate() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return h.gateSetup(c, next)
		}
	}
}

func (h *Handler) gateSetup(c echo.Context, next echo.HandlerFunc) error {
	ctx := c.Request().Context()
	if clinicctx.IsApex(ctx) {
		return h.gateApex(c, next)
	}
	return h.gateClinic(c, next)
}

func (h *Handler) gateApex(c echo.Context, next echo.HandlerFunc) error {
	if h.platform == nil {
		return next(c)
	}
	hasOps, err := h.platform.HasOperators(c.Request().Context())
	if err != nil {
		return err
	}
	if !hasOps {
		return server.HtmxRedirect(c, "/setup")
	}
	return next(c)
}

func (h *Handler) gateClinic(c echo.Context, next echo.HandlerFunc) error {
	path := c.Request().URL.Path
	if path == "/setup" || strings.HasPrefix(path, "/static/") || path == "/healthz" {
		return next(c)
	}
	clinic, ok := clinicctx.FromContext(c.Request().Context())
	if !ok {
		return next(c)
	}
	if clinic.OnboardedAt == nil {
		return server.HtmxRedirect(c, "/setup")
	}
	return next(c)
}

// SetupPage renders platform bootstrap on the apex, or clinic onboard on a subdomain.
func (h *Handler) SetupPage(c echo.Context) error {
	if clinicctx.IsApex(c.Request().Context()) {
		return h.platformSetupPage(c)
	}
	onboarded, err := h.svc.IsOnboarded(c.Request().Context())
	if err != nil {
		return err
	}
	if onboarded {
		return c.Redirect(http.StatusFound, "/auth/login")
	}
	systems, err := h.systems.ListAll(c.Request().Context())
	if err != nil {
		return err
	}
	clinic, _ := clinicctx.FromContext(c.Request().Context())
	name := ""
	if clinic != nil {
		name = clinic.Name
	}
	return server.Render(c, http.StatusOK, views.ClinicSetup(server.CSRFToken(c, h.csrf), name, systems, ""))
}

// Setup handles apex bootstrap or clinic onboard.
func (h *Handler) Setup(c echo.Context) error {
	if clinicctx.IsApex(c.Request().Context()) {
		return h.platformBootstrap(c)
	}
	onboarded, err := h.svc.IsOnboarded(c.Request().Context())
	if err != nil {
		return err
	}
	if onboarded {
		return c.Redirect(http.StatusFound, "/auth/login")
	}

	var systemIDs []ident.IdentifierSystemID
	if form, err := c.FormParams(); err == nil {
		for _, raw := range form["identifier_system_id"] {
			if sysID, err := ident.ParseIdentifierSystem(raw); err == nil {
				systemIDs = append(systemIDs, sysID)
			}
		}
	}

	_, token, err := h.svc.Onboard(c.Request().Context(),
		usecase.RegisterInput{
			Name:     c.FormValue("admin_name"),
			Email:    c.FormValue("admin_email"),
			Password: c.FormValue("admin_password"),
		}, systemIDs)
	if err != nil {
		var v *usecase.ValidationError
		systems, _ := h.systems.ListAll(c.Request().Context())
		clinic, _ := clinicctx.FromContext(c.Request().Context())
		name := ""
		if clinic != nil {
			name = clinic.Name
		}
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusBadRequest, views.ClinicSetup(server.CSRFToken(c, h.csrf), name, systems, v.Msg))
		case errors.Is(err, usecase.ErrAlreadyOnboarded):
			return c.Redirect(http.StatusFound, "/auth/login")
		default:
			return err
		}
	}

	c.SetCookie(h.sessions.Cookie(token))
	return c.Redirect(http.StatusFound, "/")
}

func (h *Handler) platformSetupPage(c echo.Context) error {
	if h.platform == nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	has, err := h.platform.HasOperators(c.Request().Context())
	if err != nil {
		return err
	}
	if has {
		return c.Redirect(http.StatusFound, "/auth/login")
	}
	return server.Render(c, http.StatusOK, views.PlatformBootstrap(server.CSRFToken(c, h.csrf), ""))
}

func (h *Handler) platformBootstrap(c echo.Context) error {
	if h.platform == nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	p, _, err := h.platform.Bootstrap(c.Request().Context(),
		c.FormValue("admin_name"),
		c.FormValue("admin_email"),
		c.FormValue("admin_password"),
	)
	if err != nil {
		if errors.Is(err, clinicusecase.ErrPlatformExists) {
			return c.Redirect(http.StatusFound, "/auth/login")
		}
		return server.Render(c, http.StatusBadRequest, views.PlatformBootstrap(server.CSRFToken(c, h.csrf), err.Error()))
	}
	token, err := h.sessions.Create(c.Request().Context(), *p)
	if err != nil {
		return err
	}
	c.SetCookie(h.sessions.Cookie(token))
	return c.Redirect(http.StatusFound, "/")
}

func (h *Handler) requireApexPlatform(c echo.Context) error {
	if !clinicctx.IsApex(c.Request().Context()) {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	p := server.Principal(c)
	if p == nil || !p.Platform {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	return nil
}

// ProvisionPage renders the apex form that creates a clinic shell.
func (h *Handler) ProvisionPage(c echo.Context) error {
	if err := h.requireApexPlatform(c); err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.Provision(server.CSRFToken(c, h.csrf), ""))
}

// Provision creates a clinic shell and its Clinic DEK.
func (h *Handler) Provision(c echo.Context) error {
	if err := h.requireApexPlatform(c); err != nil {
		return err
	}
	_, err := h.platform.Provision(c.Request().Context(), clinicusecase.ProvisionInput{
		Name:     c.FormValue("clinic_name"),
		Slug:     c.FormValue("clinic_slug"),
		TaxID:    c.FormValue("clinic_tax_id"),
		Phone:    c.FormValue("clinic_phone"),
		Email:    c.FormValue("clinic_email"),
		Street:   c.FormValue("clinic_street"),
		City:     c.FormValue("clinic_city"),
		State:    c.FormValue("clinic_state"),
		Postal:   c.FormValue("clinic_postal_code"),
		Timezone: c.FormValue("clinic_timezone"),
	})
	if err != nil {
		msg := err.Error()
		switch {
		case errors.Is(err, clinicusecase.ErrInvalidSlug):
			msg = "Enter a valid subdomain (letters, digits, hyphens)"
		case errors.Is(err, clinicusecase.ErrSlugTaken):
			msg = "That subdomain is already in use"
		}
		return server.Render(c, http.StatusBadRequest, views.Provision(server.CSRFToken(c, h.csrf), msg))
	}
	return c.Redirect(http.StatusFound, "/")
}

// LoginPage renders the sign-in form.
func (h *Handler) LoginPage(c echo.Context) error {
	next := server.ValidNext(c.QueryParam("next"))
	return server.Render(c, http.StatusOK, views.Login(server.CSRFToken(c, h.csrf), next, ""))
}

// Login validates credentials and starts a session.
func (h *Handler) Login(c echo.Context) error {
	ctx := c.Request().Context()
	if clinicctx.IsApex(ctx) {
		return h.platformLogin(c)
	}
	_, token, err := h.svc.Login(ctx, usecase.Credentials{
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

func (h *Handler) platformLogin(c echo.Context) error {
	p, err := h.platform.Login(c.Request().Context(), c.FormValue("email"), c.FormValue("password"))
	if err != nil {
		return server.Render(c, http.StatusUnauthorized,
			views.Login(server.CSRFToken(c, h.csrf), server.ValidNext(c.FormValue("next")), "Invalid email or password"))
	}
	token, err := h.sessions.Create(c.Request().Context(), *p)
	if err != nil {
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
		Name:      c.FormValue("name"),
		Email:     c.FormValue("email"),
		Password:  c.FormValue("password"),
		PatientID: c.FormValue("patient_id"),
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
	if p := server.Principal(c); p != nil && p.Platform {
		return h.platformHome(c)
	}

	clinicRow, err := h.clocks.Profile(ctx)
	if err != nil {
		return err
	}
	patients, err := h.patients.Count(ctx, clinicRow.ID.String())
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
	clock, err := h.userClock(ctx)
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
			City:     clinicRow.City,
			State:    clinicRow.State,
			TaxID:    clinicRow.TaxID,
			Phone:    clinicRow.Phone,
			Email:    clinicRow.Email,
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

func (h *Handler) platformHome(c echo.Context) error {
	clinics, err := h.platform.ListClinics(c.Request().Context())
	if err != nil {
		return err
	}
	base := ""
	if h.cfg != nil {
		base = h.cfg.BaseDomain
	}
	return server.Render(c, http.StatusOK, views.PlatformHome(server.CSRFToken(c, h.csrf), server.Principal(c), clinics, base))
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
	clock, err := h.userClock(ctx)
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

func (h *Handler) activityRows(ctx context.Context, activity []audit.EventRow, clock *clinicmodel.Clock) []views.ActivityRow {
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

// ProfilePage renders the signed-in user's profile with the color scheme
// preference.
// userClock resolves the display clock: the user's personal timezone
// when set, otherwise the clinic timezone.
func (h *Handler) userClock(ctx context.Context) (*clinicmodel.Clock, error) {
	tz := ""
	if p := server.PrincipalCtx(ctx); p != nil {
		tz = p.Timezone
	}
	return h.clocks.ClockFor(ctx, tz)
}

func (h *Handler) ProfilePage(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	return h.profilePage(ctx, c, p, http.StatusOK, "")
}

// profilePage renders the profile page with the given status and, on
// form failures, the error message explaining what went wrong — form
// submissions must get a screen back, never a bare error body.
func (h *Handler) profilePage(ctx context.Context, c echo.Context, p *auth.Principal, status int, errMsg string) error {
	user, err := h.svc.UserByID(ctx, p.ID)
	if err != nil {
		return err
	}
	clock, err := h.clocks.ClockFor(ctx, p.Timezone)
	if err != nil {
		return err
	}
	return server.Render(c, status, views.Profile(
		server.CSRFToken(c, h.csrf), p, clock.FormatStored(user.CreatedAt), errMsg))
}

// ProfileUpdate stores the user's UI theme and personal timezone.
func (h *Handler) ProfileUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	p := server.Principal(c)
	if p == nil {
		return c.Redirect(http.StatusFound, server.LoginPath)
	}
	theme := auth.UITheme(c.FormValue("ui_theme"))
	timezone := c.FormValue("timezone")
	if err := h.svc.UpdatePreferences(ctx, p.ID, timezone, theme); err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return server.Render(c, http.StatusBadRequest, views.Profile(
				server.CSRFToken(c, h.csrf), p, "", v.Msg))
		}
		return err
	}
	detail := "theme: " + theme.String()
	if timezone != "" {
		detail += ", timezone: " + timezone
	} else {
		detail += ", timezone: clinic default"
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"profile.update", "user:"+p.ID, "", detail))
	return server.HtmxRedirect(c, "/profile")
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
	result := audit.AuditResultSuccess
	detail := ""
	if err != nil {
		result = audit.AuditResultFailure
		detail = err.Error()
	}

	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, result,
		"policy.update", "policy:"+name, name, detail))

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
	if err != nil {
		h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultFailure,
			"policy.update", "policy:"+name, name, "reset to default: "+err.Error()))
		return err
	}
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		"policy.update", "policy:"+name, name, "reset to default"))
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
	clock, err := h.userClock(ctx)
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
	clock, err := h.userClock(ctx)
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
func (h *Handler) userRows(rows []usecase.ListUsersRow, clock *clinicmodel.Clock) []views.UserListRow {
	out := make([]views.UserListRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, views.UserListRow{
			ID: r.ID.String(), Name: r.DisplayName, Email: r.Email, Role: r.RoleName,
			Active: r.Active, CreatedAt: clock.FormatStored(r.CreatedAt),
		})
	}
	return out
}

// UserNewPage renders the staff account creation form.
func (h *Handler) UserNewPage(c echo.Context) error {
	ctx := c.Request().Context()
	roles, err := h.formRoles(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.UserFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), "", views.UserFormValues{},
		roles, ""))
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
				server.CSRFToken(c, h.csrf), server.Principal(c), "", createFormValues(in), nil, v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return server.Render(c, http.StatusConflict, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), "", createFormValues(in), nil, "That email is already registered"))
		default:
			return err
		}
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"user.create", "user:"+user.ID.String(), "", "role: "+in.Role))
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
	roles, err := h.formRoles(ctx)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.UserFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), user.ID.String(), views.UserFormValues{
			Name: user.DisplayName, Email: user.Email, Role: user.RoleName, Active: user.Active,
		}, roles, ""))
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
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return server.Render(c, http.StatusConflict, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, "That email is already registered"))
		case errors.Is(err, usecase.ErrCannotDemoteSelf):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, "You cannot change your own role or status"))
		case errors.Is(err, usecase.ErrLastActiveAdmin):
			return server.Render(c, http.StatusBadRequest, views.UserFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), id, updateFormValues(in), nil, "The system needs at least one active admin"))
		default:
			return err
		}
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"user.update", "user:"+id, "", h.userChanges(before, user, in)))
	return server.HtmxRedirect(c, "/users")
}

// userChanges renders the changed fields for the audit detail. The
// before row carries the role name; the submitted input carries the new
// role name.
func (h *Handler) userChanges(before *usecase.GetUserByIDRow, after *usecase.User, in usecase.UpdateUserInput) string {
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
		if after.Active {
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
	clock, err := h.userClock(ctx)
	if err != nil {
		return err
	}
	updated, err := h.svc.UpdateUser(ctx, id, server.ActorID(c), usecase.UpdateUserInput{
		Name:   user.DisplayName,
		Email:  user.Email,
		Role:   user.RoleName,
		Active: !user.Active,
	})
	if err != nil {
		msg := "Could not update the user"
		switch {
		case errors.Is(err, usecase.ErrCannotDemoteSelf):
			msg = "You cannot change your own status"
		case errors.Is(err, usecase.ErrLastActiveAdmin):
			msg = "The system needs at least one active admin"
		}
		h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultFailure,
			"user.update", "user:"+id, "", msg))
		// htmx does not swap 4xx responses, so errors return 200 with an
		// OOB alert that lands in the shell's #app-alert container.
		return server.Render(c, http.StatusOK, components.Alert(msg, true))
	}
	roleName := user.RoleName
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"user.update", "user:"+id, "", h.userChanges(user, updated, usecase.UpdateUserInput{Active: updated.Active, Role: roleName})))
	return server.Render(c, http.StatusOK, views.UserRowOnly([]views.UserListRow{{
		ID: updated.ID.String(), Name: updated.DisplayName, Email: updated.Email,
		Role: roleName, Active: updated.Active, CreatedAt: clock.FormatStored(user.CreatedAt),
	}}))
}

// specialtyViews joins the catalog with the user's selection.
func (h *Handler) specialtyViews(all, selected []usecase.Specialty) []views.SpecialtyView {
	sel := make(map[string]bool, len(selected))
	for _, sp := range selected {
		sel[sp.ID.String()] = true
	}
	out := make([]views.SpecialtyView, 0, len(all))
	for _, sp := range all {
		out = append(out, views.SpecialtyView{ID: sp.ID.String(), Name: sp.Name, Selected: sel[sp.ID.String()]})
	}
	return out
}

// SpecialtiesPage lists the clinic's specialty catalog, paginated like the other registries.
func (h *Handler) SpecialtiesPage(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	rows, total, err := h.svc.ListSpecialtiesPage(ctx, clinicID, specialtyPageSize, (page-1)*specialtyPageSize)
	if err != nil {
		return err
	}
	out := h.specialtyRows(rows)
	pager := views.SpecialtyPager{Page: page, Total: total, Shown: int64(len(out))}
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.SpecialtiesTable(
			server.CSRFToken(c, h.csrf), out, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.SpecialtiesPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), out, pager, ""))
}

const specialtyPageSize = 20

func (h *Handler) specialtyRows(rows []usecase.Specialty) []views.SpecialtyRow {
	out := make([]views.SpecialtyRow, 0, len(rows))
	for _, sp := range rows {
		out = append(out, views.SpecialtyRow{ID: sp.ID.String(), Name: sp.Name})
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
		// htmx does not swap 4xx responses, so errors return 200 and
		// re-render the form fragment inside the dialog.
		return server.Render(c, http.StatusOK, views.SpecialtyForm(
			server.CSRFToken(c, h.csrf), msg))
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"specialty.create", "specialty:"+specialty.ID.String(), specialty.Name, specialty.Name))
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
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"specialty.delete", "specialty:"+id, "", ""))
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

func (h *Handler) roleViews(rows []usecase.Role) []views.RoleView {
	out := make([]views.RoleView, 0, len(rows))
	for _, r := range rows {
		out = append(out, views.RoleView{ID: r.ID.String(), Name: r.Name, System: r.System, IsClinical: r.IsClinical})
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
		// htmx does not swap 4xx responses, so errors return 200 and
		// re-render the form fragment inside the dialog.
		return server.Render(c, http.StatusOK, views.RoleForm(
			server.CSRFToken(c, h.csrf), msg))
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"role.create", "role:"+role.ID.String(), role.Name, role.Name))
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
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"role.rename", "role:"+id, role.Name, role.Name))
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
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"role.update", "role:"+id, "", "clinical flag changed"))
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
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"role.delete", "role:"+id, "", ""))
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

// AuditIntegrity verifies the append-only audit trail hash chain and
// reports whether it is intact.
func (h *Handler) AuditIntegrity(c echo.Context) error {
	brokenAt, err := h.audit.VerifyChain(c.Request().Context())
	if err != nil {
		return err
	}
	if brokenAt != 0 {
		return c.JSON(http.StatusOK, map[string]any{"ok": false, "broken_at": brokenAt})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
