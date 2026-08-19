// Package http exposes the patient registry web routes.
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
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
	"librevita.org/internal/core/storage"
	clinicmodel "librevita.org/internal/domain/clinic/model"
	clinicusecase "librevita.org/internal/domain/clinic/usecase"
	"librevita.org/internal/domain/patient/delivery/views"
	"librevita.org/internal/domain/patient/identifier"
	"librevita.org/internal/domain/patient/usecase"
	"librevita.org/internal/types"
	"librevita.org/internal/ui/components"
)

const (
	patientListLimit        = 50
	maxBulkArchiveIDs       = 50
	patientStatusCookieName = "lv_patient_status"
)

// searchField normalizes the registry search scope: only the fields
// the SQL query understands are accepted, anything else falls back to
// the combined search. Document type scopes (system URNs) never reach
// the SQL: List routes them to the exact lookup before this point.
func searchField(s string) string {
	if s != "name" && s != "email" {
		return ""
	}
	return s
}

// Handler renders the patient pages and processes submissions.
type Handler struct {
	svc     *usecase.Service
	clocks  *clinicusecase.ClockProvider
	csrf    *auth.CSRF
	audit   *audit.Logger
	files   *storage.FileManager
	ids     *identifier.Service
	systems *identifier.SystemsService
}

// NewHandler is the Fx provider.
func NewHandler(svc *usecase.Service, clocks *clinicusecase.ClockProvider,
	csrf *auth.CSRF, auditLogger *audit.Logger, files *storage.FileManager,
	ids *identifier.Service, systems *identifier.SystemsService) *Handler {
	return &Handler{svc: svc, clocks: clocks, csrf: csrf, audit: auditLogger,
		files: files, ids: ids, systems: systems}
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
	rawField := strings.TrimSpace(c.QueryParam("field"))
	field := searchField(rawField)
	if field == "" && rawField != "" && h.isSystemField(ctx, rawField) {
		return h.documentLookup(c, rawField, q)
	}
	status := c.QueryParam("status")
	if c.QueryParams().Has("status") {
		if status == "active" || status == "inactive" {
			// #nosec G124 -- non-sensitive UI patient status filter cookie
			c.SetCookie(&http.Cookie{
				Name:     patientStatusCookieName,
				Value:    status,
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
				MaxAge:   31536000,
			})
		} else {
			// #nosec G124 -- non-sensitive UI patient status filter cookie
			c.SetCookie(&http.Cookie{
				Name:     patientStatusCookieName,
				Value:    "",
				Path:     "/",
				SameSite: http.SameSiteLaxMode,
				MaxAge:   -1,
			})
		}
	} else {
		if cookie, err := c.Cookie(patientStatusCookieName); err == nil && cookie != nil {
			if cookie.Value == "active" || cookie.Value == "inactive" {
				status = cookie.Value
			}
		}
	}
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}

	patients, total, err := h.svc.ListPage(ctx, clinicID, q, field, status, patientListLimit, (page-1)*patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(c.Request().Context(), patients)
	pager := views.PatientPager{Q: q, Field: field, Status: status, Page: page, Total: total, Shown: int64(len(rows))}

	// The search input and filters request fragments; boosted navigation
	// (sidebar links) also arrives with HX-Request but must render the
	// full page, so only non-boosted htmx requests get the fragment.
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.PatientListTable(rows, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, field, status, h.systemOptions(ctx), rows, pager, ""))
}

// isSystemField reports whether s is the URN of an active document
// system, i.e. a valid exact-lookup scope of the search dropdown.
func (h *Handler) isSystemField(ctx context.Context, s string) bool {
	systems, err := h.systems.List(ctx)
	if err != nil {
		return false
	}
	for _, sys := range systems {
		if sys.Active && sys.System == s {
			return true
		}
	}
	return false
}

