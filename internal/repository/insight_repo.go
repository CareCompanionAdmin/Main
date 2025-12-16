package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type insightRepo struct {
	db *sql.DB
}

func NewInsightRepo(db *sql.DB) InsightRepository {
	return &insightRepo{db: db}
}

func (r *insightRepo) Create(ctx context.Context, insight *models.Insight) error {
	query := `
		INSERT INTO insights (
			id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, $12,
			$13, $14, $15,
			$16, $17, $18, $19,
			$20, $21,
			$22, $23,
			$24, $25, $26,
			$27, $28, $29
		)
	`
	insight.ID = uuid.New()
	insight.IsActive = true
	insight.CreatedAt = time.Now()
	insight.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		insight.ID, insight.ChildID, insight.FamilyID, insight.Tier, insight.Category,
		insight.Title, insight.SimpleDescription, insight.DetailedDescription,
		insight.ConfidenceScore, insight.SampleSize, insight.CorrelationStrength, insight.PValue,
		insight.CohortCriteria, insight.CohortSize, insight.CohortMatchScore,
		insight.InputFactors, insight.OutputFactors, insight.LagHours, insight.DataPointCount,
		insight.DateRangeStart, insight.DateRangeEnd,
		insight.SourceIDs, insight.PatternID,
		insight.ClinicallyValidated, insight.ValidationCount, insight.LastValidatedAt,
		insight.IsActive, insight.CreatedAt, insight.UpdatedAt,
	)
	return err
}

func (r *insightRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Insight, error) {
	query := `
		SELECT id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		FROM insights
		WHERE id = $1
	`
	insight := &models.Insight{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&insight.ID, &insight.ChildID, &insight.FamilyID, &insight.Tier, &insight.Category,
		&insight.Title, &insight.SimpleDescription, &insight.DetailedDescription,
		&insight.ConfidenceScore, &insight.SampleSize, &insight.CorrelationStrength, &insight.PValue,
		&insight.CohortCriteria, &insight.CohortSize, &insight.CohortMatchScore,
		&insight.InputFactors, &insight.OutputFactors, &insight.LagHours, &insight.DataPointCount,
		&insight.DateRangeStart, &insight.DateRangeEnd,
		&insight.SourceIDs, &insight.PatternID,
		&insight.ClinicallyValidated, &insight.ValidationCount, &insight.LastValidatedAt,
		&insight.IsActive, &insight.CreatedAt, &insight.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return insight, err
}

func (r *insightRepo) Update(ctx context.Context, insight *models.Insight) error {
	query := `
		UPDATE insights SET
			title = $2, simple_description = $3, detailed_description = $4,
			confidence_score = $5, sample_size = $6, correlation_strength = $7, p_value = $8,
			cohort_criteria = $9, cohort_size = $10, cohort_match_score = $11,
			input_factors = $12, output_factors = $13, lag_hours = $14, data_point_count = $15,
			date_range_start = $16, date_range_end = $17,
			clinically_validated = $18, validation_count = $19, last_validated_at = $20,
			is_active = $21, updated_at = $22
		WHERE id = $1
	`
	insight.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		insight.ID,
		insight.Title, insight.SimpleDescription, insight.DetailedDescription,
		insight.ConfidenceScore, insight.SampleSize, insight.CorrelationStrength, insight.PValue,
		insight.CohortCriteria, insight.CohortSize, insight.CohortMatchScore,
		insight.InputFactors, insight.OutputFactors, insight.LagHours, insight.DataPointCount,
		insight.DateRangeStart, insight.DateRangeEnd,
		insight.ClinicallyValidated, insight.ValidationCount, insight.LastValidatedAt,
		insight.IsActive, insight.UpdatedAt,
	)
	return err
}

func (r *insightRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE insights SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *insightRepo) GetByChildID(ctx context.Context, childID uuid.UUID, tier *models.InsightTier, activeOnly bool) ([]models.Insight, error) {
	query := `
		SELECT id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		FROM insights
		WHERE child_id = $1
	`
	args := []interface{}{childID}
	argNum := 2

	if tier != nil {
		query += ` AND tier = $` + string(rune('0'+argNum))
		args = append(args, *tier)
		argNum++
	}
	if activeOnly {
		query += ` AND is_active = true`
	}
	query += ` ORDER BY confidence_score DESC, created_at DESC`

	return r.queryInsights(ctx, query, args...)
}

func (r *insightRepo) GetByChildIDSince(ctx context.Context, childID uuid.UUID, since time.Time) ([]models.Insight, error) {
	query := `
		SELECT id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		FROM insights
		WHERE child_id = $1 AND created_at >= $2 AND is_active = true
		ORDER BY created_at DESC
	`
	return r.queryInsights(ctx, query, childID, since)
}

func (r *insightRepo) GetGlobalInsights(ctx context.Context, category string) ([]models.Insight, error) {
	query := `
		SELECT id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		FROM insights
		WHERE tier = 1 AND child_id IS NULL AND is_active = true
	`
	args := []interface{}{}
	if category != "" {
		query += ` AND category = $1`
		args = append(args, category)
	}
	query += ` ORDER BY confidence_score DESC`

	return r.queryInsights(ctx, query, args...)
}

