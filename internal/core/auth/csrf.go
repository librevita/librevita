// Package auth provides authentication primitives: Argon2id password
// hashing, PASETO v4.local sessions with revocation support, CSRF tokens,
// and the Role type used as a principal attribute.
//
// This package is transport-agnostic. HTTP middleware lives in
// librevita.org/internal/core/server.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"librevita.org/internal/core/config"
)

// CSRFCookieName is the double-submit cookie holding the CSRF token.
const CSRFCookieName = "lv_csrf"

// CSRF issues and validates double-submit CSRF tokens. The token itself is
// random; validation compares the submitted value with the cookie value in
// constant time.
type CSRF struct {
	secure bool
}

// NewCSRF is the Fx provider.
func NewCSRF(cfg *config.Config) *CSRF {
	return &CSRF{secure: cfg.IsProduction()}
}

// NewToken returns a fresh random token.
func (c *CSRF) NewToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("auth: csrf token: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// Cookie returns the CSRF cookie descriptor for token. It is not HttpOnly so
// that HTMX and fetch can read it for the X-CSRF-Token header.
func (c *CSRF) Cookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ValidCSRF reports whether the submitted token matches the cookie value.
func ValidCSRF(submitted, cookieValue string) bool {
	if submitted == "" || cookieValue == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(submitted), []byte(cookieValue)) == 1
}