// documentLookup answers the registry search when the dropdown scope
// is a document type: the typed value is looked up exactly through the
// blind index, scoped to the chosen system, and the owner renders as a
// normal row. Like IdentifierLookup, the plaintext is never echoed
// back and the audit detail carries only the system and the hit count.
func (h *Handler) documentLookup(c echo.Context, system, q string) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	var rows []views.PatientRow
	var total int64
	if len(q) >= minLookupLen {
		hits, err := h.ids.FindByValue(ctx, clinicID, q)
		if err != nil {
			return err
		}
		matched := 0
		for _, hit := range hits {
			if hit.System != system {
				continue
			}
			pt, err := h.svc.Get(ctx, clinicID, hit.PatientID)
			if err != nil {
				continue
			}
			rows = append(rows, h.rows(ctx, []usecase.Patient{*pt})...)
			matched++
		}
		total = int64(matched)
		h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
			"identifier.search", "", "", "system: "+system+", hits: "+strconv.Itoa(matched)))
	}
	pager := views.PatientPager{Q: q, Field: system, Status: "", Page: 1, Total: total, Shown: int64(len(rows))}
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.PatientListTable(rows, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.PatientListPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), q, system, "", h.systemOptions(ctx), rows, pager, ""))
}

// NewPage renders the create form.
func (h *Handler) NewPage(c echo.Context) error {
	return server.Render(c, http.StatusOK, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), "",
		h.systemOptions(c.Request().Context()), views.PatientFormValues{}, ""))
}

// Create validates and inserts a patient, then navigates to its detail.
// An identification document typed in the form is validated and
// registered together with the patient: it is normalized first (so an
// invalid value fails before any write), checked for duplicates in the
// clinic, then encrypted into patient_identifiers after the row exists.
func (h *Handler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	input := h.input(c)
	if err := h.prepareIdentifier(ctx, clinicID, &input); err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return h.formError(c, "", input, v.Msg)
		}
		return err
	}
	patient, err := h.svc.Create(ctx, clinicID, server.ActorID(c), input)
	if err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return h.formError(c, "", input, v.Msg)
		}
		return err
	}
	if err := h.createIdentifier(ctx, clinicID, patient.ID.String(), server.ActorID(c), input); err != nil {
		// The document was registered between the pre-check and the
		// insert (rare race); the patient exists without it.
		return h.formError(c, "", input, err.Error())
	}
	h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
		"patient.create", "patient:"+patient.ID.String(), patient.DisplayName, ""))
	return server.HtmxRedirect(c, "/patients/"+patient.ID.String())
}

// Detail renders the patient record with the registrar.
func (h *Handler) Detail(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := patientID(c)
	if err != nil {
		return err
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	row, err := h.svc.GetWithCreator(ctx, clinicID, id.String())
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	clock, err := h.userClock(ctx)
	if err != nil {
		return err
	}
	createdBy := orEmpty(row.CreatorEmail)
	events, err := h.audit.ForResource(ctx, "patient:"+row.ID.String(), 50)
	if err != nil {
		return err
	}
	docs, err := h.documentRows(ctx, row.ID)
	if err != nil {
		return err
	}
	ids, err := h.ids.List(ctx, clinicID, row.ID.String())
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.PatientDetailPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), row,
		clock.FormatStored(row.CreatedAt), clock.FormatStored(row.UpdatedAt), createdBy,
		h.historyView(events, clock), docs, h.identifierRows(ctx, ids), h.systemOptions(ctx), ""))
}

