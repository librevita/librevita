package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
)

func newCSRFTestEcho(t *testing.T, csrf *auth.CSRF) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.Use(CSRFMiddleware(csrf))
	e.GET("/", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.POST("/", func(c echo.Context) error { return c.String(http.StatusOK, "ok") })
	return e
}

func doRequest(e *echo.Echo, method, path string, body string, cookies []*http.Cookie, csrfToken string) (*httptest.ResponseRecorder, []*http.Cookie) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	if csrfToken != "" {
		req.Header.Set(csrfHeaderName, csrfToken)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec, rec.Result().Cookies()
}

func testCSRF() *auth.CSRF {
	return auth.NewCSRF(&config.Config{Env: "test"})
}

func cookieValue(cookies []*http.Cookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func TestCSRFIssuesCookieOnGET(t *testing.T) {
	e := newCSRFTestEcho(t, testCSRF())

	rec, cookies := doRequest(e, http.MethodGet, "/", "", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if cookieValue(cookies, auth.CSRFCookieName) == "" {
		t.Fatal("GET did not set the CSRF cookie")
	}
}

func TestCSRFRejectsMissingToken(t *testing.T) {
	e := newCSRFTestEcho(t, testCSRF())

	_, cookies := doRequest(e, http.MethodGet, "/", "", nil, "")
	rec, _ := doRequest(e, http.MethodPost, "/", "a=b", cookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without token status = %d, want 403", rec.Code)
	}
}

func TestCSRFRejectsMismatchedToken(t *testing.T) {
	e := newCSRFTestEcho(t, testCSRF())

	_, cookies := doRequest(e, http.MethodGet, "/", "", nil, "")
	rec, _ := doRequest(e, http.MethodPost, "/", "_csrf=forged", cookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST with forged token status = %d, want 403", rec.Code)
	}
}

func TestCSRFAcceptsFormToken(t *testing.T) {
	e := newCSRFTestEcho(t, testCSRF())

	_, cookies := doRequest(e, http.MethodGet, "/", "", nil, "")
	token := cookieValue(cookies, auth.CSRFCookieName)
	rec, _ := doRequest(e, http.MethodPost, "/", "_csrf="+token, cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("POST with valid form token status = %d, want 200", rec.Code)
	}
}

func TestCSRFAcceptsHeaderToken(t *testing.T) {
	e := newCSRFTestEcho(t, testCSRF())

	_, cookies := doRequest(e, http.MethodGet, "/", "", nil, "")
	token := cookieValue(cookies, auth.CSRFCookieName)
	rec, _ := doRequest(e, http.MethodPost, "/", "", cookies, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST with valid header token status = %d, want 200", rec.Code)
	}
}
