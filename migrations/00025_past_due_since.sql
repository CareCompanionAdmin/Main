-- 00025_past_due_since.sql
--
-- Adds a dedicated `past_due_since` clock to family_subscriptions so the
-- 14-day read-only window after expiry has a stable start point.
--
-- Why we can't reuse `updated_at`: the table has a BEFORE-UPDATE trigger
-- that resets `updated_at = NOW()` on every write, so any unrelated row
-- update (a comp tweak, the next sweep tick, a webhook sync) would push
-- the termination clock back to zero. `past_due_since` is set exactly
-- once when status flips into past_due, and cleared when status leaves it.

ALTER TABLE family_subscriptions
    ADD COLUMN IF NOT EXISTS past_due_since TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_family_subscriptions_past_due_since
    ON family_subscriptions (past_due_since)
    WHERE status = 'past_due' AND past_due_since IS NOT NULL;

-- Backfill: any row currently in past_due gets its updated_at as a stand-in
-- for past_due_since (best guess). New past_due transitions set it correctly.
UPDATE family_subscriptions
SET past_due_since = updated_at
WHERE status = 'past_due' AND past_due_since IS NULL;
