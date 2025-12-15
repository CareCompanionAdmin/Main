package web

import (
	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// SetupRoutes configures all web routes
func SetupRoutes(r chi.Router, handlers *WebHandlers, authService *service.AuthService) {
	// Public routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.OptionalAuthMiddleware(authService))
		r.Get("/", handlers.Home)
		r.Get("/login", handlers.Login)
		r.Get("/register", handlers.Register)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(authService))

		r.Get("/dashboard", handlers.Dashboard)
		r.Get("/settings", handlers.Settings)
		r.Get("/family/new", handlers.NewFamily)
		r.Get("/child/new", handlers.NewChild)

		// Child-specific routes
		r.Route("/child/{childID}", func(r chi.Router) {
			r.Get("/", handlers.ChildDashboard)
			r.Get("/logs", handlers.DailyLogs)
			r.Get("/medications", handlers.Medications)
			r.Get("/alerts", handlers.Alerts)
			r.Get("/insights", handlers.Insights)
		})
	})
}
