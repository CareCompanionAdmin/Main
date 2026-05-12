-- 00036_insight_dedupe_key.sql
--
-- Adds a structured dedupe key to the insights table so scanners can
-- avoid re-emitting the same finding within a rolling window without
-- relying on title-substring matching (the existing approach in
-- ClinicalRuleScanner.alreadySurfaced — too loose; dev shows 663
-- insights/24h with mostly distinct titles but conceptually-duplicate
-- content).
--
-- Format (free text, set by the emitting scanner):
--   <scanner>:<rule>:<target>[:<extra>]
-- Examples:
--   clinical:fda-blackbox:methylphenidate
--   clinical:fda-side-effect-headache:methylphenidate
--   clinical:fda-pediatric:methylphenidate
--   clinical:med-start-changepoint:methylphenidate:sleep_quality
--
-- Lookup is by (child_id, dedupe_key, created_at) with a time window —
-- NOT a UNIQUE constraint. A reminder-style insight may legitimately
-- re-surface after the window passes, so a hard unique would over-fire.
--
-- See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

BEGIN;

ALTER TABLE insights ADD COLUMN IF NOT EXISTS dedupe_key text;

-- Composite index supports the fast "has this child seen <key> in the
-- last N days?" check. Partial — only indexes rows that have a key.
CREATE INDEX IF NOT EXISTS idx_insights_dedupe_lookup
    ON insights (child_id, dedupe_key, created_at DESC)
    WHERE dedupe_key IS NOT NULL;

COMMIT;
