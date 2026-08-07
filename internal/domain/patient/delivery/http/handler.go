// Package http exposes the patient registry web routes.
package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/clinic"
	"librevita.org/internal/domain/patient/delivery/views"
	"librevita.org/internal/domain/patient/repository"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/ui/components"
)

// utcMilliLayout matches the timestamps written by the database.
const utcMilliLayout = "2006-01-02T15:04:05.000Z"

const patientListLimit = 50

// Handler renders the patient pages and processes submissions.
type Handler struct {
	svc    *usecase.Service
	clocks *clinic.ClockProvider
	csrf   *auth.CSRF
	audit  *audit.Logger
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, clocks *clinic.ClockProvider,
	csrf *auth.CSRF, auditLogger *audit.Logger) *Handler {
	return &Handler{svc: svc, clocks: clocks, csrf: csrf, audit: auditLogger}
}

// List renders the registry page or, for htmx requests, only the table
// fragment (search).
func (h *Handler) List(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	q := c.QueryParam("q")
	patients, err := h.svc.List(ctx, clinicID, q, patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(c.Request().Context(), patients)

	if server.IsHtmx(c) {
		return render(c, http.StatusOK, views.PatientListTable(rows, ""))
	}
	return render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, rows, ""))
}

// NewPage renders the create form.
func (h *Handler) NewPage(c echo.Context) error {
	return render(c, http.StatusOK, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), "", views.PatientFormValues{}, ""))
}

// Create validates and inserts a patient, then navigates to its detail.
func (h *Handler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	input := h.input(c)
	patient, err := h.svc.Create(ctx, clinicID, input)
	if err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return render(c, http.StatusBadRequest, views.PatientFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), "", values(input), v.Msg))
		}
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: actorID(c), ActorMail: actorMail(c),
		Action: "patient.create", Resource: "patient:" + patient.ID,
		Result: audit.ResultSuccess,
		IP:     c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/patients/"+patient.ID)
}

// Detail renders the patient record.
func (h *Handler) Detail(c echo.Context) error {
	ctx := c.Request().Context()
	patient, err := h.svc.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	pt := h.detailView(patient, clock)
	return render(c, http.StatusOK, views.PatientDetailPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), pt, ""))
}

// EditPage renders the edit form with the stored values.
func (h *Handler) EditPage(c echo.Context) error {
	ctx := c.Request().Context()
	patient, err := h.svc.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	return render(c, http.StatusOK, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), patient.ID, values(patientInput(patient)), ""))
}

// Update applies the edited values and navigates to the detail page.
func (h *Handler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	input := h.input(c)
	patient, err := h.svc.Update(ctx, c.Param("id"), input)
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return render(c, http.StatusBadRequest, views.PatientFormPage(
				server.CSRFToken(c, h.csrf), server.Principal(c), c.Param("id"), values(input), v.Msg))
		case errors.Is(err, usecase.ErrNotFound):
			return echo.NewHTTPError(http.StatusNotFound)
		default:
			return err
		}
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: actorID(c), ActorMail: actorMail(c),
		Action: "patient.update", Resource: "patient:" + patient.ID,
		Result: audit.ResultSuccess,
		IP:     c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/patients/"+patient.ID)
}

// Archive sets the patient inactive. htmx responses carry only an OOB
// alert so the row is removed from the list.
func (h *Handler) Archive(c echo.Context) error {
	return h.setStatus(c, usecase.StatusInactive, "Patient archived")
}

// Restore sets the patient active again.
func (h *Handler) Restore(c echo.Context) error {
	return h.setStatus(c, usecase.StatusActive, "Patient restored")
}

