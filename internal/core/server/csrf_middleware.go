package server

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/auth"
)

const (
	csrfHeaderName = "X-CSRF-Token"
	csrfFormField  = "_csrf"
)

// CSRFMiddleware protects state-changing requests with the double-submit
// cookie pattern. Forms must include the token in the _csrf field; HTMX and
// fetch requests may send it in the X-CSRF-Token header. The cookie is
// created only by CSRFToken (when a page renders), so every issued token
// matches the cookie the browser stores.
func CSRFMiddleware(c *auth.CSRF) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			cookie, err := ctx.Cookie(auth.CSRFCookieName)
			if err == nil && cookie.Value == "" {
				cookie = nil
			}

			switch ctx.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				// Read-only requests never need validation.
			default:
				if cookie == nil {
					return echo.NewHTTPError(http.StatusForbidden, "CSRF cookie missing")
				}
				submitted := ctx.FormValue(csrfFormField)
				if submitted == "" {
					submitted = ctx.Request().Header.Get(csrfHeaderName)
				}
				if !auth.ValidCSRF(submitted, cookie.Value) {
					return echo.NewHTTPError(http.StatusForbidden, "invalid CSRF token")
				}
			}
			return next(ctx)
		}
	}
}

// CSRFToken returns the token stored in the request cookie, creating and
// setting it when absent. Views call this to render the hidden form field.
func CSRFToken(ctx echo.Context, c *auth.CSRF) string {
	cookie, err := ctx.Cookie(auth.CSRFCookieName)
	if err == nil && cookie.Value != "" {
		return cookie.Value
	}
	token := c.NewToken()
	ctx.SetCookie(c.Cookie(token))
	return token
}
