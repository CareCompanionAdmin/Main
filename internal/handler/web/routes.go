package web

import (
	"database/sql"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// SetupRoutes configures all web routes. db is required for the
// entitlement middleware that reads family_subscriptions per request.
func SetupRoutes(r chi.Router, handlers *WebHandlers, authService *service.AuthService, db *sql.DB) {
	// Stripe webhook — public, no auth, no body decoding middleware.
	// Mounted before any auth groups so middlewares can't intercept.
	r.Post("/webhooks/stripe", handlers.StripeWebhook)

	// Public routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.OptionalAuthMiddleware(authService))
		r.Get("/", handlers.Landing)
		r.Get("/login", handlers.Login)
		r.Get("/register", handlers.Register)
		r.Get("/privacy", handlers.Privacy)
		r.Get("/terms", handlers.Terms)
		r.Get("/help", handlers.Help)
		// Account-deletion landing pages. Public — the user is signed out
		// by the time they hit these. The restore page validates a token
		// from the email; the pending page is informational.
		r.Get("/account/restore", handlers.AccountRestore)
		r.Get("/account/deletion-pending", handlers.AccountDeletionPending)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(authService))
		r.Use(middleware.LoadEntitlement(db))

		r.Get("/dashboard", handlers.Dashboard)
		r.Get("/settings", handlers.Settings)
		r.Get("/chat", handlers.Chat)
		r.Get("/support", handlers.Support)
		r.Get("/family/new", handlers.NewFamily)
		r.Get("/child/new", handlers.NewChild)

		// Billing — Stripe Checkout flow.
		r.Post("/billing/checkout", handlers.CheckoutPost)
		r.Get("/billing/success", handlers.BillingSuccess)
		r.Get("/billing/cancel", handlers.BillingCancel)

		// Child-specific routes
		r.Route("/child/{childID}", func(r chi.Router) {
			r.Get("/", handlers.ChildDashboard)
			r.Get("/logs", handlers.DailyLogs)
			r.Get("/medications", handlers.Medications)
			r.Get("/alerts", handlers.Alerts)
			r.Get("/alert/{alertID}/analysis", handlers.AlertAnalysis)
			r.Get("/insights", handlers.Insights)
			r.Get("/reports", handlers.Reports)
			r.Get("/settings", handlers.ChildSettings)
		})
	})
}
