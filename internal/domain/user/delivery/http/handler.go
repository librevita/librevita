// Package http exposes the authentication and admin web routes.
package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/policy/repository"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/user/delivery/views"
	"librevita.org/internal/domain/user/usecase"
)

// utcMilliLayout matches the timestamps written by the database
// (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')).
const utcMilliLayout = "2006-01-02T15:04:05.000Z"

// Handler renders the auth pages and processes form submissions.
type Handler struct {
	svc      *usecase.Service
	csrf     *auth.CSRF
	sessions *auth.SessionManager
	policies *policy.PolicyEngine
	audit    *audit.Logger
	clocks   *clinic.ClockProvider
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, csrf *auth.CSRF, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, clocks *clinic.ClockProvider) *Handler {
	return &Handler{svc: svc, csrf: csrf, sessions: sessions, policies: policies, audit: auditLogger, clocks: clocks}
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
				return c.Redirect(http.StatusFound, "/setup")
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
	return render(c, http.StatusOK, views.Setup(server.CSRFToken(c, h.csrf), ""))
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
			return render(c, http.StatusBadRequest, views.Setup(server.CSRFToken(c, h.csrf), v.Msg))
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
	return render(c, http.StatusOK, views.Login(server.CSRFToken(c, h.csrf), ""))
}

// Login validates credentials and starts a session.
func (h *Handler) Login(c echo.Context) error {
	_, token, err := h.svc.Login(c.Request().Context(), usecase.Credentials{
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
	})
	if err != nil {
		if errors.Is(err, usecase.ErrInvalidCredentials) {
			return render(c, http.StatusUnauthorized,
				views.Login(server.CSRFToken(c, h.csrf), "Invalid email or password"))
		}
		return err
	}

	c.SetCookie(h.sessions.Cookie(token))
	return c.Redirect(http.StatusFound, "/")
}

// RegisterPage renders the account creation form.
func (h *Handler) RegisterPage(c echo.Context) error {
	return render(c, http.StatusOK, views.Register(server.CSRFToken(c, h.csrf), ""))
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
			return render(c, http.StatusBadRequest, views.Register(server.CSRFToken(c, h.csrf), v.Msg))
		case errors.Is(err, usecase.ErrEmailTaken):
			return render(c, http.StatusConflict, views.Register(server.CSRFToken(c, h.csrf), "That email is already registered"))
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

	patients, err := h.svc.UserCountByRole(ctx, "patient")
	if err != nil {
		return err
	}
	total, err := h.svc.UserCount(ctx)
	if err != nil {
		return err
	}
	policiesCount, err := h.policies.Count(ctx)
	if err != nil {
		return err
	}
	users, err := h.svc.ListRecentUsers(ctx, 8)
	if err != nil {
		return err
	}
	activity, err := h.audit.Recent(ctx, 8)
	if err != nil {
		return err
	}
	policies, err := h.policies.List(ctx)
	if err != nil {
		return err
	}
	clinicRow, err := h.clocks.Profile(ctx)
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}

	stats := views.DashboardStats{
		Patients: patients,
		Staff:    total - patients,
		Users:    total,
		Policies: policiesCount,
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
			Role:   u.Role,
			Joined: formatWhen(clock, u.CreatedAt),
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
	for _, ev := range activity {
		row := views.ActivityRow{
			When:     formatWhen(clock, ev.CreatedAt),
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
		stats.Activity = append(stats.Activity, row)
	}

	return render(c, http.StatusOK, views.Home(server.CSRFToken(c, h.csrf), server.Principal(c), stats))
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
	return render(c, http.StatusOK, views.Profile(server.CSRFToken(c, h.csrf), p, formatWhen(clock, user.CreatedAt)))
}

// Admin renders the admin-only area.
func (h *Handler) Admin(c echo.Context) error {
	return render(c, http.StatusOK, views.Admin(server.CSRFToken(c, h.csrf), server.Principal(c)))
}

// AdminPoliciesPage lists the dynamic CEL policies for editing, each with
// its recent change history.
func (h *Handler) AdminPoliciesPage(c echo.Context) error {
	policies, err := h.policies.List(c.Request().Context())
	if err != nil {
		return err
	}
	viewsList, err := h.policyViews(c, policies)
	if err != nil {
		return err
	}
	return render(c, http.StatusOK, views.AdminPolicies(server.CSRFToken(c, h.csrf), server.Principal(c), viewsList, ""))
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

	policies, listErr := h.policies.List(c.Request().Context())
	if listErr != nil {
		return listErr
	}
	viewsList, listErr := h.policyViews(c, policies)
	if listErr != nil {
		return listErr
	}
	return render(c, http.StatusOK, views.AdminPolicies(server.CSRFToken(c, h.csrf), server.Principal(c), viewsList, detail))
}

// policyViews decorates the stored policies with their change history,
// rendering timestamps in the clinic's timezone.
func (h *Handler) policyViews(c echo.Context, policies []repository.ListPoliciesRow) ([]views.PolicyView, error) {
	clock, err := h.clocks.Clock(c.Request().Context())
	if err != nil {
		return nil, err
	}
	out := make([]views.PolicyView, 0, len(policies))
	for _, p := range policies {
		history, err := h.policies.History(c.Request().Context(), p.Name, 5)
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
				When:           formatWhen(clock, v.CreatedAt),
			})
		}
		out = append(out, views.PolicyView{Name: p.Name, Expression: p.Expression, History: rows})
	}
	return out, nil
}

// formatWhen converts a database timestamp into the clinic's timezone,
// keeping the raw value when it cannot be parsed.
func formatWhen(clock *clinic.Clock, stored string) string {
	t, err := time.Parse(utcMilliLayout, stored)
	if err != nil {
		return stored
	}
	return clock.FormatUI(t)
}

func render(c echo.Context, status int, comp templ.Component) error {
	c.Response().Status = status
	return comp.Render(c.Request().Context(), c.Response())
}
