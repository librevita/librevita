package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const healthzPath = "/healthz"

// healthz is the process probe for load balancers.
func healthz(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
