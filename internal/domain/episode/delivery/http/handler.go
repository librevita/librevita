package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/clinicctx"
	"librevita.org/internal/core/server"
	clinicmodel "librevita.org/internal/domain/clinic/model"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	"librevita.org/internal/domain/episode/delivery/views"
	episodemodel "librevita.org/internal/domain/episode/model"
	"librevita.org/internal/domain/episode/usecase"
	patientusecase "librevita.org/internal/domain/patient/usecase"
	"librevita.org/pkg/ident"
)

// Handler renders SOAP chart pages nested under the patient record.
type Handler struct {
	svc      *usecase.Service
	patients *patientusecase.Service
	clocks   *clinicusecase.ClockProvider
	csrf     *auth.CSRF
	audit    *audit.Logger
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, patients *patientusecase.Service,
	clocks *clinicusecase.ClockProvider, csrf *auth.CSRF, auditLogger *audit.Logger) *Handler {
	return &Handler{svc: svc, patients: patients, clocks: clocks, csrf: csrf, audit: auditLogger}
}

// List is the chart fragment on the patient detail page.
func (h *Handler) List(c echo.Context) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	if !server.IsHtmx(c) {
		return c.Redirect(http.StatusFound, "/patients/"+patientID.String())
	}
	list, err := h.svc.ListByPatient(c.Request().Context(), principal, clinicID, patientID)
	if err != nil {
		return h.httpError(err)
	}
	clock, err := h.userClock(c.Request().Context())
	if err != nil {
		return err
	}
	rows := make([]views.EpisodeRow, 0, len(list))
	for _, ep := range list {
		rows = append(rows, views.EpisodeRow{
			ID:       ep.ID.String(),
			When:     clock.FormatStored(ep.OccurredAt),
			Type:     views.TypeLabel(ep.Type.String()),
			Status:   views.StatusLabel(ep.Status.String()),
			Draft:    ep.Status == episodemodel.EpisodeStatusDraft,
			CanAmend: ep.CanAmend(),
		})
	}
	return server.Render(c, http.StatusOK, views.ChartSection(
		server.CSRFToken(c, h.csrf), patientID.String(), canWrite(principal), rows))
}

// NewPage renders the create form.
func (h *Handler) NewPage(c echo.Context) error {
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	clock, err := h.userClock(c.Request().Context())
	if err != nil {
		return err
	}
	name, err := h.patientName(c.Request().Context(), clinicID, patientID)
	if err != nil {
		return err
	}
	values := views.EpisodeFormValues{
		Type:       string(episodemodel.EpisodeTypeConsultation),
		Class:      string(episodemodel.CareSettingAmbulatory),
		OccurredAt: clock.Format(clock.Now(), occurredLayout),
	}
	return server.Render(c, http.StatusOK, views.EpisodeFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), patientID.String(), "", name, "", values))
}

// Create inserts a draft note.
func (h *Handler) Create(c echo.Context) error {
	return h.save(c, ident.EpisodeID{})
}

// EditPage renders the draft editor.
func (h *Handler) EditPage(c echo.Context) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	ep, err := h.load(c, clinicID, patientID)
	if err != nil {
		return err
	}
	if ep.Status != episodemodel.EpisodeStatusDraft {
		return c.Redirect(http.StatusFound, "/patients/"+patientID.String()+"/episodes/"+ep.ID.String())
	}
	clock, err := h.userClock(c.Request().Context())
	if err != nil {
		return err
	}
	name, err := h.patientName(c.Request().Context(), clinicID, patientID)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.EpisodeFormPage(
		server.CSRFToken(c, h.csrf), principal, patientID.String(), ep.ID.String(), name, "",
		valuesFromEpisode(*ep, clock)))
}

// Update saves a draft, optionally finalizing.
func (h *Handler) Update(c echo.Context) error {
	episodeID, err := ident.ParseEpisode(c.Param("episodeID"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	return h.save(c, episodeID)
}

// View renders a read-only SOAP note.
func (h *Handler) View(c echo.Context) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	ep, err := h.load(c, clinicID, patientID)
	if err != nil {
		return err
	}
	clock, err := h.userClock(c.Request().Context())
	if err != nil {
		return err
	}
	name, err := h.patientName(c.Request().Context(), clinicID, patientID)
	if err != nil {
		return err
	}
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		"chart.view", "episode:"+ep.ID.String(), ep.ID.String(), ep.Status.String()))
	return server.Render(c, http.StatusOK, views.EpisodeViewPage(
		server.CSRFToken(c, h.csrf), principal, patientID.String(), name, toView(*ep, clock), canWrite(principal)))
}

