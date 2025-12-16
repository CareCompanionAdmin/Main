package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type alertRepo struct {
	db *sql.DB
}

func NewAlertRepo(db *sql.DB) AlertRepository {
	return &alertRepo{db: db}
}

func (r *alertRepo) Create(ctx context.Context, alert *models.Alert) error {
	query := `
		INSERT INTO alerts (id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, source_type, confidence_score, date_range_start, date_range_end, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`
	alert.ID = uuid.New()
	alert.Status = models.AlertStatusActive
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		alert.ID, alert.ChildID, alert.FamilyID, alert.AlertType, alert.Severity,
		alert.Status, alert.Title, alert.Description, alert.Data, alert.CorrelationID,
		alert.SourceType, alert.ConfidenceScore, alert.DateRangeStart, alert.DateRangeEnd,
		alert.CreatedAt, alert.UpdatedAt,
	)
	return err
}

func (r *alertRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	query := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, COALESCE(source_type::text, '') as source_type, confidence_score, date_range_start, date_range_end, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM alerts
		WHERE id = $1
	`
	alert := &models.Alert{}
	var confidenceScore sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&alert.ID, &alert.ChildID, &alert.FamilyID, &alert.AlertType, &alert.Severity,
		&alert.Status, &alert.Title, &alert.Description, &alert.Data, &alert.CorrelationID,
		&alert.SourceType, &confidenceScore, &alert.DateRangeStart, &alert.DateRangeEnd,
		&alert.AcknowledgedBy, &alert.AcknowledgedAt, &alert.ResolvedBy, &alert.ResolvedAt,
		&alert.CreatedAt, &alert.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if confidenceScore.Valid {
		alert.ConfidenceScore = &confidenceScore.Float64
	}
	return alert, nil
}

func (r *alertRepo) GetByChildID(ctx context.Context, childID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error) {
	query := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, COALESCE(source_type::text, '') as source_type, confidence_score, date_range_start, date_range_end, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM alerts
		WHERE child_id = $1
	`
	args := []interface{}{childID}
	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}
	query += ` ORDER BY severity DESC, created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var alert models.Alert
		var confidenceScore sql.NullFloat64
		err := rows.Scan(
			&alert.ID, &alert.ChildID, &alert.FamilyID, &alert.AlertType, &alert.Severity,
			&alert.Status, &alert.Title, &alert.Description, &alert.Data, &alert.CorrelationID,
			&alert.SourceType, &confidenceScore, &alert.DateRangeStart, &alert.DateRangeEnd,
			&alert.AcknowledgedBy, &alert.AcknowledgedAt, &alert.ResolvedBy, &alert.ResolvedAt,
			&alert.CreatedAt, &alert.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if confidenceScore.Valid {
			alert.ConfidenceScore = &confidenceScore.Float64
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (r *alertRepo) GetByFamilyID(ctx context.Context, familyID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error) {
	query := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, COALESCE(source_type::text, '') as source_type, confidence_score, date_range_start, date_range_end, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM alerts
		WHERE family_id = $1
	`
	args := []interface{}{familyID}
	if status != nil {
		query += ` AND status = $2`
		args = append(args, *status)
	}
	query += ` ORDER BY severity DESC, created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var alert models.Alert
		var confidenceScore sql.NullFloat64
		err := rows.Scan(
			&alert.ID, &alert.ChildID, &alert.FamilyID, &alert.AlertType, &alert.Severity,
			&alert.Status, &alert.Title, &alert.Description, &alert.Data, &alert.CorrelationID,
			&alert.SourceType, &confidenceScore, &alert.DateRangeStart, &alert.DateRangeEnd,
			&alert.AcknowledgedBy, &alert.AcknowledgedAt, &alert.ResolvedBy, &alert.ResolvedAt,
			&alert.CreatedAt, &alert.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if confidenceScore.Valid {
			alert.ConfidenceScore = &confidenceScore.Float64
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

func (r *alertRepo) Update(ctx context.Context, alert *models.Alert) error {
	query := `
		UPDATE alerts
		SET status = $2, title = $3, description = $4, data = $5, updated_at = $6
		WHERE id = $1
	`
	alert.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		alert.ID, alert.Status, alert.Title, alert.Description, alert.Data, alert.UpdatedAt,
	)
	return err
}

func (r *alertRepo) Acknowledge(ctx context.Context, id, userID uuid.UUID) error {
	query := `
		UPDATE alerts
		SET status = $2, acknowledged_by = $3, acknowledged_at = $4, updated_at = $4
		WHERE id = $1
	`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, id, models.AlertStatusAcknowledged, userID, now)
	return err
}

func (r *alertRepo) Resolve(ctx context.Context, id, userID uuid.UUID) error {
	query := `
		UPDATE alerts
		SET status = $2, resolved_by = $3, resolved_at = $4, updated_at = $4
		WHERE id = $1
	`
	now := time.Now()
	_, err := r.db.ExecContext(ctx, query, id, models.AlertStatusResolved, userID, now)
	return err
}

func (r *alertRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM alerts WHERE id = $1`, id)
	return err
}

