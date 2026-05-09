-- revert-split-users.sql
--
-- Inverse of migrations/00032_split_users_table.sql.
-- Restores the unified `users` table and re-points every FK back at it.
--
-- IMPORTANT: sessions are TRUNCATEd by both the forward migration AND this
-- revert (sessions table itself is rebuilt, but rows do NOT come back —
-- they were already gone after the forward migration). Users will need to
-- log in again. Acceptable.
--
-- Run as a single transaction:
--   psql ... -1 -f scripts/revert-split-users.sql
--
-- (The runtime migration runner does NOT execute this file — it's an
-- operator-driven safety net.)

BEGIN;

-- =========================================================================
-- 1. Drop the backward-compat view first, then recreate users table
-- =========================================================================

DROP VIEW IF EXISTS users;

CREATE TABLE users (
    id                      UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email                   VARCHAR(255) NOT NULL,
    password_hash           VARCHAR(255) NOT NULL,
    first_name              VARCHAR(100) NOT NULL,
    last_name               VARCHAR(100) NOT NULL,
    phone                   VARCHAR(20),
    timezone                VARCHAR(50) DEFAULT 'America/Chicago',
    status                  user_status DEFAULT 'pending_verification',
    email_verified_at       TIMESTAMPTZ,
    last_login_at           TIMESTAMPTZ,
    created_at              TIMESTAMPTZ DEFAULT NOW(),
    updated_at              TIMESTAMPTZ DEFAULT NOW(),
    preferred_analysis_view analysis_view DEFAULT 'parent',
    time_format             VARCHAR(5) DEFAULT '12h',
    language                VARCHAR(10) DEFAULT 'en',
    system_role             system_role,
    UNIQUE (email)
);
CREATE INDEX idx_users_email ON users (email);
CREATE INDEX idx_users_language ON users (language);
CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_system_role ON users (system_role) WHERE system_role IS NOT NULL;

-- =========================================================================
-- 2. Repopulate users from the split tables
-- =========================================================================

INSERT INTO users (id, email, password_hash, first_name, last_name, status, last_login_at, email_verified_at, created_at, updated_at, system_role)
SELECT id, email, password_hash, first_name, last_name, status, last_login_at, email_verified_at, created_at, updated_at, system_role
FROM admin_users;

INSERT INTO users (id, email, password_hash, first_name, last_name, phone, timezone, status, last_login_at, email_verified_at, created_at, updated_at, preferred_analysis_view, time_format, language)
SELECT id, email, password_hash, first_name, last_name, phone, timezone, status, last_login_at, email_verified_at, created_at, updated_at, preferred_analysis_view, time_format, language
FROM app_users;

-- =========================================================================
-- 3. Reverse FK retargets — admin context columns back to users(id)
-- =========================================================================

