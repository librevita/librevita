package ui

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"
)

// Module registers the static asset routes on the HTTP server.
var Module = fx.Module("ui",
	fx.Invoke(registerStatic),
)

// vendorFiles are cacheable forever because their names embed the version.
var vendorFiles = map[string]bool{
	"js/htmx-1.9.12.min.js": true,
	"js/htmx-sse-1.9.12.js": true,
}

func registerStatic(e *echo.Echo) {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic("ui: embedded static: " + err.Error())
	}
	handler := http.FileServerFS(sub)

	// StripPrefix rewrites r.URL.Path so that the FileServer resolves the
	// file inside the embedded subtree.
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if vendorFiles[path] {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Generated CSS and the application script change with releases.
			w.Header().Set("Cache-Control", "no-cache")
		}
		handler.ServeHTTP(w, r)
	}))))
}
