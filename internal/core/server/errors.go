package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Problem is an RFC 7807 problem-details response.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

// ProblemErrorHandler returns errors as application/problem+json.
func ProblemErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var he *echo.HTTPError
	status := http.StatusInternalServerError
	detail := http.StatusText(status)

	if errors.As(err, &he) {
		status = he.Code
		if msg, ok := he.Message.(string); ok {
			detail = msg
		} else {
			detail = http.StatusText(status)
		}
	}

	problem := Problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: c.Request().URL.Path,
	}

	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	if err := c.JSON(status, problem); err != nil {
		c.Logger().Error(err)
	}
}
