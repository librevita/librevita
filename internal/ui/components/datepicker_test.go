package components

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBuildDatepickerPanel(t *testing.T) {
	// December 2025 starts on a Monday: 1 leading cell (Nov 30), 31 days,
	// trailing to a full 35-cell grid.
	data := BuildDatepickerPanel(2025, 12, "2025-12-25", "", "2025-12-31")

	if data.Title != "December 2025" {
		t.Fatalf("title = %q", data.Title)
	}
	if len(data.Cells) != 35 {
		t.Fatalf("cells = %d, want 35", len(data.Cells))
	}
	if data.Cells[0].ISO != "2025-11-30" || data.Cells[0].InMonth {
		t.Fatalf("cell 0 = %q in-month=%v, want leading Nov 30", data.Cells[0].ISO, data.Cells[0].InMonth)
	}
	if data.Cells[1].ISO != "2025-12-01" || !data.Cells[1].InMonth {
		t.Fatalf("cell 1 = %q, want Dec 1", data.Cells[1].ISO)
	}
	if data.Cells[34].ISO != "2026-01-03" {
		t.Fatalf("cell 34 = %q, want 2026-01-03", data.Cells[34].ISO)
	}

	// Selection, today (the test runs whenever), and the max limit: only
	// the trailing January cells (10) fall after 2025-12-31.
	if !data.Cells[25].IsSelected { // Dec 25
		t.Fatalf("Dec 25 is not selected")
	}
	disabled := 0
	for _, c := range data.Cells {
		if c.Disabled {
			disabled++
		}
	}
	if disabled != 3 {
		t.Fatalf("disabled cells = %d, want 3 (max 2025-12-31)", disabled)
	}

	// Navigation URLs keep the selection and limits.
	for _, u := range []string{data.PrevURL, data.NextURL} {
		if !strings.Contains(u, "selected=2025-12-25") || !strings.Contains(u, "max=2025-12-31") {
			t.Fatalf("nav URL lost state: %s", u)
		}
	}
	if data.PrevURL != "/ui/datepicker?max=2025-12-31&month=11&selected=2025-12-25&year=2025" {
		t.Fatalf("prev URL = %s", data.PrevURL)
	}
	if data.NextURL != "/ui/datepicker?max=2025-12-31&month=1&selected=2025-12-25&year=2026" {
		t.Fatalf("next URL = %s", data.NextURL)
	}
}

func TestBuildDatepickerPanelDefaultsInvalidInput(t *testing.T) {
	data := BuildDatepickerPanel(0, 99, "", "", "")
	if !strings.HasSuffix(data.Title, " 20") && len(data.Title) < 8 {
		t.Fatalf("title = %q, want the current month", data.Title)
	}
	if len(data.Cells) != 42 {
		t.Fatalf("cells = %d, want 42", len(data.Cells))
	}
}

func TestParseISODate(t *testing.T) {
	if parseISODate("") != nil {
		t.Fatalf("empty string must parse to nil")
	}
	if parseISODate("2025-02-30") != nil {
		t.Fatalf("2025-02-30 must be rejected")
	}
	d := parseISODate("2025-12-25")
	if d == nil || d.Format("2006-01-02") != "2025-12-25" {
		t.Fatalf("2025-12-25 parsed wrong: %v", d)
	}
}

// TestDatepickerPanelCaching pins the ETag revalidation: a matching
// If-None-Match answers 304 without a body, and the fragment is served
// private, no-cache so the browser must revalidate.
func TestDatepickerPanelCaching(t *testing.T) {
	e := echo.New()
	e.GET("/ui/datepicker", datepickerPanelHandler)
	url := "/ui/datepicker?year=2025&month=12&selected=2025-12-25&min=2025-12-01&max=2025-12-31"

	first := httptest.NewRecorder()
	e.ServeHTTP(first, httptest.NewRequest(http.MethodGet, url, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("missing ETag on first response")
	}
	if cc := first.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Fatalf("Cache-Control = %q, want private, no-cache", cc)
	}
	if !strings.Contains(first.Body.String(), "December 2025") {
		t.Fatalf("panel body missing month title")
	}

	revalidated := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	e.ServeHTTP(revalidated, req)
	if revalidated.Code != http.StatusNotModified {
		t.Fatalf("revalidated status = %d, want 304", revalidated.Code)
	}
	if len(revalidated.Body.String()) != 0 {
		t.Fatalf("304 must not carry a body")
	}
	if got := revalidated.Header().Get("ETag"); got != etag {
		t.Fatalf("304 ETag = %q, want %q", got, etag)
	}
}
