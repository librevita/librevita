package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/auth"
	calendarhttp "librevita.org/internal/domain/calendar/delivery/http"
)

func TestCalendarPage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/calendar", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("server.principal", &auth.Principal{
		ID:   "01990000-0000-7000-8000-000000000001",
		Role: auth.RolePhysician,
		Name: "Dr. Teste",
	})

	h := calendarhttp.NewHandler()
	require.NoError(t, h.Page(c))
	assert.Equal(t, http.StatusOK, rec.Code)
}
