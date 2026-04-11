-- +goose Up
CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id),
    title VARCHAR(255) NOT NULL,
    report_type VARCHAR(20) NOT NULL DEFAULT 'on_demand'
        CHECK (report_type IN ('on_demand', 'scheduled')),
    period_type VARCHAR(10) NOT NULL
        CHECK (period_type IN ('day', 'week', 'month', 'custom')),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    data_filters TEXT[] NOT NULL DEFAULT '{}',
    file_path VARCHAR(500),
    file_size BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'generating', 'completed', 'failed')),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_reports_child ON reports(child_id);
CREATE INDEX idx_reports_family ON reports(family_id);
CREATE INDEX idx_reports_created_by ON reports(created_by);

CREATE TABLE scheduled_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id),
    frequency VARCHAR(10) NOT NULL
        CHECK (frequency IN ('daily', 'weekly', 'monthly')),
    data_filters TEXT[] NOT NULL DEFAULT '{}',
    recipients UUID[] NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scheduled_reports_next_run ON scheduled_reports(next_run_at) WHERE is_active = true;
CREATE INDEX idx_scheduled_reports_child ON scheduled_reports(child_id);

CREATE TRIGGER update_scheduled_reports_updated_at
    BEFORE UPDATE ON scheduled_reports
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS scheduled_reports;
DROP TABLE IF EXISTS reports;
