package service

import (
	"carecompanion/internal/config"
	"carecompanion/internal/database"
	"carecompanion/internal/repository"
)

// Services aggregates all service instances
type Services struct {
	Auth        *AuthService
	User        *UserService
	Family      *FamilyService
	Child       *ChildService
	Medication  *MedicationService
	Log         *LogService
	Alert       *AlertService
	Correlation *CorrelationService
}

// NewServices creates all services with their dependencies
func NewServices(repos *repository.Repositories, redis *database.Redis, cfg *config.Config) *Services {
	// Create services in dependency order
	alertService := NewAlertService(repos.Alert, repos.Child)

	return &Services{
		Auth:        NewAuthService(repos.User, repos.Family, redis, &cfg.JWT),
		User:        NewUserService(repos.User, repos.Family),
		Family:      NewFamilyService(repos.Family, repos.Child),
		Child:       NewChildService(repos.Child, repos.Family),
		Medication:  NewMedicationService(repos.Medication),
		Log:         NewLogService(repos.Log),
		Alert:       alertService,
		Correlation: NewCorrelationService(repos.Correlation, alertService, repos.Child),
	}
}
