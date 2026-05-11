-- App Store Approval Initiative — Blocker 2 (5.1.1(v) in-app account deletion).
--
-- Soft-delete semantics with a 30-day grace window. The user enters a
-- one-time email code to confirm; 14 days of self-restore via emailed
-- link; up to 30 days of support-mediated restore; on day 30 the
-- production data is archived to cold storage (S3 Intelligent Tiering)
-- and removed from the live DB; after the configured cold retention
-- window the archive is purged.
--
-- See project_carecompanion_app_store_approval.md for the full rationale.
-- See feedback_carecompanion_ticket_resolution_protocol.md for QA approach.

-- ----------------------------------------------------------------------------
-- 1. Soft-delete columns on the tables that get cascaded by the deletion.
-- ----------------------------------------------------------------------------

ALTER TABLE app_users ADD COLUMN deleted_at        TIMESTAMPTZ;
ALTER TABLE app_users ADD COLUMN deletion_request_id UUID;
ALTER TABLE families  ADD COLUMN deleted_at        TIMESTAMPTZ;

CREATE INDEX idx_app_users_deleted_at ON app_users(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_families_deleted_at  ON families(deleted_at)  WHERE deleted_at IS NOT NULL;

-- ----------------------------------------------------------------------------
-- 2. account_deletion_requests — the per-user record that drives the flow.
-- ----------------------------------------------------------------------------

CREATE TABLE account_deletion_requests (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                         UUID NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,

    -- Snapshot of identity at request time. Survives the soft-delete (which
    -- scrubs the live row's PII) so support + audit can still see who this
    -- request belonged to.
    user_email_at_request           VARCHAR(255) NOT NULL,
    user_first_name_at_request      VARCHAR(100),
    user_last_name_at_request       VARCHAR(100),

    -- One-time confirmation code (the OTP the user enters after the email).
    -- Kept hashed at rest so a DB read doesn't reveal codes; bcrypt is fine
    -- because there are at most a handful of these per second.
    confirmation_code_hash          VARCHAR(255) NOT NULL,
    confirmation_code_expires_at    TIMESTAMPTZ NOT NULL,
    confirmation_code_used_at       TIMESTAMPTZ,
    confirmation_attempts           INTEGER NOT NULL DEFAULT 0,

    -- Self-restore token — random opaque 64-char string embedded in the
    -- "Undo deletion" email link. Stored hashed for the same reason as the
    -- OTP. Marked used_at on first click; second click is rejected.
    restore_token_hash              VARCHAR(255),
    restore_token_expires_at        TIMESTAMPTZ,
    restore_token_used_at           TIMESTAMPTZ,

    -- Workflow status.
    --   pending_code   = email sent, awaiting OTP entry
    --   confirmed      = OTP valid, soft-delete applied, 30-day clock running
    --   restored       = self-restore or support-restore happened
    --   hard_deleted   = past day 30, prod data wiped (cold backup may remain)
    --   cold_purged    = cold backup retention also expired; nothing recoverable
    --   cancelled      = user did not enter the code in time or asked to cancel
    --                    before soft-delete
    status                          VARCHAR(40) NOT NULL DEFAULT 'pending_code',

    -- Snapshot of which families this user is the primary parent of at
    -- request time — JSON array of family_ids. Used to drive the
    -- cascade-delete logic at hard-delete time and to drive the disclaimer
    -- copy shown to the user before they enter the OTP.
    primary_of_families             JSONB NOT NULL DEFAULT '[]'::jsonb,

    -- Timestamps along the lifecycle.
    soft_deleted_at                 TIMESTAMPTZ,
    scheduled_hard_delete_at        TIMESTAMPTZ,
    hard_deleted_at                 TIMESTAMPTZ,
    restored_at                     TIMESTAMPTZ,
    restored_by                     UUID,           -- admin user id, NULL for self-restore

    -- Cold storage tracking.
    cold_backup_s3_key              TEXT,
    cold_backup_created_at          TIMESTAMPTZ,
    cold_backup_expires_at          TIMESTAMPTZ,    -- driven by env COLD_BACKUP_RETENTION_DAYS
    cold_purged_at                  TIMESTAMPTZ,

    -- Audit context.
    ip_at_request                   INET,
    user_agent_at_request           TEXT,

    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Look up by user (most recent first).
CREATE INDEX idx_adr_user_id            ON account_deletion_requests(user_id, created_at DESC);
CREATE INDEX idx_adr_status             ON account_deletion_requests(status);
-- Hard-delete cron picks up confirmed rows whose scheduled time has passed.
CREATE INDEX idx_adr_hard_delete_due    ON account_deletion_requests(scheduled_hard_delete_at)
    WHERE status = 'confirmed';
-- Cold-purge cron picks up hard_deleted rows whose backup retention is up.
CREATE INDEX idx_adr_cold_purge_due     ON account_deletion_requests(cold_backup_expires_at)
    WHERE status = 'hard_deleted' AND cold_backup_s3_key IS NOT NULL;
-- Restore link lookups.
CREATE INDEX idx_adr_restore_token_hash ON account_deletion_requests(restore_token_hash)
    WHERE restore_token_hash IS NOT NULL;

-- Back-pointer from app_users.deletion_request_id (helpful for sanity checks
-- but kept loosely referenced — the row in account_deletion_requests is the
-- source of truth).
ALTER TABLE app_users ADD CONSTRAINT app_users_deletion_request_id_fkey
    FOREIGN KEY (deletion_request_id) REFERENCES account_deletion_requests(id) ON DELETE SET NULL;

-- ----------------------------------------------------------------------------
-- 3. subscription_cancellations — history table that drives the 12-month
-- repeat-cancel refund forfeit rule and provides an audit trail of refunds.
-- ----------------------------------------------------------------------------

CREATE TABLE subscription_cancellations (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                     UUID NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
    family_id                   UUID,                            -- not FK'd: family may have been deleted
    family_subscription_id      UUID,                            -- ditto for the subscription row
    stripe_subscription_id      VARCHAR(255),
    stripe_customer_id          VARCHAR(255),

    cancelled_at                TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Refund amount actually issued to the customer (after admin fees).
    -- Zero means "no refund issued" — distinct from "no refund requested" —
    -- e.g. forfeited under the 12-month rule.
    refund_amount_cents         INTEGER NOT NULL DEFAULT 0,
    refund_forfeited            BOOLEAN NOT NULL DEFAULT false,
    refund_forfeit_reason       TEXT,                            -- "12-month repeat cancellation", etc.
    stripe_refund_id            VARCHAR(255),

    -- Snapshot of the refund math for audit purposes.
    period_start_at_cancel      TIMESTAMPTZ,
    period_end_at_cancel        TIMESTAMPTZ,
    days_unused                 INTEGER,
    period_amount_cents         INTEGER,
    admin_fee_cents             INTEGER NOT NULL DEFAULT 0,

    notes                       TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_sc_user_cancelled ON subscription_cancellations(user_id, cancelled_at DESC);
CREATE INDEX idx_sc_family_id       ON subscription_cancellations(family_id);

-- ----------------------------------------------------------------------------
-- 4. data_export_jobs — the opt-in export flow we send via email after a
-- deletion is confirmed. Two-stage: consent page → format choice → queued
-- → daily worker → email download link.
-- ----------------------------------------------------------------------------

CREATE TABLE data_export_jobs (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    deletion_request_id         UUID NOT NULL REFERENCES account_deletion_requests(id) ON DELETE CASCADE,

    -- Token for the first email link (consent page). Random 64-char opaque,
    -- stored hashed. One-shot — marked used when the consent form is submitted.
    consent_token_hash          VARCHAR(255) NOT NULL,
    consent_token_expires_at    TIMESTAMPTZ NOT NULL,
    consent_token_used_at       TIMESTAMPTZ,

    -- Format choices made on the consent page. At least one must be true at
    -- enqueue time.
    include_csv                 BOOLEAN NOT NULL DEFAULT false,
    include_xlsx                BOOLEAN NOT NULL DEFAULT false,
    include_sqlite              BOOLEAN NOT NULL DEFAULT false,

    -- Workflow status.
    --   awaiting_consent   = email sent, user hasn't submitted format choice
    --   queued             = user submitted, waiting for worker pickup
    --   processing         = worker is generating the bundle
    --   completed          = bundle ready, download URL emailed
    --   expired            = download URL TTL passed
    --   failed             = generation failed (see failure_reason)
    status                      VARCHAR(40) NOT NULL DEFAULT 'awaiting_consent',

    -- Output.
    s3_key                      TEXT,
    download_url_expires_at     TIMESTAMPTZ,

    -- Lifecycle timestamps.
    consent_submitted_at        TIMESTAMPTZ,
    processed_at                TIMESTAMPTZ,
    failed_at                   TIMESTAMPTZ,
    failure_reason              TEXT,

    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_dej_deletion_request   ON data_export_jobs(deletion_request_id);
CREATE INDEX idx_dej_status             ON data_export_jobs(status);
CREATE INDEX idx_dej_consent_token_hash ON data_export_jobs(consent_token_hash)
    WHERE consent_token_used_at IS NULL;
-- Worker picks up queued rows.
CREATE INDEX idx_dej_queued_at ON data_export_jobs(consent_submitted_at)
    WHERE status = 'queued';

-- ----------------------------------------------------------------------------
-- 5. updated_at trigger reuse — these tables follow the same convention as
-- the rest of the schema; the trigger function already exists.
-- ----------------------------------------------------------------------------

CREATE TRIGGER update_account_deletion_requests_updated_at
    BEFORE UPDATE ON account_deletion_requests
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_data_export_jobs_updated_at
    BEFORE UPDATE ON data_export_jobs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ----------------------------------------------------------------------------
-- Rollback (for the migration runner's commented rollback section, not
-- automatically applied; see reference_migration_runner_quirks.md):
--
--   DROP TABLE data_export_jobs;
--   DROP TABLE subscription_cancellations;
--   ALTER TABLE app_users DROP CONSTRAINT app_users_deletion_request_id_fkey;
--   DROP TABLE account_deletion_requests;
--   ALTER TABLE families  DROP COLUMN deleted_at;
--   ALTER TABLE app_users DROP COLUMN deletion_request_id;
--   ALTER TABLE app_users DROP COLUMN deleted_at;
-- ----------------------------------------------------------------------------
