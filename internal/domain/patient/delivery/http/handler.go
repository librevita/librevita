// Package http exposes the patient registry web routes.
package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
	status := c.QueryParam("status")
	patients, err := h.svc.List(ctx, clinicID, q, status, patientListLimit)
	if err != nil {
		return err
	}
	h.sort(patients, c.QueryParam("sort"))
	rows := h.rows(c.Request().Context(), patients)

	// The search input and filters request fragments; boosted navigation
	// (sidebar links) also arrives with HX-Request but must render the
	// full page, so only non-boosted htmx requests get the fragment.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return render(c, http.StatusOK, views.PatientListTable(rows, ""))
	}
	return render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, status, c.QueryParam("sort"), rows, ""))
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
	patient, err := h.svc.Create(ctx, clinicID, actorID(c), input)
	if err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return h.formError(c, "", input, v.Msg)
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

// Detail renders the patient record with the registrar.
func (h *Handler) Detail(c echo.Context) error {
	ctx := c.Request().Context()
	row, err := h.svc.GetWithCreator(ctx, c.Param("id"))
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
	createdBy := ""
	if row.CreatedByEmail.Valid {
		createdBy = row.CreatedByEmail.String
	}
	pt := h.detailView(patientFromRow(row), clock)
	return render(c, http.StatusOK, views.PatientDetailPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), pt, createdBy, ""))
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
			return h.formError(c, c.Param("id"), input, v.Msg)
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

// BulkArchive archives the patients whose ids are in the form. The
// response is the refreshed table fragment plus an OOB alert, so the
// archived rows disappear at once.
func (h *Handler) BulkArchive(c echo.Context) error {
	ctx := c.Request().Context()
	ids := c.Request().PostForm["ids"]
	archived := 0
	for _, id := range ids {
		if err := h.svc.SetStatus(ctx, id, usecase.StatusInactive); err == nil {
			archived++
		}
	}
	if archived > 0 {
		h.audit.Record(ctx, audit.Event{
			ActorID: actorID(c), ActorMail: actorMail(c),
			Action: "patient.status", Resource: "patient:bulk",
			Result: audit.ResultSuccess, Detail: "archived " + strconv.Itoa(archived),
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}

	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	patients, err := h.svc.List(ctx, clinicID, c.QueryParam("q"), c.QueryParam("status"), patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(ctx, patients)
	msg := "No patients selected"
	if archived > 0 {
		msg = strconv.Itoa(archived) + " patient(s) archived"
	}
	return render(c, http.StatusOK, templList{
		table: views.PatientListTable(rows, ""),
		alert: components.Alert(msg, true),
	})
}

// templList renders a table fragment followed by an OOB alert in one
// response.
type templList struct {
	table templ.Component
	alert templ.Component
}

func (t templList) Render(ctx context.Context, w io.Writer) error {
	if err := t.table.Render(ctx, w); err != nil {
		return err
	}
	return t.alert.Render(ctx, w)
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

// patientFromRow converts the creator-joined row into the plain patient
// model used by the detail view.
func patientFromRow(row *repository.GetPatientWithCreatorRow) *repository.Patient {
	return &repository.Patient{
		ID: row.ID, ClinicID: row.ClinicID, DisplayName: row.DisplayName,
		BirthDate: row.BirthDate, Sex: row.Sex, Document: row.Document,
		Phone: row.Phone, Email: row.Email, Street: row.Street,
		City: row.City, State: row.State, PostalCode: row.PostalCode,
		Notes: row.Notes, Status: row.Status, CreatedBy: row.CreatedBy,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
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

// sort orders the rows by the sort query parameter:
// name, -name, birth_date, -birth_date.
func (h *Handler) sort(patients []repository.Patient, sortParam string) {
	less := func(i, j int) bool {
		return patients[i].DisplayName < patients[j].DisplayName
	}
	switch sortParam {
	case "-name":
		less = func(i, j int) bool {
			return patients[i].DisplayName > patients[j].DisplayName
		}
	case "birth_date":
		less = func(i, j int) bool {
			return patients[i].BirthDate.String < patients[j].BirthDate.String
		}
	case "-birth_date":
		less = func(i, j int) bool {
			return patients[i].BirthDate.String > patients[j].BirthDate.String
		}
	}
	sort.SliceStable(patients, less)
}

// formError renders the form fragment for htmx submissions and the full
// page otherwise.
func (h *Handler) formError(c echo.Context, id string, input usecase.PatientInput, msg string) error {
	if server.IsHtmx(c) {
		// htmx only swaps 2xx/3xx responses, so the inline error must
		// arrive with 200.
		return render(c, http.StatusOK, views.PatientForm("", id, values(input), msg))
	}
	return render(c, http.StatusBadRequest, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), id, values(input), msg))
}

// CheckDocument answers the inline duplicate check of the patient form.
// The response is an empty element or the inline error, swapped into the
// #document-error container.
func (h *Handler) CheckDocument(c echo.Context) error {
	document := strings.TrimSpace(c.FormValue("document"))
	if document == "" {
		return render(c, http.StatusOK, views.PatientDocumentError(""))
	}
	clinicID, err := h.clinicID(c.Request().Context())
	if err != nil {
		return err
	}
	exists, err := h.svc.DocumentExists(c.Request().Context(), clinicID, document, c.FormValue("id"))
	if err != nil {
		return err
	}
	if exists {
		return render(c, http.StatusOK, views.PatientDocumentError("This document is already registered"))
	}
	return render(c, http.StatusOK, views.PatientDocumentError(""))
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
