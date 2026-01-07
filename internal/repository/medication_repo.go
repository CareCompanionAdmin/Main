package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"carecompanion/internal/models"
)

type medicationRepo struct {
	db *sql.DB
}

func NewMedicationRepo(db *sql.DB) MedicationRepository {
	return &medicationRepo{db: db}
}

func (r *medicationRepo) Create(ctx context.Context, med *models.Medication) error {
	query := `
		INSERT INTO medications (id, child_id, reference_id, name, dosage, dosage_unit, frequency, instructions, prescriber, pharmacy, start_date, end_date, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`
	med.ID = uuid.New()
	med.IsActive = true
	med.CreatedAt = time.Now()
	med.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		med.ID,
		med.ChildID,
		med.ReferenceID,
		med.Name,
		med.Dosage,
		med.DosageUnit,
		med.Frequency,
		med.Instructions,
		med.Prescriber,
		med.Pharmacy,
		med.StartDate,
		med.EndDate,
		med.IsActive,
		med.CreatedAt,
		med.UpdatedAt,
	)
	return err
}

func (r *medicationRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Medication, error) {
	query := `
		SELECT id, child_id, reference_id, name, dosage, dosage_unit, frequency, instructions, prescriber, pharmacy, start_date, end_date, is_active, created_at, updated_at
		FROM medications
		WHERE id = $1
	`
	med := &models.Medication{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&med.ID, &med.ChildID, &med.ReferenceID, &med.Name, &med.Dosage, &med.DosageUnit,
		&med.Frequency, &med.Instructions, &med.Prescriber, &med.Pharmacy,
		&med.StartDate, &med.EndDate, &med.IsActive, &med.CreatedAt, &med.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load schedules
	schedules, err := r.GetSchedules(ctx, med.ID)
	if err != nil {
		return nil, err
	}
	med.Schedules = schedules

	return med, nil
}

func (r *medicationRepo) GetByChildID(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.Medication, error) {
	query := `
		SELECT id, child_id, reference_id, name, dosage, dosage_unit, frequency, instructions, prescriber, pharmacy, start_date, end_date, is_active, created_at, updated_at
		FROM medications
		WHERE child_id = $1
	`
	if activeOnly {
		query += ` AND is_active = true`
	}
	query += ` ORDER BY name ASC`

	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var medications []models.Medication
	for rows.Next() {
		var med models.Medication
		err := rows.Scan(
			&med.ID, &med.ChildID, &med.ReferenceID, &med.Name, &med.Dosage, &med.DosageUnit,
			&med.Frequency, &med.Instructions, &med.Prescriber, &med.Pharmacy,
			&med.StartDate, &med.EndDate, &med.IsActive, &med.CreatedAt, &med.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		medications = append(medications, med)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load schedules for each medication
	for i := range medications {
		schedules, err := r.GetSchedules(ctx, medications[i].ID)
		if err != nil {
			return nil, err
		}
		medications[i].Schedules = schedules
	}

	return medications, nil
}

func (r *medicationRepo) Update(ctx context.Context, med *models.Medication) error {
	query := `
		UPDATE medications
		SET name = $2, dosage = $3, dosage_unit = $4, frequency = $5, instructions = $6, prescriber = $7, pharmacy = $8, start_date = $9, end_date = $10, is_active = $11, updated_at = $12
		WHERE id = $1
	`
	med.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		med.ID, med.Name, med.Dosage, med.DosageUnit, med.Frequency,
		med.Instructions, med.Prescriber, med.Pharmacy,
		med.StartDate, med.EndDate, med.IsActive, med.UpdatedAt,
	)
	return err
}

func (r *medicationRepo) Delete(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE medications SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

func (r *medicationRepo) CreateSchedule(ctx context.Context, schedule *models.MedicationSchedule) error {
	query := `
		INSERT INTO medication_schedules (id, medication_id, time_of_day, scheduled_time, days_of_week, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	schedule.ID = uuid.New()
	schedule.IsActive = true
	schedule.CreatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		schedule.ID,
		schedule.MedicationID,
		schedule.TimeOfDay,
		schedule.ScheduledTime,
		pq.Array(schedule.DaysOfWeek),
		schedule.IsActive,
		schedule.CreatedAt,
	)
	return err
}

func (r *medicationRepo) GetSchedules(ctx context.Context, medicationID uuid.UUID) ([]models.MedicationSchedule, error) {
	query := `
		SELECT id, medication_id, time_of_day, scheduled_time::text, days_of_week, is_active, created_at
		FROM medication_schedules
		WHERE medication_id = $1 AND is_active = true
		ORDER BY time_of_day ASC
	`
	rows, err := r.db.QueryContext(ctx, query, medicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []models.MedicationSchedule
	for rows.Next() {
		var s models.MedicationSchedule
		var daysOfWeek []int64 // pq.Array requires int64
		err := rows.Scan(
			&s.ID, &s.MedicationID, &s.TimeOfDay, &s.ScheduledTime,
			pq.Array(&daysOfWeek), &s.IsActive, &s.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		// Convert []int64 to []int
		s.DaysOfWeek = make([]int, len(daysOfWeek))
		for i, d := range daysOfWeek {
			s.DaysOfWeek[i] = int(d)
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

func (r *medicationRepo) UpdateSchedule(ctx context.Context, schedule *models.MedicationSchedule) error {
	query := `
		UPDATE medication_schedules
		SET time_of_day = $2, scheduled_time = $3, days_of_week = $4, is_active = $5
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query,
		schedule.ID, schedule.TimeOfDay, schedule.ScheduledTime,
		pq.Array(schedule.DaysOfWeek), schedule.IsActive,
	)
	return err
}

func (r *medicationRepo) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE medication_schedules SET is_active = false WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *medicationRepo) CreateLog(ctx context.Context, log *models.MedicationLog) error {
	query := `
		INSERT INTO medication_logs (id, medication_id, child_id, schedule_id, log_date, scheduled_time, actual_time, status, dosage_given, notes, logged_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`
	log.ID = uuid.New()
	log.CreatedAt = time.Now()
	log.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.MedicationID, log.ChildID, log.ScheduleID, log.LogDate,
		log.ScheduledTime, log.ActualTime, log.Status, log.DosageGiven,
		log.Notes, log.LoggedBy, log.CreatedAt, log.UpdatedAt,
	)
	return err
}

