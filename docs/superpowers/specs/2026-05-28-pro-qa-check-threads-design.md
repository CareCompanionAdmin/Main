# Pro QA — Check Threads (comments + attachments)

**Date:** 2026-05-28
**Status:** Approved by Bryan, implementing
**Scope:** Add a back-and-forth comment thread (markdown + S3 attachments) to each `pro_qa_requested_checks` row, mirroring the existing issue-comment shape. Status changes on a check emit an auto-comment.

## Problem

The Pro QA workspace shipped 2026-05-22 ([memory ref](../../../../.claude/projects/-home-carecomp-carecompanion/memory/project_carecompanion_pro_qa.md)) gives the paid QA user only a status toggle (`open|in_review|done`) on each requested check. There is no place to leave findings, ask Bryan clarifying questions, or attach screenshots of what she saw. Bryan identified this gap on 2026-05-28 after adding a real batch of checks for the engagement.

## Goal

A check should behave like a lightweight ticket: Bryan writes the instructions, QA adds findings, Bryan replies, QA replies again. Threaded markdown comments + optional S3 attachments per comment. Status changes are recorded inline in the thread.

## Non-goals

- No refactor of existing issue-comment infra into a polymorphic table. Rejected (YAGNI; no third entity needs comments yet; refactor risk on a feature shipped 6 days ago).
- No "open issue from check" linkage. Rejected (Bryan wants threading on checks themselves, not escalation to a separate entity).
- No notifications/emails on new check comments. Out of scope for this slice.

## Data model

New migration `migrations/00040_pro_qa_check_threads.sql`:

```sql
CREATE TABLE IF NOT EXISTS pro_qa_check_comments (
    id               UUID PRIMARY KEY,
    check_id         UUID NOT NULL REFERENCES pro_qa_requested_checks(id) ON DELETE CASCADE,
    body_md          TEXT NOT NULL,
    author_email     TEXT,
    author_name      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_status_change BOOLEAN NOT NULL DEFAULT FALSE,
    status_from      TEXT,
    status_to        TEXT
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_check_comments_check ON pro_qa_check_comments (check_id, created_at);

CREATE TABLE IF NOT EXISTS pro_qa_check_attachments (
    id              UUID PRIMARY KEY,
    check_id        UUID NOT NULL REFERENCES pro_qa_requested_checks(id) ON DELETE CASCADE,
    comment_id      UUID REFERENCES pro_qa_check_comments(id) ON DELETE SET NULL,
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    storage_driver  TEXT NOT NULL,
    storage_path    TEXT NOT NULL,
    uploaded_by_email TEXT,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_check_attachments_check ON pro_qa_check_attachments (check_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON pro_qa_check_comments TO carecomp_support_dev;
GRANT SELECT, INSERT, UPDATE, DELETE ON pro_qa_check_attachments TO carecomp_support_dev;
```

Both live on the shared support DB (= prod RDS) via `SUPPORT_DB_DSN`, same as the existing pro_qa_* tables. No FK to `users` (denormalized email/name per the existing pattern).

S3 storage: reuse the existing `proQAStorage` instance (`services.go:81`) — same bucket and same `ticket-attachments/pro-qa/` prefix as issue attachments. Object names are UUID-derived so there's no collision risk; differentiation is in the DB (`pro_qa_check_attachments` vs `pro_qa_issue_attachments`). 20MB cap matches issue attachments.

## Backend wiring

- **Models** (`internal/models/pro_qa.go`)
  - `ProQACheckComment` (mirror `ProQAIssueComment`)
  - `ProQACheckAttachment` (mirror `ProQAAttachment` but `CheckID` instead of `IssueID`)
  - `ProQARequestedCheck` gets `CommentCount` + `AttachmentCount` (populated in list query)

- **Repository** (`internal/repository/pro_qa_repository.go`) — 6 new methods:
  - `GetCheck(ctx, id)` — single check fetch for detail page
  - `ListCheckComments(ctx, checkID)`
  - `CreateCheckComment(ctx, c)`
  - `ListCheckAttachments(ctx, checkID)`
  - `CreateCheckAttachment(ctx, a)`
  - `GetCheckAttachment(ctx, id)` — for download
  - Extend `ListChecks` query to include `comment_count` + `attachment_count` via subqueries.

- **Service** (`internal/service/pro_qa_service.go`) — 5 new methods:
  - `GetCheck`
  - `ListCheckComments`
  - `AddCheckComment` (markdown, denormalized author)
  - `ChangeCheckStatus` (extracted from `UpdateCheck`; emits auto-comment when status differs)
  - `UploadCheckAttachment`, `FetchCheckAttachment` (delegate to `BlobStorage`)
  - `UpdateCheck` remains for title/body/sort, but its status path delegates to `ChangeCheckStatus` so all status changes flow through one auto-comment-emitting code path.

