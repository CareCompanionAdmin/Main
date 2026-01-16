package service

import (
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
}

// NewServices creates all services with their dependencies
func NewServices(repos *repository.Repositories, redis *database.Redis, cfg *config.Config) *Services {
	// Create services in dependency order
	alertService := NewAlertService(repos.Alert, repos.Child)
	insightService := NewInsightService(repos.Insight, repos.Correlation, repos.Child)
	cohortService := NewCohortService(repos.Cohort, repos.Child, repos.Insight)
	chatService := NewChatService(repos.Chat, repos.User, repos.Family, repos.Child)
	transparencyService := NewTransparencyService(repos.Transparency, repos.Alert, repos.Child)

	return &Services{
		Auth:              NewAuthService(repos.User, repos.Family, redis, &cfg.JWT),
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
	}
}
