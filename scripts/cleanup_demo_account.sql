-- App Store Demo Account cleanup
--
-- Removes accumulated AI-insight / alert cruft and the leftover account-
-- deletion request so the reviewer sees a clean, coherent demo account.
-- Scoped STRICTLY to the demo reviewer / family / child fixed UUIDs.
--
-- WHY this exists: the AI insight scanner used to re-emit the same concept
-- on every daily run (fuzzy title dedup missed reworded LLM output), so the
-- demo child accumulated duplicate + stale "insufficient data" insights and
-- alerts over weeks. The durable fix is in ai_insight_service.go (cross-run
-- dedupe by persisted key); this script clears the rows that accrued BEFORE
-- that fix. Run it AFTER the scanner fix is deployed so nothing re-accrues.
--
-- Idempotent + safe to re-run. For prod: requires explicit user approval.

BEGIN;

\set reviewer_id 'a99e5e51-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set family_id   'a99e5e52-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set child_id    'a99e5e53-d6b3-4a8a-9c5e-1d3c4e5f6a7b'

-- 1. Clear any leftover account-deletion request so the deletion step starts
--    from a pristine state (no "awaiting confirmation" banner on Settings).
DELETE FROM account_deletion_requests WHERE user_id = :'reviewer_id';

-- 2. Deactivate stale "insufficient data" insights — they were generated when
--    the child had only ~7 days of logs and now contradict the 90-day dataset.
UPDATE insights SET is_active = false, updated_at = now()
WHERE child_id = :'child_id' AND is_active
  AND (title ILIKE '%insufficient%' OR simple_description ILIKE '%insufficient%');

-- 3. Deduplicate the remaining active insights — keep the most recent row per
--    exact title, deactivate the rest.
UPDATE insights SET is_active = false, updated_at = now()
WHERE child_id = :'child_id' AND is_active
  AND id NOT IN (
    SELECT DISTINCT ON (title) id FROM insights
    WHERE child_id = :'child_id' AND is_active
    ORDER BY title, created_at DESC
  );

-- 4. Resolve stale "insufficient data" alerts.
UPDATE alerts SET status = 'resolved', resolved_at = now(), updated_at = now()
WHERE child_id = :'child_id' AND status = 'active' AND title ILIKE '%insufficient%';

-- 5. Deduplicate the remaining active alerts — keep the most recent per title.
UPDATE alerts SET status = 'resolved', resolved_at = now(), updated_at = now()
WHERE child_id = :'child_id' AND status = 'active'
  AND id NOT IN (
    SELECT DISTINCT ON (title) id FROM alerts
    WHERE child_id = :'child_id' AND status = 'active'
    ORDER BY title, created_at DESC
  );

COMMIT;

SELECT
  (SELECT count(*) FROM insights WHERE child_id = :'child_id' AND is_active) AS active_insights,
  (SELECT count(DISTINCT title) FROM insights WHERE child_id = :'child_id' AND is_active) AS distinct_insight_titles,
  (SELECT count(*) FROM alerts WHERE child_id = :'child_id' AND status = 'active') AS active_alerts,
  (SELECT count(*) FROM account_deletion_requests WHERE user_id = :'reviewer_id') AS leftover_deletion_requests;
