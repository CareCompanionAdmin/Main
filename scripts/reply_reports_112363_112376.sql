\set ON_ERROR_STOP on
BEGIN;

-- ===== #112363 (Holly) — report sharing / blob storage =====
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Internal (claude-triage 2026-06-23): Root cause (report PDFs on ephemeral per-instance disk) was fixed by the blob-storage refactor (migration 00028 + reportStorage = NewBlobStorage(reports namespace) wired into ReportService; ServeReportPDF route /api/reports/{id}/file). Commit is in deployed master history; prod has S3 configured (ATTACHMENT_S3_BUCKET — ticket attachments already serve cross-instance in prod, reports now share that bucket via REPORT_S3_PREFIX). Verified END-TO-END on dev (mirrors prod S3 config): generated a report for Joe's child -> stored storage_driver='s3' with a real S3 key -> fetched via /api/reports/{id}/file -> HTTP 200, application/pdf, valid %PDF- bytes. New reports across instances are now reachable. Replied to Holly, set waiting_on_user pending her confirmation on Matty's profile (cannot self-verify her exact prod report from here).$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112363;

INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Hi Holly — good news on this one. The reason you couldn't open the report Joe shared was that report PDFs used to be saved on whichever server generated them, so when your tap landed on a different server (or after one of our routine updates), the file was no longer reachable — which is exactly the error you saw. We've since moved report PDFs into shared cloud storage (the same reliable storage we already use for attachments), so any server can serve any shared report. This is now live in production. Could you ask Joe to share a fresh report from Matty's profile and confirm you can open it? If it still errors, reply here with a screenshot and we'll dig straight in.$$,
false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM support_tickets WHERE ticket_number = 112363;

UPDATE support_tickets SET status='waiting_on_user', updated_at=now() WHERE ticket_number=112363;

-- ===== #112376 (Joe) — PDF viewer trap + download error =====
INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Internal (claude-triage 2026-06-23): Two root causes, both now fixed + deployed. (1) Trapped-in-PDF: native app now uses a system browser/PDF overlay with its own Done button (reports.html "native PDF overlay on app" path, commit 41a0bcd; earlier @capacitor/browser 142da8f) + labeled Back on web. (2) The "error when downloading the pdf" Joe reported 05-09 predates the blob-storage deploy; report PDFs were on ephemeral disk and unreachable from other instances — fixed by the blob-storage refactor (migration 00028, reportStorage S3), verified end-to-end on dev (generate -> storage_driver='s3' -> ServeReportPDF 200 application/pdf valid %PDF-). Per the resolution protocol (this ticket was burned twice before), NOT marking resolved — asked Joe to retry on his phone and confirm both (a) opens/downloads and (b) exits cleanly. The native-shell aspect ideally wants a TestFlight confirmation.$$,
true, 'claude-triage@mycarecompanion.net', 'CareCompanion', 'Triage'
FROM support_tickets WHERE ticket_number = 112376;

INSERT INTO ticket_messages (ticket_id, message, is_internal, sender_email, sender_first_name, sender_last_name)
SELECT id,
$$Hi Joe — circling back on the report PDF problem, and apologies again for how many rounds this one took. There were actually two separate things going wrong, and both are now fixed and live in production:

1. Getting trapped in the PDF with no way out — reports now open in your phone's built-in browser/PDF overlay, which has its own "Done" button to return you to MyCareCompanion (no more force-quitting the app).

2. The error you hit when downloading the PDF — report files used to be saved on a single server and could become unreachable when a request landed elsewhere or after one of our updates. Reports are now stored in shared cloud storage, so opening and downloading works from any server.

When you get a chance, please open a report on your phone and try both viewing and downloading it, and let us know whether it (a) opens/downloads cleanly and (b) lets you exit back to the app without getting stuck. If anything still sticks, tell us exactly what you tapped and we'll keep at it.$$,
false, 'support@mycarecompanion.net', 'CareCompanion', 'Support'
FROM support_tickets WHERE ticket_number = 112376;

UPDATE support_tickets SET status='waiting_on_user', updated_at=now() WHERE ticket_number=112376;

SELECT ticket_number, status FROM support_tickets WHERE ticket_number IN (112363,112376) ORDER BY ticket_number;
COMMIT;