ALTER TABLE admin_audit_log     DROP CONSTRAINT admin_audit_log_admin_id_fkey,
                                ADD CONSTRAINT  admin_audit_log_admin_id_fkey
                                    FOREIGN KEY (admin_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE bounty_awards       DROP CONSTRAINT bounty_awards_awarded_by_fkey,
                                ADD CONSTRAINT  bounty_awards_awarded_by_fkey
                                    FOREIGN KEY (awarded_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE brand_config        DROP CONSTRAINT brand_config_updated_by_fkey,
                                ADD CONSTRAINT  brand_config_updated_by_fkey
                                    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE dev_mode_settings   DROP CONSTRAINT dev_mode_settings_enabled_by_fkey,
                                ADD CONSTRAINT  dev_mode_settings_enabled_by_fkey
                                    FOREIGN KEY (enabled_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE error_logs          DROP CONSTRAINT error_logs_acknowledged_by_fkey,
                                ADD CONSTRAINT  error_logs_acknowledged_by_fkey
                                    FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE error_logs          DROP CONSTRAINT error_logs_deleted_by_fkey,
                                ADD CONSTRAINT  error_logs_deleted_by_fkey
                                    FOREIGN KEY (deleted_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE family_subscriptions DROP CONSTRAINT family_subscriptions_comped_by_fkey,
                                 ADD CONSTRAINT  family_subscriptions_comped_by_fkey
                                    FOREIGN KEY (comped_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE promo_codes         DROP CONSTRAINT promo_codes_created_by_fkey,
                                ADD CONSTRAINT  promo_codes_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE promo_codes         DROP CONSTRAINT promo_codes_deactivated_by_fkey,
                                ADD CONSTRAINT  promo_codes_deactivated_by_fkey
                                    FOREIGN KEY (deactivated_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE roadmap_items       DROP CONSTRAINT roadmap_items_created_by_fkey,
                                ADD CONSTRAINT  roadmap_items_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE system_settings     DROP CONSTRAINT system_settings_updated_by_fkey,
                                ADD CONSTRAINT  system_settings_updated_by_fkey
                                    FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE version_log         DROP CONSTRAINT version_log_created_by_fkey,
                                ADD CONSTRAINT  version_log_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE beta_invitations    DROP CONSTRAINT beta_invitations_invited_by_fkey,
                                ADD CONSTRAINT  beta_invitations_invited_by_fkey
                                    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL;

-- =========================================================================
-- 4. Reverse FK retargets — app context columns back to users(id)
-- =========================================================================

ALTER TABLE alert_exports       DROP CONSTRAINT alert_exports_exported_by_user_id_fkey,
                                ADD CONSTRAINT  alert_exports_exported_by_user_id_fkey
                                    FOREIGN KEY (exported_by_user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE alert_exports       DROP CONSTRAINT alert_exports_shared_with_user_id_fkey,
                                ADD CONSTRAINT  alert_exports_shared_with_user_id_fkey
                                    FOREIGN KEY (shared_with_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE alert_feedback      DROP CONSTRAINT alert_feedback_user_id_fkey,
                                ADD CONSTRAINT  alert_feedback_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE alerts              DROP CONSTRAINT alerts_acknowledged_by_fkey,
                                ADD CONSTRAINT  alerts_acknowledged_by_fkey
                                    FOREIGN KEY (acknowledged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE alerts              DROP CONSTRAINT alerts_resolved_by_fkey,
                                ADD CONSTRAINT  alerts_resolved_by_fkey
                                    FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE behavior_logs       DROP CONSTRAINT behavior_logs_logged_by_fkey,
                                ADD CONSTRAINT  behavior_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE bounty_awards       DROP CONSTRAINT bounty_awards_recipient_user_id_fkey,
                                ADD CONSTRAINT  bounty_awards_recipient_user_id_fkey
                                    FOREIGN KEY (recipient_user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE bowel_logs          DROP CONSTRAINT bowel_logs_logged_by_fkey,
                                ADD CONSTRAINT  bowel_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE chat_messages       DROP CONSTRAINT chat_messages_sender_id_fkey,
                                ADD CONSTRAINT  chat_messages_sender_id_fkey
                                    FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE chat_participants   DROP CONSTRAINT chat_participants_user_id_fkey,
                                ADD CONSTRAINT  chat_participants_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE chat_threads        DROP CONSTRAINT chat_threads_created_by_fkey,
                                ADD CONSTRAINT  chat_threads_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE clinical_validations DROP CONSTRAINT clinical_validations_provider_user_id_fkey,
                                 ADD CONSTRAINT  clinical_validations_provider_user_id_fkey
                                    FOREIGN KEY (provider_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE correlation_requests DROP CONSTRAINT correlation_requests_requested_by_fkey,
                                 ADD CONSTRAINT  correlation_requests_requested_by_fkey
                                    FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE device_tokens       DROP CONSTRAINT device_tokens_user_id_fkey,
                                ADD CONSTRAINT  device_tokens_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE diet_logs           DROP CONSTRAINT diet_logs_logged_by_fkey,
                                ADD CONSTRAINT  diet_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE error_logs          DROP CONSTRAINT error_logs_user_id_fkey,
                                ADD CONSTRAINT  error_logs_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE families            DROP CONSTRAINT families_created_by_fkey,
                                ADD CONSTRAINT  families_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE family_memberships  DROP CONSTRAINT family_memberships_invited_by_fkey,
                                ADD CONSTRAINT  family_memberships_invited_by_fkey
                                    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE family_memberships  DROP CONSTRAINT family_memberships_user_id_fkey,
                                ADD CONSTRAINT  family_memberships_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE health_event_logs   DROP CONSTRAINT health_event_logs_logged_by_fkey,
                                ADD CONSTRAINT  health_event_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE medication_logs     DROP CONSTRAINT medication_logs_logged_by_fkey,
                                ADD CONSTRAINT  medication_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE notification_preferences DROP CONSTRAINT notification_preferences_user_id_fkey,
                                     ADD CONSTRAINT  notification_preferences_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE password_reset_tokens DROP CONSTRAINT password_reset_tokens_user_id_fkey,
                                  ADD CONSTRAINT  password_reset_tokens_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE payments            DROP CONSTRAINT payments_user_id_fkey,
                                ADD CONSTRAINT  payments_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE promo_code_usages   DROP CONSTRAINT promo_code_usages_user_id_fkey,
                                ADD CONSTRAINT  promo_code_usages_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE reports             DROP CONSTRAINT reports_created_by_fkey,
                                ADD CONSTRAINT  reports_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE roadmap_item_followers DROP CONSTRAINT roadmap_item_followers_user_id_fkey,
                                   ADD CONSTRAINT  roadmap_item_followers_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE roadmap_items       DROP CONSTRAINT roadmap_items_requester_user_id_fkey,
                                ADD CONSTRAINT  roadmap_items_requester_user_id_fkey
                                    FOREIGN KEY (requester_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE scheduled_reports   DROP CONSTRAINT scheduled_reports_created_by_fkey,
                                ADD CONSTRAINT  scheduled_reports_created_by_fkey
                                    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE seizure_logs        DROP CONSTRAINT seizure_logs_logged_by_fkey,
                                ADD CONSTRAINT  seizure_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE sensory_logs        DROP CONSTRAINT sensory_logs_logged_by_fkey,
                                ADD CONSTRAINT  sensory_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE sleep_logs          DROP CONSTRAINT sleep_logs_logged_by_fkey,
                                ADD CONSTRAINT  sleep_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE social_logs         DROP CONSTRAINT social_logs_logged_by_fkey,
                                ADD CONSTRAINT  social_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE speech_logs         DROP CONSTRAINT speech_logs_logged_by_fkey,
                                ADD CONSTRAINT  speech_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE therapy_logs        DROP CONSTRAINT therapy_logs_logged_by_fkey,
                                ADD CONSTRAINT  therapy_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE treatment_change_responses DROP CONSTRAINT treatment_change_responses_provider_user_id_fkey,
                                       ADD CONSTRAINT  treatment_change_responses_provider_user_id_fkey
                                    FOREIGN KEY (provider_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE treatment_change_responses DROP CONSTRAINT treatment_change_responses_responded_by_user_id_fkey,
                                       ADD CONSTRAINT  treatment_change_responses_responded_by_user_id_fkey
                                    FOREIGN KEY (responded_by_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE treatment_changes   DROP CONSTRAINT treatment_changes_changed_by_user_id_fkey,
                                ADD CONSTRAINT  treatment_changes_changed_by_user_id_fkey
                                    FOREIGN KEY (changed_by_user_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE user_interaction_preferences DROP CONSTRAINT user_interaction_preferences_user_id_fkey,
                                         ADD CONSTRAINT  user_interaction_preferences_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE user_subscriptions  DROP CONSTRAINT user_subscriptions_user_id_fkey,
                                ADD CONSTRAINT  user_subscriptions_user_id_fkey
                                    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
ALTER TABLE weight_logs         DROP CONSTRAINT weight_logs_logged_by_fkey,
                                ADD CONSTRAINT  weight_logs_logged_by_fkey
                                    FOREIGN KEY (logged_by) REFERENCES users(id) ON DELETE SET NULL;

-- =========================================================================
-- 5. Recombine audit_log split → user_id
-- =========================================================================

ALTER TABLE audit_log ADD COLUMN user_id UUID;
UPDATE audit_log SET user_id = COALESCE(admin_id, app_user_id);
ALTER TABLE audit_log DROP COLUMN admin_id;
ALTER TABLE audit_log DROP COLUMN app_user_id;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL;

-- =========================================================================
-- 6. Recombine sessions split → user_id (TRUNCATEd; rows do not return)
-- =========================================================================

TRUNCATE TABLE sessions;
ALTER TABLE sessions ADD COLUMN user_id UUID;
ALTER TABLE sessions DROP COLUMN admin_id;
ALTER TABLE sessions DROP COLUMN app_user_id;
ALTER TABLE sessions ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

-- =========================================================================
-- 7. Drop the new tables
-- =========================================================================

DROP TABLE admin_users;
DROP TABLE app_users;

-- =========================================================================
-- 8. Mark migration 00032 as not applied so the runtime runner doesn't skip
-- it next boot if we choose to reapply. (Operator-driven decision.)
-- =========================================================================

DELETE FROM schema_migrations WHERE version = '00032_split_users_table';

-- =========================================================================
-- 9. Sanity check
-- =========================================================================

DO $$
DECLARE
    user_count INT := (SELECT count(*) FROM users);
BEGIN
    RAISE NOTICE 'revert-split-users complete: users=% (admins+parents reunified)', user_count;
END $$;

COMMIT;
