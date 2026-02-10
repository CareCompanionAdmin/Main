-- +goose Up

-- Version log for tracking changes across dev and production environments
CREATE TABLE IF NOT EXISTS version_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    environment VARCHAR(10) NOT NULL CHECK (environment IN ('dev', 'prod')),
    entry_type VARCHAR(20) NOT NULL CHECK (entry_type IN ('feature', 'bugfix', 'improvement', 'refactor', 'security', 'performance', 'other')),
    title VARCHAR(200) NOT NULL,
    description TEXT,
    version VARCHAR(50),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_version_log_environment ON version_log(environment);
CREATE INDEX idx_version_log_entry_type ON version_log(entry_type);
CREATE INDEX idx_version_log_created_at ON version_log(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS version_log;