// Finalize locks a draft from the view page.
func (h *Handler) Finalize(c echo.Context) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	ep, err := h.load(c, clinicID, patientID)
	if err != nil {
		return err
	}
	saved, err := h.svc.Finalize(c.Request().Context(), principal, clinicID, ep.ID)
	if err != nil {
		return h.httpError(err)
	}
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		"chart.finalize", "episode:"+saved.ID.String(), saved.ID.String(), saved.Status.String()))
	return c.Redirect(http.StatusFound, "/patients/"+patientID.String()+"/episodes/"+saved.ID.String())
}

// Amend starts a new draft from a finalized note.
func (h *Handler) Amend(c echo.Context) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	ep, err := h.load(c, clinicID, patientID)
	if err != nil {
		return err
	}
	saved, err := h.svc.Amend(c.Request().Context(), principal, clinicID, ep.ID)
	if err != nil {
		return h.httpError(err)
	}
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		"chart.create", "episode:"+saved.ID.String(), saved.ID.String(), "amend:"+ep.ID.String()))
	return c.Redirect(http.StatusFound, "/patients/"+patientID.String()+"/episodes/"+saved.ID.String()+"/edit")
}

func (h *Handler) save(c echo.Context, episodeID ident.EpisodeID) error {
	principal := server.Principal(c)
	clinicID, patientID, err := h.ids(c)
	if err != nil {
		return err
	}
	clock, err := h.userClock(c.Request().Context())
	if err != nil {
		return err
	}
	values := parseForm(c)
	if add := c.FormValue("add"); add != "" {
		name, err := h.patientName(c.Request().Context(), clinicID, patientID)
		if err != nil {
			return err
		}
		idStr := ""
		if !episodeID.IsZero() {
			idStr = episodeID.String()
		}
		return server.Render(c, http.StatusOK, views.EpisodeFormPage(
			server.CSRFToken(c, h.csrf), principal, patientID.String(), idStr, name, "", applyAdd(values, add)))
	}
	authorID, err := ident.ParseUser(principal.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError)
	}
	ep := episodeFromForm(values, clinicID, patientID, authorID, clock)
	ep.ID = episodeID
	var saved *usecase.Episode
	action := "chart.create"
	if episodeID.IsZero() {
		saved, err = h.svc.Create(c.Request().Context(), principal, ep)
	} else {
		saved, err = h.svc.UpdateDraft(c.Request().Context(), principal, ep)
		action = "chart.update"
	}
	if err != nil {
		name, nerr := h.patientName(c.Request().Context(), clinicID, patientID)
		if nerr != nil {
			return nerr
		}
		idStr := ""
		if !episodeID.IsZero() {
			idStr = episodeID.String()
		}
		return server.Render(c, http.StatusBadRequest, views.EpisodeFormPage(
			server.CSRFToken(c, h.csrf), principal, patientID.String(), idStr, name, formError(err), values))
	}
	if c.FormValue("finalize") == "1" {
		saved, err = h.svc.Finalize(c.Request().Context(), principal, clinicID, saved.ID)
		if err != nil {
			return h.httpError(err)
		}
		action = "chart.finalize"
	}
	h.audit.Record(c.Request().Context(), server.EventFromRequest(c, audit.AuditResultSuccess,
		action, "episode:"+saved.ID.String(), saved.ID.String(), saved.Status.String()))
	return c.Redirect(http.StatusFound, "/patients/"+patientID.String()+"/episodes/"+saved.ID.String())
}

