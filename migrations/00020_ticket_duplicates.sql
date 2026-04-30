-- Migration: 00020_ticket_duplicates.sql
-- Description: Support for marking duplicate tickets and tracking multiple
-- requesters/followers per roadmap item, so popular requests get appropriate
-- prioritization and every interested user gets notified on release.

-- ----------------------------------------------------------------------------
-- 1. Tickets can point to a canonical ticket OR a roadmap item (mutually
--    exclusive — enforced by CHECK constraint). Both are nullable.
-- ----------------------------------------------------------------------------

ALTER TABLE support_tickets
    ADD COLUMN IF NOT EXISTS duplicate_of_ticket_id  UUID REFERENCES support_tickets(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS duplicate_of_roadmap_id UUID REFERENCES roadmap_items(id)   ON DELETE SET NULL;

DO $$ BEGIN
    ALTER TABLE support_tickets
        ADD CONSTRAINT support_tickets_dup_target_xor
        CHECK (duplicate_of_ticket_id IS NULL OR duplicate_of_roadmap_id IS NULL);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- A ticket cannot be a duplicate of itself.
DO $$ BEGIN
    ALTER TABLE support_tickets
        ADD CONSTRAINT support_tickets_dup_not_self
        CHECK (duplicate_of_ticket_id IS NULL OR duplicate_of_ticket_id <> id);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_support_tickets_dup_of_ticket
    ON support_tickets(duplicate_of_ticket_id)
    WHERE duplicate_of_ticket_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_support_tickets_dup_of_roadmap
    ON support_tickets(duplicate_of_roadmap_id)
    WHERE duplicate_of_roadmap_id IS NOT NULL;

-- ----------------------------------------------------------------------------
-- 2. Roadmap items get a many-to-many followers join. The original promoter
--    is the first follower; duplicates added later become additional ones.
--    All followers with the relevant notify_on_* flag get pinged on release.
-- ----------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS roadmap_item_followers (
    roadmap_id        UUID NOT NULL REFERENCES roadmap_items(id)   ON DELETE CASCADE,
    user_id           UUID NOT NULL REFERENCES users(id)           ON DELETE CASCADE,
    -- The ticket through which this user expressed interest. Used so
    -- release notifications post on their own ticket thread (which by then
    -- is closed but visible in /support history).
    source_ticket_id  UUID          REFERENCES support_tickets(id) ON DELETE SET NULL,
    notify_on_dev     BOOLEAN NOT NULL DEFAULT TRUE,
    notify_on_prod    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (roadmap_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_roadmap_followers_user    ON roadmap_item_followers(user_id);
CREATE INDEX IF NOT EXISTS idx_roadmap_followers_ticket  ON roadmap_item_followers(source_ticket_id);

-- ----------------------------------------------------------------------------
-- 3. Backfill: every existing roadmap item that has a known requester gets
--    a follower row so prior promotions still notify on release.
-- ----------------------------------------------------------------------------

INSERT INTO roadmap_item_followers (roadmap_id, user_id, source_ticket_id, notify_on_dev, notify_on_prod, created_at)
SELECT id, requester_user_id, source_ticket_id, notify_on_dev, notify_on_prod, created_at
FROM roadmap_items
WHERE requester_user_id IS NOT NULL
ON CONFLICT (roadmap_id, user_id) DO NOTHING;

COMMENT ON COLUMN support_tickets.duplicate_of_ticket_id  IS 'Canonical ticket this one is a duplicate of. Mutually exclusive with duplicate_of_roadmap_id.';
COMMENT ON COLUMN support_tickets.duplicate_of_roadmap_id IS 'Canonical roadmap item this one is a duplicate of. Mutually exclusive with duplicate_of_ticket_id.';
COMMENT ON TABLE  roadmap_item_followers IS 'Users to notify when a roadmap item is marked live in dev or prod.';
