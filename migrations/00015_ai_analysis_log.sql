-- AI Analysis Log - tracks Claude API usage for insight generation
CREATE TABLE IF NOT EXISTS ai_analysis_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id UUID NOT NULL REFERENCES children(id),
    analysis_type VARCHAR(50) NOT NULL,
    model_used VARCHAR(100) NOT NULL,
    input_tokens INT,
    output_tokens INT,
    insights_generated INT DEFAULT 0,
    alerts_generated INT DEFAULT 0,
    error_message TEXT,
    duration_ms INT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_analysis_log_child ON ai_analysis_log(child_id);
CREATE INDEX IF NOT EXISTS idx_ai_analysis_log_created ON ai_analysis_log(created_at);
