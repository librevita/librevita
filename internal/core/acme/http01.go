package acme

import (
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
)

// HTTP01Registry stores in-flight ACME HTTP-01 challenge authorizations.
type HTTP01Registry struct {
	tokens sync.Map
}

// NewHTTP01Registry creates a new HTTP-01 challenge registry.
func NewHTTP01Registry() *HTTP01Registry {
	return &HTTP01Registry{}
}

// Register adds a token and its calculated key authorization.
func (r *HTTP01Registry) Register(token, keyAuth string) {
	r.tokens.Store(token, keyAuth)
}

// Unregister removes a token after challenge validation completes.
func (r *HTTP01Registry) Unregister(token string) {
	r.tokens.Delete(token)
}

// Lookup retrieves the key authorization for a given token.
func (r *HTTP01Registry) Lookup(token string) (string, bool) {
	val, ok := r.tokens.Load(token)
	if !ok {
		return "", false
	}
	s, ok := val.(string)
	return s, ok
}

// Handler returns an Echo route handler for /.well-known/acme-challenge/:token.
func (r *HTTP01Registry) Handler() echo.HandlerFunc {
	return func(c echo.Context) error {
		token := c.Param("token")
		if token == "" {
			return c.NoContent(http.StatusNotFound)
		}
		keyAuth, found := r.Lookup(token)
		if !found {
			return c.NoContent(http.StatusNotFound)
		}
		return c.Blob(http.StatusOK, "application/octet-stream", []byte(keyAuth))
	}
}
