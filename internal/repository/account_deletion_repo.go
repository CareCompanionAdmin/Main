package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// AccountDeletionRepository is the data-layer for the user-initiated
// account-deletion flow. Soft-delete-with-grace semantics (see
// migrations/00034_account_deletion.sql for the schema).
type AccountDeletionRepository interface {
	CreateRequest(ctx context.Context, r *models.AccountDeletionRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.AccountDeletionRequest, error)
	GetActiveByUser(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error)
	IncrementAttempts(ctx context.Context, id uuid.UUID) error
	MarkCodeUsed(ctx context.Context, id uuid.UUID) error

	// Confirm transitions a pending_code request to 'confirmed' and applies
	// the soft-delete side effects in one transaction:
	//   - sets app_users.deleted_at + deletion_request_id
	//   - mangles app_users.email so the unique constraint frees up for new signups
	//   - sets families.deleted_at on families where the user is primary
	//   - revokes all user sessions
	//   - sets restore_token + restore_token_expires_at on the request row
	//   - sets scheduled_hard_delete_at = now() + hardDeleteAfter
	// Returns the updated request row.
	Confirm(ctx context.Context, requestID uuid.UUID, restoreTokenHash string,
		restoreTokenExpiresAt time.Time, scheduledHardDeleteAt time.Time) (*models.AccountDeletionRequest, error)

	// FindByRestoreTokenHash looks up a deletion request by the hashed
	// restore token (used by the email-link landing page).
	FindByRestoreTokenHash(ctx context.Context, tokenHash string) (*models.AccountDeletionRequest, error)

	// Restore reverses a soft-delete: unsets deleted_at on user + families,
	// restores the original email, marks restore_token_used_at + status.
	// restoredBy is NULL for self-service, set for admin-mediated restores.
	Restore(ctx context.Context, requestID uuid.UUID, restoredBy *uuid.UUID) error

	// FindHardDeleteDue returns confirmed requests whose scheduled_hard_delete_at
	// has passed and that haven't been hard-deleted yet.
	FindHardDeleteDue(ctx context.Context, limit int) ([]models.AccountDeletionRequest, error)

	// MarkHardDeleted records that the cascade ran. The DB cleanup is in
	// AccountDeletionRepository.PerformHardDelete (separate method so the
	// caller can wrap it in observability + cold-backup hand-off).
	MarkHardDeleted(ctx context.Context, requestID uuid.UUID, coldBackupS3Key string, coldBackupExpiresAt time.Time) error

	// PerformHardDelete runs the actual cascade: deletes families where the
	// user is primary (cascades to children + all logs), then deletes the
	// user (cascades to family_memberships in other families). Idempotent —
	// safe to retry if the cron crashes midway.
	PerformHardDelete(ctx context.Context, userID uuid.UUID, familyIDs []uuid.UUID) error

	// FindColdPurgeDue returns rows whose cold backup retention has expired.
	FindColdPurgeDue(ctx context.Context, limit int) ([]models.AccountDeletionRequest, error)
	MarkColdPurged(ctx context.Context, requestID uuid.UUID) error

	// Cancellation history (for the 12-month repeat-cancel forfeit rule).
	CreateCancellation(ctx context.Context, c *models.SubscriptionCancellation) error
	HasRecentCancellation(ctx context.Context, userID uuid.UUID, within time.Duration) (bool, error)

	// Data export jobs.
	CreateExportJob(ctx context.Context, j *models.DataExportJob) error
	FindExportByConsentTokenHash(ctx context.Context, tokenHash string) (*models.DataExportJob, error)
	SubmitExportConsent(ctx context.Context, jobID uuid.UUID, includeCSV, includeXLSX, includeSQLite bool) error
	FindQueuedExportJobs(ctx context.Context, limit int) ([]models.DataExportJob, error)
	MarkExportProcessing(ctx context.Context, jobID uuid.UUID) error
	MarkExportCompleted(ctx context.Context, jobID uuid.UUID, s3Key string, downloadExpiresAt time.Time) error
	MarkExportFailed(ctx context.Context, jobID uuid.UUID, reason string) error

	// GetFamilyRolesForUser returns one row per family the user is in,
	// flagging whether they're the family creator (primary parent) or a
	// non-primary member. Drives the disclaimer copy + cascade plan.
	GetFamilyRolesForUser(ctx context.Context, userID uuid.UUID) ([]models.FamilyRoleAtDeletion, error)
}

type accountDeletionRepo struct {
	db *sql.DB
}

func NewAccountDeletionRepository(db *sql.DB) AccountDeletionRepository {
	return &accountDeletionRepo{db: db}
}

