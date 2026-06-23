\set ON_ERROR_STOP on
BEGIN;

-- ---- Seed / no-submitter duplicates: resolve with internal note (no user to message) ----
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, msg, true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM (VALUES
  (112387, $$Internal (2026-06-23): Seed/auto ticket with no submitter, duplicate of #112366 ("sleep chart shows day twice"), which was investigated and could not be reproduced (resolved). Closing as duplicate/seed.$$),
  (112388, $$Internal (2026-06-23): Seed ticket, no submitter. Feature idea (CSV bulk import of past medication logs) — logged as a product idea; no real user awaiting. Closing.$$),
  (112389, $$Internal (2026-06-23): Seed ticket, no submitter, duplicate of the quick-log asks (#112372 / #112364). Closing as duplicate/seed.$$)
) AS v(num,msg)
JOIN support_tickets t ON t.ticket_number = v.num;

UPDATE support_tickets SET status='resolved', resolved_at=now(), updated_at=now()
WHERE ticket_number IN (112387,112388,112389);

-- ---- Test-family tickets (joe_*@test.com): resolve with triage notes ----
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, msg, true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM (VALUES
  (112367, $$Internal (2026-06-23): Test-family ticket. Reporter could not replicate the speech-log 500 and couldn't open the attachment. The dashboard quick-add speech 1-10/1-5 scale mismatch (the likely cause) was fixed in #112380 (commit 778e218). Closing.$$),
  (112380, $$Internal (2026-06-23): Test-family ticket. Root cause (speech quick-add form asked 1-10 but speech levels are 1-5, causing a 500) was fixed in commit 778e218 and is deployed. Closing.$$),
  (112365, $$Internal (2026-06-23): Question — "are the Three Things to Know cards clickable?" As of the #112381 fix the cards are now fully tappable and open the underlying alert/medication/pattern detail. Answered + closing.$$),
  (112362, $$Internal (2026-06-23): Test-family. The substantive ask (auto-calculate total sleep from bed/wake times, still adjustable) is delivered: sleep total auto-calculates and remains editable, and the hour-only flow was just fixed in #112398. The cosmetic field-alignment note is minor. Closing as addressed.$$),
  (112368, $$Internal (2026-06-23): Test-account (doctor login) feature request for role-based access limits. Logged as a product idea (role builder is on the roadmap; super-admin gating exists today). No real user awaiting. Closing.$$),
  (112370, $$Internal (2026-06-23): Test-account feature request to differentiate caretaker vs doctor access from parents. Same theme as #112368 — captured as a product idea (role builder). Closing.$$),
  (112371, $$Internal (2026-06-23): Test-account feature request — calendar heatmap of meltdown clusters. Logged as a product idea. Closing.$$),
  (112372, $$Internal (2026-06-23): Test-account feature request — quick-log button on the home screen. A quick-log entry exists today (see #112364); broader home-screen shortcut logged as a product idea. Closing.$$)
) AS v(num,msg)
JOIN support_tickets t ON t.ticket_number = v.num;

-- Public answer for the one that was a direct question:
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Yes — the "Three things to know" cards are clickable. Tapping a card now opens the full detail for that item (the alert, medication, or pattern it's about). Thanks for asking!$$,
false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM support_tickets WHERE ticket_number = 112365;

UPDATE support_tickets SET status='resolved', resolved_at=now(), updated_at=now()
WHERE ticket_number IN (112367,112380,112365,112362,112368,112370,112371,112372);

-- ---- #112377 bright spot (real user, fix shipped 05-09, 6-week verification window) ----
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Closing the loop on the "Today's bright spot" tile: we fixed it back in May so the calm-stretch celebration only appears when the WHOLE week has no meltdowns, aggression, or self-injury (previously it only checked the current day, which is why Matty's week with incidents still showed as calm). That fix has been live in production for several weeks with no recurrence, so we're marking this resolved. If the bright-spot tile ever celebrates a week that actually had hard days, please reply here and we'll reopen immediately.$$,
false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM support_tickets WHERE ticket_number = 112377;

UPDATE support_tickets SET status='resolved', resolved_at=now(), updated_at=now()
WHERE ticket_number = 112377;

SELECT status, count(*) FROM support_tickets GROUP BY status ORDER BY count DESC;
COMMIT;
