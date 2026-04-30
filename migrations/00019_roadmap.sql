-- Migration: 00019_roadmap.sql
-- Description: Roadmap of upcoming product work. Items can be entered
-- manually by an admin, captured as part of larger internal initiatives,
-- or promoted from a user-submitted feature_request support ticket. When
-- a feature goes live in dev (and later in prod), the original requester
-- gets notified via the linked support ticket + email.

DO $$ BEGIN
    CREATE TYPE roadmap_status AS ENUM (
        'proposed',
        'planned',
        'in_progress',
        'in_dev',
        'in_prod',
        'cancelled'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE roadmap_priority AS ENUM ('p0', 'p1', 'p2', 'p3');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE roadmap_source AS ENUM ('manual', 'internal', 'feature_request');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS roadmap_items (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title             VARCHAR(255) NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    status            roadmap_status   NOT NULL DEFAULT 'proposed',
    priority          roadmap_priority NOT NULL DEFAULT 'p2',
    source            roadmap_source   NOT NULL DEFAULT 'manual',
    source_ticket_id  UUID REFERENCES support_tickets(id) ON DELETE SET NULL,
    requester_user_id UUID REFERENCES users(id)           ON DELETE SET NULL,
    notify_on_dev     BOOLEAN NOT NULL DEFAULT TRUE,
    notify_on_prod    BOOLEAN NOT NULL DEFAULT TRUE,
    dev_released_at   TIMESTAMPTZ,
    prod_released_at  TIMESTAMPTZ,
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Tickets can only be promoted once; second attempt should reuse the
    -- existing roadmap item, not create a duplicate.
    CONSTRAINT roadmap_items_source_ticket_unique UNIQUE (source_ticket_id)
);

CREATE INDEX IF NOT EXISTS idx_roadmap_items_status   ON roadmap_items(status);
CREATE INDEX IF NOT EXISTS idx_roadmap_items_priority ON roadmap_items(priority);
CREATE INDEX IF NOT EXISTS idx_roadmap_items_source   ON roadmap_items(source);

COMMENT ON TABLE  roadmap_items IS 'Prioritized list of upcoming product work, super_admin scope.';
COMMENT ON COLUMN roadmap_items.requester_user_id IS 'For feature_request items: the user who filed the original ticket. Notified on dev/prod release.';
COMMENT ON COLUMN roadmap_items.dev_released_at   IS 'Set when admin presses "Mark live in dev"; triggers requester notification.';
COMMENT ON COLUMN roadmap_items.prod_released_at  IS 'Set when admin presses "Mark live in prod"; triggers second requester notification.';
