package api

import (
	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// Handlers aggregates all API handlers
type Handlers struct {
	Auth        *AuthHandler
	Child       *ChildHandler
	Family      *FamilyHandler
	Medication  *MedicationHandler
	Log         *LogHandler
	Alert       *AlertHandler
	Correlation *CorrelationHandler
}

// NewHandlers creates all API handlers
func NewHandlers(services *service.Services) *Handlers {
	return &Handlers{
		Auth:        NewAuthHandler(services.Auth),
		Child:       NewChildHandler(services.Child),
		Family:      NewFamilyHandler(services.Family, services.User),
		Medication:  NewMedicationHandler(services.Medication, services.Child),
		Log:         NewLogHandler(services.Log, services.Child),
		Alert:       NewAlertHandler(services.Alert, services.Child),
		Correlation: NewCorrelationHandler(services.Correlation, services.Child),
	}
}

// SetupRoutes configures all API routes
func SetupRoutes(r chi.Router, handlers *Handlers, authService *service.AuthService) {
	// Public routes
	r.Group(func(r chi.Router) {
		r.Post("/auth/register", handlers.Auth.Register)
		r.Post("/auth/login", handlers.Auth.Login)
		r.Post("/auth/refresh", handlers.Auth.RefreshToken)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(authService))

		// Auth routes
		r.Post("/auth/logout", handlers.Auth.Logout)
		r.Get("/auth/me", handlers.Auth.Me)
		r.Post("/auth/switch-family", handlers.Auth.SwitchFamily)

		// Family routes - require family context
		r.Route("/family", func(r chi.Router) {
			r.Use(middleware.RequireFamilyContext())
			r.Get("/info", handlers.Family.GetInfo)
			r.Get("/members", handlers.Family.ListMembers)
			r.Post("/members", handlers.Family.AddMember)
			r.Post("/members/lookup", handlers.Family.LookupUser)
			r.Get("/members/{memberID}", handlers.Family.GetMember)
			r.Patch("/members/{memberID}", handlers.Family.UpdateMemberRole)
			r.Delete("/members/{memberID}", handlers.Family.RemoveMember)
		})

		// Child routes - require family context
		r.Route("/children", func(r chi.Router) {
			r.Use(middleware.RequireFamilyContext())
			r.Get("/", handlers.Child.List)
			r.Post("/", handlers.Child.Create)
		})

		// Child-specific routes
		r.Route("/children/{childID}", func(r chi.Router) {
			r.Get("/", handlers.Child.Get)
			r.Put("/", handlers.Child.Update)
			r.Delete("/", handlers.Child.Delete)
			r.Get("/dashboard", handlers.Child.Dashboard)

			// Conditions
			r.Get("/conditions", handlers.Child.GetConditions)
			r.Post("/conditions", handlers.Child.AddCondition)
			r.Delete("/conditions/{id}", handlers.Child.RemoveCondition)

			// Medications
			r.Route("/medications", func(r chi.Router) {
				r.Get("/", handlers.Medication.List)
				r.Post("/", handlers.Medication.Create)
				r.Get("/due", handlers.Medication.GetDue)
				r.Get("/adherence", handlers.Medication.GetAdherence)
				r.Post("/log", handlers.Medication.Log)
				r.Get("/logs", handlers.Medication.GetLogs)
			})

			r.Route("/medications/{medID}", func(r chi.Router) {
				r.Get("/", handlers.Medication.Get)
				r.Put("/", handlers.Medication.Update)
				r.Delete("/", handlers.Medication.Delete)
			})

			// Logs
			r.Route("/logs", func(r chi.Router) {
				r.Get("/daily", handlers.Log.GetDailyLogs)
				r.Get("/dates", handlers.Log.GetDatesWithLogs)

				// Behavior logs
				r.Get("/behavior", handlers.Log.GetBehaviorLogs)
				r.Post("/behavior", handlers.Log.CreateBehaviorLog)
				r.Delete("/behavior/{id}", handlers.Log.DeleteBehaviorLog)

				// Bowel logs
				r.Get("/bowel", handlers.Log.GetBowelLogs)
				r.Post("/bowel", handlers.Log.CreateBowelLog)
				r.Delete("/bowel/{id}", handlers.Log.DeleteBowelLog)

				// Speech logs
				r.Get("/speech", handlers.Log.GetSpeechLogs)
				r.Post("/speech", handlers.Log.CreateSpeechLog)

				// Diet logs
				r.Get("/diet", handlers.Log.GetDietLogs)
				r.Post("/diet", handlers.Log.CreateDietLog)

				// Weight logs
				r.Get("/weight", handlers.Log.GetWeightLogs)
				r.Post("/weight", handlers.Log.CreateWeightLog)

				// Sleep logs
				r.Get("/sleep", handlers.Log.GetSleepLogs)
				r.Post("/sleep", handlers.Log.CreateSleepLog)

				// Sensory logs
				r.Get("/sensory", handlers.Log.GetSensoryLogs)
				r.Post("/sensory", handlers.Log.CreateSensoryLog)

				// Social logs
				r.Get("/social", handlers.Log.GetSocialLogs)
				r.Post("/social", handlers.Log.CreateSocialLog)

				// Therapy logs
				r.Get("/therapy", handlers.Log.GetTherapyLogs)
				r.Post("/therapy", handlers.Log.CreateTherapyLog)

				// Seizure logs
				r.Get("/seizure", handlers.Log.GetSeizureLogs)
				r.Post("/seizure", handlers.Log.CreateSeizureLog)

				// Health event logs
				r.Get("/health", handlers.Log.GetHealthEventLogs)
				r.Post("/health", handlers.Log.CreateHealthEventLog)
			})

			// Alerts
			r.Route("/alerts", func(r chi.Router) {
				r.Get("/", handlers.Alert.List)
				r.Get("/page", handlers.Alert.GetAlertsPage)
				r.Get("/stats", handlers.Alert.GetStats)
			})

			r.Route("/alerts/{alertID}", func(r chi.Router) {
				r.Get("/", handlers.Alert.Get)
				r.Post("/acknowledge", handlers.Alert.Acknowledge)
				r.Post("/resolve", handlers.Alert.Resolve)
				r.Get("/feedback", handlers.Alert.GetFeedback)
				r.Post("/feedback", handlers.Alert.CreateFeedback)
			})

			// Correlations & Insights
			r.Route("/insights", func(r chi.Router) {
				r.Get("/", handlers.Correlation.GetInsights)
				r.Get("/patterns", handlers.Correlation.GetPatterns)
				r.Get("/patterns/top", handlers.Correlation.GetTopPatterns)
				r.Get("/baselines", handlers.Correlation.GetBaselines)
				r.Post("/baselines/recalculate", handlers.Correlation.RecalculateBaselines)
				r.Get("/validations", handlers.Correlation.GetValidations)
				r.Post("/validations", handlers.Correlation.CreateValidation)
			})

			r.Route("/correlations", func(r chi.Router) {
				r.Get("/", handlers.Correlation.ListCorrelationRequests)
				r.Post("/", handlers.Correlation.CreateCorrelationRequest)
			})

			r.Route("/patterns/{patternID}", func(r chi.Router) {
				r.Get("/", handlers.Correlation.GetPattern)
				r.Delete("/", handlers.Correlation.DeletePattern)
			})
		})

		r.Route("/correlations/{correlationID}", func(r chi.Router) {
			r.Get("/", handlers.Correlation.GetCorrelationRequest)
		})

		// Medication reference search
		r.Get("/medication-references", handlers.Medication.SearchReferences)
	})
}
