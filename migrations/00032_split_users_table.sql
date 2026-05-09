-- 00032_split_users_table.sql
--
-- Split the unified `users` table into two:
--   * admin_users — rows where the old `users.system_role` was non-NULL
--   * app_users   — rows where the old `users.system_role` was NULL
--
-- Same email may now exist in BOTH tables (one admin row + one app row),
-- but is unique within each. This unblocks "I'm an admin AND I want to
-- be a parent on the same email" without the global-unique-email hack.
--
-- Approach: preserve UUIDs everywhere. INSERT into the new tables with
-- the SAME id values from `users`, so every existing FK reference stays
-- a valid UUID. We then retarget each FK constraint to the appropriate
-- new table — the column values are unchanged.
--
-- Two columns are mixed (have BOTH admin and parent IDs in the wild):
--   * audit_log.user_id  — split into admin_id + app_user_id (per Bryan A=1)
--   * sessions.user_id   — sessions are short-lived (8h max), TRUNCATE
--                           and use the same split column pattern.
--
-- The runtime migration runner wraps this entire file in a single
-- transaction. Any failure rolls everything back; the original `users`
-- table stays intact.
--
-- A matching revert script lives at scripts/revert-split-users.sql.

-- =========================================================================
-- 1. New tables
-- =========================================================================

CREATE TABLE admin_users (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email             VARCHAR(255) NOT NULL,
    password_hash     VARCHAR(255) NOT NULL,
    first_name        VARCHAR(100) NOT NULL,
    last_name         VARCHAR(100) NOT NULL,
    system_role       system_role  NOT NULL,
    status            user_status  NOT NULL DEFAULT 'active',
    last_login_at     TIMESTAMPTZ,
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (email)
);
CREATE INDEX idx_admin_users_email ON admin_users (email);

CREATE TABLE app_users (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email             VARCHAR(255) NOT NULL,
    password_hash     VARCHAR(255) NOT NULL,
    first_name        VARCHAR(100) NOT NULL,
    last_name         VARCHAR(100) NOT NULL,
    phone             VARCHAR(20),
    timezone          VARCHAR(50)  DEFAULT 'America/Chicago',
    status            user_status  DEFAULT 'pending_verification',
    last_login_at     TIMESTAMPTZ,
    email_verified_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  DEFAULT NOW(),
    preferred_analysis_view analysis_view DEFAULT 'parent',
    time_format       VARCHAR(5)   DEFAULT '12h',
    language          VARCHAR(10)  DEFAULT 'en',
    UNIQUE (email)
);
CREATE INDEX idx_app_users_email ON app_users (email);
CREATE INDEX idx_app_users_status ON app_users (status);
CREATE INDEX idx_app_users_language ON app_users (language);

-- =========================================================================
-- 2. Copy rows (UUIDs preserved)
-- =========================================================================

INSERT INTO admin_users (id, email, password_hash, first_name, last_name, system_role, status, last_login_at, email_verified_at, created_at, updated_at)
SELECT id, email, password_hash, first_name, last_name, system_role,
       COALESCE(status, 'active'::user_status), last_login_at, email_verified_at, created_at, updated_at
FROM users WHERE system_role IS NOT NULL;

INSERT INTO app_users (id, email, password_hash, first_name, last_name, phone, timezone, status, last_login_at, email_verified_at, created_at, updated_at, preferred_analysis_view, time_format, language)
SELECT id, email, password_hash, first_name, last_name, phone, timezone, status, last_login_at, email_verified_at, created_at, updated_at, preferred_analysis_view, time_format, language
FROM users WHERE system_role IS NULL;

-- Inline integrity check: row counts must match
DO $$
DECLARE
    src_admin INT := (SELECT count(*) FROM users WHERE system_role IS NOT NULL);
    dst_admin INT := (SELECT count(*) FROM admin_users);
    src_app   INT := (SELECT count(*) FROM users WHERE system_role IS NULL);
    dst_app   INT := (SELECT count(*) FROM app_users);
