package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type correlationRepo struct {
	db *sql.DB
}

func NewCorrelationRepo(db *sql.DB) CorrelationRepository {
	return &correlationRepo{db: db}
}

// Baselines
func (r *correlationRepo) CreateBaseline(ctx context.Context, baseline *models.ChildBaseline) error {
	query := `
		INSERT INTO child_baselines (id, child_id, metric_name, baseline_value, std_deviation, sample_size, calculated_at, valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	baseline.ID = uuid.New()
	baseline.CalculatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		baseline.ID, baseline.ChildID, baseline.MetricName,
		baseline.BaselineValue, baseline.StdDeviation, baseline.SampleSize,
		baseline.CalculatedAt, baseline.ValidUntil,
	)
	return err
}

func (r *correlationRepo) GetBaselines(ctx context.Context, childID uuid.UUID) ([]models.ChildBaseline, error) {
	query := `
		SELECT id, child_id, metric_name, baseline_value, std_deviation, sample_size, calculated_at, valid_until
		FROM child_baselines
		WHERE child_id = $1 AND (valid_until IS NULL OR valid_until > NOW())
		ORDER BY metric_name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var baselines []models.ChildBaseline
	for rows.Next() {
		var b models.ChildBaseline
		err := rows.Scan(
			&b.ID, &b.ChildID, &b.MetricName,
			&b.BaselineValue, &b.StdDeviation, &b.SampleSize,
			&b.CalculatedAt, &b.ValidUntil,
		)
		if err != nil {
			return nil, err
		}
		baselines = append(baselines, b)
	}
	return baselines, rows.Err()
}

func (r *correlationRepo) GetBaseline(ctx context.Context, childID uuid.UUID, metricName string) (*models.ChildBaseline, error) {
	query := `
		SELECT id, child_id, metric_name, baseline_value, std_deviation, sample_size, calculated_at, valid_until
		FROM child_baselines
		WHERE child_id = $1 AND metric_name = $2 AND (valid_until IS NULL OR valid_until > NOW())
		ORDER BY calculated_at DESC
		LIMIT 1
	`
	b := &models.ChildBaseline{}
	err := r.db.QueryRowContext(ctx, query, childID, metricName).Scan(
		&b.ID, &b.ChildID, &b.MetricName,
		&b.BaselineValue, &b.StdDeviation, &b.SampleSize,
		&b.CalculatedAt, &b.ValidUntil,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

func (r *correlationRepo) UpdateBaseline(ctx context.Context, baseline *models.ChildBaseline) error {
	query := `
		UPDATE child_baselines
		SET baseline_value = $2, std_deviation = $3, sample_size = $4, calculated_at = $5, valid_until = $6
		WHERE id = $1
	`
	baseline.CalculatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		baseline.ID, baseline.BaselineValue, baseline.StdDeviation,
		baseline.SampleSize, baseline.CalculatedAt, baseline.ValidUntil,
	)
	return err
}

// Correlation Requests
func (r *correlationRepo) CreateCorrelationRequest(ctx context.Context, req *models.CorrelationRequest) error {
	query := `
		INSERT INTO correlation_requests (id, child_id, requested_by, status, input_factors, output_factors, date_range_start, date_range_end, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	req.ID = uuid.New()
	req.Status = models.CorrelationStatusPending
	req.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		req.ID, req.ChildID, req.RequestedBy, req.Status,
		req.InputFactors, req.OutputFactors,
		req.DateRangeStart, req.DateRangeEnd, req.CreatedAt,
	)
	return err
}

func (r *correlationRepo) GetCorrelationRequest(ctx context.Context, id uuid.UUID) (*models.CorrelationRequest, error) {
	query := `
		SELECT id, child_id, requested_by, status, input_factors, output_factors, date_range_start, date_range_end, results, started_at, completed_at, error_message, created_at
		FROM correlation_requests
		WHERE id = $1
	`
	req := &models.CorrelationRequest{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&req.ID, &req.ChildID, &req.RequestedBy, &req.Status,
		&req.InputFactors, &req.OutputFactors,
		&req.DateRangeStart, &req.DateRangeEnd, &req.Results,
		&req.StartedAt, &req.CompletedAt, &req.ErrorMessage, &req.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return req, err
}

func (r *correlationRepo) GetCorrelationRequests(ctx context.Context, childID uuid.UUID, status *models.CorrelationStatus) ([]models.CorrelationRequest, error) {
	query := `
		SELECT id, child_id, requested_by, status, input_factors, output_factors, date_range_start, date_range_end, results, started_at, completed_at, error_message, created_at
		FROM correlation_requests
		WHERE child_id = $1
	`
	args := []interface{}{childID}
	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []models.CorrelationRequest
	for rows.Next() {
		var req models.CorrelationRequest
		err := rows.Scan(
			&req.ID, &req.ChildID, &req.RequestedBy, &req.Status,
			&req.InputFactors, &req.OutputFactors,
			&req.DateRangeStart, &req.DateRangeEnd, &req.Results,
			&req.StartedAt, &req.CompletedAt, &req.ErrorMessage, &req.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (r *correlationRepo) UpdateCorrelationRequest(ctx context.Context, req *models.CorrelationRequest) error {
	query := `
		UPDATE correlation_requests
		SET status = $2, results = $3, started_at = $4, completed_at = $5, error_message = $6
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		req.ID, req.Status, req.Results,
		req.StartedAt, req.CompletedAt, req.ErrorMessage,
	)
	return err
}

