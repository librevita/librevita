package components

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/server"
)

// datepickerPanelHandler renders the datepicker popover fragment for
// the given month. It is read-only and derives everything from the
// query parameters (year/month/selected/min/max), so it never touches
// the database; the client navigates months with htmx swaps.
//
// The fragment is a pure function of its parameters and the current
// date, so it is cached with a strong ETag. The browser revalidates
// every request (Cache-Control: private, no-cache): the same date and
// parameters answer 304 without the body, while a new day (the IsToday
// highlight) or changed parameters produce a fresh panel. private keeps
// the fragment — it sits behind the auth gate — out of shared caches.
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
	etag := datepickerETag(data)

	res := c.Response()
	res.Header().Set("ETag", etag)
	res.Header().Set("Cache-Control", "private, no-cache")
	if c.Request().Header.Get("If-None-Match") == etag {
		return c.NoContent(http.StatusNotModified)
	}
	return server.Render(c, http.StatusOK, DatepickerPanel(data))
}

// datepickerETag derives a strong validator from the panel contents:
// the rendered markup is a pure function of this data.
func datepickerETag(data DatepickerPanelData) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%v",
		data.Title, data.PrevURL, data.NextURL, data.Cells)))
	return fmt.Sprintf("\"%x\"", hash)
}
