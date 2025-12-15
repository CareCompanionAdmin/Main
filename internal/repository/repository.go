package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// UserRepository handles user data operations
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// FamilyRepository handles family data operations
type FamilyRepository interface {
	Create(ctx context.Context, family *models.Family) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Family, error)
	Update(ctx context.Context, family *models.Family) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Membership operations
	AddMember(ctx context.Context, membership *models.FamilyMembership) error
	RemoveMember(ctx context.Context, familyID, userID uuid.UUID) error
	GetMembers(ctx context.Context, familyID uuid.UUID) ([]models.FamilyMembership, error)
	GetMemberByID(ctx context.Context, memberID uuid.UUID) (*models.FamilyMembership, error)
	GetMembership(ctx context.Context, familyID, userID uuid.UUID) (*models.FamilyMembership, error)
	GetUserFamilies(ctx context.Context, userID uuid.UUID) ([]models.FamilyMembership, error)
	UpdateMemberRole(ctx context.Context, familyID, userID uuid.UUID, role models.FamilyRole) error
}

// ChildRepository handles child data operations
type ChildRepository interface {
	Create(ctx context.Context, child *models.Child) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Child, error)
	GetByFamilyID(ctx context.Context, familyID uuid.UUID) ([]models.Child, error)
	Update(ctx context.Context, child *models.Child) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Condition operations
	AddCondition(ctx context.Context, condition *models.ChildCondition) error
	GetConditions(ctx context.Context, childID uuid.UUID) ([]models.ChildCondition, error)
	UpdateCondition(ctx context.Context, condition *models.ChildCondition) error
	RemoveCondition(ctx context.Context, id uuid.UUID) error

	// Dashboard data
	GetDashboard(ctx context.Context, childID uuid.UUID, date time.Time) (*models.ChildDashboard, error)
}

// MedicationRepository handles medication data operations
type MedicationRepository interface {
	// Medication CRUD
	Create(ctx context.Context, med *models.Medication) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Medication, error)
	GetByChildID(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.Medication, error)
	Update(ctx context.Context, med *models.Medication) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Schedule operations
	CreateSchedule(ctx context.Context, schedule *models.MedicationSchedule) error
	GetSchedules(ctx context.Context, medicationID uuid.UUID) ([]models.MedicationSchedule, error)
	UpdateSchedule(ctx context.Context, schedule *models.MedicationSchedule) error
	DeleteSchedule(ctx context.Context, id uuid.UUID) error

	// Log operations
	CreateLog(ctx context.Context, log *models.MedicationLog) error
	GetLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error)
	GetLogsByMedication(ctx context.Context, medicationID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error)
	UpdateLog(ctx context.Context, log *models.MedicationLog) error

	// Due medications
	GetDueMedications(ctx context.Context, childID uuid.UUID, date time.Time) ([]models.MedicationDue, error)

	// Reference data
	GetMedicationReference(ctx context.Context, name string) (*models.MedicationReference, error)
	SearchMedicationReferences(ctx context.Context, query string) ([]models.MedicationReference, error)
}

// LogRepository handles all log types
type LogRepository interface {
	// Behavior logs
	CreateBehaviorLog(ctx context.Context, log *models.BehaviorLog) error
	GetBehaviorLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BehaviorLog, error)
	GetBehaviorLogByID(ctx context.Context, id uuid.UUID) (*models.BehaviorLog, error)
	UpdateBehaviorLog(ctx context.Context, log *models.BehaviorLog) error
	DeleteBehaviorLog(ctx context.Context, id uuid.UUID) error

	// Bowel logs
	CreateBowelLog(ctx context.Context, log *models.BowelLog) error
	GetBowelLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BowelLog, error)
	GetBowelLogByID(ctx context.Context, id uuid.UUID) (*models.BowelLog, error)
	DeleteBowelLog(ctx context.Context, id uuid.UUID) error

	// Speech logs
	CreateSpeechLog(ctx context.Context, log *models.SpeechLog) error
	GetSpeechLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SpeechLog, error)
	GetSpeechLogByID(ctx context.Context, id uuid.UUID) (*models.SpeechLog, error)
	DeleteSpeechLog(ctx context.Context, id uuid.UUID) error

	// Diet logs
	CreateDietLog(ctx context.Context, log *models.DietLog) error
	GetDietLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.DietLog, error)
	GetDietLogByID(ctx context.Context, id uuid.UUID) (*models.DietLog, error)
	DeleteDietLog(ctx context.Context, id uuid.UUID) error

	// Weight logs
	CreateWeightLog(ctx context.Context, log *models.WeightLog) error
	GetWeightLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.WeightLog, error)
	GetWeightLogByID(ctx context.Context, id uuid.UUID) (*models.WeightLog, error)
	DeleteWeightLog(ctx context.Context, id uuid.UUID) error

	// Sleep logs
	CreateSleepLog(ctx context.Context, log *models.SleepLog) error
	GetSleepLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SleepLog, error)
	GetSleepLogByID(ctx context.Context, id uuid.UUID) (*models.SleepLog, error)
	DeleteSleepLog(ctx context.Context, id uuid.UUID) error

	// Sensory logs
	CreateSensoryLog(ctx context.Context, log *models.SensoryLog) error
	GetSensoryLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SensoryLog, error)
	GetSensoryLogByID(ctx context.Context, id uuid.UUID) (*models.SensoryLog, error)
	DeleteSensoryLog(ctx context.Context, id uuid.UUID) error

	// Social logs
	CreateSocialLog(ctx context.Context, log *models.SocialLog) error
	GetSocialLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SocialLog, error)
	GetSocialLogByID(ctx context.Context, id uuid.UUID) (*models.SocialLog, error)
	DeleteSocialLog(ctx context.Context, id uuid.UUID) error

	// Therapy logs
	CreateTherapyLog(ctx context.Context, log *models.TherapyLog) error
	GetTherapyLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.TherapyLog, error)
	GetTherapyLogByID(ctx context.Context, id uuid.UUID) (*models.TherapyLog, error)
	DeleteTherapyLog(ctx context.Context, id uuid.UUID) error

	// Seizure logs
	CreateSeizureLog(ctx context.Context, log *models.SeizureLog) error
	GetSeizureLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SeizureLog, error)
	GetSeizureLogByID(ctx context.Context, id uuid.UUID) (*models.SeizureLog, error)
	DeleteSeizureLog(ctx context.Context, id uuid.UUID) error

	// Health event logs
	CreateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error
	GetHealthEventLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.HealthEventLog, error)
	GetHealthEventLogByID(ctx context.Context, id uuid.UUID) (*models.HealthEventLog, error)
	DeleteHealthEventLog(ctx context.Context, id uuid.UUID) error

	// Daily log page
	GetDailyLogs(ctx context.Context, childID uuid.UUID, date time.Time) (*models.DailyLogPage, error)
}

