-- 00024_subscription_comp.sql
--
-- Adds first-class "comp" tracking to family_subscriptions so we can tell a
-- paying subscription apart from one we've granted for free (founders
-- grandfather, partner family, QA test, internal staff, etc.).
--
-- comp_reason  : freeform tag — 'founders_grandfather', 'partner_family',
--                'qa_test', 'goodwill', 'extended_trial', etc. NULL means
--                this is a real paying (or genuinely trialing) subscription.
-- comped_by    : the admin user who granted the comp (NULL for migration-time
--                bulk grandfather rows).
-- comp_until   : the comp's own clock — informational; the load-bearing date
--                stays current_period_end. Lets future sweeps say "anyone
--                whose comp_until just passed needs a real billing decision."
--
-- Also widens the subscription_status enum with 'comped' and 'terminated'
-- so the enforcement middleware can tell apart "we owe them service" from
-- "they're past_due and need to pay" from "they were terminated and their
-- data is in cold storage."

ALTER TABLE family_subscriptions
    ADD COLUMN IF NOT EXISTS comp_reason TEXT,
    ADD COLUMN IF NOT EXISTS comped_by   UUID REFERENCES users(id),
    ADD COLUMN IF NOT EXISTS comp_until  TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_family_subscriptions_comp_reason
    ON family_subscriptions (comp_reason)
    WHERE comp_reason IS NOT NULL;

-- Widen status enum. Postgres requires this in a separate transaction from
-- any USING the new value, but the migration runner here applies the file as
-- one statement stream so we add then commit; the backfill that USES 'comped'
-- happens in a follow-up SQL script (P1.2), not in this migration.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumlabel = 'comped'
                   AND enumtypid = 'subscription_status'::regtype) THEN
        ALTER TYPE subscription_status ADD VALUE 'comped';
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumlabel = 'terminated'
                   AND enumtypid = 'subscription_status'::regtype) THEN
        ALTER TYPE subscription_status ADD VALUE 'terminated';
    END IF;
END $$;
