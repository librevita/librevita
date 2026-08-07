package server

import (
	"strings"

	"github.com/labstack/echo/v4"
)

// Content-Security-Policy without unsafe-eval or unsafe-inline. The stack
// (HTMX with allowEval=false, first-party ES5 widgets) fits this policy; the
// static handler overrides Cache-Control for cacheable assets.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// SecurityHeaders applies the security response headers. Pages carrying
// patient data must never be cached; /static responses overwrite the
// Cache-Control header afterwards.
func SecurityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		res := ctx.Response()
		res.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		res.Header().Set("X-Content-Type-Options", "nosniff")
		res.Header().Set("Referrer-Policy", "no-referrer")
		res.Header().Set("X-Frame-Options", "DENY")
		if !strings.HasPrefix(ctx.Request().URL.Path, "/static/") {
			res.Header().Set("Cache-Control", "no-store")
		}
		return next(ctx)
	}
}
