package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/patient/delivery/views"
	"librevita.org/internal/domain/patient/identifier"
	patientmodel "librevita.org/internal/domain/patient/model"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/ui/components"
)

// minLookupLen bounds the exact-document search: shorter values can
// match the raw fallback by accident and only help an enumerator.
const minLookupLen = 4

// IdentifierLookup answers the exact-document search of the registry
// toolbar. One hit navigates to the patient; several hits (the same
// value registered under different systems) render a link list; no hit
// renders the empty state. The plaintext the caller typed is never
// echoed back, and the audit detail carries only the hit count.
func (h *Handler) IdentifierLookup(c echo.Context) error {
	value := strings.TrimSpace(c.QueryParam("value"))
	if len(value) < minLookupLen {
		return server.Render(c, http.StatusOK, views.IdentifierLookupEmpty("Type at least "+strconv.Itoa(minLookupLen)+" characters"))
	}
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	hits, err := h.ids.FindByValue(ctx, clinicID, value)
	if err != nil {
		return err
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"identifier.search", "", "", "hits: "+strconv.Itoa(len(hits))))

	switch len(hits) {
	case 0:
		return server.Render(c, http.StatusOK, views.IdentifierLookupEmpty("No patient holds this document"))
	case 1:
		return server.HtmxRedirect(c, "/patients/"+hits[0].PatientID)
	default:
		lookupHits := make([]views.LookupHit, 0, len(hits))
		for _, hit := range hits {
			pt, err := h.svc.Get(ctx, clinicID, hit.PatientID)
			if err != nil {
				continue
			}
			lookupHits = append(lookupHits, views.LookupHit{
				PatientID:   hit.PatientID,
				DisplayName: pt.DisplayName,
				SystemName:  h.displayNameFor(ctx, hit.System),
			})
		}
		return server.Render(c, http.StatusOK, views.IdentifierLookupHits(lookupHits))
	}
}

// IdentifierAdd registers one identification document on the patient.
// The patient id comes from the path (already policy-filtered), never
// from the form, so the document is bound to the patient the caller is
// allowed to edit (IDOR protection). The audit detail carries the
// system URN only, never the value.
func (h *Handler) IdentifierAdd(c echo.Context) error {
	ctx := c.Request().Context()
	pt, err := h.patientOr404(c)
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, uuidStrPtr(pt.CreatedBy), pt.ID.String(), patientmodel.PatientStatus(pt.Status)); err != nil {
		return err
	}
	in := identifier.Input{
		PatientID: pt.ID.String(),
		System:    strings.TrimSpace(c.FormValue("system")),
		Value:     c.FormValue("value"),
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	created, err := h.ids.AddIdentifier(ctx, clinicID, server.ActorID(c), in)
	if err != nil {
		var v *identifier.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusOK, views.IdentifierForm(
				server.CSRFToken(c, h.csrf), pt.ID.String(), h.systemOptions(ctx), identifierFormValues(in), v.Msg))
		case errors.Is(err, identifier.ErrDuplicate):
			return h.identifierDuplicate(c, pt, in)
		default:
			return err
		}
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"identifier.create", "patient:"+pt.ID.String(), created.System, ""))
	return server.HtmxRedirect(c, "/patients/"+pt.ID.String())
}

// identifierDuplicate renders the add form with the duplicate message:
// the owner is looked up in the clinic (the global unique index may
// hold a document from another clinic, in which case only a generic
// message is shown).
func (h *Handler) identifierDuplicate(c echo.Context, pt *usecase.Patient, in identifier.Input) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	msg := "This document is already registered"
	owner, err := h.ids.FindByValue(ctx, clinicID, in.Value)
	if err == nil && len(owner) > 0 && owner[0].PatientID == pt.ID.String() {
		msg = "This patient already has this document"
	} else if err == nil && len(owner) > 0 {
		ownerPt, err := h.svc.Get(ctx, clinicID, owner[0].PatientID)
		if err == nil {
			msg = "This document belongs to " + ownerPt.DisplayName + " — open their record or use another document"
		}
	}
	return server.Render(c, http.StatusOK, views.IdentifierForm(
		server.CSRFToken(c, h.csrf), pt.ID.String(), h.systemOptions(ctx), identifierFormValues(in), msg))
}

