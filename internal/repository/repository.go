package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// UserRepository handles user data operations.
//
// Post-00032: schema is split into admin_users + app_users. This interface
// keeps a kind-AGNOSTIC face for backward compatibility (Create writes to
// app_users; UpdateStatus/UpdateLastLogin/Delete fan out to both tables;
// GetByID reads from a UNION view; GetByEmail is also via the view but
// non-deterministic when an email exists in both tables). Two new
// kind-AWARE methods exist for code paths that must distinguish:
// GetAdminByEmail and GetAppByEmail. Admin LOGIN must use GetAdminByEmail;
// parent login + register must use GetAppByEmail.
type UserRepository interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
	GetAdminByEmail(ctx context.Context, email string) (*models.User, error)
	GetAppByEmail(ctx context.Context, email string) (*models.User, error)
	Update(ctx context.Context, user *models.User) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.UserStatus) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error
	Delete(ctx context.Context, id uuid.UUID) error
	GetOnboardingState(ctx context.Context, id uuid.UUID) (*models.OnboardingState, error)
	SetOnboardingCompleted(ctx context.Context, id uuid.UUID) error
	SetOnboardingChecklistDismissed(ctx context.Context, id uuid.UUID) error
	SetOnboardingSettingsDone(ctx context.Context, id uuid.UUID) error
	SetOnboardingInviteDone(ctx context.Context, id uuid.UUID) error
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

	// Invitation operations
	CreateInvitation(ctx context.Context, familyID uuid.UUID, email, firstName, lastName string, role models.FamilyRole) error
	GetPendingInvitations(ctx context.Context, email string) ([]models.FamilyInvitation, error)
	AcceptInvitation(ctx context.Context, invitationID uuid.UUID) error

	// Aggregate stats
	GetWeekStats(ctx context.Context, familyID uuid.UUID) (*models.FamilyWeekStats, error)
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
	DeactivateAllSchedules(ctx context.Context, medicationID uuid.UUID) error
	DeactivateSchedule(ctx context.Context, id uuid.UUID) error
	ReconcileSchedules(ctx context.Context, medicationID uuid.UUID, desired []models.MedicationSchedule) error

	// Log operations
	CreateLog(ctx context.Context, log *models.MedicationLog) error
	GetLogByID(ctx context.Context, id uuid.UUID) (*models.MedicationLog, error)
	GetLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error)
	GetLogsByMedication(ctx context.Context, medicationID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error)
	GetLogsByMedicationSince(ctx context.Context, medicationID uuid.UUID, since time.Time) ([]models.MedicationLog, error)
	UpdateLog(ctx context.Context, log *models.MedicationLog) error
	DeleteLog(ctx context.Context, id uuid.UUID) error

	// Due medications
	GetDueMedications(ctx context.Context, childID uuid.UUID, date time.Time) ([]models.MedicationDue, error)

	// Reference data
	GetMedicationReference(ctx context.Context, name string) (*models.MedicationReference, error)
	SearchMedicationReferences(ctx context.Context, query string) ([]models.MedicationReference, error)

	// Discontinuation helpers
	HasMedicationLogs(ctx context.Context, medicationID uuid.UUID) (bool, error)
	HardDeleteMedication(ctx context.Context, id uuid.UUID) error
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
	UpdateBowelLog(ctx context.Context, log *models.BowelLog) error
	DeleteBowelLog(ctx context.Context, id uuid.UUID) error

	// Speech logs
	CreateSpeechLog(ctx context.Context, log *models.SpeechLog) error
	GetSpeechLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SpeechLog, error)
	GetSpeechLogByID(ctx context.Context, id uuid.UUID) (*models.SpeechLog, error)
	UpdateSpeechLog(ctx context.Context, log *models.SpeechLog) error
	DeleteSpeechLog(ctx context.Context, id uuid.UUID) error

	// Diet logs
	CreateDietLog(ctx context.Context, log *models.DietLog) error
	GetDietLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.DietLog, error)
	GetDietLogByID(ctx context.Context, id uuid.UUID) (*models.DietLog, error)
	UpdateDietLog(ctx context.Context, log *models.DietLog) error
	DeleteDietLog(ctx context.Context, id uuid.UUID) error

	// Weight logs
	CreateWeightLog(ctx context.Context, log *models.WeightLog) error
	GetWeightLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.WeightLog, error)
	GetWeightLogByID(ctx context.Context, id uuid.UUID) (*models.WeightLog, error)
	UpdateWeightLog(ctx context.Context, log *models.WeightLog) error
	DeleteWeightLog(ctx context.Context, id uuid.UUID) error

	// Sleep logs
	CreateSleepLog(ctx context.Context, log *models.SleepLog) error
	GetSleepLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SleepLog, error)
	GetSleepLogByID(ctx context.Context, id uuid.UUID) (*models.SleepLog, error)
	UpdateSleepLog(ctx context.Context, log *models.SleepLog) error
	DeleteSleepLog(ctx context.Context, id uuid.UUID) error

	// Sensory logs
	CreateSensoryLog(ctx context.Context, log *models.SensoryLog) error
	GetSensoryLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SensoryLog, error)
	GetSensoryLogByID(ctx context.Context, id uuid.UUID) (*models.SensoryLog, error)
	UpdateSensoryLog(ctx context.Context, log *models.SensoryLog) error
	DeleteSensoryLog(ctx context.Context, id uuid.UUID) error

	// Social logs
	CreateSocialLog(ctx context.Context, log *models.SocialLog) error
	GetSocialLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SocialLog, error)
	GetSocialLogByID(ctx context.Context, id uuid.UUID) (*models.SocialLog, error)
	UpdateSocialLog(ctx context.Context, log *models.SocialLog) error
	DeleteSocialLog(ctx context.Context, id uuid.UUID) error

	// Therapy logs
	CreateTherapyLog(ctx context.Context, log *models.TherapyLog) error
	GetTherapyLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.TherapyLog, error)
	GetTherapyLogByID(ctx context.Context, id uuid.UUID) (*models.TherapyLog, error)
	UpdateTherapyLog(ctx context.Context, log *models.TherapyLog) error
	DeleteTherapyLog(ctx context.Context, id uuid.UUID) error

	// Seizure logs
	CreateSeizureLog(ctx context.Context, log *models.SeizureLog) error
	GetSeizureLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SeizureLog, error)
	GetSeizureLogByID(ctx context.Context, id uuid.UUID) (*models.SeizureLog, error)
	UpdateSeizureLog(ctx context.Context, log *models.SeizureLog) error
	DeleteSeizureLog(ctx context.Context, id uuid.UUID) error

	// Health event logs
	CreateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error
	GetHealthEventLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.HealthEventLog, error)
	GetHealthEventLogByID(ctx context.Context, id uuid.UUID) (*models.HealthEventLog, error)
	UpdateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error
	DeleteHealthEventLog(ctx context.Context, id uuid.UUID) error

	// Daily log page
	GetDailyLogs(ctx context.Context, childID uuid.UUID, date time.Time) (*models.DailyLogPage, error)
	GetLogsForDateRange(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (*models.DailyLogPage, error)

	// Date listing
	GetDatesWithLogs(ctx context.Context, childID uuid.UUID, limit int) ([]models.DateWithEntryCount, error)
}

