package fhir

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/server"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	"librevita.org/pkg/log"
)

// Handler serves the SOAP FHIR R4 facade.
type Handler struct {
	svc   *usecase.Service
	audit *audit.Logger
	log   log.Logger
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, auditLogger *audit.Logger, logger log.Logger) *Handler {
	return &Handler{svc: svc, audit: auditLogger, log: logger}
}

// Metadata returns the CapabilityStatement.
func (h *Handler) Metadata(c echo.Context) error {
	base := fhirBase(c)
	return writeFHIR(c, http.StatusOK, ServerCapability(base))
}

// CreateBundle creates or replaces a draft SOAP document, optionally finalizing.
func (h *Handler) CreateBundle(c echo.Context) error {
	principal := server.Principal(c)
	if principal == nil {
		return writeOutcome(c, http.StatusUnauthorized, "error", "login", "not authenticated")
	}
	clinicID, err := clinicctx.MustClinicID(c.Request().Context())
	if err != nil {
		return writeOutcome(c, http.StatusBadRequest, "error", "invalid", err.Error())
	}
	var bundle Bundle
	if err := json.NewDecoder(c.Request().Body).Decode(&bundle); err != nil {
		return writeOutcome(c, http.StatusBadRequest, "error", "structure", "invalid JSON Bundle")
	}
	ep, err := FromDocumentBundle(&bundle)
	if err != nil {
		return writeOutcome(c, http.StatusBadRequest, "error", "invalid", err.Error())
	}
	authorID, err := uuid.Parse(principal.ID)
	if err != nil {
		return writeOutcome(c, http.StatusInternalServerError, "fatal", "exception", "invalid principal id")
	}
	ep.ClinicID = clinicID
	ep.AuthorID = authorID
	if ep.OccurredAt.IsZero() {
		ep.OccurredAt = time.Now().UTC()
	}

	ctx := c.Request().Context()
	var saved *usecase.Episode
	action := "chart.create"
	if ep.ID == uuid.Nil {
		saved, err = h.svc.Create(ctx, principal, *ep)
	} else {
		saved, err = h.svc.UpdateDraft(ctx, principal, *ep)
		if errors.Is(err, episodemodel.ErrNotFound) {
			saved, err = h.svc.Create(ctx, principal, *ep)
		} else if err == nil {
			action = "chart.update"
		}
	}
	if err != nil {
		return h.fhirError(c, err)
	}
	if WantFinalize(&bundle) {
		saved, err = h.svc.Finalize(ctx, principal, clinicID, saved.ID)
		if err != nil {
			return h.fhirError(c, err)
		}
		action = "chart.finalize"
	}
	h.audit.Record(ctx, server.EventFromRequest(c, audit.AuditResultSuccess,
		action, "episode:"+saved.ID.String(), saved.ID.String(), saved.Status.String()))
	out, err := ToDocumentBundle(*saved, DocumentContext{AuthorName: principal.Name})
	if err != nil {
		return h.fhirInternal(c, err)
	}
	status := http.StatusOK
	if action == "chart.create" {
		status = http.StatusCreated
		c.Response().Header().Set("Location", fhirBase(c)+"/Composition/"+saved.ID.String()+"/$document")
	}
	return writeFHIR(c, status, out)
}

// Document returns Composition/$document.
func (h *Handler) Document(c echo.Context) error {
	return h.writeEpisodeBundle(c, c.Param("id"))
}

// GetEncounter returns one Encounter.
func (h *Handler) GetEncounter(c echo.Context) error {
	ep, err := h.loadEpisode(c, c.Param("id"))
	if err != nil {
		return err
	}
	if ep == nil {
		return nil
	}
	bundle, err := ToDocumentBundle(*ep, DocumentContext{AuthorName: server.Principal(c).Name})
	if err != nil {
		return h.fhirInternal(c, err)
	}
	for _, e := range bundle.Entry {
		if PeekType(e.Resource) == "Encounter" {
			h.recordChartView(c, "episode:"+ep.ID.String(), ep.ID.String(), ep.Status.String())
			c.Response().Header().Set(echo.HeaderContentType, ContentTypeUTF8)
			return c.Blob(http.StatusOK, ContentType, e.Resource)
		}
	}
	return writeOutcome(c, http.StatusNotFound, "error", "not-found", "Encounter missing from document")
}

