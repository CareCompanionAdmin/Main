-- Migration: 00018_ticket_type.sql
-- Description: Add `type` column to support_tickets so users can classify
-- their submission (bug report, feature request, billing, general).

DO $$ BEGIN
    CREATE TYPE ticket_type AS ENUM ('bug_report', 'feature_request', 'billing', 'general');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE support_tickets
    ADD COLUMN IF NOT EXISTS type ticket_type NOT NULL DEFAULT 'general';

CREATE INDEX IF NOT EXISTS idx_support_tickets_type ON support_tickets(type);

COMMENT ON COLUMN support_tickets.type IS 'User-selected ticket category. feature_request items can be promoted to roadmap_items.';
