package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type cohortRepo struct {
	db *sql.DB
}

func NewCohortRepo(db *sql.DB) CohortRepository {
	return &cohortRepo{db: db}
}

// CreateCohort creates a new cohort definition
func (r *cohortRepo) CreateCohort(ctx context.Context, cohort *models.CohortDefinition) error {
	query := `
		INSERT INTO cohort_definitions (id, name, description, criteria, min_members, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	cohort.ID = uuid.New()
	cohort.IsActive = true
	cohort.CreatedAt = time.Now()
	cohort.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		cohort.ID, cohort.Name, cohort.Description, cohort.Criteria,
		cohort.MinMembers, cohort.IsActive, cohort.CreatedAt, cohort.UpdatedAt,
	)
	return err
}

// GetCohort retrieves a cohort by ID
func (r *cohortRepo) GetCohort(ctx context.Context, id uuid.UUID) (*models.CohortDefinition, error) {
	query := `
		SELECT id, name, description, criteria, min_members, is_active, created_at, updated_at
		FROM cohort_definitions
		WHERE id = $1
	`
	cohort := &models.CohortDefinition{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&cohort.ID, &cohort.Name, &cohort.Description, &cohort.Criteria,
		&cohort.MinMembers, &cohort.IsActive, &cohort.CreatedAt, &cohort.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return cohort, err
}

// GetAllCohorts retrieves all active cohorts
func (r *cohortRepo) GetAllCohorts(ctx context.Context) ([]models.CohortDefinition, error) {
	query := `
		SELECT id, name, description, criteria, min_members, is_active, created_at, updated_at
		FROM cohort_definitions
		WHERE is_active = true
		ORDER BY name
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cohorts []models.CohortDefinition
	for rows.Next() {
		var cohort models.CohortDefinition
		err := rows.Scan(
			&cohort.ID, &cohort.Name, &cohort.Description, &cohort.Criteria,
			&cohort.MinMembers, &cohort.IsActive, &cohort.CreatedAt, &cohort.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		cohorts = append(cohorts, cohort)
	}
	return cohorts, rows.Err()
}

// UpdateCohort updates a cohort definition
func (r *cohortRepo) UpdateCohort(ctx context.Context, cohort *models.CohortDefinition) error {
	query := `
		UPDATE cohort_definitions
		SET name = $2, description = $3, criteria = $4, min_members = $5, is_active = $6, updated_at = $7
		WHERE id = $1
	`
	cohort.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		cohort.ID, cohort.Name, cohort.Description, cohort.Criteria,
		cohort.MinMembers, cohort.IsActive, cohort.UpdatedAt,
	)
	return err
}

