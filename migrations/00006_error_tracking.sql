-- Migration: 00006_error_tracking.sql
-- Date: 2026-01-14
-- Purpose: Add error logging table for tracking application errors

-- Error logs table
CREATE TABLE IF NOT EXISTS error_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    error_type VARCHAR(50) NOT NULL, -- 'bad_request', 'server_error', 'auth_error', etc.
    status_code INT NOT NULL,
    path VARCHAR(500) NOT NULL,
    method VARCHAR(10) NOT NULL,
    error_message TEXT,
    stack_trace TEXT,
    user_agent TEXT,
    ip_address INET,
    request_id VARCHAR(100),
    ticket_id UUID REFERENCES support_tickets(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Indexes for error logs
CREATE INDEX IF NOT EXISTS idx_error_logs_created_at ON error_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_error_logs_error_type ON error_logs(error_type);
CREATE INDEX IF NOT EXISTS idx_error_logs_status_code ON error_logs(status_code);
CREATE INDEX IF NOT EXISTS idx_error_logs_user_id ON error_logs(user_id);

-- Response time tracking table
CREATE TABLE IF NOT EXISTS response_time_logs (
    id SERIAL PRIMARY KEY,
    path VARCHAR(500) NOT NULL,
    method VARCHAR(10) NOT NULL,
    response_time_ms FLOAT NOT NULL,
    status_code INT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for response time aggregation (only keep recent data)
CREATE INDEX IF NOT EXISTS idx_response_time_logs_created_at ON response_time_logs(created_at DESC);

-- Comment
COMMENT ON TABLE error_logs IS 'Tracks application errors for monitoring and auto-ticket generation';
COMMENT ON TABLE response_time_logs IS 'Tracks response times for performance monitoring';
