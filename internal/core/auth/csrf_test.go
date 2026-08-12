package auth

import (
	"net/http"
	"testing"

	"librevita.org/internal/core/config"
)

// TestCSRFCookieAttributes pins the CSRF cookie contract that gosec
// G124 cannot see: HttpOnly and SameSite=Lax always; Secure follows the
// environment (dev runs without TLS). The token is delivered to the JS
// through the server-rendered meta tag, so the cookie never needs to be
// readable.
func TestCSRFCookieAttributes(t *testing.T) {
	dev := NewCSRF(&config.Config{Env: "development"})
	prod := NewCSRF(&config.Config{Env: "production"})

	for name, c := range map[string]*CSRF{"development": dev, "production": prod} {
		cookie := c.Cookie("token")
		if !cookie.HttpOnly {
			t.Fatalf("%s: CSRF cookie must be HttpOnly", name)
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("%s: CSRF cookie must be SameSite=Lax", name)
		}
		wantSecure := name == "production"
		if cookie.Secure != wantSecure {
			t.Fatalf("%s: Secure = %v, want %v", name, cookie.Secure, wantSecure)
		}
	}
}
