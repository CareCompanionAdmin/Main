package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

type LogService struct {
	logRepo repository.LogRepository
}

func NewLogService(logRepo repository.LogRepository) *LogService {
	return &LogService{
		logRepo: logRepo,
	}
}

// Behavior Logs
func (s *LogService) CreateBehaviorLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateBehaviorLogRequest) (*models.BehaviorLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.BehaviorLog{
		ChildID:             childID,
		LogDate:             logDate,
		MoodLevel:           req.MoodLevel,
		EnergyLevel:         req.EnergyLevel,
		AnxietyLevel:        req.AnxietyLevel,
		Meltdowns:           req.Meltdowns,
		StimmingEpisodes:    req.StimmingEpisodes,
		AggressionIncidents: req.AggressionIncidents,
		SelfInjuryIncidents: req.SelfInjuryIncidents,
		Triggers:            models.StringArray(req.Triggers),
		PositiveBehaviors:   models.StringArray(req.PositiveBehaviors),
		LoggedBy:            loggedBy,
	}
	log.LogTime.String = req.LogTime
	log.LogTime.Valid = req.LogTime != ""
	log.Location.String = req.Location
	log.Location.Valid = req.Location != ""
	log.LocationOther.String = req.LocationOther
	log.LocationOther.Valid = req.LocationOther != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateBehaviorLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetBehaviorLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BehaviorLog, error) {
	return s.logRepo.GetBehaviorLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetBehaviorLogByID(ctx context.Context, id uuid.UUID) (*models.BehaviorLog, error) {
	return s.logRepo.GetBehaviorLogByID(ctx, id)
}

func (s *LogService) UpdateBehaviorLog(ctx context.Context, log *models.BehaviorLog) error {
	return s.logRepo.UpdateBehaviorLog(ctx, log)
}

func (s *LogService) DeleteBehaviorLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteBehaviorLog(ctx, id)
}

// Bowel Logs
func (s *LogService) CreateBowelLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateBowelLogRequest) (*models.BowelLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.BowelLog{
		ChildID:      childID,
		LogDate:      logDate,
		BristolScale: req.BristolScale,
		HadAccident:  req.HadAccident,
		PainLevel:    req.PainLevel,
		BloodPresent: req.BloodPresent,
		LoggedBy:     loggedBy,
	}
	log.LogTime.String = req.LogTime
	log.LogTime.Valid = req.LogTime != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateBowelLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetBowelLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BowelLog, error) {
	return s.logRepo.GetBowelLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetBowelLogByID(ctx context.Context, id uuid.UUID) (*models.BowelLog, error) {
	return s.logRepo.GetBowelLogByID(ctx, id)
}

func (s *LogService) UpdateBowelLog(ctx context.Context, log *models.BowelLog) error {
	return s.logRepo.UpdateBowelLog(ctx, log)
}

func (s *LogService) DeleteBowelLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteBowelLog(ctx, id)
}

// Speech Logs
func (s *LogService) CreateSpeechLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateSpeechLogRequest) (*models.SpeechLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.SpeechLog{
		ChildID:                  childID,
		LogDate:                  logDate,
		VerbalOutputLevel:        req.VerbalOutputLevel,
		ClarityLevel:             req.ClarityLevel,
		NewWords:                 models.StringArray(req.NewWords),
		LostWords:                models.StringArray(req.LostWords),
		EcholaliaLevel:           req.EcholaliaLevel,
		CommunicationAttempts:    req.CommunicationAttempts,
		SuccessfulCommunications: req.SuccessfulCommunications,
		LoggedBy:                 loggedBy,
	}
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateSpeechLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetSpeechLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SpeechLog, error) {
	return s.logRepo.GetSpeechLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetSpeechLogByID(ctx context.Context, id uuid.UUID) (*models.SpeechLog, error) {
	return s.logRepo.GetSpeechLogByID(ctx, id)
}

func (s *LogService) UpdateSpeechLog(ctx context.Context, log *models.SpeechLog) error {
	return s.logRepo.UpdateSpeechLog(ctx, log)
}

func (s *LogService) DeleteSpeechLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteSpeechLog(ctx, id)
}

