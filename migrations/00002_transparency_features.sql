-- +goose Up
-- Transparency & Progressive Disclosure Features Migration

-- ============================================================================
-- NEW ENUM TYPES
-- ============================================================================

CREATE TYPE citation_type AS ENUM (
    'medication_reference',
    'drug_interaction',
    'global_correlation',
    'dosage_guideline',
    'behavioral_reference',
    'dietary_reference'
);

CREATE TYPE authority_type AS ENUM (
    'government',
    'medical_journal',
    'professional_organization',
    'drug_manufacturer',
    'research_institution'
);

CREATE TYPE factor_type AS ENUM (
    'global_medical',
    'cohort_pattern',
    'family_history',
    'temporal_proximity',
    'amplitude',
    'medication_criticality',
    'clinical_validation'
);

CREATE TYPE change_type AS ENUM (
    'medication_added',
    'medication_discontinued',
    'medication_dose_changed',
    'medication_schedule_changed',
    'medication_switched',
    'diet_plan_started',
    'diet_plan_ended',
    'condition_added',
    'condition_removed'
);

CREATE TYPE interrogative_status AS ENUM (
    'pending',
    'prompted',
    'answered',
    'skipped',
    'expired'
);

CREATE TYPE change_source AS ENUM (
    'self_directed',
    'provider_recommended',
    'other_family_member',
    'prefer_not_to_say'
);

CREATE TYPE analysis_relation AS ENUM (
    'yes_provider_agreed',
    'partially_one_factor',
    'no_different_reason',
    'not_sure'
);

CREATE TYPE validation_strength AS ENUM (
    'strong',
    'moderate',
    'weak',
    'none'
);

CREATE TYPE export_type AS ENUM (
    'pdf_download',
    'share_to_provider',
    'print'
);

CREATE TYPE share_method AS ENUM (
    'in_app',
    'email'
);

CREATE TYPE analysis_view AS ENUM (
    'parent',
    'clinical'
);

CREATE TYPE attachment_type AS ENUM (
    'analysis_snapshot',
    'pdf_export',
    'image',
    'document'
);

-- ============================================================================
-- COHORT TABLES (required for transparency features)
-- ============================================================================

-- Cohorts for population-level pattern analysis
CREATE TABLE IF NOT EXISTS cohorts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(200) NOT NULL,
    description TEXT,

    -- Matching criteria
    age_range_min INTEGER,
    age_range_max INTEGER,
    diagnoses TEXT[],
    medications TEXT[],

    -- Statistics
    member_count INTEGER DEFAULT 0,

    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cohorts_active ON cohorts(is_active);

-- Cohort patterns (aggregated from similar families)
CREATE TABLE IF NOT EXISTS cohort_patterns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cohort_id UUID NOT NULL REFERENCES cohorts(id) ON DELETE CASCADE,

    pattern_type VARCHAR(100) NOT NULL,
    input_factor VARCHAR(100) NOT NULL,
    output_factor VARCHAR(100) NOT NULL,

    correlation_strength DECIMAL(4,3),
    confidence_score DECIMAL(3,2),
    sample_size INTEGER,
    lag_hours INTEGER DEFAULT 0,

    description TEXT,
    supporting_data JSONB,

    -- Validation tracking
    clinical_validations_count INTEGER NOT NULL DEFAULT 0,

    first_detected_at TIMESTAMPTZ,
    last_confirmed_at TIMESTAMPTZ,
    times_confirmed INTEGER DEFAULT 1,

    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_cohort_patterns_cohort ON cohort_patterns(cohort_id);
CREATE INDEX idx_cohort_patterns_active ON cohort_patterns(cohort_id, is_active);

-- ============================================================================
-- CITATIONS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS citations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- What this citation supports
    citation_type citation_type NOT NULL,

    -- Link to source record (polymorphic)
    source_table VARCHAR(50) NOT NULL,
    source_id UUID NOT NULL,

    -- Citation details
    authority_name VARCHAR(200) NOT NULL,
    authority_type authority_type NOT NULL,
    publication_title TEXT NOT NULL,
    publication_section VARCHAR(200),
    publication_date DATE,
    url TEXT,
    excerpt TEXT,

    -- Tracking
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_citations_source ON citations(source_table, source_id);
CREATE INDEX idx_citations_type ON citations(citation_type);

