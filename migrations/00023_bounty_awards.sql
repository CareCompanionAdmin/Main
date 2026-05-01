-- 00023_bounty_awards.sql
--
-- Bounty rewards program — once a month an admin reviews recently-resolved
-- bug tickets and recently-shipped feature requests and picks up to 5 of
-- each as "most significant", awarding the reporter (or roadmap follower)
-- a one-month free promo code. Considered-but-not-selected candidates can
-- be flagged "thanks_anyway" so a canned message is sent without burning
-- a slot.
--
-- All four primary keys are nullable except recipient — a single award row
-- is either tied to a support_ticket (bug) or a roadmap_item (feature),
-- never both. CHECK constraints enforce mutual exclusion and the
-- decision/promo-code coupling.

DO $$ BEGIN
    CREATE TYPE bounty_award_type AS ENUM ('bug', 'feature');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE bounty_decision AS ENUM ('selected', 'thanks_anyway');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS bounty_awards (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    award_month         DATE NOT NULL,            -- first of month, the cycle this counts toward
    award_type          bounty_award_type NOT NULL,
    decision            bounty_decision   NOT NULL,
    ticket_id           UUID REFERENCES support_tickets(id) ON DELETE SET NULL,
    roadmap_item_id     UUID REFERENCES roadmap_items(id)   ON DELETE SET NULL,
    recipient_user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    promo_code_id       UUID REFERENCES promo_codes(id) ON DELETE SET NULL,
    awarded_by          UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    awarded_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Exactly one source link must be set. The type column must agree
    -- (bug -> ticket_id, feature -> roadmap_item_id).
    CONSTRAINT bounty_awards_source_xor CHECK (
        (ticket_id IS NOT NULL AND roadmap_item_id IS NULL AND award_type = 'bug')
        OR
        (ticket_id IS NULL AND roadmap_item_id IS NOT NULL AND award_type = 'feature')
    ),

    -- Selected awards must have a promo code attached; thanks_anyway must NOT.
    CONSTRAINT bounty_awards_promo_match CHECK (
        (decision = 'selected'      AND promo_code_id IS NOT NULL) OR
        (decision = 'thanks_anyway' AND promo_code_id IS NULL)
    )
);

-- A given ticket can only be awarded once per month (its single reporter).
CREATE UNIQUE INDEX IF NOT EXISTS uq_bounty_awards_ticket_month
    ON bounty_awards (award_month, ticket_id)
    WHERE ticket_id IS NOT NULL;

-- A given (roadmap_item, user) can only be awarded once per month — one
-- roadmap item can have many followers and each could be selected, so we
-- key on the recipient too.
CREATE UNIQUE INDEX IF NOT EXISTS uq_bounty_awards_roadmap_user_month
    ON bounty_awards (award_month, roadmap_item_id, recipient_user_id)
    WHERE roadmap_item_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bounty_awards_month ON bounty_awards (award_month DESC);
CREATE INDEX IF NOT EXISTS idx_bounty_awards_recipient ON bounty_awards (recipient_user_id);
