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

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	"librevita.org/internal/core/telemetry"
	"librevita.org/internal/domain/calendar"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/patient"
	"librevita.org/internal/domain/user"
	"librevita.org/internal/ui"
	"librevita.org/internal/ui/components"
)

func main() {
	config.RegisterFlags(pflag.CommandLine)
	pflag.Parse()

	fx.New(
		config.Module,
		telemetry.Module,
		crypto.Module,
		fx.WithLogger(func(log *slog.Logger) fxevent.Logger {
			fxLogger := &fxevent.SlogLogger{Logger: log}
			fxLogger.UseLogLevel(slog.LevelDebug)
			fxLogger.UseErrorLevel(slog.LevelError)
			return fxLogger
		}),
		database.Module, // Runs embedded migrations during OnStart.
		storage.Module,
		audit.Module,
		auth.Module,
		clinic.Module,
		policy.Module,
		server.Module,
		ui.Module,
		components.Module,
		calendar.Module,
		user.Module,
		patient.Module,
	).Run()
}
