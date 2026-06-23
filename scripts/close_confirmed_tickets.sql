-- Close 8 user-confirmed support tickets. Posts a public closing message and marks resolved.
-- Idempotent-ish: only acts on tickets still in non-resolved status.
\set ON_ERROR_STOP on
BEGIN;

CREATE TEMP TABLE _closures(num bigint, msg text) ON COMMIT DROP;

INSERT INTO _closures(num, msg) VALUES
(112358, $$Glad to hear it — thanks for confirming the chat now opens ready to type after you create it. We're marking this resolved. If the message box ever fails to show up again, just reply here and we'll reopen it.$$),
(112373, $$Thanks for confirming the help and log buttons no longer overlap and that help reopens after you dismiss it. Marking this resolved — reach out anytime if the layout shifts again.$$),
(112374, $$Thanks for confirming the Daily / Weekly / Monthly toggle now sits cleanly inside the card on your phone. Marking this resolved.$$),
(112381, $$Great — glad the "Worth noticing" card now wraps its text and opens the full detail when tapped. Marking this resolved. Thanks for flagging it.$$),
(112390, $$Thanks for confirming the behavior log saves again — that range-validation bug is fixed in production. Marking this resolved. As always, reply here if anything else turns up. – Bryan$$),
(112382, $$Closing the loop on this one: the "How [child] feels" tile and its emoji buttons were unified to the 1–10 scale back in May so the tile, the behavior log, and the at-a-glance face all agree. The related save-range bug you reported separately (#112390) was confirmed fixed, and this has been live in production for several weeks with no recurrence — so we're marking it resolved. If the tile ever shows a score that doesn't match what you tapped, reply here and we'll reopen immediately.$$),
(112366, $$Thanks for the follow-up — since this isn't reproducing and doesn't appear to be an active issue, we're marking it resolved. If you ever see the same sleep date listed twice with different totals again, grab a screenshot and reply here and we'll dig straight in.$$),
(112359, $$Thanks for confirming — adding a third medication now saves cleanly. The form was waiting on a slow drug-interaction lookup; we capped that check so the form no longer hangs and the medication always saves. Marking this resolved.$$);

-- Post the closing message for each ticket (only if it still needs closing).
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT t.id, c.msg, false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM _closures c
JOIN support_tickets t ON t.ticket_number = c.num
WHERE t.status <> 'resolved';

-- Mark resolved.
UPDATE support_tickets t
SET status = 'resolved', resolved_at = now(), updated_at = now()
FROM _closures c
WHERE t.ticket_number = c.num AND t.status <> 'resolved';

-- Show result.
SELECT t.ticket_number, t.status, to_char(t.resolved_at,'YYYY-MM-DD HH24:MI') AS resolved_at
FROM support_tickets t JOIN _closures c ON c.num = t.ticket_number
ORDER BY t.ticket_number;

COMMIT;
