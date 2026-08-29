package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"librevita.org/pkg/log"
)

func requestLogFixture(t *testing.T, rec *log.Recorder) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Pre(middleware.RequestID())
	e.Use(RequestLog(rec))
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
	rec := log.NewRecorder()
	e := requestLogFixture(t, rec)
	serveLogged(e, "/ok")

	got := rec.Records()
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1 (%v)", len(got), rec.Messages())
	}
	if got[0].Level != log.Info || got[0].Message != "request" {
		t.Fatalf("record = %+v", got[0])
	}
	keys := fieldKeys(got[0].Fields)
	for _, want := range []string{"method", "path", "status", "request_id"} {
		if !strings.Contains(keys, want) {
			t.Fatalf("fields %q missing %q", keys, want)
		}
	}
}

func TestRequestLogWarnOnClientError(t *testing.T) {
	rec := log.NewRecorder()
	e := requestLogFixture(t, rec)
	serveLogged(e, "/missing")

	got := rec.Records()
	if len(got) != 1 || got[0].Level != log.Warn {
		t.Fatalf("records = %+v", got)
	}
}

func TestRequestLogErrorOnServerError(t *testing.T) {
	rec := log.NewRecorder()
	e := requestLogFixture(t, rec)
	serveLogged(e, "/broken")

	got := rec.Records()
	if len(got) != 1 || got[0].Level != log.ErrorLevel {
		t.Fatalf("records = %+v", got)
	}
}

func TestRequestLogSkipsHealthz(t *testing.T) {
	rec := log.NewRecorder()
	e := requestLogFixture(t, rec)
	serveLogged(e, healthzPath)

	if len(rec.Records()) != 0 {
		t.Fatalf("healthz was logged: %v", rec.Messages())
	}
}

func fieldKeys(fields []log.Field) string {
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f.Key)
		b.WriteByte(',')
	}
	return b.String()
}
