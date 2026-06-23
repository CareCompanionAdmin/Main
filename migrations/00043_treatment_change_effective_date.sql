-- 00043_treatment_change_effective_date.sql
-- Adds a user-controlled effective date to treatment changes so the calendar
-- pill and med-change history reflect the day the change actually took effect
-- in the user's local timezone, not the UTC server timestamp.
-- Fixes #112402 (wrong day) and underpins #112369 (editable date).

ALTER TABLE treatment_changes ADD COLUMN IF NOT EXISTS effective_date DATE;

-- Backfill historical rows from created_at in a sensible default zone.
-- (Per-row owner tz is not reliably joinable for all legacy rows; America/Chicago
-- is the app's default-ish US zone and is close enough for historical pills.)
UPDATE treatment_changes
SET effective_date = (created_at AT TIME ZONE 'America/Chicago')::date
WHERE effective_date IS NULL;

ALTER TABLE treatment_changes ALTER COLUMN effective_date SET DEFAULT CURRENT_DATE;
ALTER TABLE treatment_changes ALTER COLUMN effective_date SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_treatment_changes_effective_date
  ON treatment_changes(child_id, effective_date);

-- ROLLBACK:
-- DROP INDEX IF EXISTS idx_treatment_changes_effective_date;
-- ALTER TABLE treatment_changes DROP COLUMN IF EXISTS effective_date;