func (r *insightRepo) GetByPatternID(ctx context.Context, patternID uuid.UUID) (*models.Insight, error) {
	query := `
		SELECT id, child_id, family_id, tier, category,
			title, simple_description, detailed_description,
			confidence_score, sample_size, correlation_strength, p_value,
			cohort_criteria, cohort_size, cohort_match_score,
			input_factors, output_factors, lag_hours, data_point_count,
			date_range_start, date_range_end,
			source_ids, pattern_id,
			clinically_validated, validation_count, last_validated_at,
			is_active, created_at, updated_at
		FROM insights
		WHERE pattern_id = $1 AND is_active = true
		LIMIT 1
	`
	insight := &models.Insight{}
	err := r.db.QueryRowContext(ctx, query, patternID).Scan(
		&insight.ID, &insight.ChildID, &insight.FamilyID, &insight.Tier, &insight.Category,
		&insight.Title, &insight.SimpleDescription, &insight.DetailedDescription,
		&insight.ConfidenceScore, &insight.SampleSize, &insight.CorrelationStrength, &insight.PValue,
		&insight.CohortCriteria, &insight.CohortSize, &insight.CohortMatchScore,
		&insight.InputFactors, &insight.OutputFactors, &insight.LagHours, &insight.DataPointCount,
		&insight.DateRangeStart, &insight.DateRangeEnd,
		&insight.SourceIDs, &insight.PatternID,
		&insight.ClinicallyValidated, &insight.ValidationCount, &insight.LastValidatedAt,
		&insight.IsActive, &insight.CreatedAt, &insight.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return insight, err
}

func (r *insightRepo) IncrementValidation(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE insights
		SET validation_count = validation_count + 1,
			last_validated_at = $2,
			updated_at = $2
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *insightRepo) SetClinicallyValidated(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE insights
		SET clinically_validated = true,
			validation_count = validation_count + 1,
			last_validated_at = $2,
			updated_at = $2
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *insightRepo) CreateSource(ctx context.Context, source *models.InsightSource) error {
	query := `
		INSERT INTO insight_sources (id, name, source_type, external_id, url, data, retrieved_at, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	source.ID = uuid.New()
	source.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		source.ID, source.Name, source.SourceType, source.ExternalID,
		source.URL, source.Data, source.RetrievedAt, source.ExpiresAt, source.CreatedAt,
	)
	return err
}

func (r *insightRepo) GetSource(ctx context.Context, id uuid.UUID) (*models.InsightSource, error) {
	query := `
		SELECT id, name, source_type, external_id, url, data, retrieved_at, expires_at, created_at
		FROM insight_sources
		WHERE id = $1
	`
	source := &models.InsightSource{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&source.ID, &source.Name, &source.SourceType, &source.ExternalID,
		&source.URL, &source.Data, &source.RetrievedAt, &source.ExpiresAt, &source.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return source, err
}

func (r *insightRepo) GetSourcesByInsight(ctx context.Context, insightID uuid.UUID) ([]models.InsightSource, error) {
	// First get the insight to find source_ids
	insight, err := r.GetByID(ctx, insightID)
	if err != nil || insight == nil || len(insight.SourceIDs) == 0 {
		return nil, err
	}

	query := `
		SELECT id, name, source_type, external_id, url, data, retrieved_at, expires_at, created_at
		FROM insight_sources
		WHERE id = ANY($1)
	`
	rows, err := r.db.QueryContext(ctx, query, insight.SourceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sources []models.InsightSource
	for rows.Next() {
		var source models.InsightSource
		err := rows.Scan(
			&source.ID, &source.Name, &source.SourceType, &source.ExternalID,
			&source.URL, &source.Data, &source.RetrievedAt, &source.ExpiresAt, &source.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (r *insightRepo) Upsert(ctx context.Context, insight *models.Insight) error {
	// If there's a pattern_id, try to find existing insight for that pattern
	if insight.PatternID.Valid {
		existing, err := r.GetByPatternID(ctx, insight.PatternID.UUID)
		if err != nil {
			return err
		}
		if existing != nil {
			// Update existing insight
			insight.ID = existing.ID
			insight.CreatedAt = existing.CreatedAt
			insight.ValidationCount = existing.ValidationCount
			insight.ClinicallyValidated = existing.ClinicallyValidated
			return r.Update(ctx, insight)
		}
	}

	// Create new insight
	return r.Create(ctx, insight)
}

func (r *insightRepo) queryInsights(ctx context.Context, query string, args ...interface{}) ([]models.Insight, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var insights []models.Insight
	for rows.Next() {
		var insight models.Insight
		err := rows.Scan(
			&insight.ID, &insight.ChildID, &insight.FamilyID, &insight.Tier, &insight.Category,
			&insight.Title, &insight.SimpleDescription, &insight.DetailedDescription,
			&insight.ConfidenceScore, &insight.SampleSize, &insight.CorrelationStrength, &insight.PValue,
			&insight.CohortCriteria, &insight.CohortSize, &insight.CohortMatchScore,
			&insight.InputFactors, &insight.OutputFactors, &insight.LagHours, &insight.DataPointCount,
			&insight.DateRangeStart, &insight.DateRangeEnd,
			&insight.SourceIDs, &insight.PatternID,
			&insight.ClinicallyValidated, &insight.ValidationCount, &insight.LastValidatedAt,
			&insight.IsActive, &insight.CreatedAt, &insight.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		insights = append(insights, insight)
	}
	return insights, rows.Err()
}