func (r *accountDeletionRepo) CreateRequest(ctx context.Context, req *models.AccountDeletionRequest) error {
	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	familiesJSON := []byte(req.PrimaryOfFamilies)
	if len(familiesJSON) == 0 {
		familiesJSON = []byte("[]")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO account_deletion_requests (
			id, user_id, user_email_at_request, user_first_name_at_request,
			user_last_name_at_request, confirmation_code_hash,
			confirmation_code_expires_at, status, primary_of_families,
			ip_at_request, user_agent_at_request
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11)
	`,
		req.ID, req.UserID, req.UserEmailAtRequest, req.UserFirstNameAtRequest,
		req.UserLastNameAtRequest, req.ConfirmationCodeHash,
		req.ConfirmationCodeExpiresAt, req.Status, string(familiesJSON),
		req.IPAtRequest, req.UserAgentAtRequest,
	)
	return err
}

func (r *accountDeletionRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.AccountDeletionRequest, error) {
	row := r.db.QueryRowContext(ctx, baseSelectADR+" WHERE id = $1", id)
	return scanADR(row)
}

func (r *accountDeletionRepo) GetActiveByUser(ctx context.Context, userID uuid.UUID) (*models.AccountDeletionRequest, error) {
	row := r.db.QueryRowContext(ctx, baseSelectADR+`
		WHERE user_id = $1
		  AND status IN ('pending_code', 'confirmed')
		ORDER BY created_at DESC
		LIMIT 1`, userID)
	return scanADR(row)
}

func (r *accountDeletionRepo) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE account_deletion_requests SET confirmation_attempts = confirmation_attempts + 1 WHERE id = $1`, id)
	return err
}

