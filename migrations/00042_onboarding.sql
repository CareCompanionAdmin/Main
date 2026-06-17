-- 00042_onboarding.sql
-- Adds new-user onboarding tracking columns to app_users.
-- Rollback (manual):
--   ALTER TABLE app_users
--     DROP COLUMN IF EXISTS onboarding_completed_at,
--     DROP COLUMN IF EXISTS onboarding_checklist_dismissed_at,
--     DROP COLUMN IF EXISTS onboarding_settings_done_at,
--     DROP COLUMN IF EXISTS onboarding_invite_done_at;

ALTER TABLE app_users
    ADD COLUMN IF NOT EXISTS onboarding_completed_at          TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_checklist_dismissed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_settings_done_at       TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS onboarding_invite_done_at         TIMESTAMPTZ;

-- Backfill: every user that already exists has been using the app, so mark
-- them onboarding-complete. Only users created AFTER this migration (NULL)
-- will be routed through onboarding.
UPDATE app_users
   SET onboarding_completed_at = NOW()
 WHERE onboarding_completed_at IS NULL;
