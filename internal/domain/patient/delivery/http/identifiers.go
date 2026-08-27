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
	identifiermodel "librevita.org/internal/domain/identifier/model"
	identifierusecase "librevita.org/internal/domain/identifier/usecase"
	"librevita.org/internal/domain/patient/delivery/views"
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
		patientIDs := make([]string, 0, len(hits))
		seen := make(map[string]struct{}, len(hits))
		for _, hit := range hits {
			if _, ok := seen[hit.PatientID]; ok {
				continue
			}
			seen[hit.PatientID] = struct{}{}
			patientIDs = append(patientIDs, hit.PatientID)
		}
		patients, err := h.svc.GetMany(ctx, clinicID, patientIDs)
		if err != nil {
			return err
		}
		byID := make(map[string]*usecase.Patient, len(patients))
		for i := range patients {
			patient := patients[i]
			byID[patient.ID.String()] = &patient
		}
		lookupHits := make([]views.LookupHit, 0, len(hits))
		for _, hit := range hits {
			pt, ok := byID[hit.PatientID]
			if !ok {
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
	in := identifierusecase.Input{
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
		var v *identifiermodel.ValidationError
		switch {
		case errors.As(err, &v):
			return server.Render(c, http.StatusOK, views.IdentifierForm(
				server.CSRFToken(c, h.csrf), pt.ID.String(), h.systemOptions(ctx), identifierFormValues(in), v.Msg))
		case errors.Is(err, identifiermodel.ErrDuplicate):
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
func (h *Handler) identifierDuplicate(c echo.Context, pt *usecase.Patient, in identifierusecase.Input) error {
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
		if errors.Is(err, identifiermodel.ErrNotFound) {
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

// identifierRows decorates decrypted identifiers for display, masked.
func (h *Handler) identifierRows(ctx context.Context, rows []*identifiermodel.Identifier) []views.IdentifierRow {
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

func identifierFormValues(in identifierusecase.Input) views.IdentifierFormValues {
	return views.IdentifierFormValues{System: in.System, Value: in.Value}
}
