\set ON_ERROR_STOP on
BEGIN;

-- Run ONLY after the deploy is confirmed live in prod + migration 00043 applied.
-- Real-user tickets -> waiting_on_user (live, awaiting user confirmation, NOT resolved per protocol).

INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT t.id, v.msg, false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM (VALUES
  (112402, $$Good news — this is fixed and now live. Medication changes now show on the day they actually happened in your local time, instead of sometimes jumping to the next day. Could you make a med change and confirm it lands on the right day? (You can also tap the 💊 icon on the dashboard calendar to view or correct a change's date.)$$),
  (112369, $$Now live: on the dashboard calendar, tap the 💊 icon on a day that has a medication change — it opens the change details and lets you edit the date right there. So if you make a change today but it actually happened yesterday (or you forgot to log it earlier), you can correct the date yourself. Please give it a try and let us know it works for you.$$),
  (112397, $$Fixed and now live: changing a medication's time no longer leaves a duplicate "still due" entry for a dose you already logged that day. Could you log a morning dose, then change that medication's morning time, and confirm you no longer see the dose double up?$$),
  (112379, $$Now live: the 💊 medication-change icon on the dashboard calendar is clickable — tap it to see what the change was, and adjust its date if needed. Thanks for the suggestion.$$),
  (112398, $$Fixed and now live: total sleep auto-calculates from your bed and wake times again — and now even if you only pick the hour (for example bedtime 9 PM, wake 7 AM) it fills in the hours asleep without needing to set minutes. Please try logging sleep and confirm it populates for you.$$)
) AS v(num, msg)
JOIN support_tickets t ON t.ticket_number = v.num;

UPDATE support_tickets SET status='waiting_on_user', updated_at=now()
WHERE ticket_number IN (112402,112369,112397,112379,112398);

-- #112392 auto ticket (no user) — failure path is gone for all client versions; resolve.
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id, $$Internal (2026-06-23): Fix deployed to prod (computeDateRange now normalizes legacy period_type aliases). The 500 path is gone for any client version. Auto ticket, no user to confirm — resolving.$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112392;
UPDATE support_tickets SET status='resolved', resolved_at=now(), updated_at=now() WHERE ticket_number = 112392;

SELECT ticket_number, status FROM support_tickets WHERE ticket_number IN (112402,112369,112397,112379,112398,112392) ORDER BY ticket_number;
COMMIT;
