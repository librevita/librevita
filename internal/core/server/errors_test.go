package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"
)

// TestProblemErrorHandlerFormatsByClient pins the content negotiation:
// a browser navigation receives a readable HTML error page, while API
// clients and htmx keep the RFC 7807 problem+json envelope.
func TestProblemErrorHandlerFormatsByClient(t *testing.T) {
	for _, tc := range []struct {
		name     string
		accept   string
		htmx     bool
		wantType string
		wantBody string
	}{
		{"browser navigation", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8", false, "text/html", "Page not found"},
		{"api client", "", false, "application/problem+json", `"status":404`},
		{"api client explicit", "application/json", false, "application/problem+json", `"instance":"/missing"`},
		{"htmx request", "text/html,application/xhtml+xml,*/*;q=0.8", true, "application/problem+json", `"instance":"/missing"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			e.HTTPErrorHandler = ProblemErrorHandler
			e.GET("/missing", func(c echo.Context) error { return echo.ErrNotFound })

			req := httptest.NewRequest(http.MethodGet, "/missing", nil)
			if tc.accept != "" {
				req.Header.Set("Accept", tc.accept)
			}
			if tc.htmx {
				req.Header.Set("HX-Request", "true")
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, tc.wantType) {
				t.Fatalf("content type = %q, want %s (body=%q)", ct, tc.wantType, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body %q missing %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

func TestProblemErrorHandlerWithHints(t *testing.T) {
	e := echo.New()
	e.HTTPErrorHandler = ProblemErrorHandler
	e.GET("/with-hint", func(c echo.Context) error {
		err := echo.NewHTTPError(http.StatusBadRequest, "invalid configuration")
		return errors.WithHint(err, "Please provide a valid database connection string.")
	})

	// JSON request
	reqJSON := httptest.NewRequest(http.MethodGet, "/with-hint", nil)
	reqJSON.Header.Set("Accept", "application/json")
	recJSON := httptest.NewRecorder()
	e.ServeHTTP(recJSON, reqJSON)

	if recJSON.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recJSON.Code)
	}
	bodyJSON := recJSON.Body.String()
	if !strings.Contains(bodyJSON, `"hint":"Please provide a valid database connection string."`) {
		t.Fatalf("bodyJSON %q missing hint", bodyJSON)
	}

	// HTML request
	reqHTML := httptest.NewRequest(http.MethodGet, "/with-hint", nil)
	reqHTML.Header.Set("Accept", "text/html")
	recHTML := httptest.NewRecorder()
	e.ServeHTTP(recHTML, reqHTML)

	if recHTML.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recHTML.Code)
	}
	bodyHTML := recHTML.Body.String()
	if !strings.Contains(bodyHTML, "Please provide a valid database connection string.") {
		t.Fatalf("bodyHTML %q missing hint", bodyHTML)
	}
}