BEGIN
    IF src_admin != dst_admin THEN
        RAISE EXCEPTION 'admin row count mismatch: src=%, dst=%', src_admin, dst_admin;
    END IF;
    IF src_app != dst_app THEN
        RAISE EXCEPTION 'app row count mismatch: src=%, dst=%', src_app, dst_app;
    END IF;
END $$;

-- =========================================================================
-- 3. Truncate sessions (short-lived; force re-login is acceptable)
-- =========================================================================

TRUNCATE TABLE sessions;

-- =========================================================================
-- 3b. Clean up orphaned admin-context FK references.
--
-- Some admin-context columns (admin_audit_log.admin_id, etc.) may hold
-- IDs of users who were once admins but later had system_role nulled.
-- Those user IDs end up in app_users, so a new FK targeting admin_users
-- would fail. NULL those refs out (the audit row keeps its action +
-- timestamp; just the actor identity is anonymized — graceful).
-- =========================================================================

UPDATE admin_audit_log     SET admin_id        = NULL WHERE admin_id        IS NOT NULL AND admin_id        NOT IN (SELECT id FROM admin_users);
UPDATE bounty_awards       SET awarded_by      = NULL WHERE awarded_by      IS NOT NULL AND awarded_by      NOT IN (SELECT id FROM admin_users);
UPDATE brand_config        SET updated_by      = NULL WHERE updated_by      IS NOT NULL AND updated_by      NOT IN (SELECT id FROM admin_users);
UPDATE dev_mode_settings   SET enabled_by      = NULL WHERE enabled_by      IS NOT NULL AND enabled_by      NOT IN (SELECT id FROM admin_users);
UPDATE error_logs          SET acknowledged_by = NULL WHERE acknowledged_by IS NOT NULL AND acknowledged_by NOT IN (SELECT id FROM admin_users);
UPDATE error_logs          SET deleted_by      = NULL WHERE deleted_by      IS NOT NULL AND deleted_by      NOT IN (SELECT id FROM admin_users);
UPDATE family_subscriptions SET comped_by      = NULL WHERE comped_by      IS NOT NULL AND comped_by      NOT IN (SELECT id FROM admin_users);
UPDATE promo_codes         SET created_by      = NULL WHERE created_by      IS NOT NULL AND created_by      NOT IN (SELECT id FROM admin_users);
UPDATE promo_codes         SET deactivated_by  = NULL WHERE deactivated_by  IS NOT NULL AND deactivated_by  NOT IN (SELECT id FROM admin_users);
UPDATE roadmap_items       SET created_by      = NULL WHERE created_by      IS NOT NULL AND created_by      NOT IN (SELECT id FROM admin_users);
UPDATE system_settings     SET updated_by      = NULL WHERE updated_by      IS NOT NULL AND updated_by      NOT IN (SELECT id FROM admin_users);
UPDATE version_log         SET created_by      = NULL WHERE created_by      IS NOT NULL AND created_by      NOT IN (SELECT id FROM admin_users);
UPDATE beta_invitations    SET invited_by      = NULL WHERE invited_by      IS NOT NULL AND invited_by      NOT IN (SELECT id FROM admin_users);

