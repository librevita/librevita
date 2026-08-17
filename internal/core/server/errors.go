package server

import (
	"errors"
	"mime"
	"net/http"
	"strings"

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

// ProblemErrorHandler converts errors into responses. API clients and
// htmx requests receive application/problem+json; a plain browser
// navigation (a submitted form, an address-bar URL) gets a readable
// HTML error page instead of a bare JSON body.
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

	if !IsHtmx(c) && wantsHTML(c.Request()) {
		if err := Render(c, status, ErrorPage(status, http.StatusText(status), detail)); err != nil {
			c.Logger().Error(err)
		}
		return
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

// wantsHTML reports whether the client asked for an HTML document,
// distinguishing a browser navigation from an API client.
func wantsHTML(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		typ, _, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err == nil && (typ == "text/html" || typ == "application/xhtml+xml") {
			return true
		}
	}
	return false
}
