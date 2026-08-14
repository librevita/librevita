package ui

import (
	"io/fs"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"
)

// Module registers the static asset routes on the HTTP server.
var Module = fx.Module("ui",
	fx.Invoke(registerStatic),
)

func registerStatic(e *echo.Echo) {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic("ui: embedded static: " + err.Error())
	}
	handler := http.FileServerFS(sub)

	// Every file under /static has a content-addressed name (app-<hash>.css,
	// app-<hash>.js — the single bundle carries the HTMX runtime and its
	// SSE extension), so all of them can be cached immutably: a new
	// release always references new names. StripPrefix rewrites
	// r.URL.Path so that the FileServer resolves the file inside the
	// embedded subtree.
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		handler.ServeHTTP(w, r)
	}))))
}
