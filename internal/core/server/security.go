package server

import (
	"fmt"
	"strings"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/config"
	"librevita.org/internal/ui"
)

// Content-Security-Policy without unsafe-eval or unsafe-inline. The stack
// (HTMX with allowEval=false, first-party ES5 widgets) fits this policy;
// the assets are self-hosted and content-addressed, so 'self' covers
// everything and no CDN is involved. The only inline script is the theme
// bootstrap, allowed through its content hash (ui.ThemeScriptHash) — a
// static script needs no per-request nonce. base-uri is 'self' (not
// 'none') because Firefox resolves history.pushState URLs against the
// document base and enforces the directive; 'self' still blocks the
// cross-origin <base> hijack.
const contentSecurityPolicy = "default-src 'self'; script-src 'self' '" + ui.ThemeScriptHash + "'; style-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// crossOriginResourcePolicy keeps other origins from embedding or
// reading the application assets.
const crossOriginResourcePolicy = "same-origin"

// permissionsPolicy denies the sensitive device APIs the application
// never uses. Older browsers (the legacy floor) ignore the header.
const permissionsPolicy = "camera=(), microphone=(), geolocation=()"

// SecurityHeaders applies the security response headers. Pages carrying
// patient data must never be cached; /static responses overwrite the
// Cache-Control header afterwards. HSTS is opt-in through the config:
// over plain HTTP the header would brick the site for the whole
// max-age window.
func SecurityHeaders(cfg *config.Config) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			res := ctx.Response()
			res.Header().Set("Content-Security-Policy", contentSecurityPolicy)
			res.Header().Set("X-Content-Type-Options", "nosniff")
			res.Header().Set("Referrer-Policy", "no-referrer")
			res.Header().Set("X-Frame-Options", "DENY")
			res.Header().Set("Cross-Origin-Resource-Policy", crossOriginResourcePolicy)
			res.Header().Set("Permissions-Policy", permissionsPolicy)
			if cfg.HSTSMaxAge > 0 {
				res.Header().Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d", cfg.HSTSMaxAge))
			}
			if !strings.HasPrefix(ctx.Request().URL.Path, "/static/") {
				res.Header().Set("Cache-Control", "no-store")
			}
			return next(ctx)
		}
	}
}
