\set ON_ERROR_STOP on
BEGIN;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Internal (claude-triage 2026-06-23): Auto-captured 500 was "invalid period_type: last_30_days" on POST /reports/generate. Root cause: an OLDER mobile build sent period_type "last_30_days"; the current web + mobile report screens send the canonical day/week/month, but report_service.computeDateRange only accepted day/week/month/custom and 500'd on anything else. FIXED ON DEV (branch fix/med-change-date-cluster, commit 15e9d5b): computeDateRange now normalizes legacy aliases (last_30_days->month, last_7_days->week, weekly/monthly/today/daily, etc.) before validating; genuinely unknown values still error. Unit-tested (report_perioddate_test.go). No user to notify (auto ticket). PENDING PRODUCTION DEPLOY; can resolve once deployed since the failure path is gone for any client version.$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112392;

UPDATE support_tickets SET status='in_progress', updated_at=now() WHERE ticket_number=112392 AND status='open';
SELECT ticket_number, status FROM support_tickets WHERE ticket_number=112392;
COMMIT;
