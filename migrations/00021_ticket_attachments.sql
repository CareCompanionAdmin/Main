-- Migration: 00021_ticket_attachments.sql
-- Description: Per-ticket file attachments — screenshots, photos, videos,
-- and in-browser screen+mic recordings. All rows (and the underlying files
-- in object storage) are hard-deleted when the ticket is closed/resolved.

DO $$ BEGIN
    CREATE TYPE attachment_kind AS ENUM ('image', 'video', 'recording', 'other');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS ticket_attachments (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id     UUID NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    uploader_id   UUID          REFERENCES users(id)           ON DELETE SET NULL,
    kind          attachment_kind NOT NULL DEFAULT 'other',
    content_type  VARCHAR(100) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    -- Driver-relative path (localfs: relative to UploadDir/ticket_attachments,
    -- s3: object key inside the bucket). Never user-derived.
    storage_path  TEXT         NOT NULL,
    -- Which storage driver wrote this row. Lets us migrate buckets/dirs later.
    storage_driver VARCHAR(20) NOT NULL DEFAULT 'localfs',
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket ON ticket_attachments(ticket_id);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_uploader ON ticket_attachments(uploader_id);

COMMENT ON TABLE  ticket_attachments IS 'Files attached to a support ticket. Hard-deleted (DB row + storage object) when the ticket transitions to closed or resolved.';
COMMENT ON COLUMN ticket_attachments.kind IS 'image | video | recording (in-browser screen+mic) | other';
COMMENT ON COLUMN ticket_attachments.storage_path IS 'Opaque path within the storage driver. Never user-derived.';
