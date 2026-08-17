package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// requestLogFixture builds an Echo instance with the RequestID and
// RequestLog middlewares and handlers for the 200/404/500 paths.
func requestLogFixture(t *testing.T, buf *bytes.Buffer) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Use(middleware.RequestID())
	e.Use(RequestLog(slog.New(slog.NewTextHandler(buf, nil))))
	e.GET("/ok", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/missing", func(c echo.Context) error { return echo.ErrNotFound })
	e.GET("/broken", func(c echo.Context) error { return echo.ErrInternalServerError })
	return e
}

func serveLogged(e *echo.Echo, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestRequestLogInfo(t *testing.T) {
	var buf bytes.Buffer
	e := requestLogFixture(t, &buf)
	serveLogged(e, "/ok")

	line := buf.String()
	for _, want := range []string{"level=INFO", "msg=request", "method=GET", "path=/ok", "status=200", "request_id="} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

func TestRequestLogWarnOnClientError(t *testing.T) {
	var buf bytes.Buffer
	e := requestLogFixture(t, &buf)
	serveLogged(e, "/missing")

	line := buf.String()
	for _, want := range []string{"level=WARN", "status=404", "path=/missing"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

func TestRequestLogErrorOnServerError(t *testing.T) {
	var buf bytes.Buffer
	e := requestLogFixture(t, &buf)
	serveLogged(e, "/broken")

	line := buf.String()
	for _, want := range []string{"level=ERROR", "status=500", "path=/broken"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

func TestRequestLogSkipsHealthz(t *testing.T) {
	var buf bytes.Buffer
	e := requestLogFixture(t, &buf)
	serveLogged(e, "/healthz")

	if buf.Len() != 0 {
		t.Fatalf("healthz was logged:\n%s", buf.String())
	}
}
