\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE _notes(num bigint, msg text) ON COMMIT DROP;
INSERT INTO _notes(num, msg) VALUES
(112402, $$Internal (claude-triage 2026-06-23): Root cause — the calendar med-change pill was bucketed by DATE(created_at AT TIME ZONE 'UTC'), so an evening change in a US timezone rolled onto the next day. FIXED ON DEV (branch fix/med-change-date-cluster, commits 7bc2ccd/5d4c0b9): added treatment_changes.effective_date, stamped in the user's local tz on dosage/schedule edits, and the pill now buckets on effective_date. Verified via repo test + browser flow on dev. PENDING PRODUCTION DEPLOY — do not tell the user it is live until deployed.$$),
(112369, $$Internal (claude-triage 2026-06-23): FIXED ON DEV (branch fix/med-change-date-cluster). Added an editable effective_date on med changes: GET /api/children/{childID}/treatment-changes?date= lists a day's changes, PATCH /api/treatment-changes/{id} edits the date (family-scoped authz; non-family users rejected). The dashboard 💊 pill is now clickable -> modal with an editable date. Lets the user backdate a change they made earlier but logged late. Verified end-to-end on dev (commit 6baefe1). PENDING PRODUCTION DEPLOY.$$),
(112397, $$Internal (claude-triage 2026-06-23): Root cause — editing a medication's schedule ran DeactivateAllSchedules + CreateSchedule (new schedule ids). Today's already-logged dose stayed bound to the OLD schedule id, while GetDueMedications (LEFT JOIN on ml.schedule_id = ms.id) couldn't see it on the NEW schedule -> the dose showed both logged AND still-due = "double meds". FIXED ON DEV (branch fix/med-change-date-cluster, commit 5d4c0b9): ReconcileSchedules now updates schedules in place matched by time_of_day, preserving the schedule id and the log linkage. Verified via repo test (schedule id preserved, single due row, IsLogged=true). PENDING PRODUCTION DEPLOY.$$),
(112379, $$Internal (claude-triage 2026-06-23): Implemented as part of the med-change date work. The 💊 calendar pill is now clickable and opens a modal showing what the medication change was (change_summary) plus an editable date. Branch fix/med-change-date-cluster, commit 6baefe1, verified on dev. PENDING PRODUCTION DEPLOY.$$);

INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT t.id, n.msg, true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM _notes n JOIN support_tickets t ON t.ticket_number = n.num;

UPDATE support_tickets t SET status = 'in_progress', updated_at = now()
FROM _notes n WHERE t.ticket_number = n.num AND t.status = 'open';

SELECT ticket_number, status FROM support_tickets WHERE ticket_number IN (112402,112369,112397,112379) ORDER BY ticket_number;
COMMIT;
