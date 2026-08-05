// Package server provides the Echo HTTP server managed by Fx.
package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// New creates the Echo instance and installs global middleware.
func New() *echo.Echo {
	e := echo.New()

	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = ProblemErrorHandler

	// Middleware order is significant.
	e.Use(middleware.RequestID())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Infrastructure endpoint for load balancers and process probes.
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return e
}
