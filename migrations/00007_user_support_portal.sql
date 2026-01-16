-- Migration: 00007_user_support_portal.sql
-- Description: Add support for user-facing ticket portal with read tracking

-- Add last_user_read_at to track when user last viewed ticket messages
ALTER TABLE support_tickets ADD COLUMN IF NOT EXISTS last_user_read_at TIMESTAMPTZ DEFAULT NULL;

-- Add index for efficient unread message queries
CREATE INDEX IF NOT EXISTS idx_ticket_messages_created_at ON ticket_messages(created_at);
CREATE INDEX IF NOT EXISTS idx_support_tickets_last_user_read_at ON support_tickets(last_user_read_at);

-- Comment for documentation
COMMENT ON COLUMN support_tickets.last_user_read_at IS 'Timestamp when the ticket owner last viewed messages. NULL means never read. Used to determine unread support replies.';
