package http

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/identifier/delivery/views"
	identifiermodel "librevita.org/internal/domain/identifier/model"
	"librevita.org/internal/domain/identifier/usecase"
)

// Handler serves HTTP endpoints for administering identifier systems.
type Handler struct {
	systems usecase.SystemsService
	csrf    *auth.CSRF
	audit   *audit.Logger
}

// NewHandler creates a new HTTP handler for identifier systems.
func NewHandler(systems usecase.SystemsService, csrf *auth.CSRF, auditLogger *audit.Logger) *Handler {
	return &Handler{
		systems: systems,
		csrf:    csrf,
		audit:   auditLogger,
	}
}

// IdentifierSystemsPage lists the administrator catalog.
func (h *Handler) IdentifierSystemsPage(c echo.Context) error {
	rows, err := h.systemRows(c.Request().Context())
	if err != nil {
		return err
	}
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.IdentifierSystemsTable(server.CSRFToken(c, h.csrf), rows, ""))
	}
	return server.Render(c, http.StatusOK, views.IdentifierSystemsPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), rows, ""))
}

// IdentifierSystemCreate registers a new document system and reloads
// the registry, so the new system is usable immediately.
func (h *Handler) IdentifierSystemCreate(c echo.Context) error {
	ctx := c.Request().Context()
	values := systemFormValues(c)
	created, err := h.systems.Create(ctx, server.ActorID(c), systemInput(values))
	if err != nil {
		var v *identifiermodel.ValidationError
		if errors.As(err, &v) {
			return server.Render(c, http.StatusOK, views.SystemForm(server.CSRFToken(c, h.csrf), "", values, v.Msg))
		}
		return err
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"identifier.system.create", "", created.System, ""))
	return server.HtmxRedirect(c, "/identifier-systems")
}

// IdentifierSystemUpdate replaces a system definition. The URN is
// reused from the stored row: it is the identity of stored identifiers
// and cannot be renamed.
func (h *Handler) IdentifierSystemUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	values := systemFormValues(c)
	existing, err := h.systems.SystemByID(ctx, id.String())
	if err != nil {
		if errors.Is(err, identifiermodel.ErrSystemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	values.System = existing.System
	updated, err := h.systems.Update(ctx, id.String(), systemInput(values))
	if err != nil {
		var v *identifiermodel.ValidationError
		if errors.As(err, &v) {
			return server.Render(c, http.StatusOK, views.SystemForm(server.CSRFToken(c, h.csrf), id.String(), values, v.Msg))
		}
		if errors.Is(err, identifiermodel.ErrSystemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"identifier.system.update", "", updated.System, ""))
	return server.HtmxRedirect(c, "/identifier-systems")
}

// IdentifierSystemSetActive toggles the system, responding with the
// refreshed row so the toggle swaps in place.
func (h *Handler) IdentifierSystemSetActive(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	row, err := h.systems.SystemByID(ctx, id.String())
	if err != nil {
		if errors.Is(err, identifiermodel.ErrSystemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	if err := h.systems.SetActive(ctx, id.String(), !row.Active); err != nil {
		return err
	}
	row, err = h.systems.SystemByID(ctx, id.String())
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.SystemRowOnly(server.CSRFToken(c, h.csrf), systemRowView(row)))
}

// SystemCheckFields answers the conditional check-digit fields of the
// system form.
func (h *Handler) SystemCheckFields(c echo.Context) error {
	values := views.SystemFormValues{CheckAlgorithm: c.QueryParam("check_algorithm")}
	return server.Render(c, http.StatusOK, views.CheckFieldsPartial(values))
}

// systemRows decorates the catalog rows.
func (h *Handler) systemRows(ctx context.Context) ([]views.SystemRow, error) {
	systems, err := h.systems.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]views.SystemRow, 0, len(systems))
	for _, s := range systems {
		out = append(out, systemRowView(s))
	}
	return out, nil
}

// systemRowView decorates one stored system for the catalog.
func systemRowView(s *identifiermodel.IdentifierSystem) views.SystemRow {
	return views.SystemRow{
		ID:          s.ID.String(),
		System:      s.System,
		DisplayName: s.DisplayName,
		Pattern:     s.Pattern,
		Transform:   string(s.Transform),
		Check:       checkLabel(s),
		Active:      s.Active,
	}
}

// checkLabel renders the configured check digit as a short label.
func checkLabel(s *identifiermodel.IdentifierSystem) string {
	switch s.CheckAlgorithm {
	case identifiermodel.CheckMod11Desc:
		return "mod11 (" + strconv.Itoa(s.CheckBaseLen) + "+" + strconv.Itoa(s.CheckDVCount) + " dv)"
	case identifiermodel.CheckMod11Cyclic:
		return "mod11 cyclic (" + strconv.Itoa(s.CheckBaseLen) + "+1 dv)"
	default:
		return "none"
	}
}

func systemFormValues(c echo.Context) views.SystemFormValues {
	toInt := func(name string) string {
		return strings.TrimSpace(c.FormValue(name))
	}
	return views.SystemFormValues{
		System:           strings.TrimSpace(c.FormValue("system")),
		DisplayName:      strings.TrimSpace(c.FormValue("display_name")),
		Pattern:          strings.TrimSpace(c.FormValue("pattern")),
		Mask:             strings.TrimSpace(c.FormValue("mask")),
		Transform:        strings.TrimSpace(c.FormValue("transform")),
		CheckAlgorithm:   strings.TrimSpace(c.FormValue("check_algorithm")),
		CheckBaseLen:     toInt("check_base_len"),
		CheckDVCount:     toInt("check_dv_count"),
		CheckStartWeight: toInt("check_start_weight"),
	}
}

func systemInput(values views.SystemFormValues) usecase.SystemInput {
	toInt := func(s string, def int) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return n
	}
	return usecase.SystemInput{
		System:           values.System,
		DisplayName:      values.DisplayName,
		Pattern:          values.Pattern,
		Mask:             values.Mask,
		Transform:        identifiermodel.Transform(values.Transform),
		CheckAlgorithm:   identifiermodel.CheckAlgorithm(values.CheckAlgorithm),
		CheckBaseLen:     toInt(values.CheckBaseLen, 0),
		CheckDVCount:     toInt(values.CheckDVCount, 1),
		CheckStartWeight: toInt(values.CheckStartWeight, 10),
	}
}