// historyView turns audit events into display rows, newest first, with
// the timestamp rendered in the clinic's timezone.
func (h *Handler) historyView(events []audit.EventRow, clock *clinicmodel.Clock) []views.HistoryRow {
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
		if ev.Detail != nil && *ev.Detail == types.PatientStatusInactive.String() {
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
	id, err := patientID(c)
	if err != nil {
		return err
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	row, err := h.svc.GetWithCreator(ctx, clinicID, id.String())
	if err != nil {
		if errors.Is(err, usecase.ErrNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	return server.Render(c, http.StatusOK, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), row.ID.String(),
		h.systemOptions(ctx), values(patientInput(row)), ""))
}

// Update applies the edited values and navigates to the detail page.
// An identification document typed in the form is registered as a new
// identifier of the patient (the detail page manages the full list).
func (h *Handler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	id, err := patientID(c)
	if err != nil {
		return err
	}
	input := h.input(c)
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	before, err := h.svc.GetWithCreator(ctx, clinicID, id.String())
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, uuidStrPtr(before.CreatedBy), before.ID.String(), types.PatientStatus(before.Status)); err != nil {
		return err
	}
	if err := h.prepareIdentifier(ctx, clinicID, &input); err != nil {
		var v *usecase.ValidationError
		if errors.As(err, &v) {
			return h.formError(c, id.String(), input, v.Msg)
		}
		return err
	}
	patient, err := h.svc.Update(ctx, clinicID, id.String(), input)
	if err != nil {
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			return h.formError(c, id.String(), input, v.Msg)
		case errors.Is(err, usecase.ErrNotFound):
			return echo.NewHTTPError(http.StatusNotFound)
		default:
			return err
		}
	}
	if err := h.createIdentifier(ctx, clinicID, patient.ID.String(), server.ActorID(c), input); err != nil {
		return h.formError(c, id.String(), input, err.Error())
	}
	h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
		"patient.update", "patient:"+patient.ID.String(), patient.DisplayName, patientChanges(before, input)))
	return server.HtmxRedirect(c, "/patients/"+patient.ID.String())
}

// prepareIdentifier validates the identification document typed in the
// patient form (empty value skips it). The normalized value is checked
// against the clinic's blind index so duplicates fail before any write.
// The identifier fields are only carried in the form; they never enter
// the legacy patient columns.
func (h *Handler) prepareIdentifier(ctx context.Context, clinicID string, input *usecase.PatientInput) error {
	value := strings.TrimSpace(input.IdentifierValue)
	if value == "" {
		return nil
	}
	if _, err := h.ids.ValidateValue(input.IdentifierSystem, value); err != nil {
		return &usecase.ValidationError{Msg: err.Error()}
	}
	owner, err := h.ids.FindByValue(ctx, clinicID, value)
	if err != nil {
		return err
	}
	if len(owner) > 0 {
		return &usecase.ValidationError{Msg: "This document is already registered"}
	}
	return nil
}

// createIdentifier registers the identification document of the form
// on the patient, if one was typed.
func (h *Handler) createIdentifier(ctx context.Context, clinicID, patientID, actorID string, input usecase.PatientInput) error {
	value := strings.TrimSpace(input.IdentifierValue)
	if value == "" {
		return nil
	}
	if _, err := h.ids.AddIdentifier(ctx, clinicID, actorID, identifier.Input{
		PatientID: patientID, System: input.IdentifierSystem, Value: value,
	}); err != nil {
		return err
	}
	return nil
}

// patientChanges renders the changed fields as "name: old -> new"
// pairs, listing only fields whose stored value differs from the input.
func patientChanges(before *usecase.GetPatientWithCreatorRow, input usecase.PatientInput) string {
	type field struct {
		name string
		old  string
		new  string
	}
	fields := []field{
		{"display name", before.DisplayName, input.DisplayName},
		{"birth date", orEmpty(before.BirthDate), input.BirthDate},
		{"sex", before.Sex.String(), input.Sex.String()},
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
	return h.setStatus(c, types.PatientStatusInactive, "Patient archived")
}

// Restore sets the patient active again.
func (h *Handler) Restore(c echo.Context) error {
	return h.setStatus(c, types.PatientStatusActive, "Patient restored")
}

// BulkArchive archives the patients whose ids are in the form. The
// response is the refreshed table fragment plus an OOB alert, so the
// archived rows disappear at once.
func (h *Handler) BulkArchive(c echo.Context) error {
	ctx := c.Request().Context()
	q := strings.TrimSpace(c.QueryParam("q"))
	field := searchField(c.QueryParam("field"))
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
	for _, raw := range ids {
		// A malformed id is skipped, never a panic mid-loop.
		id, err := uuid.Parse(raw)
		if err != nil {
			continue
		}
		pt, err := h.svc.Get(ctx, clinicID, id.String())
		if err != nil {
			continue
		}
		if err := h.authorizePatientEdit(c, uuidStrPtr(pt.CreatedBy), pt.ID.String(), types.PatientStatus(pt.Status)); err != nil {
			continue
		}
		if err := h.svc.SetStatus(ctx, clinicID, id.String(), types.PatientStatusInactive); err == nil {
			archived++
			h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
				"patient.status", "patient:"+id.String(), "", types.PatientStatusInactive.String()))
		} else {
			h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultFailure,
				"patient.status", "patient:"+id.String(), "", "bulk archive failed: "+err.Error()))
		}
	}

	patients, total, err := h.svc.ListPage(ctx, clinicID, q, field, status, patientListLimit, (page-1)*patientListLimit)
	if err != nil {
		return err
	}
	rows := h.rows(ctx, patients)
	pager := views.PatientPager{Q: q, Field: field, Status: status, Page: page, Total: total, Shown: int64(len(rows))}
	msg := "No patients selected"
	if archived > 0 {
		msg = strconv.Itoa(archived) + " patient(s) archived"
	}
	return server.Render(c, http.StatusOK, views.PatientListTableWithAlert(rows, pager, msg))
}