// Diet Logs
func (s *LogService) CreateDietLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateDietLogRequest) (*models.DietLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.DietLog{
		ChildID:          childID,
		LogDate:          logDate,
		FoodsEaten:       models.StringArray(req.FoodsEaten),
		FoodsRefused:     models.StringArray(req.FoodsRefused),
		WaterIntakeOz:    req.WaterIntakeOz,
		SupplementsTaken: models.StringArray(req.SupplementsTaken),
		AllergicReaction: req.AllergicReaction,
		LoggedBy:         loggedBy,
	}
	log.MealType.String = req.MealType
	log.MealType.Valid = req.MealType != ""
	log.MealTime.String = req.MealTime
	log.MealTime.Valid = req.MealTime != ""
	log.AppetiteLevel.String = req.AppetiteLevel
	log.AppetiteLevel.Valid = req.AppetiteLevel != ""
	log.NewFoodTried.String = req.NewFoodTried
	log.NewFoodTried.Valid = req.NewFoodTried != ""
	log.ReactionDetails.String = req.ReactionDetails
	log.ReactionDetails.Valid = req.ReactionDetails != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateDietLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetDietLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.DietLog, error) {
	return s.logRepo.GetDietLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetDietLogByID(ctx context.Context, id uuid.UUID) (*models.DietLog, error) {
	return s.logRepo.GetDietLogByID(ctx, id)
}

func (s *LogService) UpdateDietLog(ctx context.Context, log *models.DietLog) error {
	return s.logRepo.UpdateDietLog(ctx, log)
}

func (s *LogService) DeleteDietLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteDietLog(ctx, id)
}

// Weight Logs
func (s *LogService) CreateWeightLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateWeightLogRequest) (*models.WeightLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.WeightLog{
		ChildID:      childID,
		LogDate:      logDate,
		WeightLbs:    req.WeightLbs,
		HeightInches: req.HeightInches,
		LoggedBy:     loggedBy,
	}
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateWeightLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetWeightLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.WeightLog, error) {
	return s.logRepo.GetWeightLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetWeightLogByID(ctx context.Context, id uuid.UUID) (*models.WeightLog, error) {
	return s.logRepo.GetWeightLogByID(ctx, id)
}

func (s *LogService) UpdateWeightLog(ctx context.Context, log *models.WeightLog) error {
	return s.logRepo.UpdateWeightLog(ctx, log)
}

func (s *LogService) DeleteWeightLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteWeightLog(ctx, id)
}

// Sleep Logs
func (s *LogService) CreateSleepLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateSleepLogRequest) (*models.SleepLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.SleepLog{
		ChildID:           childID,
		LogDate:           logDate,
		TotalSleepMinutes: req.TotalSleepMinutes,
		NightWakings:      req.NightWakings,
		TookSleepAid:      req.TookSleepAid,
		Nightmares:        req.Nightmares,
		BedWetting:        req.BedWetting,
		LoggedBy:          loggedBy,
	}
	log.Bedtime.String = req.Bedtime
	log.Bedtime.Valid = req.Bedtime != ""
	log.WakeTime.String = req.WakeTime
	log.WakeTime.Valid = req.WakeTime != ""
	log.SleepQuality.String = req.SleepQuality
	log.SleepQuality.Valid = req.SleepQuality != ""
	log.SleepAidName.String = req.SleepAidName
	log.SleepAidName.Valid = req.SleepAidName != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateSleepLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetSleepLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SleepLog, error) {
	return s.logRepo.GetSleepLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetSleepLogByID(ctx context.Context, id uuid.UUID) (*models.SleepLog, error) {
	return s.logRepo.GetSleepLogByID(ctx, id)
}

func (s *LogService) UpdateSleepLog(ctx context.Context, log *models.SleepLog) error {
	return s.logRepo.UpdateSleepLog(ctx, log)
}

func (s *LogService) DeleteSleepLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteSleepLog(ctx, id)
}

// Daily Logs
func (s *LogService) GetDailyLogs(ctx context.Context, childID uuid.UUID, date time.Time) (*models.DailyLogPage, error) {
	return s.logRepo.GetDailyLogs(ctx, childID, date)
}

func (s *LogService) GetTodaysLogs(ctx context.Context, childID uuid.UUID) (*models.DailyLogPage, error) {
	return s.logRepo.GetDailyLogs(ctx, childID, time.Now())
}

