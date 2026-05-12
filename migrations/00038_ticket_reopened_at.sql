-- 00038_ticket_reopened_at.sql
--
-- Adds reopened_at to support_tickets so user-facing reopen actions are
-- distinguishable from the original create. Without this, a reopened
-- ticket looks the same as a new one in the admin queue (no signal that
-- it was previously resolved). With this column, admin views can flag
-- reopened tickets visually and (in a future slice) auto-escalate them
-- when they reopen.
--
-- Behavior (set by application code, not by this migration):
--   * status flips from 'resolved' or 'closed' back to 'open'
--   * resolved_at is cleared to NULL
--   * reopened_at is set to NOW()
--   * a user reply on a resolved/closed ticket also triggers the same
--     flip + stamp (implicit reopen via reply).
--
-- Idempotent.

BEGIN;

ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS reopened_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_support_tickets_reopened_at
    ON support_tickets (reopened_at DESC)
    WHERE reopened_at IS NOT NULL;

COMMENT ON COLUMN support_tickets.reopened_at IS
    'Set when a user reopens a resolved/closed ticket via the Reopen button or by replying. Resets to NULL is not used; once stamped the marker stays.';

COMMIT;