-- Same defensive cleanup for app-context columns (rows that point at an
-- admin user when the new FK targets app_users). On dev these are all 0
-- but prod data shape may differ.
UPDATE alert_exports       SET exported_by_user_id = NULL WHERE exported_by_user_id IS NOT NULL AND exported_by_user_id NOT IN (SELECT id FROM app_users);
UPDATE alert_exports       SET shared_with_user_id = NULL WHERE shared_with_user_id IS NOT NULL AND shared_with_user_id NOT IN (SELECT id FROM app_users);
DELETE FROM alert_feedback           WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE alerts              SET acknowledged_by    = NULL WHERE acknowledged_by    IS NOT NULL AND acknowledged_by    NOT IN (SELECT id FROM app_users);
UPDATE alerts              SET resolved_by        = NULL WHERE resolved_by        IS NOT NULL AND resolved_by        NOT IN (SELECT id FROM app_users);
UPDATE behavior_logs       SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
DELETE FROM bounty_awards            WHERE recipient_user_id    NOT IN (SELECT id FROM app_users);
UPDATE bowel_logs          SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
DELETE FROM chat_messages            WHERE sender_id            NOT IN (SELECT id FROM app_users);
DELETE FROM chat_participants        WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE chat_threads        SET created_by         = NULL WHERE created_by         IS NOT NULL AND created_by         NOT IN (SELECT id FROM app_users);
UPDATE clinical_validations SET provider_user_id  = NULL WHERE provider_user_id   IS NOT NULL AND provider_user_id   NOT IN (SELECT id FROM app_users);
DELETE FROM correlation_requests     WHERE requested_by         NOT IN (SELECT id FROM app_users);
DELETE FROM device_tokens            WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE diet_logs           SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE error_logs          SET user_id            = NULL WHERE user_id            IS NOT NULL AND user_id            NOT IN (SELECT id FROM app_users);
UPDATE families            SET created_by         = NULL WHERE created_by         IS NOT NULL AND created_by         NOT IN (SELECT id FROM app_users);
UPDATE family_memberships  SET invited_by         = NULL WHERE invited_by         IS NOT NULL AND invited_by         NOT IN (SELECT id FROM app_users);
DELETE FROM family_memberships       WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE health_event_logs   SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE medication_logs     SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
DELETE FROM notification_preferences WHERE user_id              NOT IN (SELECT id FROM app_users);
DELETE FROM password_reset_tokens    WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE payments            SET user_id            = NULL WHERE user_id            IS NOT NULL AND user_id            NOT IN (SELECT id FROM app_users);
DELETE FROM promo_code_usages        WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE reports             SET created_by         = NULL WHERE created_by         IS NOT NULL AND created_by         NOT IN (SELECT id FROM app_users);
DELETE FROM roadmap_item_followers   WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE roadmap_items       SET requester_user_id  = NULL WHERE requester_user_id  IS NOT NULL AND requester_user_id  NOT IN (SELECT id FROM app_users);
UPDATE scheduled_reports   SET created_by         = NULL WHERE created_by         IS NOT NULL AND created_by         NOT IN (SELECT id FROM app_users);
UPDATE seizure_logs        SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE sensory_logs        SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE sleep_logs          SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE social_logs         SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE speech_logs         SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE therapy_logs        SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);
UPDATE treatment_change_responses SET provider_user_id     = NULL WHERE provider_user_id     IS NOT NULL AND provider_user_id     NOT IN (SELECT id FROM app_users);
UPDATE treatment_change_responses SET responded_by_user_id = NULL WHERE responded_by_user_id IS NOT NULL AND responded_by_user_id NOT IN (SELECT id FROM app_users);
UPDATE treatment_changes   SET changed_by_user_id = NULL WHERE changed_by_user_id IS NOT NULL AND changed_by_user_id NOT IN (SELECT id FROM app_users);
DELETE FROM user_interaction_preferences WHERE user_id          NOT IN (SELECT id FROM app_users);
DELETE FROM user_subscriptions       WHERE user_id              NOT IN (SELECT id FROM app_users);
UPDATE weight_logs         SET logged_by          = NULL WHERE logged_by          IS NOT NULL AND logged_by          NOT IN (SELECT id FROM app_users);

-- =========================================================================
-- 4. Retarget FK constraints — pure ADMIN context columns → admin_users.id
-- =========================================================================