// Date range helpers
func (s *LogService) GetLastWeekRange() (time.Time, time.Time) {
	now := time.Now()
	return now.AddDate(0, 0, -7), now
}

func (s *LogService) GetLastMonthRange() (time.Time, time.Time) {
	now := time.Now()
	return now.AddDate(0, -1, 0), now
}

func (s *LogService) GetThisWeekRange() (time.Time, time.Time) {
	now := time.Now()
	weekday := int(now.Weekday())
	startOfWeek := now.AddDate(0, 0, -weekday)
	return startOfWeek, now
}

// Sensory Logs
func (s *LogService) CreateSensoryLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateSensoryLogRequest) (*models.SensoryLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.SensoryLog{
		ChildID:                  childID,
		LogDate:                  logDate,
		SensorySeekingBehaviors:  models.StringArray(req.SensorySeekingBehaviors),
		SensoryAvoidingBehaviors: models.StringArray(req.SensoryAvoidingBehaviors),
		OverloadTriggers:         models.StringArray(req.OverloadTriggers),
		CalmingStrategiesUsed:    models.StringArray(req.CalmingStrategiesUsed),
		OverloadEpisodes:         req.OverloadEpisodes,
		OverallRegulation:        req.OverallRegulation,
		LoggedBy:                 loggedBy,
	}
	log.LogTime.String = req.LogTime
	log.LogTime.Valid = req.LogTime != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateSensoryLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetSensoryLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SensoryLog, error) {
	return s.logRepo.GetSensoryLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetSensoryLogByID(ctx context.Context, id uuid.UUID) (*models.SensoryLog, error) {
	return s.logRepo.GetSensoryLogByID(ctx, id)
}

func (s *LogService) UpdateSensoryLog(ctx context.Context, log *models.SensoryLog) error {
	return s.logRepo.UpdateSensoryLog(ctx, log)
}

func (s *LogService) DeleteSensoryLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteSensoryLog(ctx, id)
}

// Social Logs
func (s *LogService) CreateSocialLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateSocialLogRequest) (*models.SocialLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.SocialLog{
		ChildID:                childID,
		LogDate:                logDate,
		EyeContactLevel:        req.EyeContactLevel,
		SocialEngagementLevel:  req.SocialEngagementLevel,
		PeerInteractions:       req.PeerInteractions,
		PositiveInteractions:   req.PositiveInteractions,
		Conflicts:              req.Conflicts,
		ParallelPlayMinutes:    req.ParallelPlayMinutes,
		CooperativePlayMinutes: req.CooperativePlayMinutes,
		LoggedBy:               loggedBy,
	}
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateSocialLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetSocialLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SocialLog, error) {
	return s.logRepo.GetSocialLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetSocialLogByID(ctx context.Context, id uuid.UUID) (*models.SocialLog, error) {
	return s.logRepo.GetSocialLogByID(ctx, id)
}

func (s *LogService) UpdateSocialLog(ctx context.Context, log *models.SocialLog) error {
	return s.logRepo.UpdateSocialLog(ctx, log)
}

func (s *LogService) DeleteSocialLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteSocialLog(ctx, id)
}

// Therapy Logs
func (s *LogService) CreateTherapyLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateTherapyLogRequest) (*models.TherapyLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.TherapyLog{
		ChildID:         childID,
		LogDate:         logDate,
		DurationMinutes: req.DurationMinutes,
		GoalsWorkedOn:   models.StringArray(req.GoalsWorkedOn),
		LoggedBy:        loggedBy,
	}
	log.TherapyType.String = req.TherapyType
	log.TherapyType.Valid = req.TherapyType != ""
	log.TherapistName.String = req.TherapistName
	log.TherapistName.Valid = req.TherapistName != ""
	log.ProgressNotes.String = req.ProgressNotes
	log.ProgressNotes.Valid = req.ProgressNotes != ""
	log.HomeworkAssigned.String = req.HomeworkAssigned
	log.HomeworkAssigned.Valid = req.HomeworkAssigned != ""
	log.ParentNotes.String = req.ParentNotes
	log.ParentNotes.Valid = req.ParentNotes != ""

	if err := s.logRepo.CreateTherapyLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetTherapyLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.TherapyLog, error) {
	return s.logRepo.GetTherapyLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetTherapyLogByID(ctx context.Context, id uuid.UUID) (*models.TherapyLog, error) {
	return s.logRepo.GetTherapyLogByID(ctx, id)
}

func (s *LogService) UpdateTherapyLog(ctx context.Context, log *models.TherapyLog) error {
	return s.logRepo.UpdateTherapyLog(ctx, log)
}

func (s *LogService) DeleteTherapyLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteTherapyLog(ctx, id)
}

