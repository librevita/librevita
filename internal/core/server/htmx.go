package server

import (
	"net/http"

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
