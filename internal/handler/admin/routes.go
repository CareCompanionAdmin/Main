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
	pushService       *service.PushService
	roadmapService    *service.RoadmapService
	dupService        *service.TicketDuplicateService
	attachService     *service.TicketAttachmentService
	betaService       *service.BetaService
	bountyService     *service.BountyService
	liveSessionsService *service.LiveSessionsService
	proQAService        *service.ProQAService
	roleService         *service.RoleService
}

// SetRoleService wires the custom-role service for the role-builder UI.
func (h *Handler) SetRoleService(s *service.RoleService) {
	h.roleService = s
}

// SetProQAService wires the Pro QA workspace service.
func (h *Handler) SetProQAService(s *service.ProQAService) {
	h.proQAService = s
}

// SetBetaService wires the beta-invite orchestration service.
func (h *Handler) SetBetaService(s *service.BetaService) {
	h.betaService = s
}

// SetBountyService wires the monthly bounty-rewards service.
func (h *Handler) SetBountyService(s *service.BountyService) {
	h.bountyService = s
}

// SetPushService sets the push notification service for admin handlers
func (h *Handler) SetPushService(ps *service.PushService) {
	h.pushService = ps
}

// SetRoadmapService sets the roadmap service for admin handlers.
func (h *Handler) SetRoadmapService(rs *service.RoadmapService) {
	h.roadmapService = rs
}

// SetTicketDuplicateService wires the dup-handling service.
func (h *Handler) SetTicketDuplicateService(s *service.TicketDuplicateService) {
	h.dupService = s
}

// SetTicketAttachmentService wires the attachment service.
func (h *Handler) SetTicketAttachmentService(s *service.TicketAttachmentService) {
	h.attachService = s
}