func (r *alertRepo) CreateFeedback(ctx context.Context, feedback *models.AlertFeedback) error {
	query := `
		INSERT INTO alert_feedback (id, alert_id, user_id, was_helpful, feedback_text, action_taken, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	feedback.ID = uuid.New()
	feedback.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		feedback.ID, feedback.AlertID, feedback.UserID,
		feedback.WasHelpful, feedback.FeedbackText, feedback.ActionTaken,
		feedback.CreatedAt,
	)
	return err
}

func (r *alertRepo) GetFeedback(ctx context.Context, alertID uuid.UUID) ([]models.AlertFeedback, error) {
	query := `
		SELECT id, alert_id, user_id, was_helpful, feedback_text, action_taken, created_at
		FROM alert_feedback
		WHERE alert_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feedback []models.AlertFeedback
	for rows.Next() {
		var f models.AlertFeedback
		err := rows.Scan(
			&f.ID, &f.AlertID, &f.UserID,
			&f.WasHelpful, &f.FeedbackText, &f.ActionTaken,
			&f.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		feedback = append(feedback, f)
	}
	return feedback, rows.Err()
}

func (r *alertRepo) GetStats(ctx context.Context, childID uuid.UUID) (*models.AlertStats, error) {
	stats := &models.AlertStats{}

	// Get total active
	activeQuery := `SELECT COUNT(*) FROM alerts WHERE child_id = $1 AND status = 'active'`
	r.db.QueryRowContext(ctx, activeQuery, childID).Scan(&stats.TotalActive)

	// Get this week's count
	weekStart := time.Now().AddDate(0, 0, -7)
	weekQuery := `SELECT COUNT(*) FROM alerts WHERE child_id = $1 AND created_at >= $2`
	r.db.QueryRowContext(ctx, weekQuery, childID, weekStart).Scan(&stats.TotalThisWeek)

	// Get this month's count
	monthStart := time.Now().AddDate(0, -1, 0)
	monthQuery := `SELECT COUNT(*) FROM alerts WHERE child_id = $1 AND created_at >= $2`
	r.db.QueryRowContext(ctx, monthQuery, childID, monthStart).Scan(&stats.TotalThisMonth)

	// Get severity counts for active alerts
	severityQuery := `
		SELECT
			COUNT(*) FILTER (WHERE severity = 'critical') as critical,
			COUNT(*) FILTER (WHERE severity = 'warning') as warning,
			COUNT(*) FILTER (WHERE severity = 'info') as info
		FROM alerts
		WHERE child_id = $1 AND status = 'active'
	`
	r.db.QueryRowContext(ctx, severityQuery, childID).Scan(
		&stats.CriticalCount, &stats.WarningCount, &stats.InfoCount,
	)

	return stats, nil
}

func (r *alertRepo) GetStatsByType(ctx context.Context, childID uuid.UUID, alertType string) (*models.AlertTypeStats, error) {
	stats := &models.AlertTypeStats{}

	query := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status = 'acknowledged') as acknowledged,
			COUNT(*) FILTER (WHERE status = 'resolved') as resolved,
			COUNT(*) FILTER (WHERE status = 'dismissed') as dismissed,
			(SELECT COUNT(*) FROM alert_feedback af
			 JOIN alerts a ON a.id = af.alert_id
			 WHERE a.child_id = $1 AND a.alert_type = $2 AND af.was_helpful = true) as helpful_feedback
		FROM alerts
		WHERE child_id = $1 AND alert_type = $2
	`
	err := r.db.QueryRowContext(ctx, query, childID, alertType).Scan(
		&stats.Total, &stats.Acknowledged, &stats.Resolved, &stats.Dismissed, &stats.HelpfulFeedback,
	)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (r *alertRepo) GetByChildIDAndTypeSince(ctx context.Context, childID uuid.UUID, alertType string, since time.Time) ([]models.Alert, error) {
	query := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, acknowledged_at, acknowledged_by, resolved_at, resolved_by, confidence_score, created_at, updated_at
		FROM alerts
		WHERE child_id = $1 AND alert_type = $2 AND created_at >= $3
		ORDER BY created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, alertType, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []models.Alert
	for rows.Next() {
		var a models.Alert
		err := rows.Scan(
			&a.ID, &a.ChildID, &a.FamilyID, &a.AlertType, &a.Severity,
			&a.Status, &a.Title, &a.Description, &a.Data, &a.CorrelationID,
			&a.AcknowledgedAt, &a.AcknowledgedBy, &a.ResolvedAt, &a.ResolvedBy,
			&a.ConfidenceScore, &a.CreatedAt, &a.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (r *alertRepo) GetAlertsPage(ctx context.Context, childID uuid.UUID) (*models.AlertsPage, error) {
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

	page := &models.AlertsPage{
		Child: child,
	}

	// Get active alerts
	activeStatus := models.AlertStatusActive
	activeAlerts, err := r.GetByChildID(ctx, childID, &activeStatus)
	if err != nil {
		return nil, err
	}
	page.ActiveAlerts = activeAlerts

	// Get recent alerts (last 30 days, excluding active)
	recentQuery := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, correlation_id, COALESCE(source_type::text, '') as source_type, confidence_score, date_range_start, date_range_end, acknowledged_by, acknowledged_at, resolved_by, resolved_at, created_at, updated_at
		FROM alerts
		WHERE child_id = $1 AND status != 'active' AND created_at >= $2
		ORDER BY created_at DESC
		LIMIT 50
	`
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
	rows, err := r.db.QueryContext(ctx, recentQuery, childID, thirtyDaysAgo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var alert models.Alert
		var confidenceScore sql.NullFloat64
		err := rows.Scan(
			&alert.ID, &alert.ChildID, &alert.FamilyID, &alert.AlertType, &alert.Severity,
			&alert.Status, &alert.Title, &alert.Description, &alert.Data, &alert.CorrelationID,
			&alert.SourceType, &confidenceScore, &alert.DateRangeStart, &alert.DateRangeEnd,
			&alert.AcknowledgedBy, &alert.AcknowledgedAt, &alert.ResolvedBy, &alert.ResolvedAt,
			&alert.CreatedAt, &alert.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if confidenceScore.Valid {
			alert.ConfidenceScore = &confidenceScore.Float64
		}
		page.RecentAlerts = append(page.RecentAlerts, alert)
	}

	// Get stats
	stats, err := r.GetStats(ctx, childID)
	if err != nil {
		return nil, err
	}
	page.AlertStats = *stats

	return page, nil
}
