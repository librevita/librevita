package auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"librevita.org/internal/core/config"
)

// TestCSRFCookieAttributes pins the CSRF cookie contract that gosec
// G124 cannot see: HttpOnly and SameSite=Lax always; Secure follows the
// environment (dev runs without TLS). The token is delivered to the JS
// through the server-rendered meta tag, so the cookie never needs to be
// readable.
func TestCSRFCookieAttributes(t *testing.T) {
	dev := NewCSRF(&config.Config{Mode: "development"})
	prod := NewCSRF(&config.Config{Mode: "production"})

	for name, c := range map[string]*CSRF{"development": dev, "production": prod} {
		cookie := c.Cookie("token")
		assert.True(t, cookie.HttpOnly, "%s: CSRF cookie must be HttpOnly", name)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite, "%s: CSRF cookie must be SameSite=Lax", name)
		wantSecure := name == "production"
		assert.Equal(t, wantSecure, cookie.Secure, "%s: Secure mismatch", name)
	}
}