// Seizure Logs
func (s *LogService) CreateSeizureLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateSeizureLogRequest) (*models.SeizureLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.SeizureLog{
		ChildID:               childID,
		LogDate:               logDate,
		LogTime:               req.LogTime,
		DurationSeconds:       req.DurationSeconds,
		Triggers:              models.StringArray(req.Triggers),
		WarningSigns:          models.StringArray(req.WarningSigns),
		PostIctalSymptoms:     models.StringArray(req.PostIctalSymptoms),
		RescueMedicationGiven: req.RescueMedicationGiven,
		Called911:             req.Called911,
		LoggedBy:              loggedBy,
	}
	log.SeizureType.String = req.SeizureType
	log.SeizureType.Valid = req.SeizureType != ""
	log.RescueMedicationName.String = req.RescueMedicationName
	log.RescueMedicationName.Valid = req.RescueMedicationName != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if err := s.logRepo.CreateSeizureLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetSeizureLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SeizureLog, error) {
	return s.logRepo.GetSeizureLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetSeizureLogByID(ctx context.Context, id uuid.UUID) (*models.SeizureLog, error) {
	return s.logRepo.GetSeizureLogByID(ctx, id)
}

func (s *LogService) UpdateSeizureLog(ctx context.Context, log *models.SeizureLog) error {
	return s.logRepo.UpdateSeizureLog(ctx, log)
}

func (s *LogService) DeleteSeizureLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteSeizureLog(ctx, id)
}

// Health Event Logs
func (s *LogService) CreateHealthEventLog(ctx context.Context, childID, loggedBy uuid.UUID, req *models.CreateHealthEventLogRequest) (*models.HealthEventLog, error) {
	logDate := req.LogDate
	if logDate.IsZero() {
		logDate = time.Now()
	}
	log := &models.HealthEventLog{
		ChildID:      childID,
		LogDate:      logDate,
		Symptoms:     models.StringArray(req.Symptoms),
		TemperatureF: req.TemperatureF,
		LoggedBy:     loggedBy,
	}
	log.EventType.String = req.EventType
	log.EventType.Valid = req.EventType != ""
	log.Description.String = req.Description
	log.Description.Valid = req.Description != ""
	log.ProviderName.String = req.ProviderName
	log.ProviderName.Valid = req.ProviderName != ""
	log.Diagnosis.String = req.Diagnosis
	log.Diagnosis.Valid = req.Diagnosis != ""
	log.Treatment.String = req.Treatment
	log.Treatment.Valid = req.Treatment != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""
	if req.FollowUpDate != nil {
		log.FollowUpDate.Time = *req.FollowUpDate
		log.FollowUpDate.Valid = true
	}

	if err := s.logRepo.CreateHealthEventLog(ctx, log); err != nil {
		return nil, err
	}
	return log, nil
}

func (s *LogService) GetHealthEventLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.HealthEventLog, error) {
	return s.logRepo.GetHealthEventLogs(ctx, childID, startDate, endDate)
}

func (s *LogService) GetHealthEventLogByID(ctx context.Context, id uuid.UUID) (*models.HealthEventLog, error) {
	return s.logRepo.GetHealthEventLogByID(ctx, id)
}

func (s *LogService) UpdateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error {
	return s.logRepo.UpdateHealthEventLog(ctx, log)
}

func (s *LogService) DeleteHealthEventLog(ctx context.Context, id uuid.UUID) error {
	return s.logRepo.DeleteHealthEventLog(ctx, id)
}

// GetDatesWithLogs returns dates that have log entries for a child
func (s *LogService) GetDatesWithLogs(ctx context.Context, childID uuid.UUID, limit int) ([]models.DateWithEntryCount, error) {
	return s.logRepo.GetDatesWithLogs(ctx, childID, limit)
}
