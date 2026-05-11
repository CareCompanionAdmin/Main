package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AccountDeletionRequest mirrors the account_deletion_requests row.
// One per user-initiated deletion; tracked through the full 30-day
// lifecycle plus the cold-backup retention window.
type AccountDeletionRequest struct {
	ID                         uuid.UUID  `json:"id"`
	UserID                     uuid.UUID  `json:"user_id"`
	UserEmailAtRequest         string     `json:"user_email_at_request"`
	UserFirstNameAtRequest     NullString `json:"user_first_name_at_request,omitempty"`
	UserLastNameAtRequest      NullString `json:"user_last_name_at_request,omitempty"`
	ConfirmationCodeHash       string     `json:"-"`
	ConfirmationCodeExpiresAt  time.Time  `json:"confirmation_code_expires_at"`
	ConfirmationCodeUsedAt     NullTime   `json:"confirmation_code_used_at,omitempty"`
	ConfirmationAttempts       int        `json:"confirmation_attempts"`
	RestoreTokenHash           NullString `json:"-"`
	RestoreTokenExpiresAt      NullTime   `json:"restore_token_expires_at,omitempty"`
	RestoreTokenUsedAt         NullTime   `json:"restore_token_used_at,omitempty"`
	Status                     string          `json:"status"`
	PrimaryOfFamilies          json.RawMessage `json:"primary_of_families"`
	SoftDeletedAt              NullTime   `json:"soft_deleted_at,omitempty"`
	ScheduledHardDeleteAt      NullTime   `json:"scheduled_hard_delete_at,omitempty"`
	HardDeletedAt              NullTime   `json:"hard_deleted_at,omitempty"`
	RestoredAt                 NullTime   `json:"restored_at,omitempty"`
	RestoredBy                 NullString `json:"restored_by,omitempty"`
	ColdBackupS3Key            NullString `json:"-"`
	ColdBackupCreatedAt        NullTime   `json:"cold_backup_created_at,omitempty"`
	ColdBackupExpiresAt        NullTime   `json:"cold_backup_expires_at,omitempty"`
	ColdPurgedAt               NullTime   `json:"cold_purged_at,omitempty"`
	IPAtRequest                NullString `json:"-"`
	UserAgentAtRequest         NullString `json:"-"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

// Status constants for AccountDeletionRequest.Status. Strings (not enums)
// so we can evolve the workflow without ALTER TYPE migrations.
const (
	DeletionStatusPendingCode = "pending_code"
	DeletionStatusConfirmed   = "confirmed"
	DeletionStatusRestored    = "restored"
	DeletionStatusHardDeleted = "hard_deleted"
	DeletionStatusColdPurged  = "cold_purged"
	DeletionStatusCancelled   = "cancelled"
)

// SubscriptionCancellation is the per-cancellation history row that
// drives the 12-month repeat-cancel refund forfeit rule + audit trail.
type SubscriptionCancellation struct {
	ID                      uuid.UUID  `json:"id"`
	UserID                  uuid.UUID  `json:"user_id"`
	FamilyID                NullString `json:"family_id,omitempty"`
	FamilySubscriptionID    NullString `json:"family_subscription_id,omitempty"`
	StripeSubscriptionID    NullString `json:"stripe_subscription_id,omitempty"`
	StripeCustomerID        NullString `json:"stripe_customer_id,omitempty"`
	CancelledAt             time.Time  `json:"cancelled_at"`
	RefundAmountCents       int        `json:"refund_amount_cents"`
	RefundForfeited         bool       `json:"refund_forfeited"`
	RefundForfeitReason     NullString `json:"refund_forfeit_reason,omitempty"`
	StripeRefundID          NullString `json:"stripe_refund_id,omitempty"`
	PeriodStartAtCancel     NullTime   `json:"period_start_at_cancel,omitempty"`
	PeriodEndAtCancel       NullTime   `json:"period_end_at_cancel,omitempty"`
	DaysUnused              int        `json:"days_unused"`
	PeriodAmountCents       int        `json:"period_amount_cents"`
	AdminFeeCents           int        `json:"admin_fee_cents"`
	Notes                   NullString `json:"notes,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

// DataExportJob represents one opt-in export bundle generation. Created
// at deletion-confirmation time with status='awaiting_consent' (consent
// email sent); moves to 'queued' when the user submits format choices,
// then 'processing' → 'completed' or 'failed' by the worker.
type DataExportJob struct {
	ID                     uuid.UUID  `json:"id"`
	DeletionRequestID      uuid.UUID  `json:"deletion_request_id"`
	ConsentTokenHash       string     `json:"-"`
	ConsentTokenExpiresAt  time.Time  `json:"consent_token_expires_at"`
	ConsentTokenUsedAt     NullTime   `json:"consent_token_used_at,omitempty"`
	IncludeCSV             bool       `json:"include_csv"`
	IncludeXLSX            bool       `json:"include_xlsx"`
	IncludeSQLite          bool       `json:"include_sqlite"`
	Status                 string     `json:"status"`
	S3Key                  NullString `json:"-"`
	DownloadURLExpiresAt   NullTime   `json:"download_url_expires_at,omitempty"`
	ConsentSubmittedAt     NullTime   `json:"consent_submitted_at,omitempty"`
	ProcessedAt            NullTime   `json:"processed_at,omitempty"`
	FailedAt               NullTime   `json:"failed_at,omitempty"`
	FailureReason          NullString `json:"failure_reason,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// Status constants for DataExportJob.Status.
const (
	ExportStatusAwaitingConsent = "awaiting_consent"
	ExportStatusQueued          = "queued"
	ExportStatusProcessing      = "processing"
	ExportStatusCompleted       = "completed"
	ExportStatusExpired         = "expired"
	ExportStatusFailed          = "failed"
)

// FamilyRoleAtDeletion captures, for a single family that the user is in,
// whether they're the primary parent (the family creator) or a non-primary
// member, plus the role label. Used to drive role-aware disclaimer text
// in the deletion confirmation UI and to plan the cascade at hard-delete.
type FamilyRoleAtDeletion struct {
	FamilyID   uuid.UUID `json:"family_id"`
	FamilyName string    `json:"family_name"`
	IsPrimary  bool      `json:"is_primary"`
	Role       string    `json:"role"`
}