- **Handlers** (`internal/handler/admin/pro_qa_handlers.go`) — 5 new + 1 route group adjustment:
  - `ProQACheckDetailPage` (GET `/admin/pro-qa/checks/{id}`)
  - `ProQACheckComment` (POST `/admin/pro-qa/checks/{id}/comment`)
  - `ProQACheckStatus` (POST `/admin/pro-qa/checks/{id}/status`) — explicit status-only endpoint
  - `ProQACheckAttach` (POST `/admin/pro-qa/checks/{id}/attach`)
  - `ProQAFetchCheckAttachment` (GET `/admin/pro-qa/check-attachments/{id}`)

- **Routes** (`internal/handler/admin/routes.go`) — add the 5 above under existing `/admin/pro-qa` group.

## UI

- **`templates/admin/pro_qa_checks.html`** (existing list page) — each row's title becomes a link to its detail page; show small badges next to status: `💬 N` (comment count) and `📎 M` (attachment count) when > 0.
- **`templates/admin/pro_qa_check_detail.html`** (new) — modeled on `pro_qa_issue_detail.html`:
  - Header: title, status badge, link back to checks list
  - Bryan's instructions (rendered markdown from `body_md`)
  - Edit form (title / body / status / sort_order — same fields as today's inline form)
  - Comment thread (chronological; status-change rows styled differently — `is_status_change=true` renders as italic "_status changed: open → in_review_")
  - Attachments list (downloadable links)
  - Composer: textarea + post button + file upload input
- **`static/js/pro_qa.js`** — extend the existing upload handler to also POST to `/admin/pro-qa/checks/{id}/attach`.

## Auto-comment on status change

`ChangeCheckStatus` is the single point through which all status changes flow. It:
1. Reads the current status
2. If different from the requested status, runs both UPDATE and INSERT in a transaction:
   - UPDATE `pro_qa_requested_checks.status`
   - INSERT into `pro_qa_check_comments` with `is_status_change=true, status_from=old, status_to=new`

The check edit form (existing) and the new status-only POST both route through this method.

## Testing / smoke plan (on dev first, then prod)

1. Apply migration 00040 on dev → confirm tables + grants present
2. Open `/admin/pro-qa/checks` on dev → existing checks render with `0` comment/attachment counts
3. Click a check title → detail page opens with Bryan's instructions
4. Post a markdown comment (`**bold** _italic_`) → renders correctly
5. Upload a small PNG → appears in attachments list; download link returns the file
6. Change status via edit form → auto-status-change comment appears in thread
7. Log in to **prod** as super_admin → same check shows the dev-posted comment + attachment (shared support DB)
8. Apply migration 00040 on prod → idempotent (CREATE IF NOT EXISTS); should report "0 pending" if dev applied first via SUPPORT_DB_DSN

## Rollback plan

Bryan explicitly asked for revert-safety. The slice is safe to roll back at three layers:

1. **Code:** implement on a feature branch (`pro-qa-check-threads`). If the work goes sideways pre-deploy, `git checkout master` discards everything cleanly. Post-deploy: `git revert <commits>` + redeploy.
2. **Schema:** migrations are additive only (two new tables). No ALTER on existing tables. No data backfill. Rollback SQL:
   ```sql
   DROP TABLE IF EXISTS pro_qa_check_attachments;
   DROP TABLE IF EXISTS pro_qa_check_comments;
   ```
   Apply on prod RDS as `carecompanion` (not the dev role). Safe to drop because no other tables reference them.
3. **S3 objects:** check attachments share the `ticket-attachments/pro-qa/` prefix with issue attachments. To selectively clean only check attachments, join through `pro_qa_check_attachments.storage_path` and delete those keys; do NOT `aws s3 rm --recursive` the prefix (would also nuke issue attachments).

## File touch list

| File | Change |
|---|---|
| `migrations/00040_pro_qa_check_threads.sql` | new (~30 lines incl. grants) |
| `internal/models/pro_qa.go` | +~25 lines (2 structs + count fields) |
| `internal/repository/pro_qa_repository.go` | +~160 lines (6 methods + ListChecks query update) |
| `internal/service/pro_qa_service.go` | +~80 lines (5 methods + UpdateCheck refactor) |
| `internal/handler/admin/pro_qa_handlers.go` | +~120 lines (5 handlers + detail-page data load) |
| `internal/handler/admin/routes.go` | +5 lines |
| `templates/admin/pro_qa_check_detail.html` | new (~90 lines) |
| `templates/admin/pro_qa_checks.html` | small: link + count badges |
| `static/js/pro_qa.js` | small: extend uploader |

Total: ~520 LOC added, ~10 LOC modified. Single deploy via `scripts/deploy.sh`.

## Out of scope (deferred)

- Notifications/emails on new comments (could add when role-builder ships and QA has her own account).
- Per-check assignee field. Today implicit: Bryan writes, QA reads/responds. Re-evaluate if 2+ QA people ever use the workspace.
- Search/filter across check comments. Today: chronological per check is enough.

## Memory updates after ship

Append to `project_carecompanion_pro_qa.md` under a new "2026-05-28 — check threads added" section: tables, S3 prefix, rollback SQL, commits.
