// LibreVita web application.
//
// The entrypoint only assembles the Fx dependency graph.
package main

import (
	"log/slog"
	_ "time/tzdata"

	_ "github.com/breml/rootcerts"
	"github.com/spf13/pflag"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/telemetry"
)

func main() {
	config.RegisterFlags(pflag.CommandLine)
	pflag.Parse()

	fx.New(
		config.Module,
		telemetry.Module,
		fx.WithLogger(func(log *slog.Logger) fxevent.Logger {
			fxLogger := &fxevent.SlogLogger{Logger: log}
			fxLogger.UseLogLevel(slog.LevelDebug)
			fxLogger.UseErrorLevel(slog.LevelError)
			return fxLogger
		}),
		database.Module, // Runs embedded migrations during OnStart.
		server.Module,
	).Run()
}
