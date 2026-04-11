package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// ReportRepository handles report and scheduled report operations
type ReportRepository interface {
	Create(ctx context.Context, report *models.Report) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Report, error)
	GetByChildID(ctx context.Context, childID uuid.UUID, limit int) ([]models.Report, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status, filePath string, fileSize int64) error
	UpdateError(ctx context.Context, id uuid.UUID, errMsg string) error
	Delete(ctx context.Context, id uuid.UUID) error

	CreateScheduled(ctx context.Context, sr *models.ScheduledReport) error
	GetScheduledByID(ctx context.Context, id uuid.UUID) (*models.ScheduledReport, error)
	GetScheduledByChildID(ctx context.Context, childID uuid.UUID) ([]models.ScheduledReport, error)
	GetDueScheduledReports(ctx context.Context) ([]models.ScheduledReport, error)
	UpdateScheduledLastRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error
	DeactivateScheduled(ctx context.Context, id uuid.UUID) error
	DeleteScheduled(ctx context.Context, id uuid.UUID) error
}

type reportRepo struct {
	db *sql.DB
}

func NewReportRepo(db *sql.DB) ReportRepository {
	return &reportRepo{db: db}
}

func (r *reportRepo) Create(ctx context.Context, report *models.Report) error {
	report.ID = uuid.New()
	report.Status = "generating"
	report.CreatedAt = time.Now()

	query := `INSERT INTO reports (id, child_id, family_id, created_by, title, report_type, period_type, start_date, end_date, data_filters, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

	_, err := r.db.ExecContext(ctx, query,
		report.ID, report.ChildID, report.FamilyID, report.CreatedBy,
		report.Title, report.ReportType, report.PeriodType,
		report.StartDate, report.EndDate, report.DataFilters,
		report.Status, report.CreatedAt)
	return err
}

func (r *reportRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Report, error) {
	query := `SELECT id, child_id, family_id, created_by, title, report_type, period_type,
		start_date, end_date, data_filters, file_path, file_size, status, error_message, created_at, completed_at
		FROM reports WHERE id = $1`

	var report models.Report
	var fileSize sql.NullInt64
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&report.ID, &report.ChildID, &report.FamilyID, &report.CreatedBy,
		&report.Title, &report.ReportType, &report.PeriodType,
		&report.StartDate, &report.EndDate, &report.DataFilters,
		&report.FilePath, &fileSize, &report.Status, &report.ErrorMessage,
		&report.CreatedAt, &report.CompletedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if fileSize.Valid {
		report.FileSize = &fileSize.Int64
	}
	return &report, nil
}

func (r *reportRepo) GetByChildID(ctx context.Context, childID uuid.UUID, limit int) ([]models.Report, error) {
	query := `SELECT id, child_id, family_id, created_by, title, report_type, period_type,
		start_date, end_date, data_filters, file_path, file_size, status, error_message, created_at, completed_at
		FROM reports WHERE child_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, childID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []models.Report
	for rows.Next() {
		var rpt models.Report
		var fileSize sql.NullInt64
		if err := rows.Scan(
			&rpt.ID, &rpt.ChildID, &rpt.FamilyID, &rpt.CreatedBy,
			&rpt.Title, &rpt.ReportType, &rpt.PeriodType,
			&rpt.StartDate, &rpt.EndDate, &rpt.DataFilters,
			&rpt.FilePath, &fileSize, &rpt.Status, &rpt.ErrorMessage,
			&rpt.CreatedAt, &rpt.CompletedAt); err != nil {
			return nil, err
		}
		if fileSize.Valid {
			rpt.FileSize = &fileSize.Int64
		}
		reports = append(reports, rpt)
	}
	return reports, rows.Err()
}

func (r *reportRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status, filePath string, fileSize int64) error {
	query := `UPDATE reports SET status = $2, file_path = $3, file_size = $4, completed_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, status, filePath, fileSize)
	return err
}

func (r *reportRepo) UpdateError(ctx context.Context, id uuid.UUID, errMsg string) error {
	query := `UPDATE reports SET status = 'failed', error_message = $2, completed_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, errMsg)
	return err
}

func (r *reportRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM reports WHERE id = $1`, id)
	return err
}

// Scheduled reports

func (r *reportRepo) CreateScheduled(ctx context.Context, sr *models.ScheduledReport) error {
	sr.ID = uuid.New()
	sr.IsActive = true
	sr.CreatedAt = time.Now()
	sr.UpdatedAt = sr.CreatedAt

	query := `INSERT INTO scheduled_reports (id, child_id, family_id, created_by, frequency, data_filters, recipients, is_active, next_run_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	_, err := r.db.ExecContext(ctx, query,
		sr.ID, sr.ChildID, sr.FamilyID, sr.CreatedBy,
		sr.Frequency, sr.DataFilters, sr.Recipients,
		sr.IsActive, sr.NextRunAt, sr.CreatedAt, sr.UpdatedAt)
	return err
}

func (r *reportRepo) GetScheduledByID(ctx context.Context, id uuid.UUID) (*models.ScheduledReport, error) {
	query := `SELECT id, child_id, family_id, created_by, frequency, data_filters, recipients,
		is_active, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_reports WHERE id = $1`

	var sr models.ScheduledReport
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&sr.ID, &sr.ChildID, &sr.FamilyID, &sr.CreatedBy,
		&sr.Frequency, &sr.DataFilters, &sr.Recipients,
		&sr.IsActive, &sr.LastRunAt, &sr.NextRunAt, &sr.CreatedAt, &sr.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sr, nil
}

func (r *reportRepo) GetScheduledByChildID(ctx context.Context, childID uuid.UUID) ([]models.ScheduledReport, error) {
	query := `SELECT id, child_id, family_id, created_by, frequency, data_filters, recipients,
		is_active, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_reports WHERE child_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.ScheduledReport
	for rows.Next() {
		var sr models.ScheduledReport
		if err := rows.Scan(
			&sr.ID, &sr.ChildID, &sr.FamilyID, &sr.CreatedBy,
			&sr.Frequency, &sr.DataFilters, &sr.Recipients,
			&sr.IsActive, &sr.LastRunAt, &sr.NextRunAt, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, sr)
	}
	return schedules, rows.Err()
}

func (r *reportRepo) GetDueScheduledReports(ctx context.Context) ([]models.ScheduledReport, error) {
	query := `SELECT id, child_id, family_id, created_by, frequency, data_filters, recipients,
		is_active, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_reports WHERE is_active = true AND next_run_at <= NOW()`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.ScheduledReport
	for rows.Next() {
		var sr models.ScheduledReport
		if err := rows.Scan(
			&sr.ID, &sr.ChildID, &sr.FamilyID, &sr.CreatedBy,
			&sr.Frequency, &sr.DataFilters, &sr.Recipients,
			&sr.IsActive, &sr.LastRunAt, &sr.NextRunAt, &sr.CreatedAt, &sr.UpdatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, sr)
	}
	return schedules, rows.Err()
}

func (r *reportRepo) UpdateScheduledLastRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error {
	query := `UPDATE scheduled_reports SET last_run_at = NOW(), next_run_at = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, nextRunAt)
	return err
}

func (r *reportRepo) DeactivateScheduled(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE scheduled_reports SET is_active = false, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *reportRepo) DeleteScheduled(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM scheduled_reports WHERE id = $1`, id)
	return err
}
