package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/server"
	"librevita.org/internal/domain/user/delivery/views"
	"librevita.org/internal/domain/user/repository"
	"librevita.org/internal/domain/user/usecase"
	"librevita.org/internal/ui/components"
)

// StaffPage lists the physician directory with their specialties,
// paginated like the other registries.
func (h *Handler) StaffPage(c echo.Context) error {
	ctx := c.Request().Context()
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	rows, total, err := h.svc.ListPhysiciansPage(ctx, staffPageSize, (page-1)*staffPageSize)
	if err != nil {
		return err
	}
	out := h.physicianRows(rows)
	pager := views.StaffPager{Page: page, Total: total, Shown: int64(len(out))}
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.StaffTable(server.CSRFToken(c, h.csrf), out, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.StaffPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), out, pager, ""))
}

const staffPageSize = 20

func (h *Handler) physicianRows(rows []repository.ListPhysiciansPageRow) []views.PhysicianRow {
	out := make([]views.PhysicianRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, views.PhysicianRow{
			ID: r.ID, Name: r.DisplayName, Email: r.Email,
			Active: r.Active, Specialties: asString(r.Specialties),
		})
	}
	return out
}

// asString flattens the sqlc GROUP_CONCAT column, which is typed
// interface{} and may hold a NULL-derived empty value.
func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// StaffEditPage renders the physician profile form. Admins get the
// direct edit; other staff get the change-request form.
func (h *Handler) StaffEditPage(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := h.svc.GetUser(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, usecase.ErrUserNotFound) {
			return echo.NewHTTPError(http.StatusNotFound)
		}
		return err
	}
	if user.RoleIsClinical != 1 {
		return echo.NewHTTPError(http.StatusNotFound)
	}
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	specialties, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return err
	}
	selected, err := h.svc.UserSpecialties(ctx, user.ID)
	if err != nil {
		return err
	}
	admin := server.Principal(c) != nil && server.Principal(c).Role == auth.RoleAdmin
	return server.Render(c, http.StatusOK, views.StaffEditPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), user.ID, views.StaffFormValues{
			Name: user.DisplayName, Email: user.Email,
		}, h.specialtyViews(specialties, selected), admin, ""))
}

// StaffUpdate applies the admin's direct changes to the physician
// profile.
func (h *Handler) StaffUpdate(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	change := h.staffChangeFromForm(c)
	if err := h.applyStaffChange(ctx, id, change, server.ActorID(c), "admin update"); err != nil {
		return h.staffUpdateError(c, id, change, err)
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "staff.update", Resource: "user:" + id,
		Result: audit.ResultSuccess, Detail: "direct admin edit",
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/staff")
}

// StaffRequestChange records a receptionist's proposal for the admin to
// approve.
func (h *Handler) StaffRequestChange(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	change := h.staffChangeFromForm(c)
	if _, err := h.svc.CreateStaffChangeRequest(ctx, id, server.ActorID(c), change); err != nil {
		return h.staffRequestError(c, id, change, err)
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "staff.request", Resource: "user:" + id,
		Result: audit.ResultSuccess, Detail: "change requested for approval",
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/staff")
}

// staffRequestPageSize is the page size of the change request history.
const staffRequestPageSize = 20

// StaffRequestsPage lists the change request history with status and
// search filters, each row carrying a readable summary of the proposed
// changes and the decision.
func (h *Handler) StaffRequestsPage(c echo.Context) error {
	ctx := c.Request().Context()
	q := strings.TrimSpace(c.QueryParam("q"))
	status := c.QueryParam("status")
	page := 1
	if p := c.QueryParam("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	rows, total, err := h.svc.ListStaffChangeRequestsFiltered(ctx, status, q, staffRequestPageSize, (page-1)*staffRequestPageSize)
	if err != nil {
		return err
	}
	out, err := h.staffRequestViews(ctx, rows)
	if err != nil {
		return err
	}
	pager := views.StaffRequestPager{Q: q, Status: status, Page: page, Total: total, Shown: int64(len(out))}
	if server.IsHtmx(c) && c.Request().Header.Get("HX-Boosted") != "true" {
		return server.Render(c, http.StatusOK, views.StaffRequestsTable(server.CSRFToken(c, h.csrf), out, pager, ""))
	}
	return server.Render(c, http.StatusOK, views.StaffRequestsPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), out, pager, ""))
}

// MyStaffRequestsPage lists the requester's own requests with their
// current status, so a receptionist can follow up on submissions.
func (h *Handler) MyStaffRequestsPage(c echo.Context) error {
	ctx := c.Request().Context()
	rows, err := h.svc.ListMyStaffChangeRequests(ctx, server.ActorID(c))
	if err != nil {
		return err
	}
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return err
	}
	out := make([]views.StaffRequestRow, 0, len(rows))
	for _, r := range rows {
		view, err := h.staffRequestView(ctx, r.UserID, r.Changes, r.Status, "")
		if err != nil {
			return err
		}
		view.ID = r.ID
		view.UserName = r.UserName
		view.UserEmail = r.UserEmail
		view.CreatedAt = clock.FormatStored(r.CreatedAt)
		out = append(out, view)
	}
	return server.Render(c, http.StatusOK, views.MyStaffRequestsPage(
		server.CSRFToken(c, h.csrf), server.Principal(c), out, ""))
}