func (r *medicationRepo) GetLogByID(ctx context.Context, id uuid.UUID) (*models.MedicationLog, error) {
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE id = $1
	`
	log := &models.MedicationLog{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&log.ID, &log.MedicationID, &log.ChildID, &log.ScheduleID, &log.LogDate,
		&log.ScheduledTime, &log.ActualTime, &log.Status, &log.DosageGiven,
		&log.Notes, &log.LoggedBy, &log.CreatedAt, &log.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return log, nil
}

func (r *medicationRepo) GetLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error) {
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE child_id = $1 AND log_date BETWEEN $2 AND $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID, startDate, endDate)
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

func (r *medicationRepo) GetLogsByMedication(ctx context.Context, medicationID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error) {
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE medication_id = $1 AND log_date BETWEEN $2 AND $3
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, medicationID, startDate, endDate)
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

func (r *medicationRepo) GetLogsByMedicationSince(ctx context.Context, medicationID uuid.UUID, since time.Time) ([]models.MedicationLog, error) {
	query := `
		SELECT id, medication_id, child_id, schedule_id, log_date, scheduled_time::text, actual_time::text, status, dosage_given, notes, logged_by, created_at, updated_at
		FROM medication_logs
		WHERE medication_id = $1 AND log_date >= $2
		ORDER BY log_date DESC, created_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, medicationID, since)
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

func (r *medicationRepo) UpdateLog(ctx context.Context, log *models.MedicationLog) error {
	query := `
		UPDATE medication_logs
		SET status = $2, actual_time = $3, dosage_given = $4, notes = $5, updated_at = $6
		WHERE id = $1
	`
	log.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		log.ID, log.Status, log.ActualTime, log.DosageGiven, log.Notes, log.UpdatedAt,
	)
	return err
}

