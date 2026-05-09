-- Migration 00033: human-friendly support_tickets numbering.
--
-- Adds a 6+ digit sequential ticket_number alongside the existing UUID id.
-- The UUID stays as the primary key (no FK retargets); the new number is
-- a UNIQUE display column for verbal communication with users
-- ("your ticket #112358 is being looked into").
--
-- The sequence starts at 112358 (Fibonacci, intentional). Existing tickets
-- are backfilled in created_at order — oldest = 112358 — so we don't have
-- a gap or out-of-order numbers when humans reference tickets that pre-date
-- this migration.
--
-- Idempotent: re-runnable. Backfill skips rows that already have a number.

-- 1. Sequence backing the column. MINVALUE 112358 ensures no number ever
--    drops below the starting point even if someone setval()'s it.
CREATE SEQUENCE IF NOT EXISTS support_tickets_ticket_number_seq
    AS BIGINT
    INCREMENT BY 1
    MINVALUE 112358
    START WITH 112358
    CACHE 1;

-- 2. New column (nullable for the brief window during backfill, then NOT NULL).
ALTER TABLE support_tickets
    ADD COLUMN IF NOT EXISTS ticket_number BIGINT;

-- 3. Backfill existing rows in chronological order. Each row gets the next
--    nextval(); rows with a non-null ticket_number are left alone.
WITH ordered AS (
    SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn
    FROM support_tickets
    WHERE ticket_number IS NULL
)
UPDATE support_tickets st
SET ticket_number = nextval('support_tickets_ticket_number_seq')
FROM ordered o
WHERE st.id = o.id;

-- 4. Lock the column down: NOT NULL, UNIQUE, and default-from-sequence so
--    every future INSERT through the API gets a number automatically.
ALTER TABLE support_tickets
    ALTER COLUMN ticket_number SET NOT NULL,
    ALTER COLUMN ticket_number SET DEFAULT nextval('support_tickets_ticket_number_seq');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'support_tickets_ticket_number_key'
    ) THEN
        ALTER TABLE support_tickets
            ADD CONSTRAINT support_tickets_ticket_number_key UNIQUE (ticket_number);
    END IF;
END $$;

-- 5. Sanity: the sequence should now point past the last assigned number.
--    setval to MAX existing number so the next INSERT picks the right one
--    even if backfill happened in a different transaction order than future
--    inserts will.
SELECT setval(
    'support_tickets_ticket_number_seq',
    GREATEST(
        (SELECT COALESCE(MAX(ticket_number), 112357) FROM support_tickets),
        112357
    )
);