// Patterns
func (r *correlationRepo) CreatePattern(ctx context.Context, pattern *models.FamilyPattern) error {
	query := `
		INSERT INTO family_patterns (id, child_id, pattern_type, input_factor, output_factor, correlation_strength, confidence_score, sample_size, lag_hours, description, supporting_data, first_detected_at, last_confirmed_at, times_confirmed, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`
	pattern.ID = uuid.New()
	pattern.IsActive = true
	pattern.TimesConfirmed = 1
	pattern.CreatedAt = time.Now()
	pattern.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		pattern.ID, pattern.ChildID, pattern.PatternType,
		pattern.InputFactor, pattern.OutputFactor,
		pattern.CorrelationStrength, pattern.ConfidenceScore,
		pattern.SampleSize, pattern.LagHours, pattern.Description,
		pattern.SupportingData, pattern.FirstDetectedAt, pattern.LastConfirmedAt,
		pattern.TimesConfirmed, pattern.IsActive, pattern.CreatedAt, pattern.UpdatedAt,
	)
	return err
}

func (r *correlationRepo) GetPattern(ctx context.Context, id uuid.UUID) (*models.FamilyPattern, error) {
	query := `
		SELECT id, child_id, pattern_type, input_factor, output_factor, correlation_strength, confidence_score, sample_size, lag_hours, description, supporting_data, first_detected_at, last_confirmed_at, times_confirmed, is_active, created_at, updated_at
		FROM family_patterns
		WHERE id = $1
	`
	p := &models.FamilyPattern{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.ChildID, &p.PatternType,
		&p.InputFactor, &p.OutputFactor,
		&p.CorrelationStrength, &p.ConfidenceScore,
		&p.SampleSize, &p.LagHours, &p.Description,
		&p.SupportingData, &p.FirstDetectedAt, &p.LastConfirmedAt,
		&p.TimesConfirmed, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *correlationRepo) GetPatterns(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.FamilyPattern, error) {
	query := `
		SELECT id, child_id, pattern_type, input_factor, output_factor, correlation_strength, confidence_score, sample_size, lag_hours, description, supporting_data, first_detected_at, last_confirmed_at, times_confirmed, is_active, created_at, updated_at
		FROM family_patterns
		WHERE child_id = $1
	`
	if activeOnly {
		query += ` AND is_active = true`
	}
	query += ` ORDER BY correlation_strength DESC, confidence_score DESC`

	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []models.FamilyPattern
	for rows.Next() {
		var p models.FamilyPattern
		err := rows.Scan(
			&p.ID, &p.ChildID, &p.PatternType,
			&p.InputFactor, &p.OutputFactor,
			&p.CorrelationStrength, &p.ConfidenceScore,
			&p.SampleSize, &p.LagHours, &p.Description,
			&p.SupportingData, &p.FirstDetectedAt, &p.LastConfirmedAt,
			&p.TimesConfirmed, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}

func (r *correlationRepo) UpdatePattern(ctx context.Context, pattern *models.FamilyPattern) error {
	query := `
		UPDATE family_patterns
		SET correlation_strength = $2, confidence_score = $3, sample_size = $4, description = $5, supporting_data = $6, last_confirmed_at = $7, times_confirmed = $8, is_active = $9, updated_at = $10
		WHERE id = $1
	`
	pattern.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		pattern.ID, pattern.CorrelationStrength, pattern.ConfidenceScore,
		pattern.SampleSize, pattern.Description, pattern.SupportingData,
		pattern.LastConfirmedAt, pattern.TimesConfirmed, pattern.IsActive,
		pattern.UpdatedAt,
	)
	return err
}

func (r *correlationRepo) DeletePattern(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE family_patterns SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

// Clinical Validations
func (r *correlationRepo) CreateValidation(ctx context.Context, validation *models.ClinicalValidation) error {
	query := `
		INSERT INTO clinical_validations (id, pattern_id, alert_id, child_id, provider_user_id, validation_type, treatment_changed, treatment_description, parent_confirmed, parent_confirmed_at, validation_strength, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	validation.ID = uuid.New()
	validation.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		validation.ID, validation.PatternID, validation.AlertID,
		validation.ChildID, validation.ProviderUserID, validation.ValidationType,
		validation.TreatmentChanged, validation.TreatmentDescription,
		validation.ParentConfirmed, validation.ParentConfirmedAt,
		validation.ValidationStrength, validation.ExpiresAt, validation.CreatedAt,
	)
	return err
}

func (r *correlationRepo) GetValidations(ctx context.Context, childID uuid.UUID) ([]models.ClinicalValidation, error) {
	query := `
		SELECT id, pattern_id, alert_id, child_id, provider_user_id, validation_type, treatment_changed, treatment_description, parent_confirmed, parent_confirmed_at, validation_strength, expires_at, created_at
		FROM clinical_validations
		WHERE child_id = $1 AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var validations []models.ClinicalValidation
	for rows.Next() {
		var v models.ClinicalValidation
		err := rows.Scan(
			&v.ID, &v.PatternID, &v.AlertID,
			&v.ChildID, &v.ProviderUserID, &v.ValidationType,
			&v.TreatmentChanged, &v.TreatmentDescription,
			&v.ParentConfirmed, &v.ParentConfirmedAt,
			&v.ValidationStrength, &v.ExpiresAt, &v.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		validations = append(validations, v)
	}
	return validations, rows.Err()
}

func (r *correlationRepo) GetValidation(ctx context.Context, id uuid.UUID) (*models.ClinicalValidation, error) {
	query := `
		SELECT id, pattern_id, alert_id, child_id, provider_user_id, validation_type, treatment_changed, treatment_description, parent_confirmed, parent_confirmed_at, validation_strength, expires_at, created_at
		FROM clinical_validations
		WHERE id = $1
	`
	v := &models.ClinicalValidation{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&v.ID, &v.PatternID, &v.AlertID,
		&v.ChildID, &v.ProviderUserID, &v.ValidationType,
		&v.TreatmentChanged, &v.TreatmentDescription,
		&v.ParentConfirmed, &v.ParentConfirmedAt,
		&v.ValidationStrength, &v.ExpiresAt, &v.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

// Insights Page
func (r *correlationRepo) GetInsightsPage(ctx context.Context, childID uuid.UUID) (*models.InsightsPage, error) {
	// Get child
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

	page := &models.InsightsPage{
		Child: child,
	}

	// Get active patterns
	patterns, err := r.GetPatterns(ctx, childID, true)
	if err != nil {
		return nil, err
	}
	page.Patterns = patterns

	// Get recent correlations
	correlations, err := r.GetCorrelationRequests(ctx, childID, nil)
	if err != nil {
		return nil, err
	}
	if len(correlations) > 10 {
		correlations = correlations[:10]
	}
	page.RecentCorrelations = correlations

	// Get baselines
	baselines, err := r.GetBaselines(ctx, childID)
	if err != nil {
		return nil, err
	}
	page.Baselines = baselines

	return page, nil
}

// GetCorrelationData retrieves time-series data for the correlation engine
func (r *correlationRepo) GetCorrelationData(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (map[string][]models.DataPoint, error) {
	data := make(map[string][]models.DataPoint)

	// Get behavior data
	behaviorQuery := `
		SELECT log_date, mood_level, energy_level, anxiety_level, meltdowns, stimming_episodes
		FROM behavior_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
		ORDER BY log_date ASC
	`
	rows, err := r.db.QueryContext(ctx, behaviorQuery, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var logDate time.Time
		var moodLevel, energyLevel, anxietyLevel sql.NullInt64
		var meltdowns, stimmingEpisodes int
		rows.Scan(&logDate, &moodLevel, &energyLevel, &anxietyLevel, &meltdowns, &stimmingEpisodes)

		if moodLevel.Valid {
			data["mood"] = append(data["mood"], models.DataPoint{Date: logDate, Value: float64(moodLevel.Int64)})
		}
		if energyLevel.Valid {
			data["energy"] = append(data["energy"], models.DataPoint{Date: logDate, Value: float64(energyLevel.Int64)})
		}
		if anxietyLevel.Valid {
			data["anxiety"] = append(data["anxiety"], models.DataPoint{Date: logDate, Value: float64(anxietyLevel.Int64)})
		}
		data["meltdowns"] = append(data["meltdowns"], models.DataPoint{Date: logDate, Value: float64(meltdowns)})
		data["stimming"] = append(data["stimming"], models.DataPoint{Date: logDate, Value: float64(stimmingEpisodes)})
	}
	rows.Close()

	// Get sleep data
	sleepQuery := `
		SELECT log_date, total_sleep_minutes, night_wakings
		FROM sleep_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
		ORDER BY log_date ASC
	`
	rows, err = r.db.QueryContext(ctx, sleepQuery, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var logDate time.Time
		var sleepMinutes sql.NullInt64
		var nightWakings int
		rows.Scan(&logDate, &sleepMinutes, &nightWakings)

		if sleepMinutes.Valid {
			data["sleep_minutes"] = append(data["sleep_minutes"], models.DataPoint{Date: logDate, Value: float64(sleepMinutes.Int64)})
		}
		data["night_wakings"] = append(data["night_wakings"], models.DataPoint{Date: logDate, Value: float64(nightWakings)})
	}
	rows.Close()

	// Get medication adherence
	medQuery := `
		SELECT ml.log_date,
			CASE WHEN ml.status = 'taken' THEN 1.0 ELSE 0.0 END as taken
		FROM medication_logs ml
		WHERE ml.child_id = $1 AND ml.log_date BETWEEN $2 AND $3
		ORDER BY ml.log_date ASC
	`
	rows, err = r.db.QueryContext(ctx, medQuery, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var logDate time.Time
		var taken float64
		rows.Scan(&logDate, &taken)
		data["medication_adherence"] = append(data["medication_adherence"], models.DataPoint{Date: logDate, Value: taken})
	}
	rows.Close()

	// Get bowel data (bristol scale, count per day)
	bowelQuery := `
		SELECT log_date, COALESCE(AVG(bristol_scale::text::integer), 0), COUNT(*)
		FROM bowel_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
		GROUP BY log_date
		ORDER BY log_date ASC
	`
	rows, err = r.db.QueryContext(ctx, bowelQuery, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var logDate time.Time
		var avgBristol float64
		var count int
		rows.Scan(&logDate, &avgBristol, &count)
		data["bristol_scale"] = append(data["bristol_scale"], models.DataPoint{Date: logDate, Value: avgBristol})
		data["bowel_count"] = append(data["bowel_count"], models.DataPoint{Date: logDate, Value: float64(count)})
	}
	rows.Close()

	// Get diet data
	dietQuery := `
		SELECT log_date, COALESCE(water_intake_oz, 0)
		FROM diet_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
		ORDER BY log_date ASC
	`
	rows, err = r.db.QueryContext(ctx, dietQuery, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var logDate time.Time
		var waterIntake int
		rows.Scan(&logDate, &waterIntake)
		data["water_intake"] = append(data["water_intake"], models.DataPoint{Date: logDate, Value: float64(waterIntake)})
	}
	rows.Close()

	return data, nil
}
