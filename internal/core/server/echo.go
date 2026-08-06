// Package server provides the Echo HTTP server managed by Fx. It hosts the
// transport adapters for authentication and authorization middleware;
// business logic stays in the auth and policy packages.
package server

import (
	"net/http"

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
	e.Use(middleware.CORS())
	e.Use(CSRFMiddleware(csrf))

	// Infrastructure endpoint for load balancers and process probes.
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return e
}
