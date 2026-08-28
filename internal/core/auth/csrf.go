// Package auth provides authentication primitives: Argon2id password
// hashing, PASETO v4.local sessions with revocation support, CSRF tokens,
// and the Role type used as a principal attribute.
//
// This package is transport-agnostic. HTTP middleware lives in
// librevita.org/internal/core/server.
package auth

import (
	"net/http"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
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
	return &CSRF{secure: !cfg.IsDevelopment()}
}

// NewToken returns a fresh random token.
func (c *CSRF) NewToken() string {
	token, err := crypto.RandomHex(32)
	if err != nil {
		panic("auth: csrf token: " + err.Error())
	}
	return token
}

// Cookie returns the CSRF cookie descriptor for token. The token is
// also rendered in a meta tag (and the form hidden fields), so the
// cookie can stay HttpOnly: the browser sends it back on every request
// and the server compares it against the submitted header or field.
// #nosec G124 -- Secure is conditional on the environment (dev runs
// without TLS); HttpOnly and SameSite are set.
func (c *CSRF) Cookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteLaxMode,
	}
}

// ValidCSRF reports whether the submitted token matches the cookie value.
func ValidCSRF(submitted, cookieValue string) bool {
	if submitted == "" || cookieValue == "" {
		return false
	}
	return crypto.ConstantTimeCompare(submitted, cookieValue)
}
