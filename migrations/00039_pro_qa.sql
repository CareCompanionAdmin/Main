-- 00039_pro_qa.sql
--
-- Purpose: Admin-only "Pro QA" workspace for a paid QA engagement.
-- Same DB as support tickets (SUPPORT_DB_DSN) so dev/prod see the same
-- records. All tables intentionally have NO foreign keys to users(id):
-- the QA person and Bryan log in from either env (dev or prod) with
-- different admin_users UUIDs across envs, so we store denormalized
-- email/name like support_tickets does (see migration 00027).
--
-- Rollback (manual):
--   DROP TABLE pro_qa_issue_attachments;
--   DROP TABLE pro_qa_issue_comments;
--   DROP TABLE pro_qa_issues;
--   DROP TABLE pro_qa_requested_checks;
--   DROP TABLE pro_qa_info;

CREATE TABLE IF NOT EXISTS pro_qa_info (
    id           INT PRIMARY KEY DEFAULT 1,
    body_md      TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by_email TEXT,
    CONSTRAINT pro_qa_info_singleton CHECK (id = 1)
);
INSERT INTO pro_qa_info (id, body_md) VALUES (1, '') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS pro_qa_requested_checks (
    id           UUID PRIMARY KEY,
    title        TEXT NOT NULL,
    body_md      TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email TEXT,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_checks_sort ON pro_qa_requested_checks (sort_order, created_at);

CREATE TABLE IF NOT EXISTS pro_qa_issues (
    id                 UUID PRIMARY KEY,
    issue_number       SERIAL UNIQUE,
    parent_issue_id    UUID REFERENCES pro_qa_issues(id) ON DELETE SET NULL,
    title              TEXT NOT NULL,
    description_md     TEXT NOT NULL DEFAULT '',
    environment        TEXT,
    platform           TEXT,
    status             TEXT NOT NULL DEFAULT 'open',
    severity           TEXT NOT NULL DEFAULT 'medium',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email   TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_issues_status ON pro_qa_issues (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_pro_qa_issues_parent ON pro_qa_issues (parent_issue_id);

CREATE TABLE IF NOT EXISTS pro_qa_issue_comments (
    id               UUID PRIMARY KEY,
    issue_id         UUID NOT NULL REFERENCES pro_qa_issues(id) ON DELETE CASCADE,
    body_md          TEXT NOT NULL,
    author_email     TEXT,
    author_name      TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    is_status_change BOOLEAN NOT NULL DEFAULT FALSE,
    status_from      TEXT,
    status_to        TEXT
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_comments_issue ON pro_qa_issue_comments (issue_id, created_at);

CREATE TABLE IF NOT EXISTS pro_qa_issue_attachments (
    id              UUID PRIMARY KEY,
    issue_id        UUID NOT NULL REFERENCES pro_qa_issues(id) ON DELETE CASCADE,
    comment_id      UUID REFERENCES pro_qa_issue_comments(id) ON DELETE SET NULL,
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    storage_driver  TEXT NOT NULL,
    storage_path    TEXT NOT NULL,
    uploaded_by_email TEXT,
    uploaded_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pro_qa_attachments_issue ON pro_qa_issue_attachments (issue_id);