// AlertRepository handles alert operations
type AlertRepository interface {
	Create(ctx context.Context, alert *models.Alert) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error)
	GetByChildID(ctx context.Context, childID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error)
	GetByFamilyID(ctx context.Context, familyID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error)
	Update(ctx context.Context, alert *models.Alert) error
	Acknowledge(ctx context.Context, id, userID uuid.UUID) error
	Resolve(ctx context.Context, id, userID uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Feedback
	CreateFeedback(ctx context.Context, feedback *models.AlertFeedback) error
	GetFeedback(ctx context.Context, alertID uuid.UUID) ([]models.AlertFeedback, error)

	// Stats
	GetStats(ctx context.Context, childID uuid.UUID) (*models.AlertStats, error)

	// Alerts page data
	GetAlertsPage(ctx context.Context, childID uuid.UUID) (*models.AlertsPage, error)
}

// CorrelationRepository handles correlation and pattern operations
type CorrelationRepository interface {
	// Baselines
	CreateBaseline(ctx context.Context, baseline *models.ChildBaseline) error
	GetBaselines(ctx context.Context, childID uuid.UUID) ([]models.ChildBaseline, error)
	GetBaseline(ctx context.Context, childID uuid.UUID, metricName string) (*models.ChildBaseline, error)
	UpdateBaseline(ctx context.Context, baseline *models.ChildBaseline) error

	// Correlation requests
	CreateCorrelationRequest(ctx context.Context, req *models.CorrelationRequest) error
	GetCorrelationRequest(ctx context.Context, id uuid.UUID) (*models.CorrelationRequest, error)
	GetCorrelationRequests(ctx context.Context, childID uuid.UUID, status *models.CorrelationStatus) ([]models.CorrelationRequest, error)
	UpdateCorrelationRequest(ctx context.Context, req *models.CorrelationRequest) error

	// Patterns
	CreatePattern(ctx context.Context, pattern *models.FamilyPattern) error
	GetPattern(ctx context.Context, id uuid.UUID) (*models.FamilyPattern, error)
	GetPatterns(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.FamilyPattern, error)
	UpdatePattern(ctx context.Context, pattern *models.FamilyPattern) error
	DeletePattern(ctx context.Context, id uuid.UUID) error

	// Clinical validations
	CreateValidation(ctx context.Context, validation *models.ClinicalValidation) error
	GetValidations(ctx context.Context, childID uuid.UUID) ([]models.ClinicalValidation, error)
	GetValidation(ctx context.Context, id uuid.UUID) (*models.ClinicalValidation, error)

	// Insights page
	GetInsightsPage(ctx context.Context, childID uuid.UUID) (*models.InsightsPage, error)

	// Data for correlation engine
	GetCorrelationData(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (map[string][]models.DataPoint, error)
}

// Repositories aggregates all repository interfaces
type Repositories struct {
	User        UserRepository
	Family      FamilyRepository
	Child       ChildRepository
	Medication  MedicationRepository
	Log         LogRepository
	Alert       AlertRepository
	Correlation CorrelationRepository
}

// NewRepositories creates all repository implementations
func NewRepositories(db *sql.DB) *Repositories {
	return &Repositories{
		User:        NewUserRepo(db),
		Family:      NewFamilyRepo(db),
		Child:       NewChildRepo(db),
		Medication:  NewMedicationRepo(db),
		Log:         NewLogRepo(db),
		Alert:       NewAlertRepo(db),
		Correlation: NewCorrelationRepo(db),
	}
}
