package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
