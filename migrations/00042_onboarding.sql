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
-- them onboarding-complete AND treat the dashboard "finish setting up"
-- checklist as already dismissed. Without dismissing the checklist, existing
-- users (who never went through onboarding) would suddenly see a
-- "Finish setting up" card prompting them to add a child / invite a care team
-- / set basic settings — confusing for someone who's used the app for months.
-- Only users created AFTER this migration (NULL completed_at) are routed
-- through onboarding and shown the checklist.
UPDATE app_users
   SET onboarding_completed_at          = NOW(),
       onboarding_checklist_dismissed_at = NOW()
 WHERE onboarding_completed_at IS NULL;