// SearchEncounter lists encounters for a patient.
func (h *Handler) SearchEncounter(c echo.Context) error {
	principal := server.Principal(c)
	if principal == nil {
		return writeOutcome(c, http.StatusUnauthorized, "error", "login", "not authenticated")
	}
	clinicID, err := clinicctx.MustClinicID(c.Request().Context())
	if err != nil {
		return writeOutcome(c, http.StatusBadRequest, "error", "invalid", err.Error())
	}
	patientID, err := uuid.Parse(strings.TrimPrefix(c.QueryParam("patient"), "Patient/"))
	if err != nil {
		return writeOutcome(c, http.StatusBadRequest, "error", "invalid", "patient query parameter is required")
	}
	list, err := h.svc.ListByPatient(c.Request().Context(), principal, clinicID, patientID)
	if err != nil {
		return h.fhirError(c, err)
	}
	out := Bundle{ResourceType: "Bundle", Type: "searchset"}
	for i := range list {
		b, err := ToDocumentBundle(list[i], DocumentContext{})
		if err != nil {
			return h.fhirInternal(c, err)
		}
		for _, e := range b.Entry {
			if PeekType(e.Resource) == "Encounter" {
				out.Entry = append(out.Entry, e)
				break
			}
		}
	}
	h.recordChartView(c, "patient:"+patientID.String(), "", "encounters:"+strconv.Itoa(len(out.Entry)))
	return writeFHIR(c, http.StatusOK, out)
}

func (h *Handler) writeEpisodeBundle(c echo.Context, rawID string) error {
	ep, err := h.loadEpisode(c, rawID)
	if err != nil {
		return err
	}
	if ep == nil {
		return nil
	}
	principal := server.Principal(c)
	name := ""
	if principal != nil {
		name = principal.Name
	}
	out, err := ToDocumentBundle(*ep, DocumentContext{AuthorName: name})
	if err != nil {
		return h.fhirInternal(c, err)
	}
	h.recordChartView(c, "episode:"+ep.ID.String(), ep.ID.String(), ep.Status.String())
	return writeFHIR(c, http.StatusOK, out)
}

func (h *Handler) recordChartView(c echo.Context, resource, resourceName, detail string) {
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		"chart.view", resource, resourceName, detail))
}

func (h *Handler) loadEpisode(c echo.Context, rawID string) (*usecase.Episode, error) {
	principal := server.Principal(c)
	if principal == nil {
		_ = writeOutcome(c, http.StatusUnauthorized, "error", "login", "not authenticated")
		return nil, nil
	}
	clinicID, err := clinicctx.MustClinicID(c.Request().Context())
	if err != nil {
		_ = writeOutcome(c, http.StatusBadRequest, "error", "invalid", err.Error())
		return nil, nil
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		_ = writeOutcome(c, http.StatusBadRequest, "error", "invalid", "invalid id")
		return nil, nil
	}
	ep, err := h.svc.Get(c.Request().Context(), principal, clinicID, id)
	if err != nil {
		_ = h.fhirError(c, err)
		return nil, nil
	}
	return ep, nil
}

func (h *Handler) fhirError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, episodemodel.ErrNotFound), errors.Is(err, episodemodel.ErrPatientGone):
		return writeOutcome(c, http.StatusNotFound, "error", "not-found", err.Error())
	case errors.Is(err, episodemodel.ErrForbidden):
		return writeOutcome(c, http.StatusForbidden, "error", "forbidden", err.Error())
	case errors.Is(err, episodemodel.ErrNotDraft), errors.Is(err, episodemodel.ErrNotFinalized),
		errors.Is(err, episodemodel.ErrAlreadyAmended):
		return writeOutcome(c, http.StatusConflict, "error", "conflict", err.Error())
	case errors.Is(err, episodemodel.ErrInvalidSOAP):
		return writeOutcome(c, http.StatusBadRequest, "error", "invalid", err.Error())
	default:
		return h.fhirInternal(c, err)
	}
}

func (h *Handler) fhirInternal(c echo.Context, err error) error {
	h.log.ErrorContext(c.Request().Context(), "fhir internal error",
		log.Error(err),
	)
	return writeOutcome(c, http.StatusInternalServerError, "fatal", "exception", "internal error")
}

func writeFHIR(c echo.Context, status int, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Blob(status, ContentType, body)
}

func writeOutcome(c echo.Context, status int, severity, code, diagnostics string) error {
	return writeFHIR(c, status, Outcome(severity, code, diagnostics))
}

func fhirBase(c echo.Context) string {
	req := c.Request()
	scheme := "https"
	if req.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + req.Host + "/fhir/r4"
}