// AlertRepository handles alert operations
type AlertRepository interface {
	Create(ctx context.Context, alert *models.Alert) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error)
	GetByChildID(ctx context.Context, childID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error)
	GetByFamilyID(ctx context.Context, familyID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error)
	Update(ctx context.Context, alert *models.Alert) error
	Acknowledge(ctx context.Context, id, userID uuid.UUID) error
	Resolve(ctx context.Context, id, userID uuid.UUID, notes string) error
	ResolveActiveByTypeAndSeverity(ctx context.Context, childID uuid.UUID, alertType string, severity models.AlertSeverity) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Feedback
	CreateFeedback(ctx context.Context, feedback *models.AlertFeedback) error
	GetFeedback(ctx context.Context, alertID uuid.UUID) ([]models.AlertFeedback, error)

	// Stats
	GetStats(ctx context.Context, childID uuid.UUID) (*models.AlertStats, error)
	GetStatsByType(ctx context.Context, childID uuid.UUID, alertType string) (*models.AlertTypeStats, error)

	// Alert intelligence
	GetByChildIDAndTypeSince(ctx context.Context, childID uuid.UUID, alertType string, since time.Time) ([]models.Alert, error)

	// Alerts page data
	GetAlertsPage(ctx context.Context, childID uuid.UUID) (*models.AlertsPage, error)

	// Access control
	UserHasAccess(ctx context.Context, alertID, userID uuid.UUID) (bool, error)
}