func (h *Handler) load(c echo.Context, clinicID ident.ClinicID, patientID ident.PatientID) (*usecase.Episode, error) {
	episodeID, err := ident.ParseEpisode(c.Param("episodeID"))
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	ep, err := h.svc.Get(c.Request().Context(), server.Principal(c), clinicID, episodeID)
	if err != nil {
		return nil, h.httpError(err)
	}
	if ep.PatientID != patientID {
		return nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return ep, nil
}

func (h *Handler) ids(c echo.Context) (clinicID ident.ClinicID, patientID ident.PatientID, err error) {
	clinicID, err = clinicctx.MustClinicID(c.Request().Context())
	if err != nil {
		return ident.ClinicID{}, ident.PatientID{}, echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	patientID, err = ident.ParsePatient(c.Param("id"))
	if err != nil {
		return ident.ClinicID{}, ident.PatientID{}, echo.NewHTTPError(http.StatusNotFound)
	}
	return clinicID, patientID, nil
}

func (h *Handler) patientName(ctx context.Context, clinicID ident.ClinicID, patientID ident.PatientID) (string, error) {
	pt, err := h.patients.Get(ctx, clinicID.String(), patientID.String())
	if err != nil {
		if errors.Is(err, patientusecase.ErrNotFound) {
			return "", echo.NewHTTPError(http.StatusNotFound)
		}
		return "", err
	}
	return pt.DisplayName, nil
}

func (h *Handler) userClock(ctx context.Context) (*clinicmodel.Clock, error) {
	tz := ""
	if p := server.PrincipalCtx(ctx); p != nil {
		tz = p.Timezone
	}
	return h.clocks.ClockFor(ctx, tz)
}

func (h *Handler) httpError(err error) error {
	switch {
	case errors.Is(err, episodemodel.ErrNotFound), errors.Is(err, episodemodel.ErrPatientGone):
		return echo.NewHTTPError(http.StatusNotFound)
	case errors.Is(err, episodemodel.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden)
	case errors.Is(err, episodemodel.ErrNotDraft), errors.Is(err, episodemodel.ErrNotFinalized),
		errors.Is(err, episodemodel.ErrAlreadyAmended):
		return echo.NewHTTPError(http.StatusConflict)
	case errors.Is(err, episodemodel.ErrInvalidSOAP):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	default:
		return err
	}
}

func formError(err error) string {
	if errors.Is(err, episodemodel.ErrInvalidSOAP) {
		return "Check the note fields and try again."
	}
	if errors.Is(err, episodemodel.ErrNotDraft) {
		return "This note is no longer a draft."
	}
	if errors.Is(err, episodemodel.ErrForbidden) {
		return "You cannot write this chart."
	}
	return "Could not save the note."
}

func canWrite(p *auth.Principal) bool {
	return p != nil && (p.Role == auth.RoleAdmin || p.Role == auth.RolePhysician)
}

func toView(ep episodemodel.Episode, clock *clinicmodel.Clock) views.EpisodeView {
	v := views.EpisodeView{
		ID:         ep.ID.String(),
		Type:       views.TypeLabel(ep.Type.String()),
		Class:      views.ClassLabel(ep.Class.String()),
		Status:     views.StatusLabel(ep.Status.String()),
		When:       clock.FormatStored(ep.OccurredAt),
		Draft:      ep.Status == episodemodel.EpisodeStatusDraft,
		Finalized:  ep.Status == episodemodel.EpisodeStatusFinalized,
		CanAmend:   ep.CanAmend(),
		Subjective: ep.SOAP.Subjective,
		Objective:  ep.SOAP.Objective,
		Assessment: ep.SOAP.Assessment,
		Plan:       ep.SOAP.Plan,
	}
	if ep.PredecessorID != nil {
		v.PredecessorID = ep.PredecessorID.String()
	}
	for _, f := range ep.Findings {
		line := strings.TrimSpace(f.Code.Display + " " + f.Code.Code)
		if t := findingValueText(f); t != "" {
			line = strings.TrimSpace(line + " " + t)
		}
		v.Findings = append(v.Findings, line)
	}
	for _, p := range ep.Problems {
		line := strings.TrimSpace(p.Code.Display + " " + p.Code.Code)
		if p.Text != "" {
			if line != "" {
				line += " — "
			}
			line += p.Text
		}
		v.Problems = append(v.Problems, line)
	}
	for _, item := range ep.PlanItems {
		line := views.PlanKindLabel(item.Kind.String())
		if item.Description != "" {
			line += ": " + item.Description
		}
		v.PlanItems = append(v.PlanItems, line)
	}
	return v
}
