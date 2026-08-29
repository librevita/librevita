package http

import (
	"librevita.org/pkg/log"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/host"
	"librevita.org/internal/domain/clinic/model"
)

// HostMiddleware resolves the clinic from Host, attaches clinicctx, and
// wires FLE to the Clinic DEK. Unknown slugs are 404; Host values outside
// the allowlist are 400. /healthz and /static skip Host.
func HostMiddleware(cfg *config.Config, clinics model.Repository, engine *crypto.Engine, logger log.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if path == "/healthz" || strings.HasPrefix(path, "/static/") {
				return next(c)
			}

			classified, err := host.Classify(c.Request().Host, cfg.BaseDomain)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "invalid host")
			}

			ctx := c.Request().Context()
			if classified.Kind == host.KindApex {
				c.SetRequest(c.Request().WithContext(clinicctx.WithApex(ctx)))
				return next(c)
			}

			row, err := clinics.GetBySlug(ctx, classified.Slug)
			if err != nil {
				logger.ErrorContext(ctx, "clinic lookup failed",
					log.String("slug", classified.Slug),
					log.Error(err),
				)
				return echo.NewHTTPError(http.StatusInternalServerError)
			}
			if row == nil {
				return echo.NewHTTPError(http.StatusNotFound, "clinic not found")
			}

			resolved := &clinicctx.Clinic{
				ID:          row.ID,
				Slug:        row.Slug,
				Name:        row.Name,
				Timezone:    row.Timezone,
				OnboardedAt: row.OnboardedAt,
			}
			ctx = clinicctx.WithClinic(ctx, resolved)
			ctx = fle.WithClinicID(ctx, row.ID.String())

			if engine != nil {
				ctx = crypto.WithRequestKeyCache(ctx)
				defer crypto.ClearRequestKeyCache(ctx)
				dek, dekErr := engine.GetClinicDEK(ctx, row.ID)
				if errors.Is(dekErr, crypto.ErrKeyNotFound) {
					dek, dekErr = engine.EnsureClinicDEK(ctx, row.ID)
				}
				if dekErr != nil {
					logger.ErrorContext(ctx, "clinic DEK unavailable",
						log.Stringer("clinic_id", row.ID),
						log.Error(dekErr),
					)
					return echo.NewHTTPError(http.StatusInternalServerError)
				}
				enc, encErr := crypto.NewEncryptor(dek)
				if encErr != nil {
					crypto.ZeroBytes(dek)
					logger.ErrorContext(ctx, "clinic encryptor",
						log.Stringer("clinic_id", row.ID),
						log.Error(encErr),
					)
					return echo.NewHTTPError(http.StatusInternalServerError)
				}
				algorithm := cfg.Crypto.HashAlgorithm
				if algorithm == "" {
					algorithm = crypto.DefaultHashAlgorithm
				}
				h, hErr := crypto.NewHasherFromDEK(dek, crypto.WithHashAlgorithm(algorithm))
				crypto.ZeroBytes(dek)
				if hErr != nil {
					logger.ErrorContext(ctx, "clinic hasher",
						log.Stringer("clinic_id", row.ID),
						log.Error(hErr),
					)
					return echo.NewHTTPError(http.StatusInternalServerError)
				}
				ctx = fle.WithEncryptor(ctx, enc)
				ctx = fle.WithHasher(ctx, h)
				ctx = fle.WithPatientEncryptorResolver(ctx, engine)
			}

			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