// staffRequestViews builds the change request rows with readable
// summaries and decision details.
func (h *Handler) staffRequestViews(ctx context.Context, rows []repository.ListStaffChangeRequestsFilteredRow) ([]views.StaffRequestRow, error) {
	clock, err := h.clocks.Clock(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]views.StaffRequestRow, 0, len(rows))
	for _, r := range rows {
		view, err := h.staffRequestView(ctx, r.UserID, r.Changes, r.Status, r.RequesterEmail)
		if err != nil {
			return nil, err
		}
		view.ID = r.ID
		view.UserName = r.UserName
		view.UserEmail = r.UserEmail
		view.CreatedAt = clock.FormatStored(r.CreatedAt)
		view.DecisionNote = r.DecisionNote.String
		view.DecidedByEmail = r.DecidedByEmail.String
		out = append(out, view)
	}
	return out, nil
}

// staffRequestView renders the readable change summary for one request.
func (h *Handler) staffRequestView(ctx context.Context, userID, changes, status, requesterEmail string) (views.StaffRequestRow, error) {
	var change usecase.StaffChange
	if err := json.Unmarshal([]byte(changes), &change); err != nil {
		return views.StaffRequestRow{}, fmt.Errorf("decode staff changes: %w", err)
	}
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return views.StaffRequestRow{}, err
	}
	catalog, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return views.StaffRequestRow{}, err
	}
	names := make(map[string]string, len(catalog))
	for _, sp := range catalog {
		names[sp.ID] = sp.Name
	}

	// Requests snapshot the profile at creation time; fall back to the
	// current profile for requests created before the snapshot existed.
	before := change.Previous
	if before == nil || (before.Name == "" && before.Email == "") {
		current, err := h.svc.GetUser(ctx, userID)
		if err != nil {
			return views.StaffRequestRow{}, err
		}
		before = &usecase.StaffChange{Name: current.DisplayName, Email: current.Email}
	}

	var parts []string
	if change.Name != before.Name {
		parts = append(parts, "Name: "+before.Name+" → "+change.Name)
	}
	if change.Email != before.Email {
		parts = append(parts, "Email: "+before.Email+" → "+change.Email)
	}
	proposed := make([]string, 0, len(change.Specialties))
	for _, id := range change.Specialties {
		if name, ok := names[id]; ok {
			proposed = append(proposed, name)
		}
	}
	beforeSet := make([]string, 0, len(before.Specialties))
	for _, id := range before.Specialties {
		if name, ok := names[id]; ok {
			beforeSet = append(beforeSet, name)
		}
	}
	if !sameStringSet(beforeSet, proposed) {
		parts = append(parts, "Specialties: "+strings.Join(proposed, ", "))
	}
	summary := "No changes"
	if len(parts) > 0 {
		summary = strings.Join(parts, " · ")
	}
	return views.StaffRequestRow{
		UserID: userID, RequesterEmail: requesterEmail,
		Changes: summary, Status: status,
	}, nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return true
}

