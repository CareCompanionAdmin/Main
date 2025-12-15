package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type childRepo struct {
	db *sql.DB
}

func NewChildRepo(db *sql.DB) ChildRepository {
	return &childRepo{db: db}
}

func (r *childRepo) Create(ctx context.Context, child *models.Child) error {
	query := `
		INSERT INTO children (id, family_id, first_name, last_name, date_of_birth, gender, photo_url, notes, settings, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`
	child.ID = uuid.New()
	child.IsActive = true
	child.CreatedAt = time.Now()
	child.UpdatedAt = time.Now()
	if child.Settings == nil {
		child.Settings = models.JSONB{}
	}

	_, err := r.db.ExecContext(ctx, query,
		child.ID,
		child.FamilyID,
		child.FirstName,
		child.LastName,
		child.DateOfBirth,
		child.Gender,
		child.PhotoURL,
		child.Notes,
		child.Settings,
		child.IsActive,
		child.CreatedAt,
		child.UpdatedAt,
	)
	return err
}

func (r *childRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Child, error) {
	query := `
		SELECT id, family_id, first_name, last_name, date_of_birth, gender, photo_url, notes, settings, is_active, created_at, updated_at
		FROM children
		WHERE id = $1
	`
	child := &models.Child{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&child.ID,
		&child.FamilyID,
		&child.FirstName,
		&child.LastName,
		&child.DateOfBirth,
		&child.Gender,
		&child.PhotoURL,
		&child.Notes,
		&child.Settings,
		&child.IsActive,
		&child.CreatedAt,
		&child.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load conditions
	conditions, err := r.GetConditions(ctx, child.ID)
	if err != nil {
		return nil, err
	}
	child.Conditions = conditions

	return child, nil
}

func (r *childRepo) GetByFamilyID(ctx context.Context, familyID uuid.UUID) ([]models.Child, error) {
	query := `
		SELECT id, family_id, first_name, last_name, date_of_birth, gender, photo_url, notes, settings, is_active, created_at, updated_at
		FROM children
		WHERE family_id = $1 AND is_active = true
		ORDER BY first_name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var children []models.Child
	for rows.Next() {
		var child models.Child
		err := rows.Scan(
			&child.ID,
			&child.FamilyID,
			&child.FirstName,
			&child.LastName,
			&child.DateOfBirth,
			&child.Gender,
			&child.PhotoURL,
			&child.Notes,
			&child.Settings,
			&child.IsActive,
			&child.CreatedAt,
			&child.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		children = append(children, child)
	}
	return children, rows.Err()
}

func (r *childRepo) Update(ctx context.Context, child *models.Child) error {
	query := `
		UPDATE children
		SET first_name = $2, last_name = $3, date_of_birth = $4, gender = $5, photo_url = $6, notes = $7, settings = $8, updated_at = $9
		WHERE id = $1
	`
	child.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		child.ID,
		child.FirstName,
		child.LastName,
		child.DateOfBirth,
		child.Gender,
		child.PhotoURL,
		child.Notes,
		child.Settings,
		child.UpdatedAt,
	)
	return err
}

func (r *childRepo) Delete(ctx context.Context, id uuid.UUID) error {
	// Soft delete
	query := `UPDATE children SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *childRepo) AddCondition(ctx context.Context, condition *models.ChildCondition) error {
	query := `
		INSERT INTO child_conditions (id, child_id, condition_name, icd_code, diagnosed_date, diagnosed_by, severity, notes, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	condition.ID = uuid.New()
	condition.IsActive = true
	condition.CreatedAt = time.Now()
	condition.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		condition.ID,
		condition.ChildID,
		condition.ConditionName,
		condition.ICDCode,
		condition.DiagnosedDate,
		condition.DiagnosedBy,
		condition.Severity,
		condition.Notes,
		condition.IsActive,
		condition.CreatedAt,
		condition.UpdatedAt,
	)
	return err
}

func (r *childRepo) GetConditions(ctx context.Context, childID uuid.UUID) ([]models.ChildCondition, error) {
	query := `
		SELECT id, child_id, condition_name, icd_code, diagnosed_date, diagnosed_by, severity, notes, is_active, created_at, updated_at
		FROM child_conditions
		WHERE child_id = $1 AND is_active = true
		ORDER BY condition_name ASC
	`
	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conditions []models.ChildCondition
	for rows.Next() {
		var c models.ChildCondition
		err := rows.Scan(
			&c.ID, &c.ChildID, &c.ConditionName, &c.ICDCode, &c.DiagnosedDate,
			&c.DiagnosedBy, &c.Severity, &c.Notes, &c.IsActive, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, c)
	}
	return conditions, rows.Err()
}

func (r *childRepo) UpdateCondition(ctx context.Context, condition *models.ChildCondition) error {
	query := `
		UPDATE child_conditions
		SET condition_name = $2, icd_code = $3, diagnosed_date = $4, diagnosed_by = $5, severity = $6, notes = $7, updated_at = $8
		WHERE id = $1
	`
	condition.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		condition.ID,
		condition.ConditionName,
		condition.ICDCode,
		condition.DiagnosedDate,
		condition.DiagnosedBy,
		condition.Severity,
		condition.Notes,
		condition.UpdatedAt,
	)
	return err
}

func (r *childRepo) RemoveCondition(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE child_conditions SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *childRepo) GetDashboard(ctx context.Context, childID uuid.UUID, date time.Time) (*models.ChildDashboard, error) {
	child, err := r.GetByID(ctx, childID)
	if err != nil || child == nil {
		return nil, err
	}

	dashboard := &models.ChildDashboard{
		Child: *child,
		TodayLogs: models.DailyLogSummary{
			Date: date,
		},
	}

	// Get medication summary for today
	medQuery := `
		SELECT
			COUNT(*) FILTER (WHERE ml.status = 'taken') as taken,
			COUNT(*) as total
		FROM medication_schedules ms
		JOIN medications m ON m.id = ms.medication_id
		LEFT JOIN medication_logs ml ON ml.medication_id = m.id
			AND ml.schedule_id = ms.id
			AND ml.log_date = $2
		WHERE m.child_id = $1 AND m.is_active = true AND ms.is_active = true
	`
	r.db.QueryRowContext(ctx, medQuery, childID, date).Scan(
		&dashboard.TodayLogs.MedicationsTaken,
		&dashboard.TodayLogs.MedicationsTotal,
	)
	dashboard.TodayLogs.HasMedicationLog = dashboard.TodayLogs.MedicationsTaken > 0

	// Get behavior log summary
	behaviorQuery := `
		SELECT mood_level, energy_level, meltdowns
		FROM behavior_logs
		WHERE child_id = $1 AND log_date = $2
		ORDER BY created_at DESC
		LIMIT 1
	`
	var moodLevel, energyLevel sql.NullInt64
	var meltdowns int
	err = r.db.QueryRowContext(ctx, behaviorQuery, childID, date).Scan(&moodLevel, &energyLevel, &meltdowns)
	if err == nil {
		dashboard.TodayLogs.HasBehaviorLog = true
		if moodLevel.Valid {
			ml := int(moodLevel.Int64)
			dashboard.TodayLogs.MoodLevel = &ml
		}
		if energyLevel.Valid {
			el := int(energyLevel.Int64)
			dashboard.TodayLogs.EnergyLevel = &el
		}
		dashboard.TodayLogs.Meltdowns = meltdowns
	}

	// Check for bowel log
	bowelQuery := `SELECT EXISTS(SELECT 1 FROM bowel_logs WHERE child_id = $1 AND log_date = $2)`
	r.db.QueryRowContext(ctx, bowelQuery, childID, date).Scan(&dashboard.TodayLogs.HasBowelLog)

	// Get sleep log
	sleepQuery := `
		SELECT total_sleep_minutes
		FROM sleep_logs
		WHERE child_id = $1 AND log_date = $2
		LIMIT 1
	`
	var sleepMinutes sql.NullInt64
	err = r.db.QueryRowContext(ctx, sleepQuery, childID, date).Scan(&sleepMinutes)
	if err == nil && sleepMinutes.Valid {
		dashboard.TodayLogs.HasSleepLog = true
		sm := int(sleepMinutes.Int64)
		dashboard.TodayLogs.SleepMinutes = &sm
	}

	// Get active alerts
	alertQuery := `
		SELECT id, child_id, family_id, alert_type, severity, status, title, description, data, created_at, updated_at
		FROM alerts
		WHERE child_id = $1 AND status = 'active'
		ORDER BY severity DESC, created_at DESC
		LIMIT 5
	`
	alertRows, err := r.db.QueryContext(ctx, alertQuery, childID)
	if err == nil {
		defer alertRows.Close()
		for alertRows.Next() {
			var alert models.Alert
			alertRows.Scan(
				&alert.ID, &alert.ChildID, &alert.FamilyID, &alert.AlertType, &alert.Severity,
				&alert.Status, &alert.Title, &alert.Description, &alert.Data, &alert.CreatedAt, &alert.UpdatedAt,
			)
			dashboard.ActiveAlerts = append(dashboard.ActiveAlerts, alert)
		}
	}

	// Get week summary
	weekStart := date.AddDate(0, 0, -7)
	weekQuery := `
		SELECT
			COUNT(DISTINCT log_date) as days_logged,
			COALESCE(AVG(mood_level), 0) as avg_mood
		FROM behavior_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
	`
	r.db.QueryRowContext(ctx, weekQuery, childID, weekStart, date).Scan(
		&dashboard.WeekSummary.DaysLogged,
		&dashboard.WeekSummary.AverageMood,
	)

	// Medication adherence for week
	adherenceQuery := `
		SELECT
			CASE WHEN COUNT(*) = 0 THEN 0
			ELSE COUNT(*) FILTER (WHERE ml.status = 'taken')::float / COUNT(*)::float * 100
			END
		FROM medication_schedules ms
		JOIN medications m ON m.id = ms.medication_id
		LEFT JOIN medication_logs ml ON ml.medication_id = m.id AND ml.schedule_id = ms.id
			AND ml.log_date BETWEEN $2 AND $3
		WHERE m.child_id = $1 AND m.is_active = true AND ms.is_active = true
	`
	r.db.QueryRowContext(ctx, adherenceQuery, childID, weekStart, date).Scan(&dashboard.WeekSummary.MedicationAdherence)

	// Alert count for week
	alertCountQuery := `SELECT COUNT(*) FROM alerts WHERE child_id = $1 AND created_at BETWEEN $2 AND $3`
	r.db.QueryRowContext(ctx, alertCountQuery, childID, weekStart, date).Scan(&dashboard.WeekSummary.AlertCount)

	return dashboard, nil
}
