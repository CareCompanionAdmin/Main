-- 00027_support_share_across_envs.sql
--
-- Purpose: enable a shared support-tickets database across the dev and prod
-- environments. When SUPPORT_DB_DSN is set on dev, dev points its support
-- repo at prod's RDS. Both environments then read/write the same physical
-- tables for support_tickets / ticket_messages / ticket_attachments.
--
-- Two schema changes are needed to make that safe:
--
--   1. Drop the foreign-key constraints that reference users(id). Dev and
--      prod each have their own users table with different UUIDs for the
--      same person (different signup, different generated id). A ticket
--      created on dev with sender_id=DEV-UUID would otherwise fail the FK
--      check on prod's users table. We keep the columns as plain UUIDs.
--
--   2. Add denormalized email/first_name/last_name columns. Without an FK,
--      we can no longer rely on a JOIN to render the sender. The denorm
--      columns are populated from the local users table at write time, so
--      either side can render any ticket/message regardless of where the
--      author lives.
--
-- The non-user FKs (ticket_messages.ticket_id → support_tickets.id and
-- ticket_attachments.ticket_id → support_tickets.id) are intentionally
-- preserved — both child tables sit in the same shared DB, so referential
-- integrity within the support cluster still holds and CASCADE deletes
-- still work correctly.
--
-- Rollback (run by hand on the affected DB if reverting this migration):
--
--   ALTER TABLE support_tickets
--     ADD CONSTRAINT support_tickets_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE SET NULL,
--     ADD CONSTRAINT support_tickets_assigned_to_fkey FOREIGN KEY (assigned_to) REFERENCES users(id) ON DELETE SET NULL,
--     ADD CONSTRAINT support_tickets_resolved_by_fkey FOREIGN KEY (resolved_by) REFERENCES users(id) ON DELETE SET NULL;
--   ALTER TABLE ticket_messages
--     ADD CONSTRAINT ticket_messages_sender_id_fkey FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE SET NULL;
--   ALTER TABLE ticket_attachments
--     ADD CONSTRAINT ticket_attachments_uploader_id_fkey FOREIGN KEY (uploader_id) REFERENCES users(id) ON DELETE SET NULL;
--   ALTER TABLE support_tickets DROP COLUMN user_email, DROP COLUMN user_first_name, DROP COLUMN user_last_name;
--   ALTER TABLE ticket_messages DROP COLUMN sender_email, DROP COLUMN sender_first_name, DROP COLUMN sender_last_name;
--   ALTER TABLE ticket_attachments DROP COLUMN uploader_email, DROP COLUMN uploader_first_name, DROP COLUMN uploader_last_name;
--   -- Re-adding FKs may fail on rows whose user_id no longer exists locally;
--   -- those rows would need to be deleted or have user_id set to NULL first.
--
-- Note that re-adding the FK constraints during rollback may FAIL if any
-- ticket / message / attachment rows reference user_ids that don't exist
-- in the local users table (which is exactly the situation this migration
-- enables). Plan accordingly: either delete those rows, NULL their
-- user_id, or import the foreign users into the local users table before
-- attempting to restore the FKs.

ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_user_id_fkey;
ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_assigned_to_fkey;
ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_resolved_by_fkey;
ALTER TABLE ticket_messages DROP CONSTRAINT IF EXISTS ticket_messages_sender_id_fkey;
ALTER TABLE ticket_attachments DROP CONSTRAINT IF EXISTS ticket_attachments_uploader_id_fkey;

ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS user_email      varchar(255);
ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS user_first_name varchar(100);
ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS user_last_name  varchar(100);

ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS sender_email      varchar(255);
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS sender_first_name varchar(100);
ALTER TABLE ticket_messages ADD COLUMN IF NOT EXISTS sender_last_name  varchar(100);

ALTER TABLE ticket_attachments ADD COLUMN IF NOT EXISTS uploader_email      varchar(255);
ALTER TABLE ticket_attachments ADD COLUMN IF NOT EXISTS uploader_first_name varchar(100);
ALTER TABLE ticket_attachments ADD COLUMN IF NOT EXISTS uploader_last_name  varchar(100);

-- Backfill existing rows from the local users table. After this migration
-- runs on prod, every existing prod ticket gets its denorm columns filled
-- from prod's users table. After it runs on dev, dev's existing tickets get
-- denorm filled from dev's users table. Once the dev support repo is
-- pointed at prod's DB, the dev backfill becomes irrelevant — dev will
-- simply read prod's denorm values.
UPDATE support_tickets t
   SET user_email      = u.email,
       user_first_name = u.first_name,
       user_last_name  = u.last_name
  FROM users u
 WHERE t.user_id = u.id
   AND t.user_email IS NULL;

UPDATE ticket_messages m
   SET sender_email      = u.email,
       sender_first_name = u.first_name,
       sender_last_name  = u.last_name
  FROM users u
 WHERE m.sender_id = u.id
   AND m.sender_email IS NULL;

UPDATE ticket_attachments a
   SET uploader_email      = u.email,
       uploader_first_name = u.first_name,
       uploader_last_name  = u.last_name
  FROM users u
 WHERE a.uploader_id = u.id
   AND a.uploader_email IS NULL;