// StaffRequestApprove applies the proposed changes.
func (h *Handler) StaffRequestApprove(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := h.svc.ApproveStaffChangeRequest(ctx, id, server.ActorID(c)); err != nil {
		return h.requestError(c, err)
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "staff.approve", Resource: "request:" + id,
		Result: audit.ResultSuccess,
		IP:     c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/staff/requests")
}

// StaffRequestReject marks the request rejected with the given note.
func (h *Handler) StaffRequestReject(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")
	if err := h.svc.RejectStaffChangeRequest(ctx, id, server.ActorID(c), c.FormValue("note")); err != nil {
		return h.requestError(c, err)
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "staff.reject", Resource: "request:" + id,
		Result: audit.ResultSuccess, Detail: c.FormValue("note"),
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/staff/requests")
}

// staffChangeFromForm reads the shared profile fields from the form.
func (h *Handler) staffChangeFromForm(c echo.Context) usecase.StaffChange {
	return usecase.StaffChange{
		Name:        c.FormValue("name"),
		Email:       c.FormValue("email"),
		Specialties: c.Request().PostForm["specialties"],
	}
}

// applyStaffChange applies the change directly (admin path).
func (h *Handler) applyStaffChange(ctx context.Context, id string, change usecase.StaffChange, actorID, detail string) error {
	current, err := h.svc.GetUser(ctx, id)
	if err != nil {
		return err
	}
	if _, err := h.svc.UpdateUser(ctx, id, actorID, usecase.UpdateUserInput{
		Name: change.Name, Email: change.Email, Role: current.RoleName, Active: current.Active == 1,
	}); err != nil {
		return err
	}
	return h.svc.SetUserSpecialties(ctx, id, change.Specialties)
}

func (h *Handler) staffUpdateError(c echo.Context, id string, change usecase.StaffChange, err error) error {
	var v *usecase.ValidationError
	switch {
	case errors.As(err, &v):
		return server.Render(c, http.StatusBadRequest, views.StaffEditPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), id, views.StaffFormValues{
				Name: change.Name, Email: change.Email,
			}, nil, true, v.Msg))
	case errors.Is(err, usecase.ErrEmailTaken):
		return server.Render(c, http.StatusConflict, views.StaffEditPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), id, views.StaffFormValues{
				Name: change.Name, Email: change.Email,
			}, nil, true, "That email is already registered"))
	default:
		return err
	}
}

func (h *Handler) staffRequestError(c echo.Context, id string, change usecase.StaffChange, err error) error {
	var v *usecase.ValidationError
	switch {
	case errors.As(err, &v):
		return server.Render(c, http.StatusBadRequest, views.StaffEditPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), id, views.StaffFormValues{
				Name: change.Name, Email: change.Email,
			}, nil, false, v.Msg))
	case errors.Is(err, usecase.ErrEmailInUse):
		return server.Render(c, http.StatusConflict, views.StaffEditPage(
			server.CSRFToken(c, h.csrf), server.Principal(c), id, views.StaffFormValues{
				Name: change.Name, Email: change.Email,
			}, nil, false, "That email already belongs to another account"))
	default:
		return err
	}
}

func (h *Handler) requestError(c echo.Context, err error) error {
	if errors.Is(err, usecase.ErrRequestNotPending) {
		return server.Render(c, http.StatusOK, components.Alert("This request was already decided", true))
	}
	return err
}

// StaffCreatePage renders the physician creation form with the
// specialty catalog checkboxes.
func (h *Handler) StaffCreatePage(c echo.Context) error {
	ctx := c.Request().Context()
	clinicID, err := h.clocks.ClinicID(ctx)
	if err != nil {
		return err
	}
	specialties, err := h.svc.ListSpecialties(ctx, clinicID)
	if err != nil {
		return err
	}
	return server.Render(c, http.StatusOK, views.StaffCreatePage(
		server.CSRFToken(c, h.csrf), server.Principal(c),
		h.specialtyViews(specialties, nil), ""))
}

// StaffCreate creates a physician account (the role is fixed to
// physician) and assigns the submitted specialties.
func (h *Handler) StaffCreate(c echo.Context) error {
	ctx := c.Request().Context()
	user, err := h.svc.CreateUser(ctx, usecase.CreateUserInput{
		Name:     c.FormValue("name"),
		Email:    c.FormValue("email"),
		Password: c.FormValue("password"),
		Role:     auth.RolePhysician.String(),
	})
	if err != nil {
		msg := "Could not create the physician"
		var v *usecase.ValidationError
		switch {
		case errors.As(err, &v):
			msg = v.Msg
		case errors.Is(err, usecase.ErrEmailTaken):
			msg = "That email is already registered"
		}
		return server.Render(c, http.StatusOK, views.StaffCreatePage(
			server.CSRFToken(c, h.csrf), server.Principal(c), nil, msg))
	}
	if err := h.svc.SetUserSpecialties(ctx, user.ID, c.Request().PostForm["specialties"]); err != nil {
		return err
	}
	h.audit.Record(ctx, audit.Event{
		ActorID: server.ActorID(c), ActorMail: server.ActorMail(c),
		Action: "staff.create", Resource: "user:" + user.ID,
		Result: audit.ResultSuccess, Detail: "physician account created",
		IP: c.RealIP(), RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	})
	return server.HtmxRedirect(c, "/staff")
}