// IdentifierRemove deletes one identification document. The response
// is the refreshed identifiers section, so the removed item disappears
// at once; errors surface as an OOB alert.
func (h *Handler) IdentifierRemove(c echo.Context) error {
	ctx := c.Request().Context()
	pt, err := h.patientOr404(c)
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, uuidStrPtr(pt.CreatedBy), pt.ID.String(), patientmodel.PatientStatus(pt.Status)); err != nil {
		return err
	}
	identifierID, err := uuid.Parse(c.Param("identifierID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}

	rows, err := h.ids.List(ctx, clinicID, pt.ID.String())
	if err != nil {
		return err
	}
	system := ""
	for _, row := range rows {
		if row.ID == identifierID.String() {
			system = row.System
			break
		}
	}

	if err := h.ids.Remove(ctx, clinicID, pt.ID.String(), identifierID.String()); err != nil {
		if errors.Is(err, identifier.ErrNotFound) {
			return server.Render(c, http.StatusOK, components.Alert("The document no longer exists", true))
		}
		return err
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		"identifier.remove", "patient:"+pt.ID.String(), system, ""))

	rows, err = h.ids.List(ctx, clinicID, pt.ID.String())
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.PatientIdentifiersSection(
		server.CSRFToken(c, h.csrf), pt.ID.String(), h.identifierRows(ctx, rows), h.systemOptions(ctx), ""))
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
		var v *identifier.ValidationError
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
		if errors.Is(err, identifier.ErrSystemNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	values.System = existing.System
	updated, err := h.systems.Update(ctx, id.String(), systemInput(values))
	if err != nil {
		var v *identifier.ValidationError
		if errors.As(err, &v) {
			return server.Render(c, http.StatusOK, views.SystemForm(server.CSRFToken(c, h.csrf), id.String(), values, v.Msg))
		}
		if errors.Is(err, identifier.ErrSystemNotFound) {
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
		if errors.Is(err, identifier.ErrSystemNotFound) {
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

// identifierRows decorates decrypted identifiers for display, masked.
func (h *Handler) identifierRows(ctx context.Context, rows []*identifier.Identifier) []views.IdentifierRow {
	out := make([]views.IdentifierRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, views.IdentifierRow{
			ID:         row.ID,
			System:     row.System,
			SystemName: h.displayNameFor(ctx, row.System),
			Value:      row.Value,
			Masked:     views.MaskValue(row.Value),
		})
	}
	return out
}

// displayNameFor resolves the display name of a system URN; unknown or
// deactivated systems fall back to the URN itself.
func (h *Handler) displayNameFor(ctx context.Context, system string) string {
	systems, err := h.systems.List(ctx)
	if err != nil {
		return system
	}
	for _, s := range systems {
		if s.System == system {
			return s.DisplayName
		}
	}
	return system
}

// systemOptions lists the active systems for the add form select.
func (h *Handler) systemOptions(ctx context.Context) []views.SystemOption {
	systems, err := h.systems.List(ctx)
	if err != nil {
		return nil
	}
	out := make([]views.SystemOption, 0, len(systems))
	for _, s := range systems {
		if s.Active {
			out = append(out, views.SystemOption{System: s.System, DisplayName: s.DisplayName, Mask: s.Mask})
		}
	}
	return out
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
func systemRowView(s *identifier.IdentifierSystem) views.SystemRow {
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
func checkLabel(s *identifier.IdentifierSystem) string {
	switch s.CheckAlgorithm {
	case identifier.CheckMod11Desc:
		return "mod11 (" + strconv.Itoa(s.CheckBaseLen) + "+" + strconv.Itoa(s.CheckDVCount) + " dv)"
	case identifier.CheckMod11Cyclic:
		return "mod11 cyclic (" + strconv.Itoa(s.CheckBaseLen) + "+1 dv)"
	default:
		return "none"
	}
}

func identifierFormValues(in identifier.Input) views.IdentifierFormValues {
	return views.IdentifierFormValues{System: in.System, Value: in.Value}
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

func systemInput(values views.SystemFormValues) identifier.SystemInput {
	toInt := func(s string, def int) int {
		n, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		return n
	}
	return identifier.SystemInput{
		System:           values.System,
		DisplayName:      values.DisplayName,
		Pattern:          values.Pattern,
		Mask:             values.Mask,
		Transform:        identifier.Transform(values.Transform),
		CheckAlgorithm:   identifier.CheckAlgorithm(values.CheckAlgorithm),
		CheckBaseLen:     toInt(values.CheckBaseLen, 0),
		CheckDVCount:     toInt(values.CheckDVCount, 1),
		CheckStartWeight: toInt(values.CheckStartWeight, 10),
	}
}
