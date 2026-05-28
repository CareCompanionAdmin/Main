-- 00041_user_roles.sql
--
-- Purpose: Add database-backed custom admin roles alongside the four
-- hardcoded built-in roles (super_admin / support / marketing / partner).
-- A super-admin can create a new custom role + per-section permissions
-- through the new /admin/user-roles UI, then assign it to admin_users.
--
-- The four built-in roles stay in code (auth/perm.go matrix) — those are
-- the "locked" production-operator permissions. Custom roles live here.
--
-- These tables live on the MAIN DB (not the support DB), so each
-- environment maintains its own custom-role definitions. The Matrix()
-- lookup is local-DB-only.
--
-- Rollback (manual):
--   DROP TABLE IF EXISTS custom_role_permissions;
--   DROP TABLE IF EXISTS custom_roles;

CREATE TABLE IF NOT EXISTS custom_roles (
    id               UUID PRIMARY KEY,
    name             TEXT UNIQUE NOT NULL,
    display_name     TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by_email TEXT,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT custom_roles_name_format
        CHECK (name ~ '^[a-z][a-z0-9_]{1,49}$'),
    CONSTRAINT custom_roles_name_not_builtin
        CHECK (name NOT IN ('super_admin', 'support', 'marketing', 'partner'))
);

CREATE TABLE IF NOT EXISTS custom_role_permissions (
    role_id  UUID NOT NULL REFERENCES custom_roles(id) ON DELETE CASCADE,
    section  TEXT NOT NULL,
    level    TEXT NOT NULL CHECK (level IN ('read', 'write')),
    PRIMARY KEY (role_id, section)
);