func (h *Handler) setStatus(c echo.Context, status, successMsg string) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	err := h.svc.SetStatus(ctx, id, status)
	if err == nil {
		h.audit.Record(ctx, audit.Event{
			ActorID: actorID(c), ActorMail: actorMail(c),
			Action: "patient.status", Resource: "patient:" + id,
			Result: audit.ResultSuccess, Detail: status,
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}
	if server.IsHtmx(c) {
		if err != nil {
			return render(c, http.StatusBadRequest, components.Alert("Could not update the patient", true))
		}
		return render(c, http.StatusOK, components.Alert(successMsg, true))
	}
	if err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return c.String(http.StatusBadRequest, v.Msg)
		}
		return err
	}
	return c.Redirect(http.StatusFound, "/patients")
}

func (h *Handler) clinicID(ctx context.Context) (string, error) {
	profile, err := h.clocks.Profile(ctx)
	if err != nil {
		return "", err
	}
	return profile.ID, nil
}

func (h *Handler) input(c echo.Context) usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: c.FormValue("display_name"),
		BirthDate:   c.FormValue("birth_date"),
		Sex:         c.FormValue("sex"),
		Document:    c.FormValue("document"),
		Phone:       c.FormValue("phone"),
		Email:       c.FormValue("email"),
		Street:      c.FormValue("street"),
		City:        c.FormValue("city"),
		State:       c.FormValue("state"),
		PostalCode:  c.FormValue("postal_code"),
		Notes:       c.FormValue("notes"),
	}
}

func (h *Handler) rows(ctx context.Context, patients []repository.Patient) []views.PatientRow {
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		clock = clinic.NewClock(clinic.DefaultTimezone)
	}
	out := make([]views.PatientRow, 0, len(patients))
	for _, pt := range patients {
		out = append(out, views.PatientRow{
			ID:          pt.ID,
			DisplayName: pt.DisplayName,
			BirthDate:   pt.BirthDate.String,
			Sex:         pt.Sex,
			Document:    pt.Document.String,
			Phone:       pt.Phone.String,
			Email:       pt.Email.String,
			City:        pt.City.String,
			Status:      pt.Status,
			CreatedAt:   formatStored(clock, pt.CreatedAt),
		})
	}
	return out
}

func (h *Handler) detailView(patient *repository.Patient, clock *clinic.Clock) *repository.Patient {
	clone := *patient
	clone.CreatedAt = formatStored(clock, clone.CreatedAt)
	clone.UpdatedAt = formatStored(clock, clone.UpdatedAt)
	return &clone
}

func values(in usecase.PatientInput) views.PatientFormValues {
	return views.PatientFormValues{
		DisplayName: in.DisplayName,
		BirthDate:   in.BirthDate,
		Sex:         in.Sex,
		Document:    in.Document,
		Phone:       in.Phone,
		Email:       in.Email,
		Street:      in.Street,
		City:        in.City,
		State:       in.State,
		PostalCode:  in.PostalCode,
		Notes:       in.Notes,
	}
}

// formatStored renders a database timestamp in the clinic timezone,
// keeping the raw value when it cannot be parsed.
func formatStored(clock *clinic.Clock, stored string) string {
	t, err := time.Parse(utcMilliLayout, stored)
	if err != nil {
		return stored
	}
	return clock.FormatUI(t)
}

func patientInput(pt *repository.Patient) usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: pt.DisplayName,
		BirthDate:   pt.BirthDate.String,
		Sex:         pt.Sex,
		Document:    pt.Document.String,
		Phone:       pt.Phone.String,
		Email:       pt.Email.String,
		Street:      pt.Street.String,
		City:        pt.City.String,
		State:       pt.State.String,
		PostalCode:  pt.PostalCode.String,
		Notes:       pt.Notes.String,
	}
}

func actorID(c echo.Context) string {
	if p := server.Principal(c); p != nil {
		return p.ID
	}
	return ""
}

func actorMail(c echo.Context) string {
	if p := server.Principal(c); p != nil {
		return p.Email
	}
	return ""
}

func render(c echo.Context, status int, comp templ.Component) error {
	c.Response().Status = status
	return comp.Render(c.Request().Context(), c.Response())
}
