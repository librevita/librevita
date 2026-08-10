package server

import (
	"github.com/labstack/echo/v4"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/types"
)

// EventFromRequest builds an audit event with the full request context
// denormalized: the actor identity (id, email, name, role), the user
// agent, the IP and the request id. resourceName is the human-readable
// name of the affected resource (e.g. the patient display name).
func EventFromRequest(c echo.Context, result types.Result, action, resource, resourceName, detail string) audit.Event {
	ev := audit.Event{
		Action:       action,
		Resource:     resource,
		ResourceName: resourceName,
		Result:       result,
		IP:           c.RealIP(),
		RequestID:    c.Response().Header().Get(echo.HeaderXRequestID),
		Detail:       detail,
		UserAgent:    c.Request().UserAgent(),
	}
	if p := Principal(c); p != nil {
		ev.ActorID = p.ID
		ev.ActorMail = p.Email
		ev.ActorName = p.Name
		ev.ActorRole = p.Role.String()
	}
	return ev
}
