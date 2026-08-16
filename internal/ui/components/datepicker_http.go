package components

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/server"
)

// datepickerPanelHandler renders the datepicker popover fragment for
// the given month. It is read-only and derives everything from the
// query parameters (year/month/selected/min/max), so it never touches
// the database; the client navigates months with htmx swaps.
func datepickerPanelHandler(c echo.Context) error {
	year, err := strconv.Atoi(c.QueryParam("year"))
	if err != nil {
		year = 0
	}
	month, err := strconv.Atoi(c.QueryParam("month"))
	if err != nil {
		month = 0
	}
	data := BuildDatepickerPanel(year, month,
		c.QueryParam("selected"), c.QueryParam("min"), c.QueryParam("max"))
	return server.Render(c, http.StatusOK, DatepickerPanel(data))
}