// SetLiveSessionsService wires the live-sessions aggregator.
func (h *Handler) SetLiveSessionsService(s *service.LiveSessionsService) {
	h.liveSessionsService = s
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

	// Lightweight liveness probe used by admin_session_guard.js. AuthMiddleware
	// returns 401 on missing/expired/revoked session — handler just confirms 200.
	r.Get("/auth/check", h.AdminAuthCheck)

	// Sessions: super_admin + support can revoke individual sessions.
	// Role check is in the handler (not via middleware) so we can extend the
	// allowed roles in a later slice without restructuring the route tree.
	r.Delete("/sessions/{sessionID}", h.RevokeSession)

	// Live Sessions JSON + bulk + SSH kill — inline role check inside each
	// handler (super_admin / support / partner) so the routes can sit at the
	// admin top level alongside the existing single-session DELETE.
	r.Get("/sessions/live", h.ListLiveSessions)
	r.Post("/sessions/revoke", h.BulkRevokeSessions)
	r.Post("/sessions/ssh/kill", h.KillSSHSessionJSON)

	// Super admin routes — gates set per-section below (matrix-driven).
	r.Route("/super", func(r chi.Router) {
		// No blanket gate — each sub-section sets its own gate below.

		// Admin Users
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("admin_users"))
			r.Get("/admins", h.ListAdminUsers)
			r.Post("/admins", h.CreateAdminUser)
			r.Get("/admins/{id}", h.GetAdminUser)
			r.Put("/admins/{id}", h.UpdateAdminUser)
			r.Delete("/admins/{id}", h.DeleteAdminUser)
		})

		// Metrics Dashboard
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("metrics_dashboard"))
			r.Get("/metrics", h.GetSystemMetrics)
			r.Post("/metrics/refresh", h.RefreshMetrics)
		})

		// System Settings (super_admin only)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/settings", h.GetSettings)
			r.Put("/settings/{key}", h.UpdateSetting)
			r.Post("/maintenance", h.ToggleMaintenanceMode)
		})

		// Audit Log (super_admin only)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/audit-log", h.GetAuditLog)
		})

		// Infrastructure Status
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("infrastructure_status"))
			r.Get("/status", h.GetInfrastructureStatus)
			r.Post("/status/refresh", h.RefreshInfrastructureStatus)
			r.Get("/infra-files", h.ListInfraFiles)
			r.Get("/infra-files/download", h.DownloadInfraFile)
			r.Post("/infra-files/upload", h.UploadInfraFile)
			r.Get("/capacity", h.GetCapacity)
		})

		// Error Logs
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("error_logs"))
			r.Get("/errors", h.ListErrorLogs)
			r.Get("/errors/unacknowledged-count", h.GetUnacknowledgedErrorCount)
			r.Get("/errors/{id}", h.GetErrorLog)
			r.Post("/errors/{id}/acknowledge", h.AcknowledgeErrorLog)
			r.Post("/errors/acknowledge-bulk", h.AcknowledgeErrorLogsBulk)
			r.Delete("/errors/{id}", h.DeleteErrorLog)
			r.Post("/errors/delete-bulk", h.DeleteErrorLogsBulk)
			r.Post("/errors/{id}/create-ticket", h.CreateTicketFromError)
		})

		// Financials + Subscriptions (Partner=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("financials"))
			r.Get("/financials/overview", h.GetFinancialOverview)
			r.Get("/financials/calendar", h.GetExpectedRevenueCalendar)
			r.Get("/financials/payments", h.GetRecentPayments)
			r.Get("/financials/subscriptions", h.GetRecentSubscriptions)
			r.Get("/financials/plans", h.GetSubscriptionPlans)
			r.Get("/financials/report", h.GenerateFinancialReport)

			// Family-subscription admin tooling (Phase 1 of billing build).
			r.Get("/family-subscriptions", h.ListFamilySubscriptions)
			r.Get("/family-subscriptions/{family_id}", h.GetFamilySubscription)
			r.Put("/family-subscriptions/{family_id}", h.UpdateFamilySubscription)
			r.Post("/family-subscriptions/{family_id}/comp", h.CompFamilySubscription)
			r.Post("/family-subscriptions/{family_id}/cancel", h.CancelFamilySubscription)
		})

		// Promo Codes (Partner=read; full CRUD requires section=full → super_admin)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("promo_codes"))
			r.Get("/promo-codes", h.ListPromoCodes)
			r.Post("/promo-codes", h.CreatePromoCode)
			r.Get("/promo-codes/{id}", h.GetPromoCode)
			r.Put("/promo-codes/{id}", h.UpdatePromoCode)
			r.Post("/promo-codes/{id}/deactivate", h.DeactivatePromoCode)
			r.Get("/promo-codes/{id}/usages", h.GetPromoCodeUsages)
		})

		// Development Mode (super_admin only)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Post("/dev-mode/toggle", h.DevModeToggle)
			r.Post("/dev-mode/kill-session", h.DevModeKillSession)
			r.Get("/dev-mode/sessions", h.DevModeSessions)
			r.Get("/dev-mode/pem-key", h.DevModeGetPEMKey)
			r.Get("/dev-mode/pem-download", h.DevModeDownloadPEM)
			r.Get("/dev-mode/ppk-download", h.DevModeDownloadPPK)
			r.Post("/dev-mode/public-access", h.DevPublicAccessToggle)
			r.Get("/dev-mode/public-access", h.DevPublicAccessStatus)
		})

		// Version Log (Partner=read)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("version_log"))
			r.Get("/version-log", h.GetVersionLog)
		})

		// Roadmap (Partner=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("product_roadmap"))
			r.Get("/roadmap", h.ListRoadmapItems)
			r.Post("/roadmap", h.CreateRoadmapItem)
			r.Get("/roadmap/{id}", h.GetRoadmapItem)
			r.Put("/roadmap/{id}", h.UpdateRoadmapItem)
			r.Delete("/roadmap/{id}", h.DeleteRoadmapItem)
			r.Post("/roadmap/{id}/mark-live-dev", h.MarkRoadmapLiveDev)
			r.Post("/roadmap/{id}/mark-live-prod", h.MarkRoadmapLiveProd)
			// Promotion endpoint lives under super because it triggers a ticket close + email.
			r.Post("/tickets/{id}/add-to-roadmap", h.AddRoadmapFromTicket)
		})

		// Bulk-delete tickets — destructive, gated by tickets section (DELETE = full).
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("tickets"))
			r.Delete("/tickets", h.DeleteTickets)
		})
	})

	// Support routes — gated per section (tickets / users / families)
	r.Route("/support", func(r chi.Router) {
		// Tickets
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("tickets"))
			r.Get("/tickets/open-count", h.GetOpenTicketCount)
			r.Get("/tickets", h.ListTickets)
			r.Post("/tickets", h.CreateTicket)
			r.Get("/duplicate-targets", h.SearchDuplicateTargets)
			r.Get("/tickets/{id}", h.GetTicket)
			r.Put("/tickets/{id}", h.UpdateTicket)
			r.Post("/tickets/{id}/assign", h.AssignTicket)
			r.Post("/tickets/{id}/resolve", h.ResolveTicket)
			r.Get("/tickets/{id}/messages", h.GetTicketMessages)
			r.Post("/tickets/{id}/messages", h.AddTicketMessage)
			r.Post("/tickets/{id}/mark-duplicate", h.MarkTicketDuplicate)
			r.Get("/tickets/{id}/duplicates", h.ListTicketDuplicates)
			r.Get("/tickets/{id}/attachments", h.ListTicketAttachments)
			r.Get("/attachments/{id}", h.FetchTicketAttachment)
		})

		// Users
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("users"))
			r.Get("/users", h.SearchUsers)
			r.Get("/users/{id}", h.GetUser)
			r.Put("/users/{id}/status", h.UpdateUserStatus)
			r.Post("/users/{id}/reset-password", h.ResetUserPassword)
			r.Post("/users/{id}/reset-mfa", h.ResetUserMFA)
		})

		// Families
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("families"))
			r.Get("/families", h.ListFamilies)
			r.Get("/families/{id}", h.GetFamily)
		})
	})

	// Marketing routes — gated per section (matrix-driven)
	r.Route("/marketing", func(r chi.Router) {
		// Metrics Dashboard
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("metrics_dashboard"))
			r.Get("/dashboard", h.GetMarketingDashboard)
			r.Get("/metrics", h.GetMarketingMetrics)
		})

		// Tickets — read-only access via section gate (Marketing=read, Partner=full).
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("tickets"))
			r.Get("/tickets", h.ListTickets)
			r.Get("/tickets/{id}", h.GetTicket)
			r.Get("/tickets/{id}/messages", h.GetTicketMessages)
		})

		// Promo Codes — read-only for marketing/partner.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("promo_codes"))
			r.Get("/promo-codes", h.ListPromoCodes)
			r.Get("/promo-codes/{id}", h.GetPromoCode)
			r.Get("/promo-codes/{id}/usages", h.GetPromoCodeUsages)
		})

		// Marketing Materials (Partner=read; Marketing/SuperAdmin=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("copy_materials"))
			r.Get("/materials", h.ListMarketingAssets)
			r.Get("/materials/brand-config", h.GetBrandConfig)
			r.Get("/materials/assets/{id}/download", h.DownloadAsset)
			r.Get("/materials/social-templates", h.ListSocialTemplates)
			r.Post("/materials/social-graphic", h.GenerateSocialGraphic)
			r.Get("/materials/brochure", h.GenerateBrochure)
			r.Get("/materials/style-guide", h.GenerateStyleGuide)
			r.Get("/materials/logo", h.GenerateLogo)
		})

		// Beta program (marketing-managed TestFlight invites)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("beta_program"))
			r.Get("/beta/invitations", h.ListBetaInvitations)
			r.Post("/beta/invitations", h.CreateBetaInvitation)
			r.Post("/beta/invitations/{id}/resend", h.ResendBetaInvitation)
		})

		// Bounty program (monthly top-5+5 rewards) — Partner=read; Marketing=full.
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("bounty_program"))
			r.Get("/bounty/candidates", h.ListBountyCandidates)
			r.Post("/bounty/select", h.SelectBountyCandidate)
			r.Post("/bounty/thanks-anyway", h.ThanksAnywayBountyCandidate)
		})
	})

	// Super admin marketing material management (mutations on copy_materials).
	// Partner has read-only on copy_materials → these PUT/POST calls 403 for Partner.
	// Super_admin short-circuits and Marketing has full → both allowed.
	r.Route("/super/materials", func(r chi.Router) {
		r.Use(middleware.RequireSection("copy_materials"))
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

	// Protected UI routes — section gates per page (matrix-driven)
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(h.authService))
		r.Use(middleware.RequireAnyAdminRole())

		r.Get("/", h.AdminDashboard)
		r.Get("/dashboard", h.AdminDashboard)
		r.Post("/logout", h.AdminLogout)

		// Admin Users (Partner=read)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("admin_users"))
			r.Get("/admins", h.AdminUsersPage)
		})

		// User Roles — custom role builder. Super-admin only; the page
		// itself manages admin-portal permissions so anything less is a
		// privilege-escalation footgun.
		r.Route("/user-roles", func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/", h.UserRolesPage)
			r.Get("/new", h.UserRoleFormPage)
			r.Post("/", h.UserRoleCreate)
			r.Get("/{id}", h.UserRoleFormPage)
			r.Post("/{id}", h.UserRoleUpdate)
			r.Post("/{id}/delete", h.UserRoleDelete)
		})

		// Live Sessions (Partner=full, super_admin/support=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("live_sessions"))
			r.Get("/sessions", h.LiveSessionsPage)
		})

		// System pages (super_admin only — Settings, Audit, Development)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/settings", h.SettingsPage)
			r.Get("/audit", h.AuditLogPage)
			r.Get("/development", h.DevelopmentPage)
		})

		// Status page (Partner=read)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("infrastructure_status"))
			r.Get("/status", h.StatusPage)
			r.Get("/capacity", h.CapacityPage)
		})

		// Errors page (Partner=read)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("error_logs"))
			r.Get("/errors", h.ErrorsPage)
		})

		// Financials (Partner=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("financials"))
			r.Get("/financials", h.FinancialsPage)
		})

		// Subscriptions (Partner=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("subscriptions"))
			r.Get("/subscriptions", h.SubscriptionsPage)
		})

		// Promo codes — list page (Partner=read OK)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("promo_codes"))
			r.Get("/promo-codes", h.PromoCodesPage)
		})
		// Promo code create/edit (super_admin only — Partner has read, gets blocked here)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSuperAdmin())
			r.Get("/promo-codes/new", h.PromoCodeNewPage)
			r.Get("/promo-codes/{id}/edit", h.PromoCodeEditPage)
		})

		// Version Log (Partner=read)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("version_log"))
			r.Get("/version-log", h.VersionLogPage)
		})

		// Roadmap (Partner=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("product_roadmap"))
			r.Get("/roadmap", h.RoadmapListPage)
			r.Get("/roadmap/new", h.RoadmapNewPage)
			r.Get("/roadmap/{id}", h.RoadmapDetailPage)
			r.Get("/roadmap/{id}/edit", h.RoadmapEditPage)
		})

		// Pro QA workspace. Gated by the role-builder section "pro_qa";
		// super_admin still has Full implicitly, plus any custom role with
		// pro_qa read/write granted via /admin/user-roles.
		r.Route("/pro-qa", func(r chi.Router) {
			r.Use(middleware.RequireSection("pro_qa"))

			r.Get("/", h.ProQAIntroPage)
			r.Get("/intro", h.ProQAIntroPage)

			r.Get("/info", h.ProQAInfoPage)
			r.Post("/info", h.ProQAInfoSave)

			r.Get("/checks", h.ProQAChecksPage)
			r.Post("/checks", h.ProQAChecksCreate)
			r.Get("/checks/{id}", h.ProQACheckDetailPage)
			r.Post("/checks/{id}", h.ProQAChecksUpdate)
			r.Post("/checks/{id}/delete", h.ProQAChecksDelete)
			r.Post("/checks/{id}/status", h.ProQACheckChangeStatus)
			r.Post("/checks/{id}/comment", h.ProQACheckComment)
			r.Post("/checks/{id}/attach", h.ProQACheckUploadAttachment)
			r.Get("/check-attachments/{id}", h.ProQAFetchCheckAttachment)

			r.Get("/issues", h.ProQAIssuesPage)
			r.Post("/issues", h.ProQAIssueCreate)
			r.Get("/issues/{id}", h.ProQAIssueDetailPage)
			r.Post("/issues/{id}", h.ProQAIssueUpdate)
			r.Post("/issues/{id}/status", h.ProQAIssueChangeStatus)
			r.Post("/issues/{id}/comment", h.ProQAIssueComment)
			r.Post("/issues/{id}/attach", h.ProQAUploadAttachment)

			r.Get("/attachments/{id}", h.ProQAFetchAttachment)
		})

		// Tickets / Users / Families (Partner=full, Support=full)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("tickets"))
			r.Get("/tickets", h.TicketsPage)
			r.Get("/tickets/{id}", h.TicketDetailPage)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("users"))
			r.Get("/users", h.UsersPage)
		})
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("families"))
			r.Get("/families", h.FamiliesPage)
		})

		// Marketing dashboard
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("metrics_dashboard"))
			r.Get("/marketing", h.MarketingDashboardPage)
		})
		// Marketing tickets pages (read-only for marketing/partner)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("tickets"))
			r.Get("/marketing/tickets", h.TicketsPage)
			r.Get("/marketing/tickets/{id}", h.TicketDetailPage)
		})
		// Marketing materials page
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("copy_materials"))
			r.Get("/materials", h.MaterialsPage)
		})
		// Beta program page
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("beta_program"))
			r.Get("/marketing/beta", h.BetaProgramPage)
		})
		// Bounty program page
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireSection("bounty_program"))
			r.Get("/marketing/bounty", h.BountyProgramPage)
		})
	})

	return r
}
