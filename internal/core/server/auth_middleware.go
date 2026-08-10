package server

import (
	"net/url"
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
)

// LoginPath is the unauthenticated redirect destination.
const LoginPath = "/auth/login"

const principalKey = "server.principal"

// Principal returns the authenticated identity stored by RequireAuth.
// Callers must use RequireAuth (or RequirePolicy) before reading it.
func Principal(ctx echo.Context) *auth.Principal {
	if p, ok := ctx.Get(principalKey).(*auth.Principal); ok {
		return p
	}
	return nil
}

// RequireAuth rejects unauthenticated requests. Browser requests are
// redirected to the login page.
func RequireAuth(s *auth.SessionManager, log *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			cookie, err := ctx.Cookie(auth.SessionCookieName)
			if err != nil {
				return redirectLogin(ctx)
			}

			p, err := s.Authenticate(ctx.Request().Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, auth.ErrNoSession) {
					ctx.SetCookie(s.ClearCookie())
					return redirectLogin(ctx)
				}
				log.Error("session lookup failed", "error", err)
				return echo.NewHTTPError(http.StatusInternalServerError)
			}

			ctx.Set(principalKey, p)
			return next(ctx)
		}
	}
}

// RequirePolicy guards a route with the named CEL policy. It must run after
// RequireAuth, which stores the principal in the request context. Denials
// are written to the audit trail.
func RequirePolicy(pe *policy.PolicyEngine, auditLogger *audit.Logger, log *slog.Logger, name string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			p := Principal(ctx)
			if p == nil {
				return redirectLogin(ctx)
			}

			allowed, err := pe.Allowed(ctx.Request().Context(), name, p, policy.RequestInfo{
				Method: ctx.Request().Method,
				Path:   ctx.Request().URL.Path,
			})
			if err != nil {
				log.Error("policy evaluation failed", "policy", name, "error", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "authorization failure")
			}
			if !allowed {
				auditLogger.Record(ctx.Request().Context(), EventFromRequest(ctx, audit.ResultFailure,
					"authorize", "policy:"+name, name, "denied"))
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
			}
			return next(ctx)
		}
	}
}

func redirectLogin(ctx echo.Context) error {
	// Remember where the user was going so the login page can send them
	// back after authenticating.
	next := ctx.Request().URL.RequestURI()
	if next == "" || next == "/" {
		return HtmxRedirect(ctx, LoginPath)
	}
	return HtmxRedirect(ctx, LoginPath+"?next="+url.QueryEscape(next))
}