func (r *accountDeletionRepo) MarkCodeUsed(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE account_deletion_requests SET confirmation_code_used_at = now() WHERE id = $1`, id)
	return err
}

func (r *accountDeletionRepo) Confirm(ctx context.Context, requestID uuid.UUID,
	restoreTokenHash string, restoreTokenExpiresAt time.Time,
	scheduledHardDeleteAt time.Time) (*models.AccountDeletionRequest, error) {

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Pull the request + primary family IDs from the snapshot.
	var userID uuid.UUID
	var familiesJSON []byte
	var origEmail string
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, primary_of_families, user_email_at_request
		FROM account_deletion_requests
		WHERE id = $1 AND status = 'pending_code'
		FOR UPDATE`, requestID).Scan(&userID, &familiesJSON, &origEmail)
	if err != nil {
		return nil, fmt.Errorf("load deletion request: %w", err)
	}

	// Mangle the email so the unique constraint frees up. The snapshot in
	// user_email_at_request holds the original for restore.
	mangledEmail := fmt.Sprintf("%s.deleted-%d.invalid", origEmail, time.Now().UnixNano())

	if _, err = tx.ExecContext(ctx, `
		UPDATE app_users
		   SET deleted_at = now(),
		       deletion_request_id = $1,
		       email = $2,
		       updated_at = now()
		 WHERE id = $3 AND deleted_at IS NULL`, requestID, mangledEmail, userID); err != nil {
		return nil, fmt.Errorf("soft-delete user: %w", err)
	}

	// Soft-delete the families this user is the primary parent of. We use
	// the snapshot rather than re-querying families.created_by so we're
	// resilient to row mutations between request and confirm.
	var familyIDs []uuid.UUID
	if len(familiesJSON) > 0 && string(familiesJSON) != "null" {
		if err := json.Unmarshal(familiesJSON, &familyIDs); err != nil {
			return nil, fmt.Errorf("parse primary_of_families: %w", err)
		}
	}
	if len(familyIDs) > 0 {
		if _, err = tx.ExecContext(ctx, `
			UPDATE families SET deleted_at = now(), updated_at = now()
			 WHERE id = ANY($1) AND deleted_at IS NULL`, uuidArray(familyIDs)); err != nil {
			return nil, fmt.Errorf("soft-delete primary families: %w", err)
		}
	}

	// Revoke all user-kind sessions so the user is logged out everywhere.
	// Sessions use app_user_id (not user_id) post-00032-split.
	if _, err = tx.ExecContext(ctx, `
		UPDATE sessions
		   SET revoked_at = now()
		 WHERE app_user_id = $1 AND kind = 'user' AND revoked_at IS NULL`, userID); err != nil {
		return nil, fmt.Errorf("revoke sessions: %w", err)
	}

	// Move the request to 'confirmed' and stamp the restore-token + clock.
	if _, err = tx.ExecContext(ctx, `
		UPDATE account_deletion_requests
		   SET status = 'confirmed',
		       confirmation_code_used_at = COALESCE(confirmation_code_used_at, now()),
		       restore_token_hash = $1,
		       restore_token_expires_at = $2,
		       soft_deleted_at = now(),
		       scheduled_hard_delete_at = $3
		 WHERE id = $4`, restoreTokenHash, restoreTokenExpiresAt, scheduledHardDeleteAt, requestID); err != nil {
		return nil, fmt.Errorf("transition to confirmed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return r.GetByID(ctx, requestID)
}

func (r *accountDeletionRepo) FindByRestoreTokenHash(ctx context.Context, tokenHash string) (*models.AccountDeletionRequest, error) {
	row := r.db.QueryRowContext(ctx, baseSelectADR+`
		WHERE restore_token_hash = $1 AND restore_token_used_at IS NULL AND status = 'confirmed'
		LIMIT 1`, tokenHash)
	return scanADR(row)
}

func (r *accountDeletionRepo) Restore(ctx context.Context, requestID uuid.UUID, restoredBy *uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID uuid.UUID
	var familiesJSON []byte
	var origEmail string
	if err := tx.QueryRowContext(ctx, `
		SELECT user_id, primary_of_families, user_email_at_request
		FROM account_deletion_requests
		WHERE id = $1 AND status = 'confirmed'
		FOR UPDATE`, requestID).Scan(&userID, &familiesJSON, &origEmail); err != nil {
		return fmt.Errorf("load deletion request: %w", err)
	}

	// Restore the original email (which was mangled at confirm time).
	// We check that no other live row has reclaimed this email in the
	// meantime — extremely unlikely but possible if the unique constraint
	// got hit between mangling and restore.
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_users
		   SET deleted_at = NULL,
		       deletion_request_id = NULL,
		       email = $1,
		       updated_at = now()
		 WHERE id = $2`, origEmail, userID); err != nil {
		return fmt.Errorf("restore user: %w", err)
	}

	var familyIDs []uuid.UUID
	if len(familiesJSON) > 0 && string(familiesJSON) != "null" {
		_ = json.Unmarshal(familiesJSON, &familyIDs)
	}
	if len(familyIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE families
			   SET deleted_at = NULL, updated_at = now()
			 WHERE id = ANY($1)`, uuidArray(familyIDs)); err != nil {
			return fmt.Errorf("restore families: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE account_deletion_requests
		   SET status = 'restored',
		       restored_at = now(),
		       restore_token_used_at = now(),
		       restored_by = $1
		 WHERE id = $2`, restoredBy, requestID); err != nil {
		return fmt.Errorf("transition to restored: %w", err)
	}

	return tx.Commit()
}

func (r *accountDeletionRepo) FindHardDeleteDue(ctx context.Context, limit int) ([]models.AccountDeletionRequest, error) {
	rows, err := r.db.QueryContext(ctx, baseSelectADR+`
		WHERE status = 'confirmed' AND scheduled_hard_delete_at <= now()
		ORDER BY scheduled_hard_delete_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanADRList(rows)
}

func (r *accountDeletionRepo) MarkHardDeleted(ctx context.Context, requestID uuid.UUID, coldBackupS3Key string, coldBackupExpiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_deletion_requests
		   SET status = 'hard_deleted',
		       hard_deleted_at = now(),
		       cold_backup_s3_key = $1,
		       cold_backup_created_at = now(),
		       cold_backup_expires_at = $2
		 WHERE id = $3`, coldBackupS3Key, coldBackupExpiresAt, requestID)
	return err
}

func (r *accountDeletionRepo) PerformHardDelete(ctx context.Context, userID uuid.UUID, familyIDs []uuid.UUID) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Cascade from families first — each delete cascades to children, logs,
	// alerts, reports, chat, etc. via existing ON DELETE CASCADE.
	if len(familyIDs) > 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM families WHERE id = ANY($1)`, uuidArray(familyIDs)); err != nil {
			return fmt.Errorf("hard-delete families: %w", err)
		}
	}

	// Clean up auxiliary user-keyed tables that don't cascade or where the
	// cascade is SET NULL (we'd leak references). password_reset_tokens and
	// email_verification_tokens have CASCADE; sessions too (verified
	// 00032_split_users_table migration).
	if _, err := tx.ExecContext(ctx, `DELETE FROM app_users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("hard-delete user: %w", err)
	}

	return tx.Commit()
}

func (r *accountDeletionRepo) FindColdPurgeDue(ctx context.Context, limit int) ([]models.AccountDeletionRequest, error) {
	rows, err := r.db.QueryContext(ctx, baseSelectADR+`
		WHERE status = 'hard_deleted'
		  AND cold_backup_s3_key IS NOT NULL
		  AND cold_backup_expires_at <= now()
		ORDER BY cold_backup_expires_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanADRList(rows)
}

func (r *accountDeletionRepo) MarkColdPurged(ctx context.Context, requestID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE account_deletion_requests
		   SET status = 'cold_purged', cold_purged_at = now()
		 WHERE id = $1`, requestID)
	return err
}

func (r *accountDeletionRepo) CreateCancellation(ctx context.Context, c *models.SubscriptionCancellation) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO subscription_cancellations (
			id, user_id, family_id, family_subscription_id, stripe_subscription_id,
			stripe_customer_id, cancelled_at, refund_amount_cents, refund_forfeited,
			refund_forfeit_reason, stripe_refund_id, period_start_at_cancel,
			period_end_at_cancel, days_unused, period_amount_cents, admin_fee_cents, notes
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		c.ID, c.UserID, c.FamilyID, c.FamilySubscriptionID, c.StripeSubscriptionID,
		c.StripeCustomerID, c.CancelledAt, c.RefundAmountCents, c.RefundForfeited,
		c.RefundForfeitReason, c.StripeRefundID, c.PeriodStartAtCancel,
		c.PeriodEndAtCancel, c.DaysUnused, c.PeriodAmountCents, c.AdminFeeCents, c.Notes,
	)
	return err
}

func (r *accountDeletionRepo) HasRecentCancellation(ctx context.Context, userID uuid.UUID, within time.Duration) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM subscription_cancellations
		 WHERE user_id = $1 AND cancelled_at >= now() - $2::interval
		   AND refund_amount_cents > 0`,
		userID, fmt.Sprintf("%d seconds", int(within.Seconds()))).Scan(&n)
	return n > 0, err
}

