-- Migration: 00010_dev_mode_settings.sql
-- Development Mode: SSH access control from admin UI

-- Dev mode settings (singleton pattern - only one row)
CREATE TABLE dev_mode_settings (
    id TEXT PRIMARY KEY DEFAULT 'singleton',
    is_enabled BOOLEAN NOT NULL DEFAULT false,
    allowed_ip TEXT,
    sg_rule_id TEXT,
    enabled_by UUID REFERENCES users(id),
    enabled_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert singleton row
INSERT INTO dev_mode_settings (id, is_enabled) VALUES ('singleton', false);

-- Index for quick lookups
CREATE INDEX idx_dev_mode_settings_enabled ON dev_mode_settings(is_enabled);