func (r *medicationRepo) DeleteLog(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM medication_logs WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *medicationRepo) GetDueMedications(ctx context.Context, childID uuid.UUID, date time.Time) ([]models.MedicationDue, error) {
	dayOfWeek := int(date.Weekday())

	query := `
		SELECT m.id, m.child_id, m.reference_id, m.name, m.dosage, m.dosage_unit, m.frequency, m.instructions, m.prescriber, m.pharmacy, m.start_date, m.end_date, m.is_active, m.created_at, m.updated_at,
		       ms.id, ms.medication_id, ms.time_of_day, ms.scheduled_time::text, ms.days_of_week, ms.is_active, ms.created_at,
		       ml.id IS NOT NULL as is_logged,
		       COALESCE(ml.status::text, '') as logged_status
		FROM medications m
		JOIN medication_schedules ms ON ms.medication_id = m.id AND ms.is_active = true
		LEFT JOIN medication_logs ml ON ml.medication_id = m.id AND ml.schedule_id = ms.id AND ml.log_date = $2
		WHERE m.child_id = $1 AND m.is_active = true
		  AND (ms.days_of_week IS NULL OR ms.days_of_week = '{}' OR $3 = ANY(ms.days_of_week))
		ORDER BY ms.time_of_day ASC, m.name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, childID, date, dayOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dueMeds []models.MedicationDue
	for rows.Next() {
		var due models.MedicationDue
		var loggedStatusStr string
		var daysOfWeek []int64 // pq.Array requires int64, not int
		err := rows.Scan(
			&due.Medication.ID, &due.Medication.ChildID, &due.Medication.ReferenceID,
			&due.Medication.Name, &due.Medication.Dosage, &due.Medication.DosageUnit,
			&due.Medication.Frequency, &due.Medication.Instructions, &due.Medication.Prescriber,
			&due.Medication.Pharmacy, &due.Medication.StartDate, &due.Medication.EndDate,
			&due.Medication.IsActive, &due.Medication.CreatedAt, &due.Medication.UpdatedAt,
			&due.Schedule.ID, &due.Schedule.MedicationID, &due.Schedule.TimeOfDay,
			&due.Schedule.ScheduledTime, pq.Array(&daysOfWeek),
			&due.Schedule.IsActive, &due.Schedule.CreatedAt,
			&due.IsLogged, &loggedStatusStr,
		)
		if err != nil {
			return nil, err
		}
		// Convert []int64 to []int for the model
		due.Schedule.DaysOfWeek = make([]int, len(daysOfWeek))
		for i, d := range daysOfWeek {
			due.Schedule.DaysOfWeek[i] = int(d)
		}
		if loggedStatusStr != "" {
			due.LoggedStatus = models.LogStatus(loggedStatusStr)
		}
		dueMeds = append(dueMeds, due)
	}
	return dueMeds, rows.Err()
}

func (r *medicationRepo) GetMedicationReference(ctx context.Context, name string) (*models.MedicationReference, error) {
	query := `
		SELECT id, name, generic_name, drug_class, common_dosages, common_side_effects, warnings, interactions, created_at
		FROM medication_reference
		WHERE LOWER(name) = LOWER($1) OR LOWER(generic_name) = LOWER($1)
		LIMIT 1
	`
	ref := &models.MedicationReference{}
	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&ref.ID, &ref.Name, &ref.GenericName, &ref.DrugClass,
		&ref.CommonDosages, &ref.CommonSideEffects, &ref.Warnings,
		&ref.Interactions, &ref.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ref, nil
}

func (r *medicationRepo) SearchMedicationReferences(ctx context.Context, searchQuery string) ([]models.MedicationReference, error) {
	query := `
		SELECT id, name, generic_name, drug_class, common_dosages, common_side_effects, warnings, interactions, created_at
		FROM medication_reference
		WHERE LOWER(name) LIKE LOWER($1) OR LOWER(generic_name) LIKE LOWER($1)
		ORDER BY name ASC
		LIMIT 20
	`
	rows, err := r.db.QueryContext(ctx, query, "%"+searchQuery+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []models.MedicationReference
	for rows.Next() {
		var ref models.MedicationReference
		err := rows.Scan(
			&ref.ID, &ref.Name, &ref.GenericName, &ref.DrugClass,
			&ref.CommonDosages, &ref.CommonSideEffects, &ref.Warnings,
			&ref.Interactions, &ref.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (r *medicationRepo) HasMedicationLogs(ctx context.Context, medicationID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM medication_logs WHERE medication_id = $1)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, medicationID).Scan(&exists)
	return exists, err
}

func (r *medicationRepo) HardDeleteMedication(ctx context.Context, id uuid.UUID) error {
	// Delete in order: logs, schedules, then medication
	// Start a transaction for atomicity
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete any logs (should be none if we got here, but be safe)
	_, err = tx.ExecContext(ctx, `DELETE FROM medication_logs WHERE medication_id = $1`, id)
	if err != nil {
		return err
	}

	// Delete schedules
	_, err = tx.ExecContext(ctx, `DELETE FROM medication_schedules WHERE medication_id = $1`, id)
	if err != nil {
		return err
	}

	// Delete the medication itself
	_, err = tx.ExecContext(ctx, `DELETE FROM medications WHERE id = $1`, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}
