// Package http exposes the authentication web routes.
package http

import (
	"errors"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/user/delivery/views"
	"librevita.org/internal/domain/user/usecase"
)

// Handler renders the auth pages and processes form submissions.
type Handler struct {
	svc      *usecase.Service
	csrf     *auth.CSRF
	sessions *auth.SessionManager
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, csrf *auth.CSRF, sessions *auth.SessionManager) *Handler {
	return &Handler{svc: svc, csrf: csrf, sessions: sessions}
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

// Logout destroys the session and redirects to the login page.
func (h *Handler) Logout(c echo.Context) error {
	cookie, err := c.Cookie(auth.SessionCookieName)
	if err == nil {
		_ = h.svc.Logout(c.Request().Context(), cookie.Value)
	}
	c.SetCookie(h.sessions.ClearCookie())
	return c.Redirect(http.StatusFound, server.LoginPath)
}

// Home renders the authenticated dashboard.
func (h *Handler) Home(c echo.Context) error {
	return render(c, http.StatusOK, views.Home(server.CSRFToken(c, h.csrf), server.Principal(c)))
}

// Admin renders the admin-only area.
func (h *Handler) Admin(c echo.Context) error {
	return render(c, http.StatusOK, views.Admin(server.CSRFToken(c, h.csrf), server.Principal(c)))
}

func render(c echo.Context, status int, comp templ.Component) error {
	c.Response().Status = status
	return comp.Render(c.Request().Context(), c.Response())
}