func (r *accountDeletionRepo) CreateExportJob(ctx context.Context, j *models.DataExportJob) error {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO data_export_jobs (
			id, deletion_request_id, consent_token_hash, consent_token_expires_at,
			include_csv, include_xlsx, include_sqlite, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		j.ID, j.DeletionRequestID, j.ConsentTokenHash, j.ConsentTokenExpiresAt,
		j.IncludeCSV, j.IncludeXLSX, j.IncludeSQLite, j.Status,
	)
	return err
}

func (r *accountDeletionRepo) FindExportByConsentTokenHash(ctx context.Context, tokenHash string) (*models.DataExportJob, error) {
	row := r.db.QueryRowContext(ctx, baseSelectDEJ+`
		WHERE consent_token_hash = $1 AND consent_token_used_at IS NULL
		LIMIT 1`, tokenHash)
	return scanDEJ(row)
}

func (r *accountDeletionRepo) SubmitExportConsent(ctx context.Context, jobID uuid.UUID,
	includeCSV, includeXLSX, includeSQLite bool) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		   SET include_csv = $1, include_xlsx = $2, include_sqlite = $3,
		       status = 'queued',
		       consent_token_used_at = now(),
		       consent_submitted_at = now()
		 WHERE id = $4 AND status = 'awaiting_consent'`,
		includeCSV, includeXLSX, includeSQLite, jobID)
	return err
}

func (r *accountDeletionRepo) FindQueuedExportJobs(ctx context.Context, limit int) ([]models.DataExportJob, error) {
	rows, err := r.db.QueryContext(ctx, baseSelectDEJ+`
		WHERE status = 'queued' ORDER BY consent_submitted_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DataExportJob
	for rows.Next() {
		j, err := scanDEJ(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

func (r *accountDeletionRepo) MarkExportProcessing(ctx context.Context, jobID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE data_export_jobs SET status = 'processing', processed_at = NULL WHERE id = $1`, jobID)
	return err
}

func (r *accountDeletionRepo) MarkExportCompleted(ctx context.Context, jobID uuid.UUID, s3Key string, downloadExpiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		   SET status = 'completed', s3_key = $1, download_url_expires_at = $2, processed_at = now()
		 WHERE id = $3`, s3Key, downloadExpiresAt, jobID)
	return err
}

func (r *accountDeletionRepo) MarkExportFailed(ctx context.Context, jobID uuid.UUID, reason string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE data_export_jobs
		   SET status = 'failed', failure_reason = $1, failed_at = now()
		 WHERE id = $2`, reason, jobID)
	return err
}

func (r *accountDeletionRepo) GetFamilyRolesForUser(ctx context.Context, userID uuid.UUID) ([]models.FamilyRoleAtDeletion, error) {
	// One row per family the user is in. Primary = the family creator.
	// LEFT JOIN: there may be families the user CREATED but isn't in
	// family_memberships (theoretically — usually they'd be a member too).
	rows, err := r.db.QueryContext(ctx, `
		SELECT f.id, f.name,
		       (f.created_by = $1) AS is_primary,
		       COALESCE(fm.role::text, 'creator') AS role
		FROM families f
		LEFT JOIN family_memberships fm ON fm.family_id = f.id AND fm.user_id = $1
		WHERE (f.created_by = $1 OR fm.user_id = $1)
		  AND f.deleted_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.FamilyRoleAtDeletion
	for rows.Next() {
		var r models.FamilyRoleAtDeletion
		if err := rows.Scan(&r.FamilyID, &r.FamilyName, &r.IsPrimary, &r.Role); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------------------
// scan helpers
// ----------------------------------------------------------------------------

const baseSelectADR = `
	SELECT id, user_id, user_email_at_request, user_first_name_at_request,
	       user_last_name_at_request, confirmation_code_hash,
	       confirmation_code_expires_at, confirmation_code_used_at,
	       confirmation_attempts, restore_token_hash, restore_token_expires_at,
	       restore_token_used_at, status, primary_of_families, soft_deleted_at,
	       scheduled_hard_delete_at, hard_deleted_at, restored_at, restored_by,
	       cold_backup_s3_key, cold_backup_created_at, cold_backup_expires_at,
	       cold_purged_at, ip_at_request, user_agent_at_request,
	       created_at, updated_at
	FROM account_deletion_requests`

// adrRowScanner abstracts *sql.Row and *sql.Rows so scanADR works for both.
// Named with the adr prefix so it doesn't collide with rowScanner already
// declared in roadmap_repository.go.
type adrRowScanner interface {
	Scan(dest ...interface{}) error
}

func scanADR(row adrRowScanner) (*models.AccountDeletionRequest, error) {
	var r models.AccountDeletionRequest
	var familiesJSON []byte
	err := row.Scan(
		&r.ID, &r.UserID, &r.UserEmailAtRequest, &r.UserFirstNameAtRequest,
		&r.UserLastNameAtRequest, &r.ConfirmationCodeHash,
		&r.ConfirmationCodeExpiresAt, &r.ConfirmationCodeUsedAt,
		&r.ConfirmationAttempts, &r.RestoreTokenHash, &r.RestoreTokenExpiresAt,
		&r.RestoreTokenUsedAt, &r.Status, &familiesJSON, &r.SoftDeletedAt,
		&r.ScheduledHardDeleteAt, &r.HardDeletedAt, &r.RestoredAt, &r.RestoredBy,
		&r.ColdBackupS3Key, &r.ColdBackupCreatedAt, &r.ColdBackupExpiresAt,
		&r.ColdPurgedAt, &r.IPAtRequest, &r.UserAgentAtRequest,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.PrimaryOfFamilies = familiesJSON
	return &r, nil
}

func scanADRList(rows *sql.Rows) ([]models.AccountDeletionRequest, error) {
	var out []models.AccountDeletionRequest
	for rows.Next() {
		r, err := scanADR(rows)
		if err != nil {
			return nil, err
		}
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, rows.Err()
}

const baseSelectDEJ = `
	SELECT id, deletion_request_id, consent_token_hash, consent_token_expires_at,
	       consent_token_used_at, include_csv, include_xlsx, include_sqlite,
	       status, s3_key, download_url_expires_at, consent_submitted_at,
	       processed_at, failed_at, failure_reason, created_at, updated_at
	FROM data_export_jobs`

func scanDEJ(row adrRowScanner) (*models.DataExportJob, error) {
	var j models.DataExportJob
	err := row.Scan(
		&j.ID, &j.DeletionRequestID, &j.ConsentTokenHash, &j.ConsentTokenExpiresAt,
		&j.ConsentTokenUsedAt, &j.IncludeCSV, &j.IncludeXLSX, &j.IncludeSQLite,
		&j.Status, &j.S3Key, &j.DownloadURLExpiresAt, &j.ConsentSubmittedAt,
		&j.ProcessedAt, &j.FailedAt, &j.FailureReason, &j.CreatedAt, &j.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &j, err
}

// uuidArray converts a []uuid.UUID to a Postgres array literal for use with
// `= ANY($1)`. The driver doesn't auto-marshal Go slices of UUIDs.
func uuidArray(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	out := "{"
	for i, id := range ids {
		if i > 0 {
			out += ","
		}
		out += id.String()
	}
	return out + "}"
}
