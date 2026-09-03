package http_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"librevita.org/pkg/ident"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	"librevita.org/internal/core/crypto"
	"librevita.org/internal/core/database/fle"
	"librevita.org/internal/core/keystore"
	clinichttp "librevita.org/internal/domain/clinic/delivery/http"
	"librevita.org/internal/domain/clinic/model"
	modelmocks "librevita.org/internal/test/mock/domain/clinic/model"
	"librevita.org/pkg/log"
)

func TestHostMiddleware(t *testing.T) {
	logger := log.Nop()
	cfg := &config.Config{BaseDomain: "lv.test"}
	norteID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")

	cases := []struct {
		name   string
		host   string
		path   string
		code   int
		slug   string
		apex   bool
		lookup bool
		found  bool
	}{
		{name: "apex", host: "lv.test", path: "/", code: http.StatusOK, apex: true},
		{name: "www", host: "www.lv.test", path: "/", code: http.StatusOK, apex: true},
		{name: "clinic", host: "norte.lv.test", path: "/", code: http.StatusOK, slug: "norte", lookup: true, found: true},
		{name: "unknown slug", host: "ghost.lv.test", path: "/", code: http.StatusNotFound, slug: "ghost", lookup: true},
		{name: "foreign host", host: "evil.com", path: "/", code: http.StatusBadRequest},
		{name: "healthz skips", host: "evil.com", path: "/healthz", code: http.StatusOK},
		{name: "static skips", host: "evil.com", path: "/static/app.css", code: http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clinics := modelmocks.NewMockRepository(t)
			if tc.lookup {
				var row *model.Clinic
				if tc.found {
					row = &model.Clinic{ID: norteID, Slug: "norte", Name: "Norte", Timezone: "America/Sao_Paulo"}
				}
				clinics.EXPECT().GetBySlug(mock.Anything, tc.slug).Return(row, nil).Once()
			}

			e := echo.New()
			e.Pre(clinichttp.HostMiddleware(cfg, clinics, nil, logger))
			handler := func(c echo.Context) error {
				ctx := c.Request().Context()
				if tc.apex {
					assert.True(t, clinicctx.IsApex(ctx))
					_, ok := clinicctx.FromContext(ctx)
					assert.False(t, ok)
				}
				if tc.found {
					got, ok := clinicctx.FromContext(ctx)
					require.True(t, ok)
					assert.Equal(t, norteID, got.ID)
					assert.Equal(t, "norte", got.Slug)
				}
				return c.NoContent(http.StatusOK)
			}
			e.GET("/", handler)
			e.GET("/healthz", handler)
			e.GET("/static/app.css", handler)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Host = tc.host
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, tc.code, rec.Code)
		})
	}
}

func TestHostMiddlewareWithCrypto(t *testing.T) {
	logger := log.Nop()
	cfg := &config.Config{BaseDomain: "lv.test"}
	clinicID := ident.MustParseClinic("01990000-0000-7000-8000-0000000000a1")

	v, err := keystore.OpenBBolt(filepath.Join(t.TempDir(), "keystore.db"))
	require.NoError(t, err)
	defer func() { _ = v.Close() }()

	engine, err := crypto.NewMasterKey("nAmIvOXVc0vb6M9G7P9q2j2yK1WxP3sJ8q5dR4tU6wA=", v) // gitleaks:allow
	require.NoError(t, err)

	clinics := modelmocks.NewMockRepository(t)
	clinics.EXPECT().GetBySlug(mock.Anything, "norte").Return(&model.Clinic{
		ID: clinicID, Slug: "norte", Name: "Norte", Timezone: "America/Sao_Paulo",
	}, nil).Once()

	e := echo.New()
	e.Pre(clinichttp.HostMiddleware(cfg, clinics, engine, logger))
	e.GET("/", func(c echo.Context) error {
		ctx := c.Request().Context()
		enc, ok := fle.EncryptorFromContext(ctx)
		require.True(t, ok)
		require.NotNil(t, enc)
		h, ok := fle.HasherFromContext(ctx)
		require.True(t, ok)
		require.NotNil(t, h)
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "norte.lv.test"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
