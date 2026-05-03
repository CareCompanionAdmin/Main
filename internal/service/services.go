package service

import (
	"context"
	"database/sql"
	"log"

	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/repository"
)

// Services aggregates all service instances
type Services struct {
	Auth              *AuthService
	User              *UserService
	Family            *FamilyService
	Child             *ChildService
	Medication        *MedicationService
	Log               *LogService
	Alert             *AlertService
	Correlation       *CorrelationService
	Insight           *InsightService
	Cohort            *CohortService
	Chat              *ChatService
	DrugDatabase      *DrugDatabaseService
	Validation        *ValidationService
	AlertIntelligence *AlertIntelligenceService
	RealtimeDetection *RealtimeDetectionService
	Transparency      *TransparencyService
	UserSupport       *UserSupportService
	Billing           *BillingService
	Email             *EmailService
	PasswordReset     *PasswordResetService
	Push              *PushService
	Report            *ReportService
	Search            *SearchService
	Roadmap           *RoadmapService
	TicketDuplicate   *TicketDuplicateService
	TicketAttachment  *TicketAttachmentService
	AttachmentStorage AttachmentStorage
	AppStoreConnect   *AppStoreConnectService
	Beta              *BetaService
	Bounty            *BountyService
	Subscription      *SubscriptionService
	Stripe            *StripeService
	ChatHub           *ChatHub
}

// NewServices creates all services with their dependencies
func NewServices(repos *repository.Repositories, redis *database.Redis, cfg *config.Config, db *sql.DB) *Services {
	// Create services in dependency order
	emailService := NewEmailService(&cfg.SMTP)
	alertService := NewAlertService(repos.Alert, repos.Child)
	insightService := NewInsightService(repos.Insight, repos.Correlation, repos.Child)
	cohortService := NewCohortService(repos.Cohort, repos.Child, repos.Insight)
	chatService := NewChatService(repos.Chat, repos.User, repos.Family, repos.Child)
	transparencyService := NewTransparencyService(repos.Transparency, repos.Alert, repos.Child)

	pushService := NewPushService(repos.DeviceToken, cfg.FCM.ServerKey)
	pushService.InitFirebase(cfg.FCM.ServiceAccountKeyFile)

	attachmentStorage := NewAttachmentStorage(&cfg.Storage)

	// App Store Connect — nil when env vars are unset; BetaService falls back
	// to manual-add in that case rather than failing.
	ascService, ascErr := NewAppStoreConnectService(
		cfg.AppStoreConnect.IssuerID,
		cfg.AppStoreConnect.KeyID,
		cfg.AppStoreConnect.KeyPath,
		cfg.AppStoreConnect.BetaGroupName,
	)
	if ascErr != nil {
		log.Printf("[ASC] App Store Connect init failed; beta auto-add disabled: %v", ascErr)
	}

	// Wire push notifications into alert service (avoids circular constructor deps)
	alertService.SetPushService(pushService, repos.Family)

	svcs := &Services{
		Auth:              NewAuthService(repos.User, repos.Family, redis, &cfg.JWT, emailService, cfg.App.URL),
		User:              NewUserService(repos.User, repos.Family),
		Family:            NewFamilyService(repos.Family, repos.Child),
		Child:             NewChildService(repos.Child, repos.Family),
		Medication:        NewMedicationService(repos.Medication, repos.Transparency),
		Log:               NewLogService(repos.Log),
		Alert:             alertService,
		Correlation:       NewCorrelationService(repos.Correlation, alertService, repos.Child),
		Insight:           insightService,
		Cohort:            cohortService,
		Chat:              chatService,
		DrugDatabase:      NewDrugDatabaseService(),
		Validation:        NewValidationService(repos.Correlation, repos.Insight, repos.Medication),
		AlertIntelligence: NewAlertIntelligenceService(repos.Alert, repos.Correlation, repos.Insight),
		RealtimeDetection: NewRealtimeDetectionService(repos.Correlation, repos.Alert, repos.Child, repos.Medication, alertService),
		Transparency:      transparencyService,
		UserSupport:       NewUserSupportService(repos.UserSupport),
		Billing:           NewBillingService(repos.Billing, repos.Child),
		Email:             emailService,
		PasswordReset:     NewPasswordResetService(db, repos.User, emailService, cfg.App.URL),
		Push:              pushService,
		Report:            NewReportService(repos.Report, repos.Log, repos.Child, repos.Chat, cfg.Storage.UploadDir),
		Search:            NewSearchService(repos.Search),
		Roadmap:           NewRoadmapService(repos.Roadmap, repos.Admin, emailService, db),
		TicketDuplicate:   NewTicketDuplicateService(repos.Admin, repos.Roadmap, emailService),
		AttachmentStorage: attachmentStorage,
		TicketAttachment:  NewTicketAttachmentService(repos.TicketAttachment, repos.Admin, attachmentStorage, cfg.Storage.AttachmentMaxBytes, cfg.Storage.AttachmentMaxPerTkt),
		AppStoreConnect:   ascService,
		Beta:              NewBetaService(repos.BetaInvitation, emailService, ascService, cfg.App.URL, "/static/docs/beta-onboarding.html"),
		Bounty:            NewBountyService(repos.BountyAward, repos.Admin, emailService, db),
		ChatHub:           NewChatHub(),
	}
	// Subscription service has to come AFTER auth/family/child services exist
	// because we wire it INTO them below (signup → trial, add-child → bump).
	subSvc, subErr := NewSubscriptionService(db)
	if subErr != nil {
		log.Printf("[SUB] subscription service init failed; trial autoplay disabled: %v", subErr)
	} else {
		svcs.Subscription = subSvc
		svcs.Auth.SetSubscriptionService(subSvc)
		svcs.Family.SetSubscriptionService(subSvc)
		svcs.Child.SetSubscriptionService(subSvc)
	}
	// Wire attachment service into the close paths so PHI is purged on
	// every transition to closed/resolved (manual, dup, or promote).
	svcs.Roadmap.SetAttachmentService(svcs.TicketAttachment)
	svcs.TicketDuplicate.SetAttachmentService(svcs.TicketAttachment)
	// Stripe — enabled only when STRIPE_SECRET_KEY is set. EnsureAllPlansSynced
	// is fire-and-forget at boot: a Stripe outage shouldn't block the app from
	// starting (existing subscriptions keep working from DB state).
	if cfg.Stripe.Enabled() {
		svcs.Stripe = NewStripeService(cfg.Stripe, repos.Billing, cfg.App.URL)
		// Webhook dispatch needs SubscriptionService; if it's nil (plan rows
		// missing), webhook events will return an error and Stripe will retry.
		if svcs.Subscription != nil {
			svcs.Stripe.SetSubscriptionService(svcs.Subscription)
		}
		go func() {
			if err := svcs.Stripe.EnsureAllPlansSynced(context.Background()); err != nil {
				log.Printf("[STRIPE] plan sync failed: %v", err)
			}
		}()
	} else {
		log.Printf("[STRIPE] disabled (STRIPE_SECRET_KEY not set)")
	}
	return svcs
}
