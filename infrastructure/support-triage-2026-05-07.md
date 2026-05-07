# Support Ticket Triage — 2026-05-07

L1 review of 16 open tickets in shared support DB (prod RDS, accessed via dev's `SUPPORT_DB_DSN`). All actions logged here for revertability.

## Phase 1: Categorization (this round)

All 16 tickets were submitted as `type='general'` (the default). 15 reclassified, 1 left alone.

| Ticket | Subject | From → To | Reasoning |
|---|---|---|---|
| 1b5a3436 | Quick log category | general → feature_request | User wants a category dropdown — net-new capability. |
| df835d04 | Report sharing | general → bug_report | Got an error viewing a shared report. |
| acd98313 | Chat Isn't Working | general → bug_report | No message input UI after creating chat. |
| b99dd240 | Back button/reports | general → bug_report | App-stuck-must-restart counts as broken navigation. |
| 063915c6 | Sleep Time | general → feature_request | Auto-calc of total sleep is the substantive ask; UI alignment is cosmetic and called out as nice-to-have. |
| 538d8d09 | Log/help button overlap | general → bug_report | UI overlap + reopen-help missing. |
| 2aac7344 | Speech failed to log | general → bug_report | Internal server error reproduction. |
| 9d104e20 | Log Seizure Failed | general → bug_report | Internal server error reproduction. |
| 0fcf90c4 | "Three things to know" | general → general (no change) | User question, not bug or feature. Will reply for clarification later. |
| f7d0b9c3 | Care taker/doctor access | general → feature_request | Asks for differentiated role access — net-new capability. |
| 1838f638 | Editing medication change | general → feature_request | Asks for ability to backdate change_date — net-new capability. |
| 2c4d517a | Doctor access | general → feature_request | Asks for role-based access limits. |
| 7f63d261 | Feature: Quick-log button | general → feature_request | Subject literally says "Feature:". |
| 43dfd616 | Sleep chart yesterday twice | general → bug_report | Data anomaly with reproduction. |
| 4f719750 | Feature: Calendar view | general → feature_request | Subject literally says "Feature:". |
| 5bc054ab | App crashes adding 3rd med | general → bug_report | High-priority crash with reproduction. |

**Final tally**: 7 feature_request, 8 bug_report, 1 general.

### Revert SQL (Phase 1)

If categorization needs to be undone wholesale:

```sql
UPDATE support_tickets
SET type = 'general'
WHERE id IN (
  '1b5a3436-698e-4396-9042-855207aca63e',
  'df835d04-442f-4927-89c8-198ca7247a3b',
  'acd98313-7f68-4734-a0e5-d341b1456fb8',
  'b99dd240-5f62-441f-96ee-29806d0c45c6',
  '063915c6-c03c-4efb-8dba-e7c05ec0c16a',
  '538d8d09-d666-469b-8b43-1c798d5e0c0b',
  '2aac7344-4ddf-4e1d-b7d6-19fad22cf451',
  '9d104e20-ea3d-4196-b106-9fe7cb1be5e9',
  'f7d0b9c3-b9f8-475b-9a56-be44219787dc',
  '1838f638-6cf4-4daa-b159-3e300aa8f4b9',
  '2c4d517a-6377-4e48-9eba-ec0944303a05',
  '7f63d261-f088-5fac-8b1d-ada59691fe4b',
  '43dfd616-af54-5cf4-a7a0-542860fe422b',
  '4f719750-a54e-5b40-b6a9-8c0fa654264f',
  '5bc054ab-b14f-51eb-b475-e361592c8a24'
);

-- Internal triage notes can be removed individually or all at once:
DELETE FROM ticket_messages
WHERE is_internal = TRUE
  AND sender_email = 'claude-triage@mycarecompanion.net'
  AND created_at::date = '2026-05-07';
```

## Phase 2: Bug investigation — COMPLETE

All 8 bugs triaged. Code fixes live on dev only — none deployed to prod yet.

| Ticket | Subject | Outcome | Code change | Status set |
|---|---|---|---|---|
| 5bc054ab | App crashes adding 3rd med | FDA-interaction lookup hung the request → user perceives crash. Bounded with 5s timeout. | medication_handler.go: context.WithTimeout around CheckInteractions | waiting_on_user (asked if updated build still reproduces) |
| 2aac7344 | Speech failed to log → 500 | Could not reproduce; no error_logs access from triage role. Asked for repro details. | none | waiting_on_user |
| 9d104e20 | Seizure failed to log → 500 | Same as 2aac7344. Asked for repro details. | none | waiting_on_user |
| acd98313 | Chat: no message-input UI after creating thread | Reload after thread create dropped the new thread from the URL, so the input never rendered. | chat.html: redirect to ?thread={id} + DOMContentLoaded auto-select | resolved |
| 43dfd616 | Sleep chart shows yesterday twice | Could not reproduce on test data; likely a TZ-boundary issue but needs the user's actual log timestamps. | none | waiting_on_user |
| 538d8d09 | Log/help button overlap, can't reopen mascot | Help-mascot floated under the compose-FAB and had no re-open affordance. | partials/mascot.html: lifted to bottom-28 + collapsed pill that swaps in on dismiss | resolved |
| b99dd240 | Stuck in reports, must restart app | The only exit was an unlabeled X icon; on long PDFs the top "Back" link scrolls offscreen on mobile. | reports.html: replaced X with labeled "Back" button next to title | resolved |
| df835d04 | Shared-report error | Architectural: report PDFs live on EC2 ephemeral disk, so any deploy or cross-instance LB hop loses them. Real engineering fix needed (mirror ticket-attachments S3 path). | none in this session — kicked back to engineering | in_progress |

### Pending follow-ups

- Code fixes need a prod deploy to actually help users. Affected files (uncommitted as of 2026-05-07 EOD):
  - `internal/handler/api/medication_handler.go`
  - `templates/chat.html`
  - `templates/partials/mascot.html`
  - `templates/reports.html`
- `df835d04` needs the report-storage refactor (~4-6 hours): generalize `internal/service/attachment_storage.go` to also handle report PDFs, route writes/reads through it, drop the local-FS-only paths in `report_service.go` and `report_handler.go::ServeReportFile`.
- `2aac7344` / `9d104e20` / `43dfd616` are blocked on user repro details. Triage role does not have read access to `error_logs`; if those come back without more info, escalate to a role that does.

### Revert SQL (Phase 2)

Per-ticket undo if a status change needs to be backed out. Internal notes from this session can be wiped wholesale via the Phase-1 DELETE block above (already filters on `claude-triage@mycarecompanion.net` + today's date).

```sql
-- Revert resolved/in_progress changes back to open
UPDATE support_tickets
SET status = 'open'::ticket_status, resolved_at = NULL, updated_at = NOW()
WHERE id IN (
  '5bc054ab-b14f-51eb-b475-e361592c8a24', -- waiting_on_user
  '2aac7344-4ddf-4e1d-b7d6-19fad22cf451', -- waiting_on_user
  '9d104e20-ea3d-4196-b106-9fe7cb1be5e9', -- waiting_on_user
  'acd98313-7f68-4734-a0e5-d341b1456fb8', -- resolved
  '43dfd616-af54-5cf4-a7a0-542860fe422b', -- waiting_on_user
  '538d8d09-d666-469b-8b43-1c798d5e0c0b', -- resolved
  'b99dd240-5f62-441f-96ee-29806d0c45c6', -- resolved
  'df835d04-442f-4927-89c8-198ca7247a3b'  -- in_progress
);
```
