\set ON_ERROR_STOP on
BEGIN;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, msg, true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM (VALUES
  (112364, $$Internal (2026-06-23): BUILT ON DEV (branch fix/quicknote-category-and-pattern-detail, commit 33af607). Added a "Save as" category dropdown (Behavior/Sleep/Meal) to the dashboard quick-note compose sheet; saveQuickNote() now routes the notes-only entry to /logs/{behavior|sleep|diet} accordingly (all three accept a notes-only body; only log_date is required). Verified on dev via Playwright: a sleep note lands in sleep_logs, a meal note in diet_logs, a behavior note in behavior_logs, with zero leakage into behavior_logs. PENDING PROD DEPLOY.$$),
  (112391, $$Internal (2026-06-23): BUILT ON DEV (branch fix/quicknote-category-and-pattern-detail, commit 2a03533). The analysis page (/child/{childID}/alert/{alertID}/analysis, Parent View + disclaimer) already existed but was undiscoverable (buried behind the confidence-meter modal; alerts page showed only a raw correlation UUID). Now: dashboard "new pattern" (pattern_discovered) alert cards link straight to that analysis page, and the alerts page shows a clear "See what this pattern is" link for pattern alerts (gated on alert_type, since these alerts have no correlation_id). Verified on dev: analysis API returns 200 + parent-friendly content even for detail-less alerts (graceful "not yet available" fallback); dashboard + alerts-page links resolve correctly. PENDING PROD DEPLOY.$$)
) AS v(num,msg)
JOIN support_tickets t ON t.ticket_number = v.num;
SELECT ticket_number, status FROM support_tickets WHERE ticket_number IN (112364,112391);
COMMIT;
