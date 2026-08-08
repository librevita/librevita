package server

import (
	"net/http"
	"strings"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// SetupGate is the middleware factory that redirects navigation to /setup
// while the system is not onboarded. The user module provides it; other
// modules consume it so every route shares the gate.
type SetupGate func() echo.MiddlewareFunc

// IsHtmx reports whether the request was issued by htmx (HX-Request
// header). Such requests expect fragments, not full documents: page
// routes render only their fragment, and redirects become HX-Redirect.
func IsHtmx(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// HtmxRedirect responds to a navigation request. htmx requests receive
// the HX-Redirect header and no body; regular requests get a 302. Use it
// where a fragment submission must move the browser (login expiry,
// post-create navigation).
func HtmxRedirect(c echo.Context, path string) error {
	if IsHtmx(c) {
		c.Response().Header().Set("HX-Redirect", path)
		c.Response().Status = http.StatusOK
		return nil
	}
	return c.Redirect(http.StatusFound, path)
}

// Render writes a templ component with the given status.
func Render(c echo.Context, status int, comp templ.Component) error {
	c.Response().Status = status
	return comp.Render(c.Request().Context(), c.Response())
}

// ActorID returns the signed-in user's id, or "" when absent.
func ActorID(c echo.Context) string {
	if p := Principal(c); p != nil {
		return p.ID
	}
	return ""
}

// ActorMail returns the signed-in user's email, or "" when absent.
func ActorMail(c echo.Context) string {
	if p := Principal(c); p != nil {
		return p.Email
	}
	return ""
}

// ValidNext accepts only same-site absolute paths, rejecting open
// redirects (protocol-relative and external URLs).
func ValidNext(s string) string {
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.Contains(s, "://") {
		return ""
	}
	return s
}
