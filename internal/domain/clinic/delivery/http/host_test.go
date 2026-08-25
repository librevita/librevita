package http_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/config"
	clinichttp "librevita.org/internal/domain/clinic/delivery/http"
	"librevita.org/internal/domain/clinic/model"
	modelmocks "librevita.org/tests/mocks/domain/clinic/model"
)

func TestHostMiddleware(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	cfg := &config.Config{BaseDomain: "lv.test"}
	norteID := uuid.MustParse("01990000-0000-7000-8000-0000000000a1")

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
			e.Pre(clinichttp.HostMiddleware(cfg, clinics, nil, log))
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
