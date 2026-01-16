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
	marketingService  *service.MarketingService
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

// SetMarketingService sets the Marketing service for material generation
func (h *Handler) SetMarketingService(ms *service.MarketingService) {
	h.marketingService = ms
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

		// Infrastructure Status
		r.Get("/status", h.GetInfrastructureStatus)
		r.Post("/status/refresh", h.RefreshInfrastructureStatus)

		// Error Logs
		r.Get("/errors", h.ListErrorLogs)
		r.Get("/errors/unacknowledged-count", h.GetUnacknowledgedErrorCount)
		r.Get("/errors/{id}", h.GetErrorLog)
		r.Post("/errors/{id}/acknowledge", h.AcknowledgeErrorLog)
		r.Post("/errors/acknowledge-bulk", h.AcknowledgeErrorLogsBulk)
		r.Delete("/errors/{id}", h.DeleteErrorLog)
		r.Post("/errors/delete-bulk", h.DeleteErrorLogsBulk)
		r.Post("/errors/{id}/create-ticket", h.CreateTicketFromError)

		// Financials
		r.Get("/financials/overview", h.GetFinancialOverview)
		r.Get("/financials/calendar", h.GetExpectedRevenueCalendar)
		r.Get("/financials/payments", h.GetRecentPayments)
		r.Get("/financials/subscriptions", h.GetRecentSubscriptions)
		r.Get("/financials/plans", h.GetSubscriptionPlans)
		r.Get("/financials/report", h.GenerateFinancialReport)

		// Promo Codes (full CRUD)
		r.Get("/promo-codes", h.ListPromoCodes)
		r.Post("/promo-codes", h.CreatePromoCode)
		r.Get("/promo-codes/{id}", h.GetPromoCode)
		r.Put("/promo-codes/{id}", h.UpdatePromoCode)
		r.Post("/promo-codes/{id}/deactivate", h.DeactivatePromoCode)
		r.Get("/promo-codes/{id}/usages", h.GetPromoCodeUsages)
	})

	// Support routes
	r.Route("/support", func(r chi.Router) {
		r.Use(middleware.RequireSupport())
		r.Get("/tickets/open-count", h.GetOpenTicketCount)
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
		// Read-only promo code access for marketing
		r.Get("/promo-codes", h.ListPromoCodes)
		r.Get("/promo-codes/{id}", h.GetPromoCode)
		r.Get("/promo-codes/{id}/usages", h.GetPromoCodeUsages)

		// Marketing Materials (read access for both marketing and super_admin)
		r.Get("/materials", h.ListMarketingAssets)
		r.Get("/materials/brand-config", h.GetBrandConfig)
		r.Get("/materials/assets/{id}/download", h.DownloadAsset)
		r.Get("/materials/social-templates", h.ListSocialTemplates)
		r.Post("/materials/social-graphic", h.GenerateSocialGraphic)
		r.Get("/materials/brochure", h.GenerateBrochure)
		r.Get("/materials/style-guide", h.GenerateStyleGuide)
		r.Get("/materials/logo", h.GenerateLogo)
	})

	// Super admin marketing material management (edit access)
	r.Route("/super/materials", func(r chi.Router) {
		r.Use(middleware.RequireSuperAdmin())
		r.Put("/brand-config", h.UpdateBrandConfig)
		r.Post("/regenerate/{type}", h.RegenerateAsset)
		r.Post("/regenerate-all", h.RegenerateAllAssets)
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

			// New super admin pages
			r.Get("/status", h.StatusPage)
			r.Get("/errors", h.ErrorsPage)
			r.Get("/financials", h.FinancialsPage)
			// Promo code create/edit (super_admin only)
			r.Get("/promo-codes/new", h.PromoCodeNewPage)
			r.Get("/promo-codes/{id}/edit", h.PromoCodeEditPage)
		})

		// Support pages
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSupport())
			r.Get("/tickets", h.TicketsPage)
			r.Get("/tickets/{id}", h.TicketDetailPage)
			r.Get("/users", h.UsersPage)
			r.Get("/families", h.FamiliesPage)
		})

		// Marketing pages (marketing role only)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireMarketing())
			r.Get("/marketing", h.MarketingDashboardPage)
			// Read-only ticket access for marketing
			r.Get("/marketing/tickets", h.TicketsPage)
			r.Get("/marketing/tickets/{id}", h.TicketDetailPage)
			// Marketing materials page
			r.Get("/materials", h.MaterialsPage)
		})

		// Promo codes - accessible by both super_admin and marketing (view only for marketing)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireMarketing())
			r.Get("/promo-codes", h.PromoCodesPage)
		})
	})

	return r
}
