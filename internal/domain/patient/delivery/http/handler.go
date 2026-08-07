// Package http exposes the patient registry web routes.
package http

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

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
	after := c.QueryParam("after")
	sortParam := c.QueryParam("sort")

	// Load-more requests append rows and refresh the cursor button via
	// OOB; they must not re-render the whole table.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" && after != "" {
		return h.moreRows(c, ctx, clinicID, q, status, after, sortParam)
	}

	limit := patientListLimit + 1
	patients, err := h.svc.List(ctx, clinicID, q, status, after, limit)
	if err != nil {
		return err
	}
	hasMore := len(patients) > patientListLimit
	if hasMore {
		patients = patients[:patientListLimit]
	}
	h.sort(patients, sortParam)
	rows := h.rows(c.Request().Context(), patients)

	// The search input and filters request fragments; boosted navigation
	// (sidebar links) also arrives with HX-Request but must render the
	// full page, so only non-boosted htmx requests get the fragment.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.PatientListTable(rows, cursor(sortParam, rows, hasMore), ""))
	}
	return server.Render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, status, sortParam, rows,
		cursor(sortParam, rows, hasMore), ""))
}

// NewPage renders the create form.
func (h *Handler) NewPage(c echo.Context) error {
	return server.Render(c, http.StatusOK, views.PatientFormPage(
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
	patient, err := h.svc.Create(ctx, clinicID, server.ActorID(c), input)
	if err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return h.formError(c, "", input, v.Msg)
		}
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
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
	createdBy := orEmpty(row.CreatedByEmail)
	pt := h.detailView(row, clock)
	return server.Render(c, http.StatusOK, views.PatientDetailPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), pt, createdBy, ""))
}

// EditPage renders the edit form with the stored values.
func (h *Handler) EditPage(c echo.Context) error {
	ctx := c.Request().Context()
	row, err := h.svc.GetWithCreator(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	return server.Render(c, http.StatusOK, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), row.ID, values(patientInput(row)), ""))
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
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
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
	q := c.QueryParam("q")
	status := c.QueryParam("status")
	sortParam := c.QueryParam("sort")
	ids := c.Request().PostForm["ids"]
	archived := 0
	for _, id := range ids {
		if err := h.svc.SetStatus(ctx, id, usecase.StatusInactive); err == nil {
			archived++
		}
	}
	if archived > 0 {
		h.audit.Record(ctx, audit.Event{
			ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
			Action: "patient.status", Resource: "patient:bulk",
			Result: audit.ResultSuccess, Detail: "archived " + strconv.Itoa(archived),
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}

	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	patients, err := h.svc.List(ctx, clinicID, q, status, "", patientListLimit+1)
	if err != nil {
		return err
	}
	hasMore := len(patients) > patientListLimit
	if hasMore {
		patients = patients[:patientListLimit]
	}
	rows := h.rows(ctx, patients)
	msg := "No patients selected"
	if archived > 0 {
		msg = strconv.Itoa(archived) + " patient(s) archived"
	}
	return server.Render(c, http.StatusOK, views.PatientListTableWithAlert(rows, cursor(sortParam, rows, hasMore), msg))
}

func (h *Handler) setStatus(c echo.Context, status, successMsg string) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	err := h.svc.SetStatus(ctx, id, status)
	if err == nil {
		h.audit.Record(ctx, audit.Event{
			ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
			Action: "patient.status", Resource: "patient:" + id,
			Result: audit.ResultSuccess, Detail: status,
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}
	if server.IsHtmx(c) {
		if err != nil {
			return server.Render(c, http.StatusBadRequest, components.Alert("Could not update the patient", true))
		}
		return server.Render(c, http.StatusOK, components.Alert(successMsg, true))
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
	return h.clocks.ClinicID(ctx)
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

// moreRows answers the Load more button: only the next rows plus an OOB
// refresh of the cursor button.
func (h *Handler) moreRows(c echo.Context, ctx context.Context, clinicID, q, status, after, sortParam string) error {
	patients, err := h.svc.List(ctx, clinicID, q, status, after, patientListLimit+1)
	if err != nil {
		return err
	}
	hasMore := len(patients) > patientListLimit
	if hasMore {
		patients = patients[:patientListLimit]
	}
	h.sort(patients, sortParam)
	rows := h.rows(ctx, patients)
	return server.Render(c, http.StatusOK, views.PatientListMoreRows(rows, cursor(sortParam, rows, hasMore)))
}

// cursor builds the next Load more cursor from the last visible row. It
// is empty when the page is exhausted or the sort order is not by name.
func cursor(sortParam string, rows []views.PatientRow, hasMore bool) string {
	if sortParam != "" && sortParam != "name" {
		return ""
	}
	if !hasMore || len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1].DisplayName
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
			BirthDate:   orEmpty(pt.BirthDate),
			Sex:         pt.Sex,
			Document:    orEmpty(pt.Document),
			Phone:       orEmpty(pt.Phone),
			Email:       orEmpty(pt.Email),
			City:        orEmpty(pt.City),
			Status:      pt.Status,
			CreatedAt:   clock.FormatStored(pt.CreatedAt),
		})
	}
	return out
}

func (h *Handler) detailView(row *repository.GetPatientWithCreatorRow, clock *clinic.Clock) *repository.GetPatientWithCreatorRow {
	clone := *row
	clone.CreatedAt = clock.FormatStored(clone.CreatedAt)
	clone.UpdatedAt = clock.FormatStored(clone.UpdatedAt)
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

func patientInput(pt *repository.GetPatientWithCreatorRow) usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: pt.DisplayName,
		BirthDate:   orEmpty(pt.BirthDate),
		Sex:         pt.Sex,
		Document:    orEmpty(pt.Document),
		Phone:       orEmpty(pt.Phone),
		Email:       orEmpty(pt.Email),
		Street:      orEmpty(pt.Street),
		City:        orEmpty(pt.City),
		State:       orEmpty(pt.State),
		PostalCode:  orEmpty(pt.PostalCode),
		Notes:       orEmpty(pt.Notes),
	}
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
			return orEmpty(patients[i].BirthDate) < orEmpty(patients[j].BirthDate)
		}
	case "-birth_date":
		less = func(i, j int) bool {
			return orEmpty(patients[i].BirthDate) > orEmpty(patients[j].BirthDate)
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
		return server.Render(c, http.StatusOK, views.PatientForm("", id, values(input), msg))
	}
	return server.Render(c, http.StatusBadRequest, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), id, values(input), msg))
}

// CheckDocument answers the inline duplicate check of the patient form.
// The response is an empty element or the inline error, swapped into the
// #document-error container.
func (h *Handler) CheckDocument(c echo.Context) error {
	document := strings.TrimSpace(c.FormValue("document"))
	if document == "" {
		return server.Render(c, http.StatusOK, views.PatientDocumentError(""))
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
		return server.Render(c, http.StatusOK, views.PatientDocumentError("This document is already registered"))
	}
	return server.Render(c, http.StatusOK, views.PatientDocumentError(""))
}
