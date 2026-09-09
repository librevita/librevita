package clinic

import (
	"context"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/acme"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	clinichttp "librevita.org/internal/domain/clinic/delivery/http"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/internal/domain/clinic/repository"
	"librevita.org/internal/domain/clinic/usecase"
	"librevita.org/pkg/log"
)

// Module provides clinic-domain services and Host-based clinic resolution.
var Module = fx.Module("clinic",
	fx.Provide(repository.NewClinicRepository),
	fx.Provide(repository.NewPlatformUserRepository),
	fx.Provide(usecase.NewClockProvider),
	fx.Provide(usecase.NewPlatformService),
	fx.Invoke(registerHostMiddleware),
)

type hostParams struct {
	fx.In
	Echo        *echo.Echo
	Config      *config.Config
	Clinics     model.Repository
	Engine      *crypto.Engine
	Logger      log.Logger
	ACMEManager *acme.Manager `optional:"true"`
}

// registerHostMiddleware runs before Echo.Use middleware (Pre) so Host
// is classified before CSRF and route auth, without the core server
// package importing this domain. It also wires on-demand TLS domain authorization.
func registerHostMiddleware(p hostParams) {
	p.Echo.Pre(clinichttp.HostMiddleware(p.Config, p.Clinics, p.Engine, p.Logger))
	if p.ACMEManager != nil {
		p.ACMEManager.SetDomainAuthorizer(func(ctx context.Context, domain string) (bool, error) {
			row, err := p.Clinics.GetByDomain(ctx, domain)
			if err != nil {
				return false, err
			}
			return row != nil, nil
		})
	}
}