-- ============================================================================
-- ALERT CONFIDENCE FACTORS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_confidence_factors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,

    -- Factor identification
    factor_order SMALLINT NOT NULL,
    factor_type factor_type NOT NULL,

    -- Human-readable description
    description TEXT NOT NULL,

    -- Scoring
    score DECIMAL(3,2) NOT NULL CHECK (score >= 0 AND score <= 1),
    weight DECIMAL(3,2) NOT NULL CHECK (weight >= 0 AND weight <= 1),
    contribution DECIMAL(3,2) NOT NULL,

    -- Source references (one will be populated based on factor_type)
    citation_id UUID REFERENCES citations(id),
    cohort_id UUID REFERENCES cohorts(id),
    family_pattern_id UUID REFERENCES family_patterns(id),

    -- Cohort-specific details (when factor_type = 'cohort_pattern')
    cohort_match_criteria JSONB,
    cohort_sample_size INTEGER,
    cohort_confirmation_rate DECIMAL(3,2),

    -- Family history details (when factor_type = 'family_history')
    family_history_instances JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_confidence_factors_alert ON alert_confidence_factors(alert_id);

-- ============================================================================
-- ALERT ANALYSIS DETAILS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_analysis_details (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,

    -- Algorithm metadata
    engine_version VARCHAR(20) NOT NULL,
    baseline_model VARCHAR(50) NOT NULL,
    analysis_timestamp TIMESTAMPTZ NOT NULL,

    -- Anomaly detection details
    baseline_mean DECIMAL(10,3),
    baseline_stddev DECIMAL(10,3),
    baseline_sample_size INTEGER,
    observed_value DECIMAL(10,3),
    deviation_score DECIMAL(5,2),
    anomaly_threshold DECIMAL(5,2),

    -- Cause search details
    search_window_hours INTEGER,
    data_sources_checked TEXT[],

    -- Alternative causes evaluated
    alternatives_evaluated JSONB,

    -- Temporal analysis
    time_gap_hours DECIMAL(5,2),
    expected_delay_min_hours DECIMAL(5,2),
    expected_delay_max_hours DECIMAL(5,2),
    delay_sources JSONB,

    -- Full calculation details
    calculation_details JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_alert_analysis_details_alert ON alert_analysis_details(alert_id);

-- ============================================================================
-- ALERT COHORT MATCHING TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_cohort_matching (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    matched_cohort_id UUID NOT NULL REFERENCES cohorts(id),

    -- Criteria that were used for matching
    criteria_used JSONB NOT NULL,

    -- Criteria considered but NOT used, with explanations
    criteria_excluded JSONB,

    -- Cohort statistics
    cohort_size INTEGER NOT NULL,
    pattern_presentations INTEGER NOT NULL,
    pattern_confirmations INTEGER NOT NULL,
    pattern_denials INTEGER NOT NULL,
    pattern_no_response INTEGER NOT NULL,
    confirmation_rate DECIMAL(5,4) NOT NULL,

    -- Trend data for chart
    confirmation_trend JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_cohort_matching_alert ON alert_cohort_matching(alert_id);

-- ============================================================================
-- TREATMENT CHANGES TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS treatment_changes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,

    -- What changed
    change_type change_type NOT NULL,

    -- Reference to source record
    source_table VARCHAR(50) NOT NULL,
    source_id UUID NOT NULL,

    -- Change details
    previous_value JSONB,
    new_value JSONB,
    change_summary TEXT NOT NULL,

    -- Who made the change
    changed_by_user_id UUID NOT NULL REFERENCES users(id),

    -- Potential correlation to analysis
    potentially_related_alert_id UUID REFERENCES alerts(id),
    potentially_related_share_thread_id UUID REFERENCES chat_threads(id),
    days_since_analysis_shared INTEGER,

    -- Interrogative status
    interrogative_status interrogative_status NOT NULL DEFAULT 'pending',
    interrogative_prompted_at TIMESTAMPTZ,
    interrogative_answered_at TIMESTAMPTZ,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_treatment_changes_child ON treatment_changes(child_id);
CREATE INDEX idx_treatment_changes_status ON treatment_changes(interrogative_status);
CREATE INDEX idx_treatment_changes_alert ON treatment_changes(potentially_related_alert_id);

-- ============================================================================
-- TREATMENT CHANGE RESPONSES TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS treatment_change_responses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    treatment_change_id UUID NOT NULL REFERENCES treatment_changes(id) ON DELETE CASCADE,
    responded_by_user_id UUID NOT NULL REFERENCES users(id),

    -- Question 1: Who initiated the change?
    change_source change_source NOT NULL,

    -- Question 2: Was it related to our analysis? (only if provider_recommended)
    related_to_analysis analysis_relation,

    -- Optional: Which provider? (never exposed externally)
    provider_user_id UUID REFERENCES users(id),
    provider_name_freetext VARCHAR(100),

    -- Additional context
    notes TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_treatment_change_responses_change ON treatment_change_responses(treatment_change_id);

-- ============================================================================
-- USER INTERACTION PREFERENCES TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_interaction_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- When to ask
    treatment_change_prompt_delay_hours INTEGER NOT NULL DEFAULT 4,

    -- Quiet hours (don't disturb during these times)
    interrogative_quiet_start TIME,
    interrogative_quiet_end TIME,

    -- Which days to ask (0=Sunday, 6=Saturday)
    interrogative_preferred_days INTEGER[] NOT NULL DEFAULT '{0,1,2,3,4,5,6}',

    -- Batching
    batch_interrogatives BOOLEAN NOT NULL DEFAULT FALSE,

    -- Limits
    max_interrogatives_per_day INTEGER NOT NULL DEFAULT 3,

    -- Reminders
    interrogative_reminder_hours INTEGER DEFAULT 24,
    max_interrogative_reminders INTEGER NOT NULL DEFAULT 2,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_user_interaction_preferences_user ON user_interaction_preferences(user_id);

-- ============================================================================
-- ALERT EXPORTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS alert_exports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    exported_by_user_id UUID NOT NULL REFERENCES users(id),

    export_type export_type NOT NULL,

    -- For share_to_provider
    shared_with_user_id UUID REFERENCES users(id),
    shared_via share_method,

    -- Which view was exported
    view_mode analysis_view NOT NULL DEFAULT 'parent',

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alert_exports_alert ON alert_exports(alert_id);
CREATE INDEX idx_alert_exports_user ON alert_exports(exported_by_user_id);

-- ============================================================================
-- CHAT ATTACHMENTS TABLE
-- ============================================================================

CREATE TABLE IF NOT EXISTS chat_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,

    attachment_type attachment_type NOT NULL,

    -- For analysis_snapshot: complete analysis frozen at share time
    snapshot_data JSONB,

    -- For file attachments
    file_name VARCHAR(255),
    file_size_bytes INTEGER,
    mime_type VARCHAR(100),
    storage_path TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_attachments_message ON chat_attachments(message_id);

-- ============================================================================
-- TABLE MODIFICATIONS
-- ============================================================================

-- Add preferred analysis view to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS preferred_analysis_view analysis_view DEFAULT 'parent';

-- Add columns to chat_threads for analysis sharing
ALTER TABLE chat_threads ADD COLUMN IF NOT EXISTS linked_alert_id UUID REFERENCES alerts(id);
ALTER TABLE chat_threads ADD COLUMN IF NOT EXISTS auto_archive BOOLEAN DEFAULT TRUE;

-- Add clinical validations count to family_patterns
ALTER TABLE family_patterns ADD COLUMN IF NOT EXISTS clinical_validations_count INTEGER NOT NULL DEFAULT 0;

-- Update existing clinical_validations table with new columns
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS cohort_pattern_id UUID REFERENCES cohort_patterns(id);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS treatment_change_id UUID REFERENCES treatment_changes(id);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS treatment_change_response_id UUID REFERENCES treatment_change_responses(id);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS family_confidence_boost DECIMAL(3,2);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS cohort_confidence_boost DECIMAL(3,2);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS child_age_at_validation DECIMAL(4,2);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS child_weight_at_validation DECIMAL(5,1);
ALTER TABLE clinical_validations ADD COLUMN IF NOT EXISTS validation_date DATE;

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Calculate validation decay based on time, age, and weight changes
CREATE OR REPLACE FUNCTION calculate_validation_decay(
    p_validation_date DATE,
    p_child_age_at_validation DECIMAL,
    p_current_child_age DECIMAL,
    p_child_weight_at_validation DECIMAL,
    p_current_child_weight DECIMAL,
    p_validation_strength VARCHAR
) RETURNS DECIMAL AS $$
DECLARE
    v_base_weight DECIMAL;
    v_time_decay DECIMAL;
    v_age_decay DECIMAL;
    v_weight_decay DECIMAL;
    v_months_elapsed DECIMAL;
    v_age_difference DECIMAL;
    v_weight_change_pct DECIMAL;
BEGIN
    -- Base weight from validation strength
    v_base_weight := CASE p_validation_strength
        WHEN 'strong' THEN 1.0
        WHEN 'moderate' THEN 0.6
        WHEN 'weak' THEN 0.3
        ELSE 0.0
    END;

    -- Time decay: half-life of 18 months
    v_months_elapsed := EXTRACT(EPOCH FROM (CURRENT_DATE - p_validation_date)) / (30.44 * 24 * 60 * 60);
    v_time_decay := POWER(0.5, v_months_elapsed / 18.0);

    -- Age decay: -15% per year of age difference beyond 2 years
    v_age_difference := p_current_child_age - p_child_age_at_validation;
    IF v_age_difference > 2 THEN
        v_age_decay := 1.0 - (0.15 * (v_age_difference - 2));
        v_age_decay := GREATEST(v_age_decay, 0.3); -- Floor at 30%
    ELSE
        v_age_decay := 1.0;
    END IF;

    -- Weight decay: -20% if weight changed more than 20%
    IF p_child_weight_at_validation > 0 AND p_current_child_weight > 0 THEN
        v_weight_change_pct := ABS(p_current_child_weight - p_child_weight_at_validation) / p_child_weight_at_validation;
        IF v_weight_change_pct > 0.20 THEN
            v_weight_decay := 0.80;
        ELSE
            v_weight_decay := 1.0;
        END IF;
    ELSE
        v_weight_decay := 1.0;
    END IF;

    -- Combined decay with floor of 0.1
    RETURN GREATEST(v_base_weight * v_time_decay * v_age_decay * v_weight_decay, 0.1);
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================================
-- VIEWS
-- ============================================================================

-- Safe view for displaying clinical validations (excludes provider identity)
CREATE OR REPLACE VIEW safe_clinical_validation_display AS
SELECT
    cv.id,
    cv.alert_id,
    cv.pattern_id,
    cv.child_id,
    cv.validation_type,
    cv.treatment_changed,
    cv.treatment_description,
    cv.parent_confirmed,
    cv.parent_confirmed_at,
    cv.validation_strength,
    cv.validation_date,
    cv.created_at,
    tc.change_type,
    tc.change_summary,
    DATE(tc.created_at) as change_date
    -- Deliberately excludes: provider_user_id, provider_name_freetext, specific notes
FROM clinical_validations cv
LEFT JOIN treatment_changes tc ON cv.treatment_change_id = tc.id;

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Update triggers for new tables
CREATE TRIGGER update_cohorts_updated_at BEFORE UPDATE ON cohorts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_cohort_patterns_updated_at BEFORE UPDATE ON cohort_patterns FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_user_interaction_preferences_updated_at BEFORE UPDATE ON user_interaction_preferences FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down

-- Drop views
DROP VIEW IF EXISTS safe_clinical_validation_display;

-- Drop functions
DROP FUNCTION IF EXISTS calculate_validation_decay;

-- Drop triggers
DROP TRIGGER IF EXISTS update_user_interaction_preferences_updated_at ON user_interaction_preferences;
DROP TRIGGER IF EXISTS update_cohort_patterns_updated_at ON cohort_patterns;
DROP TRIGGER IF EXISTS update_cohorts_updated_at ON cohorts;

-- Remove added columns from existing tables
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS cohort_pattern_id;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS treatment_change_id;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS treatment_change_response_id;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS family_confidence_boost;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS cohort_confidence_boost;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS child_age_at_validation;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS child_weight_at_validation;
ALTER TABLE clinical_validations DROP COLUMN IF EXISTS validation_date;

ALTER TABLE family_patterns DROP COLUMN IF EXISTS clinical_validations_count;

ALTER TABLE chat_threads DROP COLUMN IF EXISTS linked_alert_id;
ALTER TABLE chat_threads DROP COLUMN IF EXISTS auto_archive;

ALTER TABLE users DROP COLUMN IF EXISTS preferred_analysis_view;

-- Drop new tables
DROP TABLE IF EXISTS chat_attachments CASCADE;
DROP TABLE IF EXISTS alert_exports CASCADE;
DROP TABLE IF EXISTS user_interaction_preferences CASCADE;
DROP TABLE IF EXISTS treatment_change_responses CASCADE;
DROP TABLE IF EXISTS treatment_changes CASCADE;
DROP TABLE IF EXISTS alert_cohort_matching CASCADE;
DROP TABLE IF EXISTS alert_analysis_details CASCADE;
DROP TABLE IF EXISTS alert_confidence_factors CASCADE;
DROP TABLE IF EXISTS citations CASCADE;
DROP TABLE IF EXISTS cohort_patterns CASCADE;
DROP TABLE IF EXISTS cohorts CASCADE;

-- Drop enum types
DROP TYPE IF EXISTS attachment_type CASCADE;
DROP TYPE IF EXISTS analysis_view CASCADE;
DROP TYPE IF EXISTS share_method CASCADE;
DROP TYPE IF EXISTS export_type CASCADE;
DROP TYPE IF EXISTS validation_strength CASCADE;
DROP TYPE IF EXISTS analysis_relation CASCADE;
DROP TYPE IF EXISTS change_source CASCADE;
DROP TYPE IF EXISTS interrogative_status CASCADE;
DROP TYPE IF EXISTS change_type CASCADE;
DROP TYPE IF EXISTS factor_type CASCADE;
DROP TYPE IF EXISTS authority_type CASCADE;
DROP TYPE IF EXISTS citation_type CASCADE;
