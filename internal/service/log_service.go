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
