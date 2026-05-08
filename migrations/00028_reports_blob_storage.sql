-- Migration: 00028_reports_blob_storage.sql
-- Description: Move report PDFs off ephemeral EC2 disk by adding the same
-- (storage_driver, storage_path) pair we already use on ticket_attachments.
-- Existing local-disk reports get backfilled so they keep working on dev.
-- Production rows from before the migration may have unreachable PDFs
-- (the bug being fixed); their metadata is preserved either way.
--
-- Rollback (run by hand on the affected DB if reverting this migration):
--
--   ALTER TABLE reports
--       DROP COLUMN IF EXISTS storage_path,
--       DROP COLUMN IF EXISTS storage_driver;

ALTER TABLE reports
    ADD COLUMN IF NOT EXISTS storage_driver VARCHAR(20),
    ADD COLUMN IF NOT EXISTS storage_path   TEXT;

COMMENT ON COLUMN reports.storage_driver IS 'localfs | s3 — which BlobStorage driver wrote the PDF';
COMMENT ON COLUMN reports.storage_path   IS 'Driver-relative path inside the reports namespace (e.g., <reportID>/<uuid>.pdf)';

-- Backfill: legacy rows wrote to local disk under uploads/reports/<filename>.pdf.
-- Post-migration the localfs driver root for the report namespace is also
-- {UploadDir}/reports, so the basename (just <uuid>.pdf, sitting at the root
-- with no subdir) keeps resolving against the localfs driver. Anything that
-- doesn't match this pattern is left null and will surface as 404 instead of
-- 500 — preferable to silently serving the wrong file.
UPDATE reports
SET storage_driver = 'localfs',
    storage_path   = regexp_replace(file_path, '^.*/', '')
WHERE storage_driver IS NULL
  AND file_path IS NOT NULL
  AND file_path ~ '\.pdf$';