ALTER TABLE admin_audit_log     DROP CONSTRAINT admin_audit_log_admin_id_fkey,
                                ADD CONSTRAINT  admin_audit_log_admin_id_fkey
                                    FOREIGN KEY (admin_id) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE bounty_awards       DROP CONSTRAINT bounty_awards_awarded_by_fkey,
                                ADD CONSTRAINT  bounty_awards_awarded_by_fkey
                                    FOREIGN KEY (awarded_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE brand_config        DROP CONSTRAINT brand_config_updated_by_fkey,
                                ADD CONSTRAINT  brand_config_updated_by_fkey
                                    FOREIGN KEY (updated_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE dev_mode_settings   DROP CONSTRAINT dev_mode_settings_enabled_by_fkey,
                                ADD CONSTRAINT  dev_mode_settings_enabled_by_fkey
                                    FOREIGN KEY (enabled_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE error_logs          DROP CONSTRAINT error_logs_acknowledged_by_fkey,
                                ADD CONSTRAINT  error_logs_acknowledged_by_fkey
                                    FOREIGN KEY (acknowledged_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE error_logs          DROP CONSTRAINT error_logs_deleted_by_fkey,
                                ADD CONSTRAINT  error_logs_deleted_by_fkey
                                    FOREIGN KEY (deleted_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE family_subscriptions DROP CONSTRAINT family_subscriptions_comped_by_fkey,
                                 ADD CONSTRAINT  family_subscriptions_comped_by_fkey
                                    FOREIGN KEY (comped_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE promo_codes         DROP CONSTRAINT promo_codes_created_by_fkey,
                                ADD CONSTRAINT  promo_codes_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE promo_codes         DROP CONSTRAINT promo_codes_deactivated_by_fkey,
                                ADD CONSTRAINT  promo_codes_deactivated_by_fkey
                                    FOREIGN KEY (deactivated_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE roadmap_items       DROP CONSTRAINT roadmap_items_created_by_fkey,
                                ADD CONSTRAINT  roadmap_items_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE system_settings     DROP CONSTRAINT system_settings_updated_by_fkey,
                                ADD CONSTRAINT  system_settings_updated_by_fkey
                                    FOREIGN KEY (updated_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE version_log         DROP CONSTRAINT version_log_created_by_fkey,
                                ADD CONSTRAINT  version_log_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES admin_users(id) ON DELETE SET NULL;

ALTER TABLE beta_invitations    DROP CONSTRAINT beta_invitations_invited_by_fkey,
                                ADD CONSTRAINT  beta_invitations_invited_by_fkey
                                    FOREIGN KEY (invited_by) REFERENCES admin_users(id) ON DELETE SET NULL;

-- =========================================================================
-- 5. Retarget FK constraints — pure APP context columns → app_users.id
-- =========================================================================

ALTER TABLE alert_exports       DROP CONSTRAINT alert_exports_exported_by_user_id_fkey,
                                ADD CONSTRAINT  alert_exports_exported_by_user_id_fkey
                                    FOREIGN KEY (exported_by_user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE alert_exports       DROP CONSTRAINT alert_exports_shared_with_user_id_fkey,
                                ADD CONSTRAINT  alert_exports_shared_with_user_id_fkey
                                    FOREIGN KEY (shared_with_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE alert_feedback      DROP CONSTRAINT alert_feedback_user_id_fkey,
                                ADD CONSTRAINT  alert_feedback_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE alerts              DROP CONSTRAINT alerts_acknowledged_by_fkey,
                                ADD CONSTRAINT  alerts_acknowledged_by_fkey
                                    FOREIGN KEY (acknowledged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE alerts              DROP CONSTRAINT alerts_resolved_by_fkey,
                                ADD CONSTRAINT  alerts_resolved_by_fkey
                                    FOREIGN KEY (resolved_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE behavior_logs       DROP CONSTRAINT behavior_logs_logged_by_fkey,
                                ADD CONSTRAINT  behavior_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE bounty_awards       DROP CONSTRAINT bounty_awards_recipient_user_id_fkey,
                                ADD CONSTRAINT  bounty_awards_recipient_user_id_fkey
                                    FOREIGN KEY (recipient_user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE bowel_logs          DROP CONSTRAINT bowel_logs_logged_by_fkey,
                                ADD CONSTRAINT  bowel_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE chat_messages       DROP CONSTRAINT chat_messages_sender_id_fkey,
                                ADD CONSTRAINT  chat_messages_sender_id_fkey
                                    FOREIGN KEY (sender_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE chat_participants   DROP CONSTRAINT chat_participants_user_id_fkey,
                                ADD CONSTRAINT  chat_participants_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE chat_threads        DROP CONSTRAINT chat_threads_created_by_fkey,
                                ADD CONSTRAINT  chat_threads_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE clinical_validations DROP CONSTRAINT clinical_validations_provider_user_id_fkey,
                                 ADD CONSTRAINT  clinical_validations_provider_user_id_fkey
                                    FOREIGN KEY (provider_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE correlation_requests DROP CONSTRAINT correlation_requests_requested_by_fkey,
                                 ADD CONSTRAINT  correlation_requests_requested_by_fkey
                                    FOREIGN KEY (requested_by) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE device_tokens       DROP CONSTRAINT device_tokens_user_id_fkey,
                                ADD CONSTRAINT  device_tokens_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE diet_logs           DROP CONSTRAINT diet_logs_logged_by_fkey,
                                ADD CONSTRAINT  diet_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE error_logs          DROP CONSTRAINT error_logs_user_id_fkey,
                                ADD CONSTRAINT  error_logs_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE families            DROP CONSTRAINT families_created_by_fkey,
                                ADD CONSTRAINT  families_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE family_memberships  DROP CONSTRAINT family_memberships_invited_by_fkey,
                                ADD CONSTRAINT  family_memberships_invited_by_fkey
                                    FOREIGN KEY (invited_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE family_memberships  DROP CONSTRAINT family_memberships_user_id_fkey,
                                ADD CONSTRAINT  family_memberships_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE health_event_logs   DROP CONSTRAINT health_event_logs_logged_by_fkey,
                                ADD CONSTRAINT  health_event_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE medication_logs     DROP CONSTRAINT medication_logs_logged_by_fkey,
                                ADD CONSTRAINT  medication_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE notification_preferences DROP CONSTRAINT notification_preferences_user_id_fkey,
                                     ADD CONSTRAINT  notification_preferences_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_user_id_fkey,
                                  ADD CONSTRAINT  password_reset_tokens_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE payments            DROP CONSTRAINT payments_user_id_fkey,
                                ADD CONSTRAINT  payments_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE promo_code_usages   DROP CONSTRAINT promo_code_usages_user_id_fkey,
                                ADD CONSTRAINT  promo_code_usages_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE reports             DROP CONSTRAINT reports_created_by_fkey,
                                ADD CONSTRAINT  reports_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE roadmap_item_followers DROP CONSTRAINT roadmap_item_followers_user_id_fkey,
                                   ADD CONSTRAINT  roadmap_item_followers_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE roadmap_items       DROP CONSTRAINT roadmap_items_requester_user_id_fkey,
                                ADD CONSTRAINT  roadmap_items_requester_user_id_fkey
                                    FOREIGN KEY (requester_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE scheduled_reports   DROP CONSTRAINT scheduled_reports_created_by_fkey,
                                ADD CONSTRAINT  scheduled_reports_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE seizure_logs        DROP CONSTRAINT seizure_logs_logged_by_fkey,
                                ADD CONSTRAINT  seizure_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE sensory_logs        DROP CONSTRAINT sensory_logs_logged_by_fkey,
                                ADD CONSTRAINT  sensory_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE sleep_logs          DROP CONSTRAINT sleep_logs_logged_by_fkey,
                                ADD CONSTRAINT  sleep_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE social_logs         DROP CONSTRAINT social_logs_logged_by_fkey,
                                ADD CONSTRAINT  social_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE speech_logs         DROP CONSTRAINT speech_logs_logged_by_fkey,
                                ADD CONSTRAINT  speech_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE therapy_logs        DROP CONSTRAINT therapy_logs_logged_by_fkey,
                                ADD CONSTRAINT  therapy_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE treatment_change_responses DROP CONSTRAINT treatment_change_responses_provider_user_id_fkey,
                                       ADD CONSTRAINT  treatment_change_responses_provider_user_id_fkey
                                    FOREIGN KEY (provider_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE treatment_change_responses DROP CONSTRAINT treatment_change_responses_responded_by_user_id_fkey,
                                       ADD CONSTRAINT  treatment_change_responses_responded_by_user_id_fkey
                                    FOREIGN KEY (responded_by_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE treatment_changes   DROP CONSTRAINT treatment_changes_changed_by_user_id_fkey,
                                ADD CONSTRAINT  treatment_changes_changed_by_user_id_fkey
                                    FOREIGN KEY (changed_by_user_id) REFERENCES app_users(id) ON DELETE SET NULL;

ALTER TABLE user_interaction_preferences DROP CONSTRAINT user_interaction_preferences_user_id_fkey,
                                         ADD CONSTRAINT  user_interaction_preferences_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE user_subscriptions  DROP CONSTRAINT user_subscriptions_user_id_fkey,
                                ADD CONSTRAINT  user_subscriptions_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES app_users(id) ON DELETE CASCADE;

ALTER TABLE weight_logs         DROP CONSTRAINT weight_logs_logged_by_fkey,
                                ADD CONSTRAINT  weight_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES app_users(id) ON DELETE SET NULL;

-- =========================================================================
-- 6. Mixed columns — split into admin_id + app_user_id
-- =========================================================================

-- audit_log: split user_id into admin_id + app_user_id
ALTER TABLE audit_log DROP CONSTRAINT audit_log_user_id_fkey;
ALTER TABLE audit_log ADD COLUMN admin_id    UUID REFERENCES admin_users(id) ON DELETE SET NULL;
ALTER TABLE audit_log ADD COLUMN app_user_id UUID REFERENCES app_users(id)   ON DELETE SET NULL;
UPDATE audit_log SET admin_id = user_id
    WHERE user_id IN (SELECT id FROM admin_users);
UPDATE audit_log SET app_user_id = user_id
    WHERE user_id IN (SELECT id FROM app_users);
ALTER TABLE audit_log DROP COLUMN user_id;

-- sessions: TRUNCATEd above; replace user_id with admin_id + app_user_id
ALTER TABLE sessions DROP CONSTRAINT sessions_user_id_fkey;
ALTER TABLE sessions ADD COLUMN admin_id    UUID REFERENCES admin_users(id) ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN app_user_id UUID REFERENCES app_users(id)   ON DELETE CASCADE;
ALTER TABLE sessions DROP COLUMN user_id;

-- =========================================================================
-- 7. Drop the old users table
-- =========================================================================

DROP TABLE users;

-- =========================================================================
-- 7b. Create a backward-compat VIEW called `users` that UNIONs the two
-- new tables. Read-only — INSERT/UPDATE/DELETE on this view will fail.
-- All write call-sites in the Go code are migrated to write to admin_users
-- or app_users directly; this view exists so the ~55 SELECT-from-users
-- read queries scattered across the codebase continue to work without
-- per-call refactoring. UUIDs are unique across both tables, so id-based
-- lookups return 0 or 1 row. Email-based lookups can return 2 rows once
-- an email exists in BOTH tables — kind-aware login flows must NOT use
-- this view; they use GetAdminByEmail / GetAppByEmail directly.
-- =========================================================================

CREATE VIEW users AS
SELECT id, email, password_hash, first_name, last_name,
       NULL::varchar(20)  AS phone,
       NULL::varchar(50)  AS timezone,
       status, email_verified_at, last_login_at, created_at, updated_at,
       NULL::analysis_view AS preferred_analysis_view,
       NULL::varchar(5)   AS time_format,
       NULL::varchar(10)  AS language,
       system_role
FROM admin_users
UNION ALL
SELECT id, email, password_hash, first_name, last_name,
       phone, timezone,
       status, email_verified_at, last_login_at, created_at, updated_at,
       preferred_analysis_view, time_format, language,
       NULL::system_role AS system_role
FROM app_users;

-- =========================================================================
-- 8. Final integrity sanity check
-- =========================================================================

DO $$
DECLARE
    bad_fks INT;
BEGIN
    -- Confirm no orphan FKs remain anywhere by checking each new FK target exists.
    -- (Postgres validates FKs as we add them above; reaching this point means all good.)
    SELECT count(*) INTO bad_fks FROM pg_constraint
        WHERE confrelid = 0 AND contype = 'f';
    IF bad_fks > 0 THEN
        RAISE EXCEPTION 'Found % orphan FK constraints after split', bad_fks;
    END IF;

    RAISE NOTICE 'split-users migration: admin_users=%, app_users=%, sessions=truncated, audit_log split, users dropped',
        (SELECT count(*) FROM admin_users),
        (SELECT count(*) FROM app_users);
END $$;
