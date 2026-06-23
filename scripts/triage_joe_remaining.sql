\set ON_ERROR_STOP on
BEGIN;

-- #112364 Quick log category — CONFIRMED real bug (saveQuickNote hardcodes /logs/behavior)
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Internal (2026-06-23): CONFIRMED. templates/child_dashboard.html saveQuickNote() always POSTs the free-text note to /api/children/{id}/logs/behavior, so a sleep/meal note typed in the quick-compose box is filed as a behavior entry. Fix = add a category selector to the compose sheet and route to the matching log endpoint (requires the sleep/meal/etc. endpoints to accept a notes-only entry — verify each). Small feature, queued. Set in_progress.$$,
true,'claude-triage@mycarecompanion.net','CareCompanion','Triage' FROM support_tickets WHERE ticket_number=112364;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$You're right, and thanks for the clear description — typing a quick note and hitting Save currently always files it as a Behavior note, even when you meant a sleep or meal note. That's a real bug, not intended. We're going to add a category picker to the quick-note box so your note goes to the right place (sleep, meal, behavior, etc.). I've logged this as in progress and we'll update you when it ships.$$,
false,'support@mycarecompanion.net','CareCompanion','Support' FROM support_tickets WHERE ticket_number=112364;
UPDATE support_tickets SET status='in_progress', updated_at=now() WHERE ticket_number=112364;

-- #112391 New Pattern Alert — real gap (no way to view the pattern)
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Internal (2026-06-23): Real gap — "new pattern" alerts surface but don't link to a detail view of the correlation/pattern. Joe attached an image. Fix = make the pattern alert open the pattern/correlation detail (inputs, output, confidence, sample size) — the data exists (correlation/insight services). Queued, in_progress.$$,
true,'claude-triage@mycarecompanion.net','CareCompanion','Triage' FROM support_tickets WHERE ticket_number=112391;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$You're right — the "new pattern" alerts let you know something was detected but don't currently give you a way to open and see what the pattern actually is. That's a gap on our side. We're making those alerts open into the pattern detail so you can see what changed, the correlation it found, and how confident it is. Marking this in progress — thanks for flagging it (and for the screenshot).$$,
false,'support@mycarecompanion.net','CareCompanion','Support' FROM support_tickets WHERE ticket_number=112391;
UPDATE support_tickets SET status='in_progress', updated_at=now() WHERE ticket_number=112391;

-- #112396 Logged out — session improvements shipped since the report; ask to confirm
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Internal (2026-06-23): Reported 05-22. Since then sessions were extended (JWT ~8h) with silent background refresh + persistent login (cookie split / persistent sessions work). Likely already mitigated. Asked Joe to confirm; set waiting_on_user. If still recurring, investigate token TTL + refresh on desktop.$$,
true,'claude-triage@mycarecompanion.net','CareCompanion','Triage' FROM support_tickets WHERE ticket_number=112396;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Since you reported this we've extended login sessions (to about 8 hours) and added silent background refresh plus persistent login, so a short time away shouldn't bump you out anymore. Have you still been getting logged out unexpectedly recently? If so, roughly how long were you away, and on what device/browser? That'll help us confirm it's fully resolved.$$,
false,'support@mycarecompanion.net','CareCompanion','Support' FROM support_tickets WHERE ticket_number=112396;
UPDATE support_tickets SET status='waiting_on_user', updated_at=now() WHERE ticket_number=112396;

-- #112401 Cursor position — needs specifics to reproduce
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Internal (2026-06-23): "Cursor in weird position across multiple log sections" with an image. Likely a mobile focus/caret issue in the log form inputs. Need device + which fields + on-focus vs on-type to reproduce. Asked Joe; set waiting_on_user.$$,
true,'claude-triage@mycarecompanion.net','CareCompanion','Triage' FROM support_tickets WHERE ticket_number=112401;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Thanks for the screenshot. To make sure we fix the right thing: is this happening in the phone app specifically, and in which log sections (behavior, sleep, notes, etc.)? And does the cursor land in the wrong spot when the field first gets focus, or as you're typing? Any of those details will help us reproduce and fix it.$$,
false,'support@mycarecompanion.net','CareCompanion','Support' FROM support_tickets WHERE ticket_number=112401;
UPDATE support_tickets SET status='waiting_on_user', updated_at=now() WHERE ticket_number=112401;

SELECT status, count(*) FROM support_tickets GROUP BY status ORDER BY count DESC;
COMMIT;
