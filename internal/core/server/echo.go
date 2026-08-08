// Package server provides the Echo HTTP server managed by Fx. It hosts the
// transport adapters for authentication and authorization middleware;
// business logic stays in the auth and policy packages.
package server

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

// New creates the Echo instance and installs global middleware.
func New(csrf *auth.CSRF, cfg *config.Config) *echo.Echo {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ProblemErrorHandler

	// The default Echo extractor trusts X-Forwarded-For from anyone, which
	// would let clients spoof their IP for rate limiting and audit.
	// Without configured proxies the remote address is used directly.
	if strings.TrimSpace(cfg.TrustedProxies) != "" {
		var trust []echo.TrustOption
		for _, p := range strings.Split(cfg.TrustedProxies, ",") {
			p = strings.TrimSpace(p)
			if _, ipnet, err := net.ParseCIDR(p); err == nil {
				trust = append(trust, echo.TrustIPRange(ipnet))
				continue
			}
			if ip := net.ParseIP(p); ip != nil {
				trust = append(trust, echo.TrustIPRange(&net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}))
			}
		}
		e.IPExtractor = echo.ExtractIPFromXFFHeader(trust...)
	} else {
		e.IPExtractor = func(r *http.Request) string {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				return r.RemoteAddr
			}
			return host
		}
	}

	// Middleware order is significant.
	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())
	// Same-origin application; no CORS is configured. Do not add a
	// permissive CORS middleware for future authenticated endpoints.
	e.Use(SecurityHeaders)
	e.Use(middleware.BodyLimit("1M"))
	e.Use(CSRFMiddleware(csrf))

	// Read timeouts protect against slow-loris attacks. WriteTimeout stays
	// zero so that future Server-Sent Events are not interrupted.
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second

	// Infrastructure endpoint for load balancers and process probes.
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return e
}
