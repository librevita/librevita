package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/host"
	"librevita.org/internal/domain/clinic/model"
	"librevita.org/pkg/log"
)

// HostMiddleware resolves the clinic from Host, attaches clinicctx, and
// wires FLE to the Clinic DEK. Unknown slugs are 404; Host values outside
// the allowlist are 400. /healthz and /static skip Host.
func HostMiddleware(cfg *config.Config, clinics model.Repository, engine *crypto.Engine, logger log.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return serveHost(c, next, cfg, clinics, engine, logger)
		}
	}
}

func serveHost(c echo.Context, next echo.HandlerFunc, cfg *config.Config, clinics model.Repository, engine *crypto.Engine, logger log.Logger) error {
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
		var attachErr error
		ctx, attachErr = attachClinicCrypto(ctx, row.ID, engine, cfg, logger)
		defer crypto.ClearRequestKeyCache(ctx)
		if attachErr != nil {
			return attachErr
		}
	}

	c.SetRequest(c.Request().WithContext(ctx))
	return next(c)
}

func attachClinicCrypto(ctx context.Context, clinicID uuid.UUID, engine *crypto.Engine, cfg *config.Config, logger log.Logger) (context.Context, error) {
	ctx = crypto.WithRequestKeyCache(ctx)
	dek, dekErr := engine.GetClinicDEK(ctx, clinicID)
	if errors.Is(dekErr, crypto.ErrKeyNotFound) {
		dek, dekErr = engine.EnsureClinicDEK(ctx, clinicID)
	}
	if dekErr != nil {
		logger.ErrorContext(ctx, "clinic DEK unavailable",
			log.Stringer("clinic_id", clinicID),
			log.Error(dekErr),
		)
		return ctx, echo.NewHTTPError(http.StatusInternalServerError)
	}
	enc, encErr := crypto.NewEncryptor(dek)
	if encErr != nil {
		crypto.ZeroBytes(dek)
		logger.ErrorContext(ctx, "clinic encryptor",
			log.Stringer("clinic_id", clinicID),
			log.Error(encErr),
		)
		return ctx, echo.NewHTTPError(http.StatusInternalServerError)
	}
	algorithm := cfg.Crypto.HashAlgorithm
	if algorithm == "" {
		algorithm = crypto.DefaultHashAlgorithm
	}
	h, hErr := crypto.NewHasherFromDEK(dek, crypto.WithHashAlgorithm(algorithm))
	crypto.ZeroBytes(dek)
	if hErr != nil {
		logger.ErrorContext(ctx, "clinic hasher",
			log.Stringer("clinic_id", clinicID),
			log.Error(hErr),
		)
		return ctx, echo.NewHTTPError(http.StatusInternalServerError)
	}
	ctx = fle.WithEncryptor(ctx, enc)
	ctx = fle.WithHasher(ctx, h)
	ctx = fle.WithPatientEncryptorResolver(ctx, engine)
	return ctx, nil
}
