package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/calendar/delivery/views"
)

// Handler renders the calendar page. The page is a visual mock: the month
// grid comes from the server clock and the appointments are static
// fixtures, so there are no forms and no CSRF token is rendered.
type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

// Page renders the calendar for the current month and week.
func (h *Handler) Page(c echo.Context) error {
	now := time.Now()
	month := views.BuildMonthGrid(now, views.Fixtures)
	week := views.BuildWeekGrid(now, views.Fixtures)
	return server.Render(c, http.StatusOK, views.CalendarPage("", server.Principal(c), month, week))
}