// DeleteCohort soft-deletes a cohort
func (r *cohortRepo) DeleteCohort(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE cohort_definitions SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

// AddMember adds an anonymous member to a cohort
func (r *cohortRepo) AddMember(ctx context.Context, cohortID uuid.UUID, childHash string, matchScore float64) error {
	query := `
		INSERT INTO cohort_memberships (id, cohort_id, child_hash, match_score, joined_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (cohort_id, child_hash) DO UPDATE SET match_score = $4
	`
	_, err := r.db.ExecContext(ctx, query, uuid.New(), cohortID, childHash, matchScore, time.Now())
	return err
}

// RemoveMember removes a member from a cohort
func (r *cohortRepo) RemoveMember(ctx context.Context, cohortID uuid.UUID, childHash string) error {
	query := `DELETE FROM cohort_memberships WHERE cohort_id = $1 AND child_hash = $2`
	_, err := r.db.ExecContext(ctx, query, cohortID, childHash)
	return err
}

// GetMemberCount returns the number of members in a cohort
func (r *cohortRepo) GetMemberCount(ctx context.Context, cohortID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM cohort_memberships WHERE cohort_id = $1`
	var count int
	err := r.db.QueryRowContext(ctx, query, cohortID).Scan(&count)
	return count, err
}

// IsMember checks if a child hash is a member of a cohort
func (r *cohortRepo) IsMember(ctx context.Context, cohortID uuid.UUID, childHash string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM cohort_memberships WHERE cohort_id = $1 AND child_hash = $2)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, cohortID, childHash).Scan(&exists)
	return exists, err
}

// CreatePattern creates a new cohort pattern
func (r *cohortRepo) CreatePattern(ctx context.Context, pattern *models.CohortPattern) error {
	query := `
		INSERT INTO cohort_patterns (
			id, cohort_id, pattern_type, input_factor, output_factor,
			families_affected, families_total, avg_correlation, std_deviation,
			confidence_interval_low, confidence_interval_high,
			simple_description, detailed_description,
			first_observed_at, last_updated_at, is_active
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	pattern.ID = uuid.New()
	pattern.IsActive = true
	pattern.LastUpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		pattern.ID, pattern.CohortID, pattern.PatternType, pattern.InputFactor, pattern.OutputFactor,
		pattern.FamiliesAffected, pattern.FamiliesTotal, pattern.AvgCorrelation, pattern.StdDeviation,
		pattern.ConfidenceIntervalLow, pattern.ConfidenceIntervalHigh,
		pattern.SimpleDescription, pattern.DetailedDescription,
		pattern.FirstObservedAt, pattern.LastUpdatedAt, pattern.IsActive,
	)
	return err
}

// GetCohortPatterns retrieves all patterns for a cohort
func (r *cohortRepo) GetCohortPatterns(ctx context.Context, cohortID uuid.UUID) ([]models.CohortPattern, error) {
	return r.queryPatterns(ctx, `
		SELECT id, cohort_id, pattern_type, input_factor, output_factor,
			families_affected, families_total, avg_correlation, std_deviation,
			confidence_interval_low, confidence_interval_high,
			simple_description, detailed_description,
			first_observed_at, last_updated_at, is_active
		FROM cohort_patterns
		WHERE cohort_id = $1
		ORDER BY families_affected DESC
	`, cohortID)
}

// GetActivePatterns retrieves active patterns for a cohort
func (r *cohortRepo) GetActivePatterns(ctx context.Context, cohortID uuid.UUID) ([]models.CohortPattern, error) {
	return r.queryPatterns(ctx, `
		SELECT id, cohort_id, pattern_type, input_factor, output_factor,
			families_affected, families_total, avg_correlation, std_deviation,
			confidence_interval_low, confidence_interval_high,
			simple_description, detailed_description,
			first_observed_at, last_updated_at, is_active
		FROM cohort_patterns
		WHERE cohort_id = $1 AND is_active = true
		ORDER BY families_affected DESC
	`, cohortID)
}

// UpdatePattern updates a cohort pattern
func (r *cohortRepo) UpdatePattern(ctx context.Context, pattern *models.CohortPattern) error {
	query := `
		UPDATE cohort_patterns SET
			families_affected = $2, families_total = $3,
			avg_correlation = $4, std_deviation = $5,
			confidence_interval_low = $6, confidence_interval_high = $7,
			simple_description = $8, detailed_description = $9,
			last_updated_at = $10, is_active = $11
		WHERE id = $1
	`
	pattern.LastUpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		pattern.ID,
		pattern.FamiliesAffected, pattern.FamiliesTotal,
		pattern.AvgCorrelation, pattern.StdDeviation,
		pattern.ConfidenceIntervalLow, pattern.ConfidenceIntervalHigh,
		pattern.SimpleDescription, pattern.DetailedDescription,
		pattern.LastUpdatedAt, pattern.IsActive,
	)
	return err
}

// DeletePattern soft-deletes a pattern
func (r *cohortRepo) DeletePattern(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE cohort_patterns SET is_active = false, last_updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *cohortRepo) queryPatterns(ctx context.Context, query string, args ...interface{}) ([]models.CohortPattern, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patterns []models.CohortPattern
	for rows.Next() {
		var p models.CohortPattern
		err := rows.Scan(
			&p.ID, &p.CohortID, &p.PatternType, &p.InputFactor, &p.OutputFactor,
			&p.FamiliesAffected, &p.FamiliesTotal, &p.AvgCorrelation, &p.StdDeviation,
			&p.ConfidenceIntervalLow, &p.ConfidenceIntervalHigh,
			&p.SimpleDescription, &p.DetailedDescription,
			&p.FirstObservedAt, &p.LastUpdatedAt, &p.IsActive,
		)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}
