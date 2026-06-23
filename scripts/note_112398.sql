\set ON_ERROR_STOP on
BEGIN;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Internal (claude-triage 2026-06-23): Reproduced on dev. Root cause: the Calm Hybrid redesign replaced the sleep <input type=time> with custom hour/minute/AMPM selects; updateSleepTimeHidden() in templates/daily_logs.html only built the hidden bedtime/wake_time (and triggered calcTotalSleepFromHidden) when BOTH hour AND minute were selected. The common flow "bedtime 9 PM, wake 7 AM" with no explicit minutes left the hidden field empty, so Total Sleep never auto-populated. FIXED ON DEV (branch fix/med-change-date-cluster, commit dbd013b): empty minutes now default to :00 when an hour is chosen. Verified via Playwright on dev — hour-only (21:00->07:00 = 10h 0m) AND full selection (21:00->07:30 = 10h 30m) both auto-calc, no console errors. Also satisfies the substantive ask in #112362. PENDING PRODUCTION DEPLOY (template/JS change — ships with next ASG refresh, not a TestFlight build).$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112398;
UPDATE support_tickets SET status='in_progress', updated_at=now() WHERE ticket_number=112398 AND status='open';
SELECT ticket_number, status FROM support_tickets WHERE ticket_number=112398;
COMMIT;
