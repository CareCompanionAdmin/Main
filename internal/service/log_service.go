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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
	if logDate.IsZero() {
		logDate = time.Now()
	}
	// Handle overall_regulation - treat 0 or out-of-range as NULL (check constraint requires 1-5)
	var overallRegulation *int
	if req.OverallRegulation != nil && *req.OverallRegulation >= 1 && *req.OverallRegulation <= 5 {
		overallRegulation = req.OverallRegulation
	}
	log := &models.SensoryLog{
		ChildID:                  childID,
		LogDate:                  logDate,
		SensorySeekingBehaviors:  models.StringArray(req.SensorySeekingBehaviors),
		SensoryAvoidingBehaviors: models.StringArray(req.SensoryAvoidingBehaviors),
		OverloadTriggers:         models.StringArray(req.OverloadTriggers),
		CalmingStrategiesUsed:    models.StringArray(req.CalmingStrategiesUsed),
		OverloadEpisodes:         req.OverloadEpisodes,
		OverallRegulation:        overallRegulation,
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
	logDate := req.LogDate.Time
	if logDate.IsZero() {
		logDate = time.Now()
	}
	// Handle level fields - treat 0 or out-of-range as NULL (check constraints require 1-5)
	var eyeContactLevel *int
	if req.EyeContactLevel != nil && *req.EyeContactLevel >= 1 && *req.EyeContactLevel <= 5 {
		eyeContactLevel = req.EyeContactLevel
	}
	var socialEngagementLevel *int
	if req.SocialEngagementLevel != nil && *req.SocialEngagementLevel >= 1 && *req.SocialEngagementLevel <= 5 {
		socialEngagementLevel = req.SocialEngagementLevel
	}
	log := &models.SocialLog{
		ChildID:                childID,
		LogDate:                logDate,
		EyeContactLevel:        eyeContactLevel,
		SocialEngagementLevel:  socialEngagementLevel,
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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
	logDate := req.LogDate.Time
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

// DaySummaryData holds the score and details for a day
type DaySummaryData struct {
	Score   float64
	HasData bool
	Details string
}

// GetQuickSummary returns summary data for a category over a date range
func (s *LogService) GetQuickSummary(ctx context.Context, childID uuid.UUID, category string, startDate, endDate time.Time) (map[string]DaySummaryData, error) {
	result := make(map[string]DaySummaryData)

	switch category {
	case "behavior":
		logs, err := s.logRepo.GetBehaviorLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		// Group by date and calculate scores
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			// Score calculation: mood (positive), energy (positive), anxiety (negative), meltdowns (negative), etc.
			// Start with 50 (neutral) and adjust
			score := 50.0
			count := 0

			if log.MoodLevel != nil && *log.MoodLevel > 0 {
				score += float64(*log.MoodLevel-5) * 5 // -25 to +25
				count++
			}
			if log.EnergyLevel != nil && *log.EnergyLevel > 0 {
				score += float64(*log.EnergyLevel-5) * 3 // -15 to +15
				count++
			}
			if log.AnxietyLevel != nil && *log.AnxietyLevel > 0 {
				score -= float64(*log.AnxietyLevel-5) * 3 // anxiety is negative
				count++
			}
			score -= float64(log.Meltdowns) * 10
			score -= float64(log.AggressionIncidents) * 15
			score -= float64(log.SelfInjuryIncidents) * 20

			if count > 0 || log.Meltdowns > 0 || log.AggressionIncidents > 0 || log.SelfInjuryIncidents > 0 {
				// Clamp between 0 and 100
				if score < 0 {
					score = 0
				} else if score > 100 {
					score = 100
				}
				// Average with existing if there are multiple logs per day
				if existing.Score > 0 {
					existing.Score = (existing.Score + score) / 2
				} else {
					existing.Score = score
				}
			}

			result[dateKey] = existing
		}

	case "sleep":
		logs, err := s.logRepo.GetSleepLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 50.0
			// Quality-based scoring
			if log.SleepQuality.Valid {
				switch log.SleepQuality.String {
				case "excellent":
					score = 100
				case "good":
					score = 75
				case "fair":
					score = 50
				case "poor":
					score = 25
				case "very_poor":
					score = 0
				}
			}
			// Adjust for night wakings
			score -= float64(log.NightWakings) * 10
			// Adjust for nightmares/bed wetting
			if log.Nightmares {
				score -= 15
			}
			if log.BedWetting {
				score -= 10
			}

			if score < 0 {
				score = 0
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "diet":
		logs, err := s.logRepo.GetDietLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 50.0
			if log.AppetiteLevel.Valid {
				switch log.AppetiteLevel.String {
				case "excellent":
					score = 100
				case "good":
					score = 80
				case "normal":
					score = 60
				case "poor":
					score = 30
				case "none":
					score = 10
				}
			}
			if log.AllergicReaction {
				score -= 30
			}
			if len(log.FoodsRefused) > 3 {
				score -= 10
			}

			if score < 0 {
				score = 0
			}
			if existing.Score > 0 {
				existing.Score = (existing.Score + score) / 2
			} else {
				existing.Score = score
			}
			result[dateKey] = existing
		}

	case "bowel":
		logs, err := s.logRepo.GetBowelLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 70.0 // Having a bowel movement is generally positive
			// Bristol scale: 3-4 is ideal
			if log.BristolScale != nil {
				bs := *log.BristolScale
				if bs >= 3 && bs <= 4 {
					score = 100
				} else if bs == 2 || bs == 5 {
					score = 70
				} else {
					score = 40
				}
			}
			if log.HadAccident {
				score -= 20
			}
			if log.BloodPresent {
				score -= 40
			}
			if log.PainLevel != nil && *log.PainLevel > 0 {
				score -= float64(*log.PainLevel) * 5
			}

			if score < 0 {
				score = 0
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "speech":
		logs, err := s.logRepo.GetSpeechLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 50.0
			if log.VerbalOutputLevel != nil {
				score = float64(*log.VerbalOutputLevel) * 10
			}
			// Bonus for new words, penalty for lost words
			if len(log.NewWords) > 0 {
				score += float64(len(log.NewWords)) * 10
			}
			if len(log.LostWords) > 0 {
				score -= float64(len(log.LostWords)) * 15
			}

			if score < 0 {
				score = 0
			} else if score > 100 {
				score = 100
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "sensory":
		logs, err := s.logRepo.GetSensoryLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 50.0
			if log.OverallRegulation != nil {
				score = float64(*log.OverallRegulation) * 10
			}
			// More triggers = worse day
			if len(log.OverloadTriggers) > 0 {
				score -= float64(len(log.OverloadTriggers)) * 5
			}

			if score < 0 {
				score = 0
			} else if score > 100 {
				score = 100
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "social":
		logs, err := s.logRepo.GetSocialLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			score := 50.0
			if log.SocialEngagementLevel != nil {
				score = float64(*log.SocialEngagementLevel) * 10
			}
			// Positive interactions vs conflicts
			score += float64(log.PositiveInteractions) * 5
			score -= float64(log.Conflicts) * 10

			if score < 0 {
				score = 0
			} else if score > 100 {
				score = 100
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "therapy":
		logs, err := s.logRepo.GetTherapyLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			// Attending therapy is positive, score based on duration and having goals
			score := 70.0
			if log.DurationMinutes != nil && *log.DurationMinutes > 0 {
				// Longer sessions = more engagement
				score = 60 + float64(*log.DurationMinutes)/60*20 // 60-80 for 0-60 min
			}
			// Having progress notes is positive
			if log.ProgressNotes.Valid && log.ProgressNotes.String != "" {
				score += 10
			}

			if score > 100 {
				score = 100
			}
			existing.Score = score
			result[dateKey] = existing
		}

	case "seizure":
		logs, err := s.logRepo.GetSeizureLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			// Having seizures is concerning, score inversely
			score := 100.0
			if log.DurationSeconds != nil {
				score -= float64(*log.DurationSeconds) / 60 * 20 // Lose 20 points per minute
			}
			// Rescue medication indicates more serious seizure
			if log.RescueMedicationGiven {
				score -= 30
			}
			// Called 911 indicates severe seizure
			if log.Called911 {
				score -= 40
			}

			if score < 0 {
				score = 0
			}
			// Multiple seizures per day lower the score
			if existing.Score > 0 {
				existing.Score = (existing.Score + score) / 2
			} else {
				existing.Score = score
			}
			result[dateKey] = existing
		}

	case "health_event":
		logs, err := s.logRepo.GetHealthEventLogs(ctx, childID, startDate, endDate)
		if err != nil {
			return nil, err
		}
		for _, log := range logs {
			dateKey := log.LogDate.Format("2006-01-02")
			existing := result[dateKey]
			existing.HasData = true

			// Health events are generally concerning
			score := 50.0
			// High fever indicates more serious event
			if log.TemperatureF != nil && *log.TemperatureF >= 101.0 {
				score -= 20
			}
			// Certain event types are more concerning
			if log.EventType.Valid {
				switch log.EventType.String {
				case "injury", "emergency":
					score -= 20
				case "illness", "infection":
					score -= 10
				case "checkup", "vaccination":
					score = 70 // Routine care is positive
				}
			}

			existing.Score = score
			result[dateKey] = existing
		}

	default:
		// For unknown categories, return empty data
	}

	return result, nil
}
