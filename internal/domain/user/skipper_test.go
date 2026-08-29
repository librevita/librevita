package user

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBodyLimitSkipperMatchesAvatarRoute(t *testing.T) {
	e := echo.New()
	e.POST("/profile/avatar", func(c echo.Context) error {
		if !bodyLimitSkipper()(c) {
			t.Fatal("expected body-limit skipper to match avatar upload route")
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/profile/avatar", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
