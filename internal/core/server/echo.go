// Package server provides the Echo HTTP server managed by Fx. It hosts the
// transport adapters for authentication and authorization middleware;
// business logic stays in the auth and policy packages.
package server

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"librevita.org/internal/core/auth"
)

// New creates the Echo instance and installs global middleware.
func New(csrf *auth.CSRF) *echo.Echo {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ProblemErrorHandler

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