// InsightRepository handles insight operations (Three-Tier Learning System)
type InsightRepository interface {
	// CRUD operations
	Create(ctx context.Context, insight *models.Insight) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Insight, error)
	Update(ctx context.Context, insight *models.Insight) error
	Delete(ctx context.Context, id uuid.UUID) error

	// Query operations
	GetByChildID(ctx context.Context, childID uuid.UUID, tier *models.InsightTier, activeOnly bool) ([]models.Insight, error)
	GetByChildIDSince(ctx context.Context, childID uuid.UUID, since time.Time) ([]models.Insight, error)
	GetGlobalInsights(ctx context.Context, category string) ([]models.Insight, error)
	GetByPatternID(ctx context.Context, patternID uuid.UUID) (*models.Insight, error)
	ExistsRecentByDedupeKey(ctx context.Context, childID uuid.UUID, key string, window time.Duration) (bool, error)
	CountRecentByDedupeKeyPrefix(ctx context.Context, childID uuid.UUID, prefix string, window time.Duration) (int, error)

	// Validation
	IncrementValidation(ctx context.Context, id uuid.UUID) error
	SetClinicallyValidated(ctx context.Context, id uuid.UUID) error

	// Sources
	CreateSource(ctx context.Context, source *models.InsightSource) error
	GetSource(ctx context.Context, id uuid.UUID) (*models.InsightSource, error)
	GetSourcesByInsight(ctx context.Context, insightID uuid.UUID) ([]models.InsightSource, error)

	// Upsert for correlation engine
	Upsert(ctx context.Context, insight *models.Insight) error
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
	IncrementPatternValidation(ctx context.Context, id uuid.UUID) error

	// Clinical validations
	CreateValidation(ctx context.Context, validation *models.ClinicalValidation) error
	GetValidations(ctx context.Context, childID uuid.UUID) ([]models.ClinicalValidation, error)
	GetValidation(ctx context.Context, id uuid.UUID) (*models.ClinicalValidation, error)
	GetValidationStats(ctx context.Context, childID uuid.UUID) (*models.ValidationStats, error)

	// Insights page
	GetInsightsPage(ctx context.Context, childID uuid.UUID) (*models.InsightsPage, error)

	// Data for correlation engine
	GetCorrelationData(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (map[string][]models.DataPoint, error)
}

// ChatRepository handles chat operations
type ChatRepository interface {
	// Thread operations
	CreateThread(ctx context.Context, thread *models.ChatThread) error
	GetThread(ctx context.Context, id uuid.UUID) (*models.ChatThread, error)
	GetThreadsByFamily(ctx context.Context, familyID uuid.UUID) ([]models.ChatThread, error)
	GetThreadsByChild(ctx context.Context, childID uuid.UUID) ([]models.ChatThread, error)
	UpdateThread(ctx context.Context, thread *models.ChatThread) error
	DeleteThread(ctx context.Context, id uuid.UUID) error

	// Participant operations
	AddParticipant(ctx context.Context, threadID, userID uuid.UUID, role models.FamilyRole) error
	RemoveParticipant(ctx context.Context, threadID, userID uuid.UUID) error
	GetParticipants(ctx context.Context, threadID uuid.UUID) ([]models.ChatParticipant, error)
	IsParticipant(ctx context.Context, threadID, userID uuid.UUID) (bool, error)
	UpdateLastRead(ctx context.Context, threadID, userID uuid.UUID) error

	// Message operations
	CreateMessage(ctx context.Context, message *models.ChatMessage) error
	GetMessages(ctx context.Context, threadID uuid.UUID, limit, offset int) ([]models.ChatMessage, error)
	GetMessage(ctx context.Context, id uuid.UUID) (*models.ChatMessage, error)
	UpdateMessage(ctx context.Context, message *models.ChatMessage) error
	DeleteMessage(ctx context.Context, id uuid.UUID) error

	// Unread counts
	GetUnreadCount(ctx context.Context, threadID, userID uuid.UUID) (int, error)
	GetTotalUnreadCount(ctx context.Context, familyID, userID uuid.UUID) (int, error)
}

// CohortRepository handles cohort operations for Tier 2 insights
type CohortRepository interface {
	// Cohort definitions
	CreateCohort(ctx context.Context, cohort *models.CohortDefinition) error
	GetCohort(ctx context.Context, id uuid.UUID) (*models.CohortDefinition, error)
	GetAllCohorts(ctx context.Context) ([]models.CohortDefinition, error)
	UpdateCohort(ctx context.Context, cohort *models.CohortDefinition) error
	DeleteCohort(ctx context.Context, id uuid.UUID) error

	// Membership operations
	AddMember(ctx context.Context, cohortID uuid.UUID, childHash string, matchScore float64) error
	RemoveMember(ctx context.Context, cohortID uuid.UUID, childHash string) error
	GetMemberCount(ctx context.Context, cohortID uuid.UUID) (int, error)
	IsMember(ctx context.Context, cohortID uuid.UUID, childHash string) (bool, error)

	// Pattern operations
	CreatePattern(ctx context.Context, pattern *models.CohortPattern) error
	GetCohortPatterns(ctx context.Context, cohortID uuid.UUID) ([]models.CohortPattern, error)
	GetActivePatterns(ctx context.Context, cohortID uuid.UUID) ([]models.CohortPattern, error)
	UpdatePattern(ctx context.Context, pattern *models.CohortPattern) error
	DeletePattern(ctx context.Context, id uuid.UUID) error
}

