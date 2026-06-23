\set ON_ERROR_STOP on
-- Run ONLY after the second deploy is confirmed live.
BEGIN;
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT t.id, v.msg, false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM (VALUES
  (112364, $$Fixed and now live: the quick-note box on the dashboard now has a "Save as" dropdown — pick Behavior, Sleep, or Meal and your typed note is filed under that category instead of always going in as a behavior note. Could you try jotting a sleep or meal note and confirm it lands in the right place?$$),
  (112391, $$Now live: you can finally see what a "new pattern" alert found. Tapping a "new pattern" card on the dashboard now opens a plain-language breakdown of the pattern (what we noticed in your logged data, with a confidence indicator), and on the Alerts page each pattern alert has a "See what this pattern is" link. Please take a look next time one shows up and let us know it makes sense.$$)
) AS v(num, msg)
JOIN support_tickets t ON t.ticket_number = v.num;

UPDATE support_tickets SET status='waiting_on_user', updated_at=now()
WHERE ticket_number IN (112364,112391);

SELECT ticket_number, status FROM support_tickets WHERE ticket_number IN (112364,112391) ORDER BY ticket_number;
COMMIT;
