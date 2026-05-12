-- 00037_correlation_type_automatic.sql
--
-- Adds 'automatic' to the correlation_type enum. The Go model
-- (models/common.go CorrelationTypeAutomatic) has had this value since
-- the auto-correlation scanner shipped, but the original migration
-- 00001 only declared 3 values. Dev was patched by hand at some point;
-- prod was never patched, so the insight generator was failing to
-- create alerts for any family whose source_type resolved to
-- 'automatic' with:
--   ERROR: invalid input value for enum correlation_type: "automatic"
-- Visible in prod startup log 2026-05-09 06:33:54 for Matty / Thomas /
-- Joe / Holly. This migration brings the prod enum in sync with the
-- Go-side definition.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_enum WHERE enumlabel = 'automatic'
                   AND enumtypid = 'correlation_type'::regtype) THEN
        ALTER TYPE correlation_type ADD VALUE 'automatic';
    END IF;
END $$;
