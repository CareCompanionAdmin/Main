package admin

import (
	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

// Handler holds dependencies for admin handlers
type Handler struct {
	adminRepo         repository.AdminRepository
	authService       *service.AuthService
	cloudwatchService *service.CloudWatchService
}

// NewHandler creates a new admin handler
func NewHandler(adminRepo repository.AdminRepository, authService *service.AuthService) *Handler {
	return &Handler{
		adminRepo:   adminRepo,
		authService: authService,
	}
}

// SetCloudWatchService sets the CloudWatch service for metrics collection
func (h *Handler) SetCloudWatchService(cw *service.CloudWatchService) {
	h.cloudwatchService = cw
}

// Routes returns the admin router
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// All admin routes require authentication
	r.Use(middleware.AuthMiddleware(h.authService))

	// Super admin routes
	r.Route("/super", func(r chi.Router) {
		r.Use(middleware.RequireSuperAdmin())
		r.Get("/admins", h.ListAdminUsers)
		r.Post("/admins", h.CreateAdminUser)
		r.Get("/admins/{id}", h.GetAdminUser)
		r.Put("/admins/{id}", h.UpdateAdminUser)
		r.Delete("/admins/{id}", h.DeleteAdminUser)
		r.Get("/metrics", h.GetSystemMetrics)
		r.Post("/metrics/refresh", h.RefreshMetrics)
		r.Get("/settings", h.GetSettings)
		r.Put("/settings/{key}", h.UpdateSetting)
		r.Post("/maintenance", h.ToggleMaintenanceMode)
		r.Get("/audit-log", h.GetAuditLog)
	})

	// Support routes
	r.Route("/support", func(r chi.Router) {
		r.Use(middleware.RequireSupport())
		r.Get("/tickets", h.ListTickets)
		r.Post("/tickets", h.CreateTicket)
		r.Get("/tickets/{id}", h.GetTicket)
		r.Put("/tickets/{id}", h.UpdateTicket)
		r.Post("/tickets/{id}/assign", h.AssignTicket)
		r.Post("/tickets/{id}/resolve", h.ResolveTicket)
		r.Get("/tickets/{id}/messages", h.GetTicketMessages)
		r.Post("/tickets/{id}/messages", h.AddTicketMessage)
		r.Get("/users", h.SearchUsers)
		r.Get("/users/{id}", h.GetUser)
		r.Put("/users/{id}/status", h.UpdateUserStatus)
		r.Post("/users/{id}/reset-password", h.ResetUserPassword)
		r.Post("/users/{id}/reset-mfa", h.ResetUserMFA)
		r.Get("/families", h.ListFamilies)
		r.Get("/families/{id}", h.GetFamily)
	})

	// Marketing routes
	r.Route("/marketing", func(r chi.Router) {
		r.Use(middleware.RequireMarketing())
		r.Get("/dashboard", h.GetMarketingDashboard)
		r.Get("/metrics", h.GetMarketingMetrics)
		// Read-only ticket access for marketing (to help manage user experiences)
		r.Get("/tickets", h.ListTickets)
		r.Get("/tickets/{id}", h.GetTicket)
		r.Get("/tickets/{id}/messages", h.GetTicketMessages)
	})

	return r
}

// UIRoutes returns routes for admin UI pages
func (h *Handler) UIRoutes() chi.Router {
	r := chi.NewRouter()

	// Login page (no auth required)
	r.Get("/login", h.AdminLoginPage)
	r.Post("/login", h.AdminLoginSubmit)

	// Protected UI routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(h.authService))
		r.Use(middleware.RequireAnyAdminRole())

		r.Get("/", h.AdminDashboard)
		r.Get("/dashboard", h.AdminDashboard)

		// Super admin pages
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/admins", h.AdminUsersPage)
			r.Get("/settings", h.SettingsPage)
			r.Get("/audit", h.AuditLogPage)
		})

		// Support pages
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSupport())
			r.Get("/tickets", h.TicketsPage)
			r.Get("/tickets/{id}", h.TicketDetailPage)
			r.Get("/users", h.UsersPage)
			r.Get("/families", h.FamiliesPage)
		})

		// Marketing pages
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireMarketing())
			r.Get("/marketing", h.MarketingDashboardPage)
			// Read-only ticket access for marketing
			r.Get("/marketing/tickets", h.TicketsPage)
			r.Get("/marketing/tickets/{id}", h.TicketDetailPage)
		})
	})

	return r
}
