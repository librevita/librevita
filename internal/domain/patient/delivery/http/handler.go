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

const (
	patientListLimit  = 50
	maxBulkArchiveIDs = 50
)

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
// fragment (search, filter, pager).
func (h *Handler) List(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	q := strings.TrimSpace(c.QueryParam("q"))
	status := c.QueryParam("status")
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	patients, total, err := h.svc.ListPage(ctx, clinicID, q, status, patientListLimit, (page-1)*patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(c.Request().Context(), patients)
	pager := views.PatientPager{Q: q, Status: status, Page: page, Total: total, Shown: int64(len(rows))}

	// The search input and filters request fragments; boosted navigation
	// (sidebar links) also arrives with HX-Request but must render the
	// full page, so only non-boosted htmx requests get the fragment.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.PatientListTable(rows, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, status, rows, pager, ""))
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
	events, err := h.audit.ForResource(ctx, "patient:"+row.ID, 50)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.PatientDetailPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), pt, createdBy,
		h.historyView(events, clock), ""))
}

// historyView turns audit events into display rows, newest first, with
// the timestamp rendered in the clinic's timezone.
func (h *Handler) historyView(events []audit.EventRow, clock *clinic.Clock) []views.HistoryRow {
	out := make([]views.HistoryRow, 0, len(events))
	for _, ev := range events {
		out = append(out, views.HistoryRow{
			When: clock.FormatStored(ev.CreatedAt),
			Text: historyText(ev),
		})
	}
	return out
}

// historyText renders a human-readable description of a patient event.
func historyText(ev audit.EventRow) string {
	actor := "an unknown user"
	if ev.ActorEmail != nil && *ev.ActorEmail != "" {
		actor = *ev.ActorEmail
	}
	switch ev.Action {
	case "patient.create":
		return "Registered by " + actor
	case "patient.update":
		if ev.Detail != nil && *ev.Detail != "" {
			return "Updated by " + actor + " (" + *ev.Detail + ")"
		}
		return "Updated by " + actor
	case "patient.status":
		if ev.Detail != nil && *ev.Detail == usecase.StatusInactive {
			return "Archived by " + actor
		}
		return "Restored by " + actor
	default:
		return ev.Action + " by " + actor
	}
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
	id := c.Param("id")
	input := h.input(c)
	before, err := h.svc.GetWithCreator(ctx, id)
	if err != nil {
		return err
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, before.CreatedBy, before.ID, before.Status); err != nil {
		return err
	}
	patient, err := h.svc.Update(ctx, clinicID, id, input)
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return h.formError(c, id, input, v.Msg)
		case errors.Is(err, usecase.ErrNotFound):
			return echo.NewHTTPError(http.StatusNotFound)
		default:
			return err
		}
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "patient.update", Resource: "patient:" + patient.ID,
		Result: audit.ResultSuccess, Detail: patientChanges(before, input),
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/patients/"+patient.ID)
}

// patientChanges renders the changed fields as "name: old -> new"
// pairs, listing only fields whose stored value differs from the input.
func patientChanges(before *repository.GetPatientWithCreatorRow, input usecase.PatientInput) string {
	type field struct {
		name string
		old  string
		new  string
	}
	fields := []field{
		{"display name", before.DisplayName, input.DisplayName},
		{"birth date", orEmpty(before.BirthDate), input.BirthDate},
		{"sex", before.Sex, input.Sex},
		{"document", orEmpty(before.Document), input.Document},
		{"phone", orEmpty(before.Phone), input.Phone},
		{"email", orEmpty(before.Email), input.Email},
		{"street", orEmpty(before.Street), input.Street},
		{"city", orEmpty(before.City), input.City},
		{"state", orEmpty(before.State), input.State},
		{"postal code", orEmpty(before.PostalCode), input.PostalCode},
		{"notes", orEmpty(before.Notes), input.Notes},
	}
	parts := make([]string, 0, 3)
	for _, f := range fields {
		old, next := strings.TrimSpace(f.old), strings.TrimSpace(f.new)
		if old == next {
			continue
		}
		if old == "" {
			parts = append(parts, f.name+": "+displayValue(next))
			continue
		}
		parts = append(parts, f.name+": "+displayValue(old)+" -> "+displayValue(next))
	}
	return strings.Join(parts, ", ")
}

