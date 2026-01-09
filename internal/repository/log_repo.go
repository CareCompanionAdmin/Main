package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type logRepo struct {
	db *sql.DB
}

func NewLogRepo(db *sql.DB) LogRepository {
	return &logRepo{db: db}
}

// Behavior Logs
func (r *logRepo) CreateBehaviorLog(ctx context.Context, log *models.BehaviorLog) error {
	query := `
		INSERT INTO behavior_logs (id, child_id, log_date, log_time, time_scope, mood_level, energy_level, anxiety_level, interpersonal_behavior, meltdowns, stimming_episodes, stimming_level, aggression_incidents, self_injury_incidents, location, location_other, triggers, positive_behaviors, notes, logged_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()
	log.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.LogTime, log.TimeScope,
		log.MoodLevel, log.EnergyLevel, log.AnxietyLevel, log.InterpersonalBehavior,
		log.Meltdowns, log.StimmingEpisodes, log.StimmingLevel,
		log.AggressionIncidents, log.SelfInjuryIncidents,
		log.Location, log.LocationOther,
		log.Triggers, log.PositiveBehaviors, log.Notes, log.LoggedBy,
		log.CreatedAt, log.UpdatedAt,
	)
	return err
}

func (r *logRepo) GetBehaviorLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BehaviorLog, error) {
	// Format dates as strings to avoid timezone conversion issues
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, log_time, time_scope, mood_level, energy_level, anxiety_level, interpersonal_behavior, meltdowns, stimming_episodes, stimming_level, aggression_incidents, self_injury_incidents, location, location_other, triggers, positive_behaviors, notes, logged_by, created_at, updated_at
		FROM behavior_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.BehaviorLog
	for rows.Next() {
		var log models.BehaviorLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.LogTime, &log.TimeScope,
			&log.MoodLevel, &log.EnergyLevel, &log.AnxietyLevel, &log.InterpersonalBehavior,
			&log.Meltdowns, &log.StimmingEpisodes, &log.StimmingLevel,
			&log.AggressionIncidents, &log.SelfInjuryIncidents,
			&log.Location, &log.LocationOther,
			&log.Triggers, &log.PositiveBehaviors, &log.Notes, &log.LoggedBy,
			&log.CreatedAt, &log.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetBehaviorLogByID(ctx context.Context, id uuid.UUID) (*models.BehaviorLog, error) {
	query := `
		SELECT id, child_id, log_date, log_time, time_scope, mood_level, energy_level, anxiety_level, interpersonal_behavior, meltdowns, stimming_episodes, stimming_level, aggression_incidents, self_injury_incidents, location, location_other, triggers, positive_behaviors, notes, logged_by, created_at, updated_at
		FROM behavior_logs
		WHERE id = $1
	`
	log := &models.BehaviorLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.LogTime, &log.TimeScope,
		&log.MoodLevel, &log.EnergyLevel, &log.AnxietyLevel, &log.InterpersonalBehavior,
		&log.Meltdowns, &log.StimmingEpisodes, &log.StimmingLevel,
		&log.AggressionIncidents, &log.SelfInjuryIncidents,
		&log.Location, &log.LocationOther,
		&log.Triggers, &log.PositiveBehaviors, &log.Notes, &log.LoggedBy,
		&log.CreatedAt, &log.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateBehaviorLog(ctx context.Context, log *models.BehaviorLog) error {
	query := `
		UPDATE behavior_logs
		SET log_time = $2, time_scope = $3, mood_level = $4, energy_level = $5, anxiety_level = $6, interpersonal_behavior = $7, meltdowns = $8, stimming_episodes = $9, stimming_level = $10, aggression_incidents = $11, self_injury_incidents = $12, triggers = $13, positive_behaviors = $14, notes = $15, updated_at = $16
		WHERE id = $1
	`
	log.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.LogTime, log.TimeScope,
		log.MoodLevel, log.EnergyLevel, log.AnxietyLevel, log.InterpersonalBehavior,
		log.Meltdowns, log.StimmingEpisodes, log.StimmingLevel,
		log.AggressionIncidents, log.SelfInjuryIncidents,
		log.Triggers, log.PositiveBehaviors, log.Notes, log.UpdatedAt,
	)
	return err
}

func (r *logRepo) DeleteBehaviorLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM behavior_logs WHERE id = $1`, id)
	return err
}