// Repositories aggregates all repository interfaces
type Repositories struct {
	User         UserRepository
	Family       FamilyRepository
	Child        ChildRepository
	Medication   MedicationRepository
	Log          LogRepository
	Alert        AlertRepository
	Insight      InsightRepository
	Correlation  CorrelationRepository
	Cohort       CohortRepository
	Chat         ChatRepository
	Transparency *TransparencyRepository
	Admin        AdminRepository       // Admin portal (PHI-isolated)
	UserSupport  UserSupportRepository // User-facing support tickets
	Marketing    MarketingRepository   // Marketing materials center
	DevMode      DevModeRepository     // Development mode SSH control
	Billing      BillingRepository     // Family-based billing
	DeviceToken  DeviceTokenRepository // Mobile device tokens for push notifications
	Report       ReportRepository     // Reports and scheduled reports
	Search       SearchRepository     // Global search
	Roadmap      RoadmapRepository    // Product roadmap items
	TicketAttachment TicketAttachmentRepository // Per-ticket file attachments
	BetaInvitation   BetaInvitationRepository   // Marketing-managed TestFlight beta invites
	BountyAward      BountyAwardRepository      // Monthly top-5+5 bounty rewards
	Session          SessionRepository          // Persistent server-side sessions
	SessionProd      SessionRepository          // Optional cross-env (prod) sessions read pool — nil when SESSIONS_PROD_DB_DSN unset
	AccountDeletion  AccountDeletionRepository  // User-initiated account deletion (App Store Blocker 2)
	ProQA            ProQARepository            // Admin-only Pro QA workspace (shared support DB)
	Role             RoleRepository             // Custom admin roles (per-env, main DB)
}

// NewRepositories creates all repository implementations.
//
// supportDB is the connection pool for the three shared support-ticket
// tables (support_tickets, ticket_messages, ticket_attachments). Pass the
// same handle as `db` for environments that don't share tickets across
// envs; the admin/user-support/ticket-attachment repos route those three
// tables to supportDB while continuing to use db for everything else
// (users lookup for denorm, audit log, etc.).
//
// adminMirrorDB, when non-nil, enables bidirectional admin_users replication.
// The Admin repo is wrapped in a dual-writer that mirrors every admin user
// CRUD to both pools. See replicating_admin_repo.go.
func NewRepositories(db, supportDB *sql.DB, sessionsProdDB *sql.DB, adminMirrorDB *sql.DB) *Repositories {
	baseAdmin := NewAdminRepo(db, supportDB)
	var adminRepo AdminRepository = baseAdmin
	if adminMirrorDB != nil {
		adminRepo = NewReplicatingAdminRepo(baseAdmin, db, adminMirrorDB)
	}
	repos := &Repositories{
		User:         NewUserRepo(db),
		Family:       NewFamilyRepo(db),
		Child:        NewChildRepo(db),
		Medication:   NewMedicationRepo(db),
		Log:          NewLogRepo(db),
		Alert:        NewAlertRepo(db),
		Insight:      NewInsightRepo(db),
		Correlation:  NewCorrelationRepo(db),
		Cohort:       NewCohortRepo(db),
		Chat:         NewChatRepo(db),
		Transparency: NewTransparencyRepository(db),
		Admin:        adminRepo,
		UserSupport:  NewUserSupportRepo(db, supportDB),
		Marketing:    NewMarketingRepo(db),
		DevMode:      NewDevModeRepo(db),
		Billing:      NewBillingRepo(db),
		DeviceToken:  NewDeviceTokenRepo(db),
		Report:       NewReportRepo(db),
		Search:       NewSearchRepo(db),
		Roadmap:      NewRoadmapRepo(db),
		TicketAttachment: NewTicketAttachmentRepo(db, supportDB),
		BetaInvitation:   NewBetaInvitationRepo(db),
		BountyAward:      NewBountyAwardRepo(db),
		Session:          NewSessionRepo(db),
		AccountDeletion:  NewAccountDeletionRepository(db),
		ProQA:            NewProQARepo(supportDB),
		Role:             NewRoleRepo(db),
	}
	if sessionsProdDB != nil {
		repos.SessionProd = NewSessionRepo(sessionsProdDB)
	}
	return repos
}
