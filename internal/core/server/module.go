package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
)

// Module manages the HTTP server lifecycle through Fx.
var Module = fx.Module("server",
	fx.Provide(New),
	fx.Invoke(registerLifecycle),
)

// registerLifecycle starts Echo asynchronously and shuts it down gracefully.
func registerLifecycle(lc fx.Lifecycle, e *echo.Echo, cfg *config.Config, log *slog.Logger, shutdown fx.Shutdowner) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("HTTP server listening", "addr", cfg.HTTPAddr)
				if err := e.Start(cfg.HTTPAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error("HTTP server failed", "error", err)
					_ = shutdown.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("shutting down HTTP server")
			return e.Shutdown(ctx)
		},
	})
}
