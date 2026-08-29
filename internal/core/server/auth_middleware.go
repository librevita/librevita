package server

import (
	"context"
	"net/http"
	"net/url"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/pkg/log"
)

// LoginPath is the unauthenticated redirect destination.
const LoginPath = "/auth/login"

// principalKey is the echo store key; principalCtxKey is the typed
// variant for the request context, so values never collide with other
// packages using plain strings.
const principalKey = "server.principal"

type contextKey string

const principalCtxKey = contextKey("server.principal")

// Principal returns the authenticated identity stored by RequireAuth.
// Callers must use RequireAuth (or RequirePolicy) before reading it.
func Principal(ctx echo.Context) *auth.Principal {
	if p, ok := ctx.Get(principalKey).(*auth.Principal); ok {
		return p
	}
	return nil
}

// PrincipalCtx reads the principal from a request context, for helpers
// that receive only the context. It returns nil when the request was
// not authenticated.
func PrincipalCtx(ctx context.Context) *auth.Principal {
	if p, ok := ctx.Value(principalCtxKey).(*auth.Principal); ok {
		return p
	}
	return nil
}

// RequireAuth rejects unauthenticated requests. Browser requests are
// redirected to the login page.
func RequireAuth(s *auth.SessionManager, logger log.Logger) echo.MiddlewareFunc {
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
				logger.ErrorContext(ctx.Request().Context(), "session lookup failed", log.Error(err))
				return echo.NewHTTPError(http.StatusInternalServerError)
			}

			ctx.Set(principalKey, p)
			// The principal also rides on the request context, so helper
			// functions without an echo.Context (rows, staff views) can
			// resolve the user's timezone preference.
			rctx := context.WithValue(ctx.Request().Context(), principalCtxKey, p)
			ctx.SetRequest(ctx.Request().WithContext(rctx))
			return next(ctx)
		}
	}
}

// RequirePolicy guards a route with the named CEL policy. It must run after
// RequireAuth, which stores the principal in the request context. Denials
// are written to the audit trail.
func RequirePolicy(pe *policy.PolicyEngine, auditLogger *audit.Logger, logger log.Logger, name string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			p := Principal(ctx)
			if p == nil {
				return redirectLogin(ctx)
			}
			if p.Platform {
				if name == "dashboard.view" {
					return next(ctx)
				}
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
			}

			allowed, err := pe.AllowedResource(ctx.Request().Context(), name, p, policy.RequestInfo{
				Method: ctx.Request().Method,
				Path:   ctx.Request().URL.Path,
			}, policyResource(ctx), nil)
			if err != nil {
				logger.ErrorContext(ctx.Request().Context(), "policy evaluation failed",
					log.String("policy", name),
					log.Error(err),
				)
				return echo.NewHTTPError(http.StatusInternalServerError, "authorization failure")
			}
			if !allowed {
				auditLogger.Record(ctx.Request().Context(), EventFromRequest(ctx, audit.AuditResultFailure,
					"authorize", "policy:"+name, name, "denied"))
				return echo.NewHTTPError(http.StatusForbidden, "insufficient permissions")
			}
			return next(ctx)
		}
	}
}

func policyResource(ctx echo.Context) map[string]any {
	id := ctx.Param("id")
	return map[string]any{
		"id":         id,
		"patient_id": id,
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
