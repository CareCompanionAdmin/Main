-- Migration: 00030_partner_role.sql
-- Adds the 'partner' system_role enum value. Partner is a fourth admin role
-- with section-scoped access defined in internal/auth/perm.go (matrix-driven).
--
-- ALTER TYPE ... ADD VALUE is allowed inside a transaction in PostgreSQL 12+;
-- the new value cannot be USED in the same transaction, but adding it and
-- recording the migration row in the schema_migrations table works fine.
--
-- Rollback (run by hand if needed):
--   PostgreSQL doesn't support DROP VALUE on enums. To revert, re-create the
--   type without 'partner' and update the column. Avoid unless absolutely
--   necessary.

ALTER TYPE system_role ADD VALUE IF NOT EXISTS 'partner';