// displayValue shortens long stored values (e.g. notes) for the audit
// detail so the change list stays readable.
func displayValue(s string) string {
	if len(s) > 40 {
		return s[:37] + "..."
	}
	return s
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
	q := strings.TrimSpace(c.QueryParam("q"))
	status := c.QueryParam("status")
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	// Bound the number of writes a single request can trigger.
	ids := c.Request().PostForm["ids"]
	if len(ids) > maxBulkArchiveIDs {
		ids = ids[:maxBulkArchiveIDs]
	}
	archived := 0
	for _, id := range ids {
		pt, err := h.svc.Get(ctx, id)
		if err != nil {
			continue
		}
		if err := h.authorizePatientEdit(c, pt.CreatedBy, pt.ID, pt.Status); err != nil {
			continue
		}
		if err := h.svc.SetStatus(ctx, clinicID, id, usecase.StatusInactive); err == nil {
			archived++
			h.audit.Record(ctx, audit.Event{
				ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
				Action: "patient.status", Resource: "patient:" + id,
				Result: audit.ResultSuccess, Detail: usecase.StatusInactive,
				IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			})
		} else {
			h.audit.Record(ctx, audit.Event{
				ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
				Action: "patient.status", Resource: "patient:" + id, Result: audit.ResultFailure,
				Detail: "bulk archive failed: " + err.Error(),
				IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			})
		}
	}

	patients, total, err := h.svc.ListPage(ctx, clinicID, q, status, patientListLimit, (page-1)*patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(ctx, patients)
	pager := views.PatientPager{Q: q, Status: status, Page: page, Total: total, Shown: int64(len(rows))}
	msg := "No patients selected"
	if archived > 0 {
		msg = strconv.Itoa(archived) + " patient(s) archived"
	}
	return server.Render(c, http.StatusOK, views.PatientListTableWithAlert(rows, pager, msg))
}

func (h *Handler) setStatus(c echo.Context, status, successMsg string) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	pt, err := h.svc.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, pt.CreatedBy, pt.ID, pt.Status); err != nil {
		return err
	}
	err = h.svc.SetStatus(ctx, clinicID, id, status)
	if err == nil {
		h.audit.Record(ctx, audit.Event{
			ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
			Action: "patient.status", Resource: "patient:" + id,
			Result: audit.ResultSuccess, Detail: status,
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	} else {
		h.audit.Record(ctx, audit.Event{
			ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
			Action: "patient.status", Resource: "patient:" + id, Result: audit.ResultFailure,
			Detail: "could not set status " + status,
			IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
		})
	}
	if server.IsHtmx(c) {
		if err != nil {
			// htmx does not swap 4xx responses, so errors return 200 with
			// an OOB alert that lands in the shell's #app-alert container.
			return server.Render(c, http.StatusOK, components.Alert("Could not update the patient", true))
		}
		// Re-render the row in place with the new status. The response
		// must contain only the row: sibling elements would break the
		// htmx fragment parser for table content.
		patient, err := h.svc.Get(ctx, id)
		if err != nil {
			return err
		}
		clock, err := h.clocks.Clock(ctx)
		if err != nil {
			return err
		}
		return server.Render(c, http.StatusOK, views.PatientRowOnly(
			[]views.PatientRow{h.rowOf(patient, clock)}))
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

// nextLimit returns the limit for the Load more button: zero when the
// page is exhausted or the sort order is not by name.
func nextLimit(sortParam string, pageLimit int, hasMore bool) int {
	if sortParam != "" && sortParam != "name" {
		return 0
	}
	if !hasMore {
		return 0
	}
	return pageLimit + patientListLimit
}

func (h *Handler) rows(ctx context.Context, patients []repository.Patient) []views.PatientRow {
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		clock = clinic.NewClock(clinic.DefaultTimezone)
	}
	out := make([]views.PatientRow, 0, len(patients))
	for _, pt := range patients {
		out = append(out, h.rowOf(&pt, clock))
	}
	return out
}

func (h *Handler) rowOf(pt *repository.Patient, clock *clinic.Clock) views.PatientRow {
	return views.PatientRow{
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
	}
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


// authorizePatientEdit enforces the fine-grained patient.edit policy
// against the record and audits denials.
func (h *Handler) authorizePatientEdit(c echo.Context, createdBy *string, ptID, ptStatus string) error {
	principal := server.Principal(c)
	if principal == nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	ctx := c.Request().Context()
	if err := h.svc.AuthorizePatientEdit(ctx, principal, ptID, createdBy, ptStatus); err != nil {
		if errors.Is(err, usecase.ErrForbidden) {
			h.audit.Record(ctx, audit.Event{
				ActorID: principal.ID, ActorMail: principal.Email,
				Action: "authorize", Resource: "policy:patient.edit", Result: audit.ResultFailure,
				Detail: "denied patient " + ptID,
				IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
			})
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return err
	}
	return nil
}
