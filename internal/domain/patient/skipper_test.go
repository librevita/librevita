package patient

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBodyLimitSkipperMatchesDocumentRoute(t *testing.T) {
	e := echo.New()
	e.POST("/patients/:id/documents", func(c echo.Context) error {
		if !bodyLimitSkipper()(c) {
			t.Fatal("expected body-limit skipper to match document upload route")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/patients/01990000-0000-7000-8000-0000000000aa/documents", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