func (h *Handler) setStatus(c echo.Context, status types.PatientStatus, successMsg string) error {
	ctx := c.Request().Context()
	id, err := patientID(c)
	if err != nil {
		return err
	}
	clinicID, err := h.clinicID(ctx)
	if err != nil {
		return err
	}
	pt, err := h.svc.Get(ctx, clinicID, id.String())
	if err != nil {
		return err
	}
	if err := h.authorizePatientEdit(c, uuidStrPtr(pt.CreatedBy), pt.ID.String(), types.PatientStatus(pt.Status)); err != nil {
		return err
	}
	err = h.svc.SetStatus(ctx, clinicID, id.String(), status)
	if err == nil {
		h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultSuccess,
			"patient.status", "patient:"+id.String(), "", status.String()))
	} else {
		h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultFailure,
			"patient.status", "patient:"+id.String(), "", "could not set status "+status.String()))
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
		patient, err := h.svc.Get(ctx, clinicID, id.String())
		if err != nil {
			return err
		}
		// The refreshed row carries the documents column too, so the
		// mask survives the archive/restore swap.
		refreshed := h.rows(ctx, []usecase.Patient{*patient})
		return server.Render(c, http.StatusOK, views.PatientRowOnly(refreshed))
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

// userClock resolves the display clock: the user's personal timezone
// when set, otherwise the clinic timezone.
func (h *Handler) userClock(ctx context.Context) (*clinicmodel.Clock, error) {
	tz := ""
	if p := server.PrincipalCtx(ctx); p != nil {
		tz = p.Timezone
	}
	return h.clocks.ClockFor(ctx, tz)
}

func (h *Handler) input(c echo.Context) usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName:      c.FormValue("display_name"),
		BirthDate:        c.FormValue("birth_date"),
		Sex:              types.Sex(c.FormValue("sex")),
		Phone:            c.FormValue("phone"),
		Email:            c.FormValue("email"),
		Street:           c.FormValue("street"),
		City:             c.FormValue("city"),
		State:            c.FormValue("state"),
		PostalCode:       c.FormValue("postal_code"),
		Notes:            c.FormValue("notes"),
		IdentifierSystem: strings.TrimSpace(c.FormValue("identifier_system")),
		IdentifierValue:  c.FormValue("identifier_value"),
	}
}

// patientID parses the id path parameter; a value that is not a uuid is
// a 404, not a panic.
func patientID(c echo.Context) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return uuid.Nil, echo.NewHTTPError(http.StatusNotFound)
	}
	return id, nil
}

// formError renders the form fragment for htmx submissions and the full
// page otherwise.
func (h *Handler) rows(ctx context.Context, patients []usecase.Patient) []views.PatientRow {
	clock, err := h.userClock(ctx)
	if err != nil {
		clock = clinicmodel.NewClock(clinicmodel.DefaultTimezone)
	}
	// The documents column comes from the encrypted identifiers: every
	// patient of the page is decrypted in one query, then masked for
	// display. A failed lookup degrades to the dash, never to an error
	// page.
	docs := map[string][]string{}
	ids := make([]string, 0, len(patients))
	for _, pt := range patients {
		ids = append(ids, pt.ID.String())
	}
	if all, err := h.ids.ListByPatients(ctx, ids); err == nil {
		docs = all
	}
	out := make([]views.PatientRow, 0, len(patients))
	for _, pt := range patients {
		row := h.rowOf(&pt, clock)
		if values, ok := docs[pt.ID.String()]; ok {
			masked := make([]string, 0, len(values))
			for _, v := range values {
				masked = append(masked, views.MaskValue(v))
			}
			row.Document = strings.Join(masked, ", ")
		}
		out = append(out, row)
	}
	return out
}