// Bowel Logs
func (r *logRepo) CreateBowelLog(ctx context.Context, log *models.BowelLog) error {
	query := `
		INSERT INTO bowel_logs (id, child_id, log_date, log_time, bristol_scale, had_accident, pain_level, blood_present, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.LogTime,
		log.BristolScale, log.HadAccident, log.PainLevel, log.BloodPresent,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetBowelLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.BowelLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, log_time, bristol_scale, had_accident, pain_level, blood_present, notes, logged_by, created_at
		FROM bowel_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.BowelLog
	for rows.Next() {
		var log models.BowelLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
			&log.BristolScale, &log.HadAccident, &log.PainLevel, &log.BloodPresent,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetBowelLogByID(ctx context.Context, id uuid.UUID) (*models.BowelLog, error) {
	query := `
		SELECT id, child_id, log_date, log_time, bristol_scale, had_accident, pain_level, blood_present, notes, logged_by, created_at
		FROM bowel_logs
		WHERE id = $1
	`
	log := &models.BowelLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
		&log.BristolScale, &log.HadAccident, &log.PainLevel, &log.BloodPresent,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateBowelLog(ctx context.Context, log *models.BowelLog) error {
	query := `
		UPDATE bowel_logs
		SET log_time = $2, bristol_scale = $3, had_accident = $4, pain_level = $5, blood_present = $6, notes = $7
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.LogTime, log.BristolScale, log.HadAccident, log.PainLevel, log.BloodPresent, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteBowelLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bowel_logs WHERE id = $1`, id)
	return err
}

// Speech Logs
func (r *logRepo) CreateSpeechLog(ctx context.Context, log *models.SpeechLog) error {
	query := `
		INSERT INTO speech_logs (id, child_id, log_date, verbal_output_level, clarity_level, new_words, lost_words, echolalia_level, communication_attempts, successful_communications, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate,
		log.VerbalOutputLevel, log.ClarityLevel, log.NewWords, log.LostWords,
		log.EcholaliaLevel, log.CommunicationAttempts, log.SuccessfulCommunications,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetSpeechLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SpeechLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, verbal_output_level, clarity_level, new_words, lost_words, echolalia_level, communication_attempts, successful_communications, notes, logged_by, created_at
		FROM speech_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SpeechLog
	for rows.Next() {
		var log models.SpeechLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate,
			&log.VerbalOutputLevel, &log.ClarityLevel, &log.NewWords, &log.LostWords,
			&log.EcholaliaLevel, &log.CommunicationAttempts, &log.SuccessfulCommunications,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetSpeechLogByID(ctx context.Context, id uuid.UUID) (*models.SpeechLog, error) {
	query := `
		SELECT id, child_id, log_date, verbal_output_level, clarity_level, new_words, lost_words, echolalia_level, communication_attempts, successful_communications, notes, logged_by, created_at
		FROM speech_logs
		WHERE id = $1
	`
	log := &models.SpeechLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate,
		&log.VerbalOutputLevel, &log.ClarityLevel, &log.NewWords, &log.LostWords,
		&log.EcholaliaLevel, &log.CommunicationAttempts, &log.SuccessfulCommunications,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateSpeechLog(ctx context.Context, log *models.SpeechLog) error {
	query := `
		UPDATE speech_logs
		SET verbal_output_level = $2, clarity_level = $3, new_words = $4, lost_words = $5, echolalia_level = $6, communication_attempts = $7, successful_communications = $8, notes = $9
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.VerbalOutputLevel, log.ClarityLevel, log.NewWords, log.LostWords,
		log.EcholaliaLevel, log.CommunicationAttempts, log.SuccessfulCommunications, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteSpeechLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM speech_logs WHERE id = $1`, id)
	return err
}

// Diet Logs
func (r *logRepo) CreateDietLog(ctx context.Context, log *models.DietLog) error {
	query := `
		INSERT INTO diet_logs (id, child_id, log_date, meal_type, meal_time, foods_eaten, foods_refused, appetite_level, water_intake_oz, supplements_taken, new_food_tried, new_food_acceptance, allergic_reaction, reaction_details, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.MealType, log.MealTime,
		log.FoodsEaten, log.FoodsRefused, log.AppetiteLevel, log.WaterIntakeOz,
		log.SupplementsTaken, log.NewFoodTried, log.NewFoodAcceptance, log.AllergicReaction, log.ReactionDetails,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetDietLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.DietLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, meal_type, meal_time, foods_eaten, foods_refused, appetite_level, water_intake_oz, supplements_taken, new_food_tried, new_food_acceptance, allergic_reaction, reaction_details, notes, logged_by, created_at
		FROM diet_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.DietLog
	for rows.Next() {
		var log models.DietLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.MealType, &log.MealTime,
			&log.FoodsEaten, &log.FoodsRefused, &log.AppetiteLevel, &log.WaterIntakeOz,
			&log.SupplementsTaken, &log.NewFoodTried, &log.NewFoodAcceptance, &log.AllergicReaction, &log.ReactionDetails,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetDietLogByID(ctx context.Context, id uuid.UUID) (*models.DietLog, error) {
	query := `
		SELECT id, child_id, log_date, meal_type, meal_time, foods_eaten, foods_refused, appetite_level, water_intake_oz, supplements_taken, new_food_tried, allergic_reaction, reaction_details, notes, logged_by, created_at
		FROM diet_logs
		WHERE id = $1
	`
	log := &models.DietLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.MealType, &log.MealTime,
		&log.FoodsEaten, &log.FoodsRefused, &log.AppetiteLevel, &log.WaterIntakeOz,
		&log.SupplementsTaken, &log.NewFoodTried, &log.AllergicReaction, &log.ReactionDetails,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateDietLog(ctx context.Context, log *models.DietLog) error {
	query := `
		UPDATE diet_logs
		SET meal_type = $2, meal_time = $3, foods_eaten = $4, foods_refused = $5, appetite_level = $6, water_intake_oz = $7, supplements_taken = $8, new_food_tried = $9, new_food_acceptance = $10, allergic_reaction = $11, reaction_details = $12, notes = $13
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.MealType, log.MealTime, log.FoodsEaten, log.FoodsRefused, log.AppetiteLevel,
		log.WaterIntakeOz, log.SupplementsTaken, log.NewFoodTried, log.NewFoodAcceptance,
		log.AllergicReaction, log.ReactionDetails, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteDietLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM diet_logs WHERE id = $1`, id)
	return err
}

// Weight Logs
func (r *logRepo) CreateWeightLog(ctx context.Context, log *models.WeightLog) error {
	query := `
		INSERT INTO weight_logs (id, child_id, log_date, weight_lbs, height_inches, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate,
		log.WeightLbs, log.HeightInches, log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetWeightLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.WeightLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, weight_lbs, height_inches, notes, logged_by, created_at
		FROM weight_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.WeightLog
	for rows.Next() {
		var log models.WeightLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate,
			&log.WeightLbs, &log.HeightInches, &log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetWeightLogByID(ctx context.Context, id uuid.UUID) (*models.WeightLog, error) {
	query := `
		SELECT id, child_id, log_date, weight_lbs, height_inches, notes, logged_by, created_at
		FROM weight_logs
		WHERE id = $1
	`
	log := &models.WeightLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate,
		&log.WeightLbs, &log.HeightInches, &log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateWeightLog(ctx context.Context, log *models.WeightLog) error {
	query := `
		UPDATE weight_logs
		SET weight_lbs = $2, height_inches = $3, notes = $4
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, log.ID, log.WeightLbs, log.HeightInches, log.Notes)
	return err
}

func (r *logRepo) DeleteWeightLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM weight_logs WHERE id = $1`, id)
	return err
}

// Sleep Logs
func (r *logRepo) CreateSleepLog(ctx context.Context, log *models.SleepLog) error {
	query := `
		INSERT INTO sleep_logs (id, child_id, log_date, bedtime, wake_time, total_sleep_minutes, night_wakings, sleep_quality, took_sleep_aid, sleep_aid_name, nightmares, bed_wetting, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.Bedtime, log.WakeTime,
		log.TotalSleepMinutes, log.NightWakings, log.SleepQuality,
		log.TookSleepAid, log.SleepAidName, log.Nightmares, log.BedWetting,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetSleepLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SleepLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, bedtime, wake_time, total_sleep_minutes, night_wakings, sleep_quality, took_sleep_aid, sleep_aid_name, nightmares, bed_wetting, notes, logged_by, created_at
		FROM sleep_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SleepLog
	for rows.Next() {
		var log models.SleepLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.Bedtime, &log.WakeTime,
			&log.TotalSleepMinutes, &log.NightWakings, &log.SleepQuality,
			&log.TookSleepAid, &log.SleepAidName, &log.Nightmares, &log.BedWetting,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetSleepLogByID(ctx context.Context, id uuid.UUID) (*models.SleepLog, error) {
	query := `
		SELECT id, child_id, log_date, bedtime, wake_time, total_sleep_minutes, night_wakings, sleep_quality, took_sleep_aid, sleep_aid_name, nightmares, bed_wetting, notes, logged_by, created_at
		FROM sleep_logs
		WHERE id = $1
	`
	log := &models.SleepLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.Bedtime, &log.WakeTime,
		&log.TotalSleepMinutes, &log.NightWakings, &log.SleepQuality,
		&log.TookSleepAid, &log.SleepAidName, &log.Nightmares, &log.BedWetting,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateSleepLog(ctx context.Context, log *models.SleepLog) error {
	query := `
		UPDATE sleep_logs
		SET bedtime = $2, wake_time = $3, total_sleep_minutes = $4, night_wakings = $5, sleep_quality = $6, took_sleep_aid = $7, sleep_aid_name = $8, nightmares = $9, bed_wetting = $10, notes = $11
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.Bedtime, log.WakeTime, log.TotalSleepMinutes, log.NightWakings,
		log.SleepQuality, log.TookSleepAid, log.SleepAidName, log.Nightmares, log.BedWetting, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteSleepLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sleep_logs WHERE id = $1`, id)
	return err
}

// Sensory Logs
func (r *logRepo) CreateSensoryLog(ctx context.Context, log *models.SensoryLog) error {
	query := `
		INSERT INTO sensory_logs (id, child_id, log_date, log_time, sensory_seeking_behaviors, sensory_avoiding_behaviors, overload_triggers, calming_strategies_used, overload_episodes, overall_regulation, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.LogTime,
		log.SensorySeekingBehaviors, log.SensoryAvoidingBehaviors,
		log.OverloadTriggers, log.CalmingStrategiesUsed,
		log.OverloadEpisodes, log.OverallRegulation,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetSensoryLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SensoryLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, log_time, sensory_seeking_behaviors, sensory_avoiding_behaviors, overload_triggers, calming_strategies_used, overload_episodes, overall_regulation, notes, logged_by, created_at
		FROM sensory_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SensoryLog
	for rows.Next() {
		var log models.SensoryLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
			&log.SensorySeekingBehaviors, &log.SensoryAvoidingBehaviors,
			&log.OverloadTriggers, &log.CalmingStrategiesUsed,
			&log.OverloadEpisodes, &log.OverallRegulation,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetSensoryLogByID(ctx context.Context, id uuid.UUID) (*models.SensoryLog, error) {
	query := `
		SELECT id, child_id, log_date, log_time, sensory_seeking_behaviors, sensory_avoiding_behaviors, overload_triggers, calming_strategies_used, overload_episodes, overall_regulation, notes, logged_by, created_at
		FROM sensory_logs
		WHERE id = $1
	`
	log := &models.SensoryLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
		&log.SensorySeekingBehaviors, &log.SensoryAvoidingBehaviors,
		&log.OverloadTriggers, &log.CalmingStrategiesUsed,
		&log.OverloadEpisodes, &log.OverallRegulation,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateSensoryLog(ctx context.Context, log *models.SensoryLog) error {
	query := `
		UPDATE sensory_logs
		SET log_time = $2, sensory_seeking_behaviors = $3, sensory_avoiding_behaviors = $4, overload_triggers = $5, calming_strategies_used = $6, overload_episodes = $7, overall_regulation = $8, notes = $9
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.LogTime, log.SensorySeekingBehaviors, log.SensoryAvoidingBehaviors,
		log.OverloadTriggers, log.CalmingStrategiesUsed, log.OverloadEpisodes, log.OverallRegulation, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteSensoryLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sensory_logs WHERE id = $1`, id)
	return err
}

// Social Logs
func (r *logRepo) CreateSocialLog(ctx context.Context, log *models.SocialLog) error {
	query := `
		INSERT INTO social_logs (id, child_id, log_date, eye_contact_level, social_engagement_level, peer_interactions, positive_interactions, conflicts, parallel_play_minutes, cooperative_play_minutes, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate,
		log.EyeContactLevel, log.SocialEngagementLevel,
		log.PeerInteractions, log.PositiveInteractions, log.Conflicts,
		log.ParallelPlayMinutes, log.CooperativePlayMinutes,
		log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetSocialLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SocialLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, eye_contact_level, social_engagement_level, peer_interactions, positive_interactions, conflicts, parallel_play_minutes, cooperative_play_minutes, notes, logged_by, created_at
		FROM social_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SocialLog
	for rows.Next() {
		var log models.SocialLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate,
			&log.EyeContactLevel, &log.SocialEngagementLevel,
			&log.PeerInteractions, &log.PositiveInteractions, &log.Conflicts,
			&log.ParallelPlayMinutes, &log.CooperativePlayMinutes,
			&log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetSocialLogByID(ctx context.Context, id uuid.UUID) (*models.SocialLog, error) {
	query := `
		SELECT id, child_id, log_date, eye_contact_level, social_engagement_level, peer_interactions, positive_interactions, conflicts, parallel_play_minutes, cooperative_play_minutes, notes, logged_by, created_at
		FROM social_logs
		WHERE id = $1
	`
	log := &models.SocialLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate,
		&log.EyeContactLevel, &log.SocialEngagementLevel,
		&log.PeerInteractions, &log.PositiveInteractions, &log.Conflicts,
		&log.ParallelPlayMinutes, &log.CooperativePlayMinutes,
		&log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateSocialLog(ctx context.Context, log *models.SocialLog) error {
	query := `
		UPDATE social_logs
		SET eye_contact_level = $2, social_engagement_level = $3, peer_interactions = $4, positive_interactions = $5, conflicts = $6, parallel_play_minutes = $7, cooperative_play_minutes = $8, notes = $9
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.EyeContactLevel, log.SocialEngagementLevel, log.PeerInteractions,
		log.PositiveInteractions, log.Conflicts, log.ParallelPlayMinutes, log.CooperativePlayMinutes, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteSocialLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM social_logs WHERE id = $1`, id)
	return err
}

// Therapy Logs
func (r *logRepo) CreateTherapyLog(ctx context.Context, log *models.TherapyLog) error {
	query := `
		INSERT INTO therapy_logs (id, child_id, log_date, therapy_type, therapist_name, duration_minutes, goals_worked_on, progress_notes, homework_assigned, parent_notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate,
		log.TherapyType, log.TherapistName, log.DurationMinutes,
		log.GoalsWorkedOn, log.ProgressNotes, log.HomeworkAssigned,
		log.ParentNotes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetTherapyLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.TherapyLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, therapy_type, therapist_name, duration_minutes, goals_worked_on, progress_notes, homework_assigned, parent_notes, logged_by, created_at
		FROM therapy_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.TherapyLog
	for rows.Next() {
		var log models.TherapyLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate,
			&log.TherapyType, &log.TherapistName, &log.DurationMinutes,
			&log.GoalsWorkedOn, &log.ProgressNotes, &log.HomeworkAssigned,
			&log.ParentNotes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetTherapyLogByID(ctx context.Context, id uuid.UUID) (*models.TherapyLog, error) {
	query := `
		SELECT id, child_id, log_date, therapy_type, therapist_name, duration_minutes, goals_worked_on, progress_notes, homework_assigned, parent_notes, logged_by, created_at
		FROM therapy_logs
		WHERE id = $1
	`
	log := &models.TherapyLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate,
		&log.TherapyType, &log.TherapistName, &log.DurationMinutes,
		&log.GoalsWorkedOn, &log.ProgressNotes, &log.HomeworkAssigned,
		&log.ParentNotes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateTherapyLog(ctx context.Context, log *models.TherapyLog) error {
	query := `
		UPDATE therapy_logs
		SET therapy_type = $2, therapist_name = $3, duration_minutes = $4, goals_worked_on = $5, progress_notes = $6, homework_assigned = $7, parent_notes = $8
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.TherapyType, log.TherapistName, log.DurationMinutes,
		log.GoalsWorkedOn, log.ProgressNotes, log.HomeworkAssigned, log.ParentNotes,
	)
	return err
}

func (r *logRepo) DeleteTherapyLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM therapy_logs WHERE id = $1`, id)
	return err
}

// Seizure Logs
func (r *logRepo) CreateSeizureLog(ctx context.Context, log *models.SeizureLog) error {
	query := `
		INSERT INTO seizure_logs (id, child_id, log_date, log_time, seizure_type, duration_seconds, triggers, warning_signs, post_ictal_symptoms, rescue_medication_given, rescue_medication_name, called_911, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.LogTime,
		log.SeizureType, log.DurationSeconds, log.Triggers, log.WarningSigns,
		log.PostIctalSymptoms, log.RescueMedicationGiven, log.RescueMedicationName,
		log.Called911, log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetSeizureLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.SeizureLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, log_time, seizure_type, duration_seconds, triggers, warning_signs, post_ictal_symptoms, rescue_medication_given, rescue_medication_name, called_911, notes, logged_by, created_at
		FROM seizure_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.SeizureLog
	for rows.Next() {
		var log models.SeizureLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
			&log.SeizureType, &log.DurationSeconds, &log.Triggers, &log.WarningSigns,
			&log.PostIctalSymptoms, &log.RescueMedicationGiven, &log.RescueMedicationName,
			&log.Called911, &log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetSeizureLogByID(ctx context.Context, id uuid.UUID) (*models.SeizureLog, error) {
	query := `
		SELECT id, child_id, log_date, log_time, seizure_type, duration_seconds, triggers, warning_signs, post_ictal_symptoms, rescue_medication_given, rescue_medication_name, called_911, notes, logged_by, created_at
		FROM seizure_logs
		WHERE id = $1
	`
	log := &models.SeizureLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.LogTime,
		&log.SeizureType, &log.DurationSeconds, &log.Triggers, &log.WarningSigns,
		&log.PostIctalSymptoms, &log.RescueMedicationGiven, &log.RescueMedicationName,
		&log.Called911, &log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateSeizureLog(ctx context.Context, log *models.SeizureLog) error {
	query := `
		UPDATE seizure_logs
		SET log_time = $2, seizure_type = $3, duration_seconds = $4, triggers = $5, warning_signs = $6, post_ictal_symptoms = $7, rescue_medication_given = $8, rescue_medication_name = $9, called_911 = $10, notes = $11
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.LogTime, log.SeizureType, log.DurationSeconds, log.Triggers,
		log.WarningSigns, log.PostIctalSymptoms, log.RescueMedicationGiven, log.RescueMedicationName,
		log.Called911, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteSeizureLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM seizure_logs WHERE id = $1`, id)
	return err
}

// Health Event Logs
func (r *logRepo) CreateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error {
	query := `
		INSERT INTO health_event_logs (id, child_id, log_date, event_type, description, symptoms, temperature_f, provider_name, diagnosis, treatment, follow_up_date, notes, logged_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.ChildID, log.LogDate, log.EventType, log.Description,
		log.Symptoms, log.TemperatureF, log.ProviderName, log.Diagnosis,
		log.Treatment, log.FollowUpDate, log.Notes, log.LoggedBy, log.CreatedAt,
	)
	return err
}

func (r *logRepo) GetHealthEventLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.HealthEventLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, child_id, log_date, event_type, description, symptoms, temperature_f, provider_name, diagnosis, treatment, follow_up_date, notes, logged_by, created_at
		FROM health_event_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.HealthEventLog
	for rows.Next() {
		var log models.HealthEventLog
		err := rows.Scan(
			&log.ID, &log.ChildID, &log.LogDate, &log.EventType, &log.Description,
			&log.Symptoms, &log.TemperatureF, &log.ProviderName, &log.Diagnosis,
			&log.Treatment, &log.FollowUpDate, &log.Notes, &log.LoggedBy, &log.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) GetHealthEventLogByID(ctx context.Context, id uuid.UUID) (*models.HealthEventLog, error) {
	query := `
		SELECT id, child_id, log_date, event_type, description, symptoms, temperature_f, provider_name, diagnosis, treatment, follow_up_date, notes, logged_by, created_at
		FROM health_event_logs
		WHERE id = $1
	`
	log := &models.HealthEventLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.ChildID, &log.LogDate, &log.EventType, &log.Description,
		&log.Symptoms, &log.TemperatureF, &log.ProviderName, &log.Diagnosis,
		&log.Treatment, &log.FollowUpDate, &log.Notes, &log.LoggedBy, &log.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return log, err
}

func (r *logRepo) UpdateHealthEventLog(ctx context.Context, log *models.HealthEventLog) error {
	query := `
		UPDATE health_event_logs
		SET event_type = $2, description = $3, symptoms = $4, temperature_f = $5, provider_name = $6, diagnosis = $7, treatment = $8, follow_up_date = $9, notes = $10
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.EventType, log.Description, log.Symptoms, log.TemperatureF,
		log.ProviderName, log.Diagnosis, log.Treatment, log.FollowUpDate, log.Notes,
	)
	return err
}

func (r *logRepo) DeleteHealthEventLog(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM health_event_logs WHERE id = $1`, id)
	return err
}

// Daily Logs Page
func (r *logRepo) GetDailyLogs(ctx context.Context, childID uuid.UUID, date time.Time) (*models.DailyLogPage, error) {
	// Get child first
	childQuery := `
		SELECT id, family_id, first_name, last_name, date_of_birth, gender, photo_url, notes, settings, is_active, created_at, updated_at
		FROM children
		WHERE id = $1
	`
	child := models.Child{}
	err := r.db.QueryRowContext(ctx, childQuery, childID).Scan(
		&child.ID, &child.FamilyID, &child.FirstName, &child.LastName,
		&child.DateOfBirth, &child.Gender, &child.PhotoURL, &child.Notes,
		&child.Settings, &child.IsActive, &child.CreatedAt, &child.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	page := &models.DailyLogPage{
		Child: child,
		Date:  date,
	}

	// Get medication logs
	medLogs, err := r.getMedicationLogsForDate(ctx, childID, date)
	if err != nil {
		return nil, err
	}
	page.MedicationLogs = medLogs

	// Get behavior logs
	behaviorLogs, err := r.GetBehaviorLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.BehaviorLogs = behaviorLogs

	// Get bowel logs
	bowelLogs, err := r.GetBowelLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.BowelLogs = bowelLogs

	// Get speech logs
	speechLogs, err := r.GetSpeechLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.SpeechLogs = speechLogs

	// Get diet logs
	dietLogs, err := r.GetDietLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.DietLogs = dietLogs

	// Get weight logs
	weightLogs, err := r.GetWeightLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.WeightLogs = weightLogs

	// Get sleep logs
	sleepLogs, err := r.GetSleepLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.SleepLogs = sleepLogs

	// Get sensory logs
	sensoryLogs, err := r.GetSensoryLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.SensoryLogs = sensoryLogs

	// Get social logs
	socialLogs, err := r.GetSocialLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.SocialLogs = socialLogs

	// Get therapy logs
	therapyLogs, err := r.GetTherapyLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.TherapyLogs = therapyLogs

	// Get seizure logs
	seizureLogs, err := r.GetSeizureLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.SeizureLogs = seizureLogs

	// Get health event logs
	healthEventLogs, err := r.GetHealthEventLogs(ctx, childID, date, date)
	if err != nil {
		return nil, err
	}
	page.HealthEventLogs = healthEventLogs

	return page, nil
}

// GetLogsForDateRange returns all logs for a child within a date range (for weekly view)
func (r *logRepo) GetLogsForDateRange(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (*models.DailyLogPage, error) {
	// Get child first
	childQuery := `
		SELECT id, family_id, first_name, last_name, date_of_birth, gender, photo_url, notes, settings, is_active, created_at, updated_at
		FROM children
		WHERE id = $1
	`
	child := models.Child{}
	err := r.db.QueryRowContext(ctx, childQuery, childID).Scan(
		&child.ID, &child.FamilyID, &child.FirstName, &child.LastName,
		&child.DateOfBirth, &child.Gender, &child.PhotoURL, &child.Notes,
		&child.Settings, &child.IsActive, &child.CreatedAt, &child.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	page := &models.DailyLogPage{
		Child:   child,
		Date:    startDate,
		EndDate: endDate,
	}

	// Get medication logs for date range
	medLogs, err := r.getMedicationLogsForDateRange(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.MedicationLogs = medLogs

	// Get behavior logs
	behaviorLogs, err := r.GetBehaviorLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.BehaviorLogs = behaviorLogs

	// Get bowel logs
	bowelLogs, err := r.GetBowelLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.BowelLogs = bowelLogs

	// Get speech logs
	speechLogs, err := r.GetSpeechLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.SpeechLogs = speechLogs

	// Get diet logs
	dietLogs, err := r.GetDietLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.DietLogs = dietLogs

	// Get weight logs
	weightLogs, err := r.GetWeightLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.WeightLogs = weightLogs

	// Get sleep logs
	sleepLogs, err := r.GetSleepLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.SleepLogs = sleepLogs

	// Get sensory logs
	sensoryLogs, err := r.GetSensoryLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.SensoryLogs = sensoryLogs

	// Get social logs
	socialLogs, err := r.GetSocialLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.SocialLogs = socialLogs

	// Get therapy logs
	therapyLogs, err := r.GetTherapyLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.TherapyLogs = therapyLogs

	// Get seizure logs
	seizureLogs, err := r.GetSeizureLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.SeizureLogs = seizureLogs

	// Get health event logs
	healthEventLogs, err := r.GetHealthEventLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	page.HealthEventLogs = healthEventLogs

	return page, nil
}

func (r *logRepo) getMedicationLogsForDateRange(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error) {
	startStr := startDate.Format("2006-01-02")
	endStr := endDate.Format("2006-01-02")
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE child_id = $1 AND log_date >= $2 AND log_date <= $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.MedicationLog
	for rows.Next() {
		var log models.MedicationLog
		err := rows.Scan(
			&log.ID, &log.MedicationID, &log.ChildID, &log.ScheduleID, &log.LogDate,
			&log.ScheduledTime, &log.ActualTime, &log.Status, &log.DosageGiven,
			&log.Notes, &log.LoggedBy, &log.CreatedAt, &log.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *logRepo) getMedicationLogsForDate(ctx context.Context, childID uuid.UUID, date time.Time) ([]models.MedicationLog, error) {
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE child_id = $1 AND log_date = $2
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.MedicationLog
	for rows.Next() {
		var log models.MedicationLog
		err := rows.Scan(
			&log.ID, &log.MedicationID, &log.ChildID, &log.ScheduleID, &log.LogDate,
			&log.ScheduledTime, &log.ActualTime, &log.Status, &log.DosageGiven,
			&log.Notes, &log.LoggedBy, &log.CreatedAt, &log.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// GetDatesWithLogs returns dates that have log entries for a child
func (r *logRepo) GetDatesWithLogs(ctx context.Context, childID uuid.UUID, limit int) ([]models.DateWithEntryCount, error) {
	// Query to get dates with entry counts across all log tables
	query := `
		WITH all_logs AS (
			SELECT log_date AS date FROM behavior_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM bowel_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM speech_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM diet_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM weight_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM sleep_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM sensory_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM social_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM therapy_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM seizure_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM health_event_logs WHERE child_id = $1
			UNION ALL
			SELECT log_date AS date FROM medication_logs WHERE child_id = $1
		)
		SELECT date, COUNT(*) as entry_count
		FROM all_logs
		GROUP BY date
		ORDER BY date DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, childID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []models.DateWithEntryCount
	for rows.Next() {
		var d models.DateWithEntryCount
		err := rows.Scan(&d.Date, &d.EntryCount)
		if err != nil {
			return nil, err
		}
		dates = append(dates, d)
	}
	return dates, rows.Err()
}
