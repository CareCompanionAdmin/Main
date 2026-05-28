-- 00040_pro_qa_check_threads.sql
--
-- Purpose: Add a back-and-forth comment thread + attachments to each
-- pro_qa_requested_checks row, so the paid QA user can leave findings,
-- attach screenshots, and Bryan can reply. Mirrors the shape of
-- pro_qa_issue_comments / pro_qa_issue_attachments from 00039.
--
-- Same DB as the rest of the support / pro_qa tables (SUPPORT_DB_DSN),
-- so dev and prod see the same rows. No FK to users(id) for the same
-- reason 00039 doesn't have one — admin UUIDs differ across envs.
--
-- Rollback (manual, against prod RDS as the carecompanion role):
--   DROP TABLE IF EXISTS pro_qa_check_attachments;
--   DROP TABLE IF EXISTS pro_qa_check_comments;

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

-- GRANTs are applied OUT-OF-BAND against prod RDS as the carecompanion role
-- (see docs/deploys/2026-05-28-pro-qa-check-threads.md). They are NOT in
-- this file because the migration runner also executes against dev's local
-- postgres, where the carecomp_support_dev role does not exist; a GRANT to a
-- missing role would abort the transaction and leave the migration unapplied.
