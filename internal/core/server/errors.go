package server

import (
	"mime"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/ui/pages"
	"librevita.org/pkg/log"
)

// Problem is an RFC 7807 problem-details response.
type Problem struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

// ProblemErrorHandler converts errors into responses. API clients and
// htmx requests receive application/problem+json; a plain browser
// navigation (a submitted form, an address-bar URL) gets a readable
// HTML error page instead of a bare JSON body.
func ProblemErrorHandler(appLog log.Logger) echo.HTTPErrorHandler {
	if appLog == nil {
		appLog = log.Nop()
	}
	return func(err error, c echo.Context) {
		handleProblem(err, c, appLog)
	}
}

func handleProblem(err error, c echo.Context, appLog log.Logger) {
	if c.Response().Committed {
		return
	}

	status, detail, hint := problemStatus(err)
	if status >= http.StatusInternalServerError {
		var he *echo.HTTPError
		if !errors.As(err, &he) {
			appLog.ErrorContext(c.Request().Context(), "server error",
				log.Error(err),
				log.Int("status", status),
				log.String("path", c.Request().URL.Path),
			)
		}
	}

	if !IsHtmx(c) && wantsHTML(c.Request()) {
		writeHTMLError(c, appLog, status, detail, hint)
		return
	}
	writeProblemJSON(c, appLog, status, detail, hint)
}

func problemStatus(err error) (status int, detail, hint string) {
	status = http.StatusInternalServerError
	detail = http.StatusText(status)
	hint = errors.FlattenHints(err)

	var he *echo.HTTPError
	if !errors.As(err, &he) {
		return status, detail, hint
	}
	status = he.Code
	if msg, ok := he.Message.(string); ok {
		return status, msg, hint
	}
	return status, http.StatusText(status), hint
}

func writeHTMLError(c echo.Context, appLog log.Logger, status int, detail, hint string) {
	c.Response().Header().Set(echo.HeaderContentType, "text/html; charset=utf-8")
	title := http.StatusText(status)
	if status == http.StatusNotFound {
		title = "Page not found"
		detail = "Oops! Looks like you followed a bad link. If you think this is a problem with us, please tell us."
	} else if status >= http.StatusInternalServerError {
		title = "Something has gone seriously wrong"
		detail = "It's always time for a coffee break. We should be back by the time you finish your coffee."
	}
	if hint != "" && status < http.StatusInternalServerError {
		detail = detail + " — " + hint
	}
	if renderErr := Render(c, status, pages.ErrorPage(status, title, detail)); renderErr != nil {
		appLog.ErrorContext(c.Request().Context(), "error page render failed", log.Error(renderErr))
	}
}

func writeProblemJSON(c echo.Context, appLog log.Logger, status int, detail, hint string) {
	problem := Problem{
		Type:     "about:blank",
		Title:    http.StatusText(status),
		Status:   status,
		Detail:   detail,
		Instance: c.Request().URL.Path,
		Hint:     hint,
	}
	c.Response().Header().Set(echo.HeaderContentType, "application/problem+json")
	if jsonErr := c.JSON(status, problem); jsonErr != nil {
		appLog.ErrorContext(c.Request().Context(), "problem json encode failed", log.Error(jsonErr))
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
