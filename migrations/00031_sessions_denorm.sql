-- Migration: 00031_sessions_denorm.sql
-- Adds denormalized user/family/env columns to sessions so the Live Sessions
-- admin page can render rows from a cross-env DB pool (where JOINs to the
-- local users/families tables aren't possible). Populated at login time
-- going forward; existing rows are backfilled via JOIN here.
--
-- Rollback (run by hand if needed):
--   ALTER TABLE sessions
--       DROP COLUMN env_name,
--       DROP COLUMN family_name,
--       DROP COLUMN user_last_name,
--       DROP COLUMN user_first_name,
--       DROP COLUMN user_email;

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS user_email      TEXT,
    ADD COLUMN IF NOT EXISTS user_first_name TEXT,
    ADD COLUMN IF NOT EXISTS user_last_name  TEXT,
    ADD COLUMN IF NOT EXISTS family_name     TEXT,
    ADD COLUMN IF NOT EXISTS env_name        TEXT;

COMMENT ON COLUMN sessions.user_email      IS 'Snapshot of users.email at login. Drifts if user changes email.';
COMMENT ON COLUMN sessions.user_first_name IS 'Snapshot of users.first_name at login.';
COMMENT ON COLUMN sessions.user_last_name  IS 'Snapshot of users.last_name at login.';
COMMENT ON COLUMN sessions.family_name     IS 'Snapshot of families.name at login (null for kind=admin).';
COMMENT ON COLUMN sessions.env_name        IS 'Environment that issued the session: development | production. Set from APP_ENV.';

-- Backfill existing rows.
UPDATE sessions s
SET user_email      = u.email,
    user_first_name = u.first_name,
    user_last_name  = u.last_name
FROM users u
WHERE s.user_id = u.id
  AND s.user_email IS NULL;

UPDATE sessions s
SET family_name = f.name
FROM families f
WHERE s.family_id = f.id
  AND s.family_name IS NULL;

UPDATE sessions
SET env_name = 'development'
WHERE env_name IS NULL;
