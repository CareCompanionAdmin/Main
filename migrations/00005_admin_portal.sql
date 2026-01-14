-- Migration: 00005_admin_portal.sql
-- Date: 2026-01-13
-- Purpose: Add admin portal support with system roles (super_admin, support, marketing)
-- IMPORTANT: Admin roles are completely isolated from patient data (PHI)

-- ============================================================================
-- SYSTEM ROLES
-- ============================================================================

-- Create system role enum (separate from family roles)
DO $$ BEGIN
    CREATE TYPE system_role AS ENUM ('super_admin', 'support', 'marketing');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Add system_role column to users (NULL for regular users)
ALTER TABLE users ADD COLUMN IF NOT EXISTS system_role system_role DEFAULT NULL;

-- Index for efficient admin user queries
CREATE INDEX IF NOT EXISTS idx_users_system_role ON users(system_role) WHERE system_role IS NOT NULL;

-- ============================================================================
-- SUPPORT TICKETS
-- ============================================================================

-- Support ticket status enum
DO $$ BEGIN
    CREATE TYPE ticket_status AS ENUM ('open', 'in_progress', 'waiting_on_user', 'resolved', 'closed');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Support ticket priority enum
DO $$ BEGIN
    CREATE TYPE ticket_priority AS ENUM ('low', 'normal', 'high', 'urgent');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Support tickets table
CREATE TABLE IF NOT EXISTS support_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    subject VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status ticket_status DEFAULT 'open',
    priority ticket_priority DEFAULT 'normal',
    assigned_to UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    resolved_by UUID REFERENCES users(id) ON DELETE SET NULL
);

-- Ticket messages (conversation thread)
CREATE TABLE IF NOT EXISTS ticket_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id UUID REFERENCES support_tickets(id) ON DELETE CASCADE,
    sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    message TEXT NOT NULL,
    is_internal BOOLEAN DEFAULT FALSE, -- Internal notes not visible to users
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for tickets
CREATE INDEX IF NOT EXISTS idx_support_tickets_user_id ON support_tickets(user_id);
CREATE INDEX IF NOT EXISTS idx_support_tickets_status ON support_tickets(status);
CREATE INDEX IF NOT EXISTS idx_support_tickets_assigned_to ON support_tickets(assigned_to);
CREATE INDEX IF NOT EXISTS idx_support_tickets_created_at ON support_tickets(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket_id ON ticket_messages(ticket_id);

-- ============================================================================
-- SYSTEM METRICS CACHE
-- ============================================================================

-- Cached metrics for marketing dashboard (aggregates only, no PHI)
CREATE TABLE IF NOT EXISTS system_metrics_cache (
    id SERIAL PRIMARY KEY,
    metric_name VARCHAR(100) UNIQUE NOT NULL,
    metric_value JSONB NOT NULL,
    calculated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Pre-populate metric names
INSERT INTO system_metrics_cache (metric_name, metric_value) VALUES
    ('user_counts', '{"total": 0, "active_24h": 0, "active_7d": 0, "new_this_week": 0}'),
    ('family_counts', '{"total": 0}'),
    ('entry_counts', '{"total": 0, "this_week": 0, "avg_per_day": 0}'),
    ('growth_metrics', '{"user_growth_percent": 0, "new_users_this_week": 0, "new_users_last_week": 0}'),
    ('system_health', '{"uptime_percent": 100, "avg_response_ms": 0}')
ON CONFLICT (metric_name) DO NOTHING;

-- ============================================================================
-- ADMIN AUDIT LOG
-- ============================================================================

-- Audit log for all admin actions
CREATE TABLE IF NOT EXISTS admin_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    target_type VARCHAR(50), -- 'user', 'family', 'ticket', 'system', 'admin'
    target_id UUID,
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for audit log
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_admin_id ON admin_audit_log(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_action ON admin_audit_log(action);
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_created_at ON admin_audit_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_log_target ON admin_audit_log(target_type, target_id);

-- ============================================================================
-- SYSTEM SETTINGS
-- ============================================================================

-- System-wide settings managed by super_admin
CREATE TABLE IF NOT EXISTS system_settings (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Default system settings
INSERT INTO system_settings (key, value, description) VALUES
    ('maintenance_mode', '{"enabled": false, "message": "", "allowed_ips": []}', 'System maintenance mode settings'),
    ('metrics_cache_ttl', '{"seconds": 300}', 'How often to refresh marketing metrics (in seconds)'),
    ('support_settings', '{"auto_assign": false, "default_priority": "normal"}', 'Support ticket default settings'),
    ('registration_enabled', '{"enabled": true}', 'Whether new user registration is allowed')
ON CONFLICT (key) DO NOTHING;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON COLUMN users.system_role IS 'System-level admin role (super_admin, support, marketing). NULL for regular users.';
COMMENT ON TABLE support_tickets IS 'Support tickets submitted by users. NO PHI stored here.';
COMMENT ON TABLE ticket_messages IS 'Messages/replies in support tickets. NO PHI should be stored.';
COMMENT ON TABLE system_metrics_cache IS 'Cached aggregate metrics for marketing dashboard. AGGREGATES ONLY, NO PHI.';
COMMENT ON TABLE admin_audit_log IS 'Audit trail of all admin actions for compliance.';
COMMENT ON TABLE system_settings IS 'System-wide configuration managed by super_admin.';
COMMENT ON COLUMN ticket_messages.is_internal IS 'If true, message is internal note visible only to support staff.';

-- ============================================================================
-- ROLLBACK
-- ============================================================================
-- DROP TABLE IF EXISTS system_settings;
-- DROP TABLE IF EXISTS admin_audit_log;
-- DROP TABLE IF EXISTS system_metrics_cache;
-- DROP TABLE IF EXISTS ticket_messages;
-- DROP TABLE IF EXISTS support_tickets;
-- DROP INDEX IF EXISTS idx_users_system_role;
-- ALTER TABLE users DROP COLUMN IF EXISTS system_role;
-- DROP TYPE IF EXISTS ticket_priority;
-- DROP TYPE IF EXISTS ticket_status;
-- DROP TYPE IF EXISTS system_role;
