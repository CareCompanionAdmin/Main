-- Migration: 00029_sessions_table.sql
-- Description: Persistent session identity layer. Replaces the unused
-- user_sessions scaffolding from migration 00001 with a sessions table the
-- code actually reads. JWTs gain a sid claim that points at a row here.
-- Revoking a session = setting revoked_at; auth middleware rejects revoked.
--
-- IP is recorded for the Live Sessions admin display only — it is never
-- validated against the request's source IP. Multiple users from the same
-- NAT'd IP must continue to work.
--
-- Rollback (run by hand):
--
--   DROP TABLE IF EXISTS sessions;
--   DROP TYPE  IF EXISTS session_kind;
--   -- (do not restore user_sessions; nothing read it)

DROP TABLE IF EXISTS user_sessions CASCADE;

DO $$ BEGIN
    CREATE TYPE session_kind AS ENUM ('user', 'admin');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind          session_kind NOT NULL,
    system_role   VARCHAR(32),
    family_id     UUID REFERENCES families(id) ON DELETE SET NULL,
    ip_at_start   INET,
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at    TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_active
    ON sessions(user_id) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_kind_active
    ON sessions(kind) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);

COMMENT ON TABLE  sessions IS 'One row per active or revoked user/admin login. JWTs carry sid pointing here.';
COMMENT ON COLUMN sessions.kind IS 'user | admin — selects which cookie name carries the JWT.';
COMMENT ON COLUMN sessions.ip_at_start IS 'Recorded for display only. Never validated against request IP.';
COMMENT ON COLUMN sessions.revoked_at IS 'NULL = active. Set by logout, kill-session, or "another login took over".';
