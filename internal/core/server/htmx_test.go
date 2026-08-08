package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func testHtmxCtx(req *http.Request) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestIsHtmx(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/patients", nil)
	c, _ := testHtmxCtx(req)
	if IsHtmx(c) {
		t.Fatal("plain request detected as htmx")
	}

	req = httptest.NewRequest(http.MethodGet, "/patients", nil)
	req.Header.Set("HX-Request", "true")
	c, _ = testHtmxCtx(req)
	if !IsHtmx(c) {
		t.Fatal("HX-Request request not detected as htmx")
	}
}

func TestHtmxRedirectSetsHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/patients", nil)
	req.Header.Set("HX-Request", "true")
	c, rec := testHtmxCtx(req)

	if err := HtmxRedirect(c, "/auth/login"); err != nil {
		t.Fatal(err)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/auth/login" {
		t.Fatalf("HX-Redirect = %q, want /auth/login", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
}

func TestHtmxRedirectFallsBackTo302(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/patients", nil)
	c, rec := testHtmxCtx(req)

	if err := HtmxRedirect(c, "/auth/login"); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/auth/login" {
		t.Fatalf("Location = %q, want /auth/login", got)
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Fatal("HX-Redirect set on a plain request")
	}
}

func TestValidNext(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/patients", "/patients"},
		{"/patients?q=ana", "/patients?q=ana"},
		{"//evil.com", ""},
		{"https://evil.com", ""},
		{"javascript:alert(1)", ""},
	}
	for _, tc := range cases {
		if got := ValidNext(tc.in); got != tc.want {
			t.Errorf("ValidNext(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
