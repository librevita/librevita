package server

import (
	"mime"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
	"librevita.org/internal/ui/pages"
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
	} else if status >= http.StatusInternalServerError {
		c.Logger().Errorf("server error: %+v", err)
	}

	if !IsHtmx(c) && wantsHTML(c.Request()) {
		c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
		title := http.StatusText(status)
		if status == http.StatusNotFound {
			title = "Page not found"
			detail = "Oops! Looks like you followed a bad link. If you think this is a problem with us, please tell us."
		} else if status >= http.StatusInternalServerError {
			title = "Something has gone seriously wrong"
			detail = "It's always time for a coffee break. We should be back by the time you finish your coffee."
		}
		if err := Render(c, status, pages.ErrorPage(status, title, detail)); err != nil {
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
