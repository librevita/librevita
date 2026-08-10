// Package user wires the authentication domain into the application.
package user

import (
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"librevita.org/internal/core/audit"
	"librevita.org/internal/core/auth"
	"librevita.org/internal/core/policy"
	"librevita.org/internal/core/server"
	httphandler "librevita.org/internal/domain/user/delivery/http"
	"librevita.org/internal/domain/user/usecase"
)

// Per-client rate limits. Setup is the only way to become the initial
// admin, so it is deliberately strict.
const (
	loginLimit    = 10
	registerLimit = 5
	setupLimit    = 5
	limitWindow   = time.Minute
)

// Module provides the authentication service, handlers, and routes.
var Module = fx.Module("user",
	fx.Provide(usecase.NewService),
	fx.Provide(httphandler.NewHandler),
	// Share the setup gate with other modules so every route is gated
	// uniformly.
	fx.Provide(func(h *httphandler.Handler) server.SetupGate {
		return h.SetupGate
	}),
	fx.Invoke(registerRoutes),
)

func registerRoutes(e *echo.Echo, h *httphandler.Handler, sessions *auth.SessionManager,
	policies *policy.PolicyEngine, auditLogger *audit.Logger, log *slog.Logger) {

	loginLimiter := server.NewRateLimiter(loginLimit, limitWindow)
	registerLimiter := server.NewRateLimiter(registerLimit, limitWindow)
	setupLimiter := server.NewRateLimiter(setupLimit, limitWindow)
	gate := h.SetupGate()

	e.GET("/setup", h.SetupPage)
	e.POST("/setup", h.Setup, server.RateLimit(setupLimiter))
	e.GET("/auth/login", h.LoginPage, gate)
	e.POST("/auth/login", h.Login, gate, server.RateLimit(loginLimiter))
	// Registration is never public; the users.register policy decides who
	// may create accounts.
	e.GET("/auth/register", h.RegisterPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.register"))
	e.POST("/auth/register", h.Register, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.register"), server.RateLimit(registerLimiter))
	e.POST("/auth/logout", h.Logout, gate, server.RequireAuth(sessions, log))

	e.GET("/", h.Home, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/activity/recent", h.HomeActivity, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.GET("/profile", h.ProfilePage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "dashboard.view"))
	e.POST("/profile", h.ProfileUpdate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "profile.update"))
	e.GET("/policies", h.AdminPoliciesPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.POST("/policies", h.AdminPolicySave, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.POST("/policies/reset", h.AdminPolicyReset, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.GET("/users", h.UsersPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.GET("/users/new", h.UserNewPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/users", h.UserCreate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.GET("/users/:id/edit", h.UserEditPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/users/:id", h.UserUpdate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/users/:id/status", h.UserStatus, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.GET("/specialties", h.SpecialtiesPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/specialties", h.SpecialtyCreate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/specialties/:id/delete", h.SpecialtyDelete, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.GET("/roles", h.RolesPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/roles", h.RoleCreate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/roles/:id/rename", h.RoleRename, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/roles/:id/clinical", h.RoleClinical, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))
	e.POST("/roles/:id/delete", h.RoleDelete, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "users.manage"))

	// Physician directory and the approval workflow.
	e.GET("/staff", h.StaffPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.view"))
	e.GET("/staff/new", h.StaffCreatePage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.edit"))
	e.POST("/staff", h.StaffCreate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.edit"))
	e.GET("/staff/:id/edit", h.StaffEditPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.view"))
	e.POST("/staff/:id", h.StaffUpdate, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.edit"))
	e.POST("/staff/:id/request", h.StaffRequestChange, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.request"))
	e.GET("/staff/my-requests", h.MyStaffRequestsPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.request"))
	e.GET("/staff/requests", h.StaffRequestsPage, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.approve"))
	e.GET("/audit/integrity", h.AuditIntegrity, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "admin.view"))
	e.POST("/staff/requests/:id/approve", h.StaffRequestApprove, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.approve"))
	e.POST("/staff/requests/:id/reject", h.StaffRequestReject, gate, server.RequireAuth(sessions, log), server.RequirePolicy(policies, auditLogger, log, "staff.approve"))
}