func (h *Handler) rowOf(pt *usecase.Patient, clock *clinicmodel.Clock) views.PatientRow {
	return views.PatientRow{
		ID:          pt.ID.String(),
		DisplayName: pt.DisplayName,
		BirthDate:   orEmpty(pt.BirthDate),
		Sex:         pt.Sex.String(),
		Document:    "",
		Phone:       orEmpty(pt.Phone),
		Email:       orEmpty(pt.Email),
		City:        orEmpty(pt.City),
		Status:      pt.Status.String(),
		CreatedAt:   clock.FormatStored(pt.CreatedAt),
	}
}

func values(in usecase.PatientInput) views.PatientFormValues {
	return views.PatientFormValues{
		DisplayName:      in.DisplayName,
		BirthDate:        in.BirthDate,
		Sex:              in.Sex.String(),
		Phone:            in.Phone,
		Email:            in.Email,
		Street:           in.Street,
		City:             in.City,
		State:            in.State,
		PostalCode:       in.PostalCode,
		Notes:            in.Notes,
		IdentifierSystem: in.IdentifierSystem,
		IdentifierValue:  in.IdentifierValue,
	}
}

func patientInput(pt *usecase.GetPatientWithCreatorRow) usecase.PatientInput {
	return usecase.PatientInput{
		DisplayName: pt.DisplayName,
		BirthDate:   orEmpty(pt.BirthDate),
		Sex:         pt.Sex,
		Phone:       orEmpty(pt.Phone),
		Email:       orEmpty(pt.Email),
		Street:      orEmpty(pt.Street),
		City:        orEmpty(pt.City),
		State:       orEmpty(pt.State),
		PostalCode:  orEmpty(pt.PostalCode),
		Notes:       orEmpty(pt.Notes),
	}
}

// uuidStrPtr maps a stored uuid to the nullable string form the policy
// checks use; a Nil uuid (no registrar recorded) becomes nil.
func uuidStrPtr(u *uuid.UUID) *string {
	if u == nil || *u == uuid.Nil {
		return nil
	}
	s := u.String()
	return &s
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formError renders the form fragment for htmx submissions and the full
// page otherwise.
func (h *Handler) formError(c echo.Context, id string, input usecase.PatientInput, msg string) error {
	ctx := c.Request().Context()
	if server.IsHtmx(c) {
		// htmx only swaps 2xx/3xx responses, so the inline error must
		// arrive with 200.
		return server.Render(c, http.StatusOK, views.PatientForm("", id, h.systemOptions(ctx), values(input), msg))
	}
	return server.Render(c, http.StatusBadRequest, views.PatientFormPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), id, h.systemOptions(ctx), values(input), msg))
}

// authorizePatientEdit enforces the fine-grained patient.edit policy
// against the record and audits denials.
func (h *Handler) authorizePatientEdit(c echo.Context, createdBy *string, ptID string, ptStatus types.PatientStatus) error {
	principal := server.Principal(c)
	if principal == nil {
		return echo.NewHTTPError(http.StatusForbidden)
	}
	ctx := c.Request().Context()
	if err := h.svc.AuthorizePatientEdit(ctx, principal, ptID, createdBy, ptStatus); err != nil {
		if errors.Is(err, usecase.ErrForbidden) {
			h.audit.Record(ctx, server.EventFromRequest(c, types.AuditResultFailure,
				"authorize", "policy:patient.edit", "", "denied patient "+ptID))
			return echo.NewHTTPError(http.StatusForbidden)
		}
		return err
	}
	return nil
}
