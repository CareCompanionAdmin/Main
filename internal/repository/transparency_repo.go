package repository

import (
	"context"
	"database/sql"
	"time"

	"carecompanion/internal/models"

	"github.com/lib/pq"
)

type TransparencyRepository struct {
	db *sql.DB
}

func NewTransparencyRepository(db *sql.DB) *TransparencyRepository {
	return &TransparencyRepository{db: db}
}

// ============================================================================
// ALERT ANALYSIS DETAILS
// ============================================================================

// GetAnalysisDetails retrieves full analysis details for an alert
func (r *TransparencyRepository) GetAnalysisDetails(ctx context.Context, alertID string) (*models.AlertAnalysisDetails, error) {
	query := `
		SELECT id, alert_id, parent_view, clinical_view, data_points_used,
			   analysis_window_days, algorithm_version, processing_time_ms,
			   created_at, updated_at
		FROM alert_analysis
		WHERE alert_id = $1`

	var d models.AlertAnalysisDetails
	err := r.db.QueryRowContext(ctx, query, alertID).Scan(
		&d.ID, &d.AlertID, &d.ParentView, &d.ClinicalView, &d.DataPointsUsed,
		&d.AnalysisWindowDays, &d.AlgorithmVersion, &d.ProcessingTimeMs,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateAnalysisDetails creates analysis details for an alert
func (r *TransparencyRepository) CreateAnalysisDetails(ctx context.Context, d *models.AlertAnalysisDetails) error {
	query := `
		INSERT INTO alert_analysis (
			alert_id, parent_view, clinical_view, data_points_used,
			analysis_window_days, algorithm_version, processing_time_ms
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	return r.db.QueryRowContext(ctx, query,
		d.AlertID, d.ParentView, d.ClinicalView, d.DataPointsUsed,
		d.AnalysisWindowDays, d.AlgorithmVersion, d.ProcessingTimeMs,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

// ============================================================================
// CONFIDENCE FACTORS
// ============================================================================

// GetConfidenceFactors retrieves all confidence factors for an alert
func (r *TransparencyRepository) GetConfidenceFactors(ctx context.Context, alertID string) ([]models.AlertConfidenceFactor, error) {
	query := `
		SELECT id, alert_id, factor_order, factor_type, description, label, icon,
			   score, weight, contribution, citation_id, cohort_id, family_pattern_id,
			   cohort_match_criteria, cohort_sample_size, cohort_confirmation_rate,
			   family_history_instances, created_at
		FROM alert_confidence_factors
		WHERE alert_id = $1
		ORDER BY factor_order`

	rows, err := r.db.QueryContext(ctx, query, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var factors []models.AlertConfidenceFactor
	for rows.Next() {
		var f models.AlertConfidenceFactor
		err := rows.Scan(
			&f.ID, &f.AlertID, &f.FactorOrder, &f.FactorType, &f.Description,
			&f.Label, &f.Icon, &f.Score, &f.Weight, &f.Contribution,
			&f.CitationID, &f.CohortID, &f.FamilyPatternID, &f.CohortMatchCriteria,
			&f.CohortSampleSize, &f.CohortConfirmationRate, &f.FamilyHistoryInstances,
			&f.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		factors = append(factors, f)
	}
	return factors, rows.Err()
}

// CreateConfidenceFactor creates a confidence factor for an alert
func (r *TransparencyRepository) CreateConfidenceFactor(ctx context.Context, f *models.AlertConfidenceFactor) error {
	query := `
		INSERT INTO alert_confidence_factors (
			alert_id, factor_order, factor_type, description, score, weight,
			contribution, citation_id, cohort_id, family_pattern_id,
			cohort_match_criteria, cohort_sample_size, cohort_confirmation_rate,
			family_history_instances
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query,
		f.AlertID, f.FactorOrder, f.FactorType, f.Description, f.Score, f.Weight,
		f.Contribution, f.CitationID, f.CohortID, f.FamilyPatternID,
		f.CohortMatchCriteria, f.CohortSampleSize, f.CohortConfirmationRate,
		f.FamilyHistoryInstances,
	).Scan(&f.ID, &f.CreatedAt)
}

// ============================================================================
// COHORT MATCHING
// ============================================================================

// GetCohortMatching retrieves cohort matching info for an alert
func (r *TransparencyRepository) GetCohortMatching(ctx context.Context, alertID string) ([]models.AlertCohortMatching, error) {
	query := `
		SELECT acm.id, acm.alert_id, acm.matched_cohort_id, acm.criteria_used,
			   acm.criteria_excluded, acm.cohort_size, acm.pattern_presentations,
			   acm.pattern_confirmations, acm.pattern_denials, acm.pattern_no_response,
			   acm.confirmation_rate, acm.confirmation_trend, acm.created_at,
			   c.name as cohort_name, c.description as cohort_description
		FROM alert_cohort_matching acm
		JOIN cohorts c ON c.id = acm.matched_cohort_id
		WHERE acm.alert_id = $1`

	rows, err := r.db.QueryContext(ctx, query, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []models.AlertCohortMatching
	for rows.Next() {
		var m models.AlertCohortMatching
		var cohortName string
		var cohortDesc sql.NullString
		err := rows.Scan(
			&m.ID, &m.AlertID, &m.MatchedCohortID, &m.CriteriaUsed,
			&m.CriteriaExcluded, &m.CohortSize, &m.PatternPresentations,
			&m.PatternConfirmations, &m.PatternDenials, &m.PatternNoResponse,
			&m.ConfirmationRate, &m.ConfirmationTrend, &m.CreatedAt,
			&cohortName, &cohortDesc,
		)
		if err != nil {
			return nil, err
		}
		m.Cohort = &models.Cohort{
			ID:          m.MatchedCohortID,
			Name:        cohortName,
			Description: cohortDesc,
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// ============================================================================
// CITATIONS
// ============================================================================

// GetCitation retrieves a citation by ID
func (r *TransparencyRepository) GetCitation(ctx context.Context, id string) (*models.Citation, error) {
	query := `
		SELECT id, citation_type, source_table, source_id, authority_name,
			   authority_type, publication_title, publication_section, publication_date,
			   url, excerpt, retrieved_at, created_at
		FROM citations
		WHERE id = $1`

	var c models.Citation
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.CitationType, &c.SourceTable, &c.SourceID, &c.AuthorityName,
		&c.AuthorityType, &c.PublicationTitle, &c.PublicationSection, &c.PublicationDate,
		&c.URL, &c.Excerpt, &c.RetrievedAt, &c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CreateCitation creates a new citation
func (r *TransparencyRepository) CreateCitation(ctx context.Context, c *models.Citation) error {
	query := `
		INSERT INTO citations (
			citation_type, source_table, source_id, authority_name, authority_type,
			publication_title, publication_section, publication_date, url, excerpt
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, retrieved_at, created_at`

	return r.db.QueryRowContext(ctx, query,
		c.CitationType, c.SourceTable, c.SourceID, c.AuthorityName, c.AuthorityType,
		c.PublicationTitle, c.PublicationSection, c.PublicationDate, c.URL, c.Excerpt,
	).Scan(&c.ID, &c.RetrievedAt, &c.CreatedAt)
}

// ============================================================================
// TREATMENT CHANGES
// ============================================================================

// GetPendingInterrogatives retrieves pending treatment change questions for a user
func (r *TransparencyRepository) GetPendingInterrogatives(ctx context.Context, userID string) ([]models.TreatmentChange, error) {
	query := `
		SELECT tc.id, tc.child_id, tc.change_type, tc.source_table, tc.source_id,
			   tc.previous_value, tc.new_value, tc.change_summary, tc.changed_by_user_id,
			   tc.potentially_related_alert_id, tc.potentially_related_share_thread_id,
			   tc.days_since_analysis_shared, tc.interrogative_status,
			   tc.interrogative_prompted_at, tc.interrogative_answered_at, tc.created_at
		FROM treatment_changes tc
		JOIN children c ON c.id = tc.child_id
		JOIN family_memberships fm ON fm.family_id = c.family_id AND fm.user_id = $1
		WHERE tc.interrogative_status IN ('pending', 'prompted')
		ORDER BY tc.created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []models.TreatmentChange
	for rows.Next() {
		var tc models.TreatmentChange
		err := rows.Scan(
			&tc.ID, &tc.ChildID, &tc.ChangeType, &tc.SourceTable, &tc.SourceID,
			&tc.PreviousValue, &tc.NewValue, &tc.ChangeSummary, &tc.ChangedByUserID,
			&tc.PotentiallyRelatedAlertID, &tc.PotentiallyRelatedShareThreadID,
			&tc.DaysSinceAnalysisShared, &tc.InterrogativeStatus,
			&tc.InterrogativePromptedAt, &tc.InterrogativeAnsweredAt, &tc.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		changes = append(changes, tc)
	}
	return changes, rows.Err()
}

// CreateTreatmentChange creates a new treatment change record
func (r *TransparencyRepository) CreateTreatmentChange(ctx context.Context, tc *models.TreatmentChange) error {
	query := `
		INSERT INTO treatment_changes (
			child_id, change_type, source_table, source_id, previous_value, new_value,
			change_summary, changed_by_user_id, potentially_related_alert_id,
			potentially_related_share_thread_id, days_since_analysis_shared
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, interrogative_status, created_at`

	return r.db.QueryRowContext(ctx, query,
		tc.ChildID, tc.ChangeType, tc.SourceTable, tc.SourceID, tc.PreviousValue,
		tc.NewValue, tc.ChangeSummary, tc.ChangedByUserID, tc.PotentiallyRelatedAlertID,
		tc.PotentiallyRelatedShareThreadID, tc.DaysSinceAnalysisShared,
	).Scan(&tc.ID, &tc.InterrogativeStatus, &tc.CreatedAt)
}

// UpdateTreatmentChangeStatus updates the interrogative status
func (r *TransparencyRepository) UpdateTreatmentChangeStatus(ctx context.Context, id string, status models.InterrogativeStatus) error {
	query := `UPDATE treatment_changes SET interrogative_status = $1`
	args := []interface{}{status}

	switch status {
	case models.InterrogativeStatusPrompted:
		query += `, interrogative_prompted_at = $2 WHERE id = $3`
		args = append(args, time.Now(), id)
	case models.InterrogativeStatusAnswered:
		query += `, interrogative_answered_at = $2 WHERE id = $3`
		args = append(args, time.Now(), id)
	default:
		query += ` WHERE id = $2`
		args = append(args, id)
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// CreateTreatmentChangeResponse creates a response to a treatment change question
func (r *TransparencyRepository) CreateTreatmentChangeResponse(ctx context.Context, resp *models.TreatmentChangeResponse) error {
	query := `
		INSERT INTO treatment_change_responses (
			treatment_change_id, responded_by_user_id, change_source,
			related_to_analysis, provider_user_id, provider_name_freetext, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query,
		resp.TreatmentChangeID, resp.RespondedByUserID, resp.ChangeSource,
		resp.RelatedToAnalysis, resp.ProviderUserID, resp.ProviderNameFreetext, resp.Notes,
	).Scan(&resp.ID, &resp.CreatedAt)
}

// ============================================================================
// USER INTERACTION PREFERENCES
// ============================================================================

// GetUserInteractionPreferences retrieves preferences for a user
func (r *TransparencyRepository) GetUserInteractionPreferences(ctx context.Context, userID string) (*models.UserInteractionPreferences, error) {
	query := `
		SELECT id, user_id, treatment_change_prompt_delay_hours,
			   interrogative_quiet_start, interrogative_quiet_end,
			   interrogative_preferred_days, batch_interrogatives,
			   max_interrogatives_per_day, interrogative_reminder_hours,
			   max_interrogative_reminders, created_at, updated_at
		FROM user_interaction_preferences
		WHERE user_id = $1`

	var p models.UserInteractionPreferences
	var preferredDays pq.Int64Array
	err := r.db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.TreatmentChangePromptDelayHours,
		&p.InterrogativeQuietStart, &p.InterrogativeQuietEnd,
		&preferredDays, &p.BatchInterrogatives,
		&p.MaxInterrogativesPerDay, &p.InterrogativeReminderHours,
		&p.MaxInterrogativeReminders, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Return defaults
		return &models.UserInteractionPreferences{
			UserID:                          userID,
			TreatmentChangePromptDelayHours: 4,
			InterrogativePreferredDays:      []int{0, 1, 2, 3, 4, 5, 6},
			BatchInterrogatives:             false,
			MaxInterrogativesPerDay:         3,
			MaxInterrogativeReminders:       2,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	p.InterrogativePreferredDays = make([]int, len(preferredDays))
	for i, v := range preferredDays {
		p.InterrogativePreferredDays[i] = int(v)
	}
	return &p, nil
}

// UpsertUserInteractionPreferences creates or updates preferences
func (r *TransparencyRepository) UpsertUserInteractionPreferences(ctx context.Context, p *models.UserInteractionPreferences) error {
	query := `
		INSERT INTO user_interaction_preferences (
			user_id, treatment_change_prompt_delay_hours,
			interrogative_quiet_start, interrogative_quiet_end,
			interrogative_preferred_days, batch_interrogatives,
			max_interrogatives_per_day, interrogative_reminder_hours,
			max_interrogative_reminders
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id) DO UPDATE SET
			treatment_change_prompt_delay_hours = EXCLUDED.treatment_change_prompt_delay_hours,
			interrogative_quiet_start = EXCLUDED.interrogative_quiet_start,
			interrogative_quiet_end = EXCLUDED.interrogative_quiet_end,
			interrogative_preferred_days = EXCLUDED.interrogative_preferred_days,
			batch_interrogatives = EXCLUDED.batch_interrogatives,
			max_interrogatives_per_day = EXCLUDED.max_interrogatives_per_day,
			interrogative_reminder_hours = EXCLUDED.interrogative_reminder_hours,
			max_interrogative_reminders = EXCLUDED.max_interrogative_reminders,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

	preferredDays := make(pq.Int64Array, len(p.InterrogativePreferredDays))
	for i, v := range p.InterrogativePreferredDays {
		preferredDays[i] = int64(v)
	}

	return r.db.QueryRowContext(ctx, query,
		p.UserID, p.TreatmentChangePromptDelayHours,
		p.InterrogativeQuietStart, p.InterrogativeQuietEnd,
		preferredDays, p.BatchInterrogatives,
		p.MaxInterrogativesPerDay, p.InterrogativeReminderHours,
		p.MaxInterrogativeReminders,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// ============================================================================
// ALERT EXPORTS
// ============================================================================

// CreateAlertExport records an export of an alert
func (r *TransparencyRepository) CreateAlertExport(ctx context.Context, e *models.AlertExport) error {
	query := `
		INSERT INTO alert_exports (
			alert_id, exported_by_user_id, export_type,
			shared_with_user_id, shared_via, view_mode
		) VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`

	return r.db.QueryRowContext(ctx, query,
		e.AlertID, e.ExportedByUserID, e.ExportType,
		e.SharedWithUserID, e.SharedVia, e.ViewMode,
	).Scan(&e.ID, &e.CreatedAt)
}

// ============================================================================
// COHORTS
// ============================================================================

// GetCohort retrieves a cohort by ID
func (r *TransparencyRepository) GetCohort(ctx context.Context, id string) (*models.Cohort, error) {
	query := `
		SELECT id, name, description, age_range_min, age_range_max,
			   diagnoses, medications, member_count, is_active, created_at, updated_at
		FROM cohorts
		WHERE id = $1`

	var c models.Cohort
	var diagnoses, medications pq.StringArray
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Description, &c.AgeRangeMin, &c.AgeRangeMax,
		&diagnoses, &medications, &c.MemberCount, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.Diagnoses = diagnoses
	c.Medications = medications
	return &c, nil
}

// FindMatchingCohorts finds cohorts that match a child's profile
func (r *TransparencyRepository) FindMatchingCohorts(ctx context.Context, childID string) ([]models.Cohort, error) {
	query := `
		SELECT c.id, c.name, c.description, c.age_range_min, c.age_range_max,
			   c.diagnoses, c.medications, c.member_count, c.is_active, c.created_at, c.updated_at
		FROM cohorts c
		JOIN children ch ON ch.id = $1
		WHERE c.is_active = true
		AND (c.age_range_min IS NULL OR EXTRACT(YEAR FROM AGE(ch.date_of_birth)) >= c.age_range_min)
		AND (c.age_range_max IS NULL OR EXTRACT(YEAR FROM AGE(ch.date_of_birth)) <= c.age_range_max)`

	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cohorts []models.Cohort
	for rows.Next() {
		var c models.Cohort
		var diagnoses, medications pq.StringArray
		err := rows.Scan(
			&c.ID, &c.Name, &c.Description, &c.AgeRangeMin, &c.AgeRangeMax,
			&diagnoses, &medications, &c.MemberCount, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		c.Diagnoses = diagnoses
		c.Medications = medications
		cohorts = append(cohorts, c)
	}
	return cohorts, rows.Err()
}

// ============================================================================
// ALERT FEEDBACK
// ============================================================================

// CreateAlertFeedback records user feedback on an alert
func (r *TransparencyRepository) CreateAlertFeedback(ctx context.Context, alertID, userID string, wasHelpful bool, feedbackText, actionTaken string) error {
	query := `
		INSERT INTO alert_feedback (alert_id, user_id, was_helpful, feedback_text, action_taken)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.db.ExecContext(ctx, query, alertID, userID, wasHelpful, feedbackText, actionTaken)
	return err
}
