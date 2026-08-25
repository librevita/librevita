package clinic

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	clinichttp "librevita.org/internal/domain/clinic/delivery/http"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/clinic/usecase"
)

// Module provides clinic-domain services and Host-based clinic resolution.
var Module = fx.Module("clinic",
	fx.Provide(repository.NewClinicRepository),
	fx.Provide(repository.NewPlatformUserRepository),
	fx.Provide(usecase.NewClockProvider),
	fx.Provide(usecase.NewPlatformService),
	fx.Invoke(registerHostMiddleware),
)

// registerHostMiddleware runs before Echo.Use middleware (Pre) so Host
// is classified before CSRF and route auth, without the core server
// package importing this domain.
func registerHostMiddleware(e *echo.Echo, cfg *config.Config, clinics model.Repository, engine *crypto.Engine, log *slog.Logger) {
	e.Pre(clinichttp.HostMiddleware(cfg, clinics, engine, log))
}
