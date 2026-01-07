package api

import (
	"github.com/go-chi/chi/v5"

	"carecompanion/internal/config"
	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// Handlers aggregates all API handlers
type Handlers struct {
	Auth         *AuthHandler
	Child        *ChildHandler
	Family       *FamilyHandler
	Medication   *MedicationHandler
	Log          *LogHandler
	Alert        *AlertHandler
	Correlation  *CorrelationHandler
	Insight      *InsightHandler
	Chat         *ChatHandler
	Transparency *TransparencyHandler
}

// NewHandlers creates all API handlers
func NewHandlers(services *service.Services, cfg *config.Config) *Handlers {
	return &Handlers{
		Auth:         NewAuthHandler(services.Auth),
		Child:        NewChildHandler(services.Child),
		Family:       NewFamilyHandler(services.Family, services.User),
		Medication:   NewMedicationHandler(services.Medication, services.Child, services.DrugDatabase, services.Insight),
		Log:          NewLogHandler(services.Log, services.Child),
		Alert:        NewAlertHandler(services.Alert, services.Child),
		Correlation:  NewCorrelationHandler(services.Correlation, services.Child),
		Insight:      NewInsightHandler(services.Insight, services.Child),
		Chat:         NewChatHandler(services.Chat, services.Family, &cfg.Storage),
		Transparency: NewTransparencyHandler(services.Transparency),
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
			r.Put("/conditions/{id}", handlers.Child.UpdateCondition)
			r.Delete("/conditions/{id}", handlers.Child.RemoveCondition)

			// Medications
			r.Route("/medications", func(r chi.Router) {
				r.Get("/", handlers.Medication.List)
				r.Post("/", handlers.Medication.Create)
				r.Get("/due", handlers.Medication.GetDue)
				r.Get("/adherence", handlers.Medication.GetAdherence)
				r.Post("/log", handlers.Medication.Log)
				r.Get("/logs", handlers.Medication.GetLogs)
				r.Put("/logs/{logID}", handlers.Medication.UpdateLog)
				r.Delete("/logs/{logID}", handlers.Medication.DeleteLog)
				r.Get("/interactions", handlers.Medication.CheckInteractions)
				r.Get("/medical-insights", handlers.Medication.GetMedicalInsights)
				r.Get("/history", handlers.Medication.GetHistory)

				// Specific medication routes (must be after static routes)
				r.Route("/{medID}", func(r chi.Router) {
					r.Get("/", handlers.Medication.Get)
					r.Put("/", handlers.Medication.Update)
					r.Delete("/", handlers.Medication.Delete)
					r.Post("/discontinue", handlers.Medication.Discontinue)
				})
			})

			// Logs
			r.Route("/logs", func(r chi.Router) {
				r.Get("/daily", handlers.Log.GetDailyLogs)
				r.Get("/dates", handlers.Log.GetDatesWithLogs)
				r.Get("/quick-summary", handlers.Log.GetQuickSummary)

				// Behavior logs
				r.Get("/behavior", handlers.Log.GetBehaviorLogs)
				r.Post("/behavior", handlers.Log.CreateBehaviorLog)
				r.Put("/behavior/{id}", handlers.Log.UpdateBehaviorLog)
				r.Delete("/behavior/{id}", handlers.Log.DeleteBehaviorLog)

				// Bowel logs
				r.Get("/bowel", handlers.Log.GetBowelLogs)
				r.Post("/bowel", handlers.Log.CreateBowelLog)
				r.Put("/bowel/{id}", handlers.Log.UpdateBowelLog)
				r.Delete("/bowel/{id}", handlers.Log.DeleteBowelLog)

				// Speech logs
				r.Get("/speech", handlers.Log.GetSpeechLogs)
				r.Post("/speech", handlers.Log.CreateSpeechLog)
				r.Put("/speech/{id}", handlers.Log.UpdateSpeechLog)
				r.Delete("/speech/{id}", handlers.Log.DeleteSpeechLog)

				// Diet logs
				r.Get("/diet", handlers.Log.GetDietLogs)
				r.Post("/diet", handlers.Log.CreateDietLog)
				r.Put("/diet/{id}", handlers.Log.UpdateDietLog)
				r.Delete("/diet/{id}", handlers.Log.DeleteDietLog)

				// Weight logs
				r.Get("/weight", handlers.Log.GetWeightLogs)
				r.Post("/weight", handlers.Log.CreateWeightLog)
				r.Put("/weight/{id}", handlers.Log.UpdateWeightLog)
				r.Delete("/weight/{id}", handlers.Log.DeleteWeightLog)

				// Sleep logs
				r.Get("/sleep", handlers.Log.GetSleepLogs)
				r.Post("/sleep", handlers.Log.CreateSleepLog)
				r.Put("/sleep/{id}", handlers.Log.UpdateSleepLog)
				r.Delete("/sleep/{id}", handlers.Log.DeleteSleepLog)

				// Sensory logs
				r.Get("/sensory", handlers.Log.GetSensoryLogs)
				r.Post("/sensory", handlers.Log.CreateSensoryLog)
				r.Put("/sensory/{id}", handlers.Log.UpdateSensoryLog)
				r.Delete("/sensory/{id}", handlers.Log.DeleteSensoryLog)

				// Social logs
				r.Get("/social", handlers.Log.GetSocialLogs)
				r.Post("/social", handlers.Log.CreateSocialLog)
				r.Put("/social/{id}", handlers.Log.UpdateSocialLog)
				r.Delete("/social/{id}", handlers.Log.DeleteSocialLog)

				// Therapy logs
				r.Get("/therapy", handlers.Log.GetTherapyLogs)
				r.Post("/therapy", handlers.Log.CreateTherapyLog)
				r.Put("/therapy/{id}", handlers.Log.UpdateTherapyLog)
				r.Delete("/therapy/{id}", handlers.Log.DeleteTherapyLog)

				// Seizure logs
				r.Get("/seizure", handlers.Log.GetSeizureLogs)
				r.Post("/seizure", handlers.Log.CreateSeizureLog)
				r.Put("/seizure/{id}", handlers.Log.UpdateSeizureLog)
				r.Delete("/seizure/{id}", handlers.Log.DeleteSeizureLog)

				// Health event logs
				r.Get("/health", handlers.Log.GetHealthEventLogs)
				r.Post("/health", handlers.Log.CreateHealthEventLog)
				r.Put("/health/{id}", handlers.Log.UpdateHealthEventLog)
				r.Delete("/health/{id}", handlers.Log.DeleteHealthEventLog)
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
				r.Get("/tiered", handlers.Insight.GetInsightsByTier) // Three-Tier Learning System
				r.Get("/top", handlers.Insight.GetTopInsights)       // Top insights across tiers
				r.Get("/patterns", handlers.Correlation.GetPatterns)
				r.Get("/patterns/top", handlers.Correlation.GetTopPatterns)
				r.Get("/baselines", handlers.Correlation.GetBaselines)
				r.Post("/baselines/recalculate", handlers.Correlation.RecalculateBaselines)
				r.Get("/validations", handlers.Correlation.GetValidations)
				r.Post("/validations", handlers.Correlation.CreateValidation)
			})

			r.Route("/insights/{insightID}", func(r chi.Router) {
				r.Post("/validate", handlers.Insight.ValidateInsight)
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

		// Drug database routes (FDA validation)
		r.Route("/drugs", func(r chi.Router) {
			r.Get("/validate", handlers.Medication.ValidateDrug)
			r.Get("/info", handlers.Medication.GetDrugInfo)
			r.Get("/image-proxy", handlers.Medication.ProxyDrugImage)
		})

		// Chat routes - require family context
		r.Route("/chat", func(r chi.Router) {
			r.Use(middleware.RequireFamilyContext())
			r.Get("/threads", handlers.Chat.ListThreads)
			r.Post("/threads", handlers.Chat.CreateThread)
			r.Get("/unread", handlers.Chat.GetUnreadCount)
			r.Get("/files/{filename}", handlers.Chat.ServeFile)

			r.Route("/threads/{threadID}", func(r chi.Router) {
				r.Get("/", handlers.Chat.GetThread)
				r.Delete("/", handlers.Chat.DeleteThread)
				r.Get("/messages", handlers.Chat.GetMessages)
				r.Post("/messages", handlers.Chat.SendMessage)
				r.Get("/participants", handlers.Chat.GetParticipants)
				r.Post("/participants", handlers.Chat.AddParticipant)
				r.Delete("/participants/{participantID}", handlers.Chat.RemoveParticipant)
				r.Post("/upload", handlers.Chat.UploadFile)
			})
		})

		// Transparency routes - alert analysis and confidence breakdown
		r.Route("/alerts/{alertID}", func(r chi.Router) {
			r.Get("/analysis", handlers.Transparency.GetAlertAnalysis)
			r.Get("/confidence-factors", handlers.Transparency.GetConfidenceFactors)
			r.Post("/feedback", handlers.Transparency.SubmitAlertFeedback)
			r.Post("/export", handlers.Transparency.ExportAlert)
		})

		// Treatment change interrogatives
		r.Route("/treatment-changes", func(r chi.Router) {
			r.Get("/pending-questions", handlers.Transparency.GetPendingInterrogatives)
			r.Post("/{changeID}/respond", handlers.Transparency.RespondToTreatmentChange)
		})

		// Interaction alerts (for remind me later)
		r.Post("/interaction-alerts", handlers.Transparency.CreateInteractionAlert)

		// User interaction preferences
		r.Get("/users/me/interaction-preferences", handlers.Transparency.GetInteractionPreferences)
		r.Put("/users/me/interaction-preferences", handlers.Transparency.UpdateInteractionPreferences)

		// User display preferences (timezone, theme)
		r.Get("/users/me/preferences", handlers.Family.GetUserPreferences)
		r.Put("/users/me/preferences", handlers.Family.UpdateUserPreferences)
	})
}
