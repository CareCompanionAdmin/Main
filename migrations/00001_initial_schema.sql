-- +goose Up
-- CareCompanion Database Schema

-- ============================================================================
-- EXTENSIONS
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- ENUM TYPES
-- ============================================================================

CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended', 'pending_verification');
CREATE TYPE family_role AS ENUM ('parent', 'caregiver', 'medical_provider');
CREATE TYPE medication_frequency AS ENUM ('once_daily', 'twice_daily', 'three_times_daily', 'four_times_daily', 'as_needed', 'weekly', 'custom');
CREATE TYPE medication_time_of_day AS ENUM ('morning', 'afternoon', 'evening', 'night', 'with_breakfast', 'with_lunch', 'with_dinner', 'bedtime');
CREATE TYPE log_status AS ENUM ('taken', 'missed', 'skipped', 'partial');
CREATE TYPE alert_severity AS ENUM ('info', 'warning', 'critical');
CREATE TYPE alert_status AS ENUM ('active', 'acknowledged', 'resolved', 'dismissed');
CREATE TYPE mood_level AS ENUM ('1', '2', '3', '4', '5');
CREATE TYPE bristol_scale AS ENUM ('1', '2', '3', '4', '5', '6', '7');
CREATE TYPE sleep_quality AS ENUM ('poor', 'fair', 'good', 'excellent');
CREATE TYPE appetite_level AS ENUM ('none', 'low', 'normal', 'high');
CREATE TYPE correlation_type AS ENUM ('global_medical', 'cohort_pattern', 'family_specific');
CREATE TYPE correlation_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- ============================================================================
-- CORE IDENTITY TABLES
-- ============================================================================

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100) NOT NULL,
    phone VARCHAR(20),
    timezone VARCHAR(50) DEFAULT 'America/Chicago',
    status user_status DEFAULT 'pending_verification',
    email_verified_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_status ON users(status);

-- Families table
CREATE TABLE families (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Family memberships (users can belong to multiple families)
CREATE TABLE family_memberships (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role family_role NOT NULL,
    permissions JSONB DEFAULT '{}',
    invited_by UUID REFERENCES users(id),
    invited_at TIMESTAMPTZ,
    accepted_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(family_id, user_id)
);

CREATE INDEX idx_family_memberships_user ON family_memberships(user_id);
CREATE INDEX idx_family_memberships_family ON family_memberships(family_id);

-- Children table
CREATE TABLE children (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    first_name VARCHAR(100) NOT NULL,
    last_name VARCHAR(100),
    date_of_birth DATE NOT NULL,
    gender VARCHAR(20),
    photo_url VARCHAR(500),
    notes TEXT,
    settings JSONB DEFAULT '{}',
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_children_family ON children(family_id);

-- Child conditions (diagnoses)
CREATE TABLE child_conditions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    condition_name VARCHAR(255) NOT NULL,
    icd_code VARCHAR(20),
    diagnosed_date DATE,
    diagnosed_by VARCHAR(255),
    severity VARCHAR(50),
    notes TEXT,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_child_conditions_child ON child_conditions(child_id);

-- ============================================================================
-- MEDICATION TABLES
-- ============================================================================

-- Medication reference (master list)
CREATE TABLE medication_reference (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    generic_name VARCHAR(255),
    drug_class VARCHAR(100),
    common_dosages TEXT[],
    common_side_effects TEXT[],
    warnings TEXT[],
    interactions JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_medication_reference_name ON medication_reference(name);

-- Child medications (prescribed to specific child)
CREATE TABLE medications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    reference_id UUID REFERENCES medication_reference(id),
    name VARCHAR(255) NOT NULL,
    dosage VARCHAR(100) NOT NULL,
    dosage_unit VARCHAR(50) NOT NULL,
    frequency medication_frequency NOT NULL,
    instructions TEXT,
    prescriber VARCHAR(255),
    pharmacy VARCHAR(255),
    start_date DATE,
    end_date DATE,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_medications_child ON medications(child_id);
CREATE INDEX idx_medications_active ON medications(child_id, is_active);

-- Medication schedules (when to take)
CREATE TABLE medication_schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    medication_id UUID NOT NULL REFERENCES medications(id) ON DELETE CASCADE,
    time_of_day medication_time_of_day NOT NULL,
    scheduled_time TIME,
    days_of_week INTEGER[] DEFAULT '{0,1,2,3,4,5,6}',
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_medication_schedules_med ON medication_schedules(medication_id);

-- Medication logs (daily tracking)
CREATE TABLE medication_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    medication_id UUID NOT NULL REFERENCES medications(id) ON DELETE CASCADE,
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    schedule_id UUID REFERENCES medication_schedules(id),
    log_date DATE NOT NULL,
    scheduled_time TIME,
    actual_time TIME,
    status log_status NOT NULL,
    dosage_given VARCHAR(100),
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_medication_logs_child_date ON medication_logs(child_id, log_date);
CREATE INDEX idx_medication_logs_medication ON medication_logs(medication_id, log_date);

-- ============================================================================
-- DAILY LOG TABLES
-- ============================================================================

-- Behavior logs
CREATE TABLE behavior_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    log_time TIME,
    mood_level mood_level,
    energy_level INTEGER CHECK (energy_level BETWEEN 1 AND 5),
    anxiety_level INTEGER CHECK (anxiety_level BETWEEN 1 AND 5),
    meltdowns INTEGER DEFAULT 0,
    stimming_episodes INTEGER DEFAULT 0,
    aggression_incidents INTEGER DEFAULT 0,
    self_injury_incidents INTEGER DEFAULT 0,
    triggers TEXT[],
    positive_behaviors TEXT[],
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_behavior_logs_child_date ON behavior_logs(child_id, log_date);

-- Bowel logs
CREATE TABLE bowel_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    log_time TIME,
    bristol_scale bristol_scale,
    had_accident BOOLEAN DEFAULT false,
    pain_level INTEGER CHECK (pain_level BETWEEN 0 AND 10),
    blood_present BOOLEAN DEFAULT false,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bowel_logs_child_date ON bowel_logs(child_id, log_date);

-- Speech logs
CREATE TABLE speech_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    verbal_output_level INTEGER CHECK (verbal_output_level BETWEEN 1 AND 5),
    clarity_level INTEGER CHECK (clarity_level BETWEEN 1 AND 5),
    new_words TEXT[],
    lost_words TEXT[],
    echolalia_level INTEGER CHECK (echolalia_level BETWEEN 0 AND 5),
    communication_attempts INTEGER,
    successful_communications INTEGER,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_speech_logs_child_date ON speech_logs(child_id, log_date);

-- Diet logs
CREATE TABLE diet_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    meal_type VARCHAR(50),
    meal_time TIME,
    foods_eaten TEXT[],
    foods_refused TEXT[],
    appetite_level appetite_level,
    water_intake_oz INTEGER,
    supplements_taken TEXT[],
    new_food_tried VARCHAR(255),
    allergic_reaction BOOLEAN DEFAULT false,
    reaction_details TEXT,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_diet_logs_child_date ON diet_logs(child_id, log_date);

-- Weight logs
CREATE TABLE weight_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    weight_lbs DECIMAL(5,2),
    height_inches DECIMAL(5,2),
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_weight_logs_child_date ON weight_logs(child_id, log_date);

-- Sleep logs
CREATE TABLE sleep_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    bedtime TIME,
    wake_time TIME,
    total_sleep_minutes INTEGER,
    night_wakings INTEGER DEFAULT 0,
    sleep_quality sleep_quality,
    took_sleep_aid BOOLEAN DEFAULT false,
    sleep_aid_name VARCHAR(255),
    nightmares BOOLEAN DEFAULT false,
    bed_wetting BOOLEAN DEFAULT false,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sleep_logs_child_date ON sleep_logs(child_id, log_date);

-- Sensory logs
CREATE TABLE sensory_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    log_time TIME,
    sensory_seeking_behaviors TEXT[],
    sensory_avoiding_behaviors TEXT[],
    overload_triggers TEXT[],
    calming_strategies_used TEXT[],
    overload_episodes INTEGER DEFAULT 0,
    overall_regulation INTEGER CHECK (overall_regulation BETWEEN 1 AND 5),
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_sensory_logs_child_date ON sensory_logs(child_id, log_date);

-- Social logs
CREATE TABLE social_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    eye_contact_level INTEGER CHECK (eye_contact_level BETWEEN 1 AND 5),
    social_engagement_level INTEGER CHECK (social_engagement_level BETWEEN 1 AND 5),
    peer_interactions INTEGER DEFAULT 0,
    positive_interactions INTEGER DEFAULT 0,
    conflicts INTEGER DEFAULT 0,
    parallel_play_minutes INTEGER,
    cooperative_play_minutes INTEGER,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_social_logs_child_date ON social_logs(child_id, log_date);

-- Therapy logs
CREATE TABLE therapy_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    therapy_type VARCHAR(100),
    therapist_name VARCHAR(255),
    duration_minutes INTEGER,
    goals_worked_on TEXT[],
    progress_notes TEXT,
    homework_assigned TEXT,
    parent_notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_therapy_logs_child_date ON therapy_logs(child_id, log_date);

-- Seizure logs
CREATE TABLE seizure_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    log_time TIME NOT NULL,
    seizure_type VARCHAR(100),
    duration_seconds INTEGER,
    triggers TEXT[],
    warning_signs TEXT[],
    post_ictal_symptoms TEXT[],
    rescue_medication_given BOOLEAN DEFAULT false,
    rescue_medication_name VARCHAR(255),
    called_911 BOOLEAN DEFAULT false,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_seizure_logs_child_date ON seizure_logs(child_id, log_date);

-- Health event logs (illness, doctor visits, etc.)
CREATE TABLE health_event_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    log_date DATE NOT NULL,
    event_type VARCHAR(100),
    description TEXT,
    symptoms TEXT[],
    temperature_f DECIMAL(4,1),
    provider_name VARCHAR(255),
    diagnosis VARCHAR(255),
    treatment TEXT,
    follow_up_date DATE,
    notes TEXT,
    logged_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_health_event_logs_child_date ON health_event_logs(child_id, log_date);

-- ============================================================================
-- ALERT TABLES
-- ============================================================================

-- Alerts
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    alert_type VARCHAR(100) NOT NULL,
    severity alert_severity NOT NULL,
    status alert_status DEFAULT 'active',
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    data JSONB DEFAULT '{}',
    correlation_id UUID,
    source_type correlation_type,
    confidence_score DECIMAL(3,2),
    date_range_start DATE,
    date_range_end DATE,
    acknowledged_by UUID REFERENCES users(id),
    acknowledged_at TIMESTAMPTZ,
    resolved_by UUID REFERENCES users(id),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_alerts_child ON alerts(child_id);
CREATE INDEX idx_alerts_family ON alerts(family_id);
CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_created ON alerts(created_at DESC);

-- Alert feedback (user feedback on alert usefulness)
CREATE TABLE alert_feedback (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    was_helpful BOOLEAN,
    feedback_text TEXT,
    action_taken VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- CORRELATION ENGINE TABLES
-- ============================================================================

-- Child baselines (what's "normal" for this child)
CREATE TABLE child_baselines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    metric_name VARCHAR(100) NOT NULL,
    baseline_value DECIMAL(10,4),
    std_deviation DECIMAL(10,4),
    sample_size INTEGER,
    calculated_at TIMESTAMPTZ DEFAULT NOW(),
    valid_until TIMESTAMPTZ,
    UNIQUE(child_id, metric_name)
);

CREATE INDEX idx_child_baselines_child ON child_baselines(child_id);

-- Correlation requests (on-demand analysis)
CREATE TABLE correlation_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    requested_by UUID NOT NULL REFERENCES users(id),
    status correlation_status DEFAULT 'pending',
    input_factors TEXT[],
    output_factors TEXT[],
    date_range_start DATE,
    date_range_end DATE,
    results JSONB,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_correlation_requests_child ON correlation_requests(child_id);
CREATE INDEX idx_correlation_requests_status ON correlation_requests(status);

-- Family patterns (patterns specific to this family/child)
CREATE TABLE family_patterns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    pattern_type VARCHAR(100) NOT NULL,
    input_factor VARCHAR(100) NOT NULL,
    output_factor VARCHAR(100) NOT NULL,
    correlation_strength DECIMAL(4,3),
    confidence_score DECIMAL(3,2),
    sample_size INTEGER,
    lag_hours INTEGER DEFAULT 0,
    description TEXT,
    supporting_data JSONB,
    first_detected_at TIMESTAMPTZ,
    last_confirmed_at TIMESTAMPTZ,
    times_confirmed INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_family_patterns_child ON family_patterns(child_id);
CREATE INDEX idx_family_patterns_active ON family_patterns(child_id, is_active);

-- Clinical validations (implicit endorsement tracking)
CREATE TABLE clinical_validations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pattern_id UUID REFERENCES family_patterns(id),
    alert_id UUID REFERENCES alerts(id),
    child_id UUID NOT NULL REFERENCES children(id) ON DELETE CASCADE,
    provider_user_id UUID REFERENCES users(id),
    validation_type VARCHAR(50),
    treatment_changed BOOLEAN DEFAULT false,
    treatment_description TEXT,
    parent_confirmed BOOLEAN,
    parent_confirmed_at TIMESTAMPTZ,
    validation_strength DECIMAL(3,2),
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_clinical_validations_pattern ON clinical_validations(pattern_id);
CREATE INDEX idx_clinical_validations_child ON clinical_validations(child_id);

-- ============================================================================
-- CHAT TABLES
-- ============================================================================

-- Chat threads
CREATE TABLE chat_threads (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
    child_id UUID REFERENCES children(id) ON DELETE SET NULL,
    title VARCHAR(255),
    thread_type VARCHAR(50) DEFAULT 'general',
    related_alert_id UUID REFERENCES alerts(id),
    is_active BOOLEAN DEFAULT true,
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chat_threads_family ON chat_threads(family_id);

-- Chat participants
CREATE TABLE chat_participants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role family_role NOT NULL,
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    last_read_at TIMESTAMPTZ,
    is_active BOOLEAN DEFAULT true,
    UNIQUE(thread_id, user_id)
);

-- Chat messages
CREATE TABLE chat_messages (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    thread_id UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id),
    message_text TEXT NOT NULL,
    attachments JSONB DEFAULT '[]',
    is_edited BOOLEAN DEFAULT false,
    edited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_chat_messages_thread ON chat_messages(thread_id, created_at);

-- ============================================================================
-- SESSION & DEVICE TABLES
-- ============================================================================

-- User sessions
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id UUID REFERENCES families(id),
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    device_info JSONB,
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    last_used_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_user_sessions_user ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_token ON user_sessions(token_hash);
CREATE INDEX idx_user_sessions_expires ON user_sessions(expires_at);

-- Notification preferences
CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id UUID REFERENCES families(id) ON DELETE CASCADE,
    medication_reminders BOOLEAN DEFAULT true,
    missed_medication_alerts BOOLEAN DEFAULT true,
    behavior_change_alerts BOOLEAN DEFAULT true,
    pattern_discovery_alerts BOOLEAN DEFAULT true,
    chat_notifications BOOLEAN DEFAULT true,
    daily_summary BOOLEAN DEFAULT false,
    weekly_summary BOOLEAN DEFAULT true,
    quiet_hours_start TIME,
    quiet_hours_end TIME,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, family_id)
);

-- ============================================================================
-- AUDIT LOG
-- ============================================================================

CREATE TABLE audit_log (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id),
    family_id UUID REFERENCES families(id),
    action VARCHAR(100) NOT NULL,
    entity_type VARCHAR(100),
    entity_id UUID,
    old_values JSONB,
    new_values JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_log_user ON audit_log(user_id);
CREATE INDEX idx_audit_log_family ON audit_log(family_id);
CREATE INDEX idx_audit_log_created ON audit_log(created_at DESC);

-- ============================================================================
-- FUNCTIONS & TRIGGERS
-- ============================================================================

-- Updated at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply updated_at trigger to all relevant tables
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_families_updated_at BEFORE UPDATE ON families FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_family_memberships_updated_at BEFORE UPDATE ON family_memberships FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_children_updated_at BEFORE UPDATE ON children FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_medications_updated_at BEFORE UPDATE ON medications FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_medication_logs_updated_at BEFORE UPDATE ON medication_logs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_behavior_logs_updated_at BEFORE UPDATE ON behavior_logs FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_alerts_updated_at BEFORE UPDATE ON alerts FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_family_patterns_updated_at BEFORE UPDATE ON family_patterns FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_chat_threads_updated_at BEFORE UPDATE ON chat_threads FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TABLE IF EXISTS audit_log CASCADE;
DROP TABLE IF EXISTS notification_preferences CASCADE;
DROP TABLE IF EXISTS user_sessions CASCADE;
DROP TABLE IF EXISTS chat_messages CASCADE;
DROP TABLE IF EXISTS chat_participants CASCADE;
DROP TABLE IF EXISTS chat_threads CASCADE;
DROP TABLE IF EXISTS clinical_validations CASCADE;
DROP TABLE IF EXISTS family_patterns CASCADE;
DROP TABLE IF EXISTS correlation_requests CASCADE;
DROP TABLE IF EXISTS child_baselines CASCADE;
DROP TABLE IF EXISTS alert_feedback CASCADE;
DROP TABLE IF EXISTS alerts CASCADE;
DROP TABLE IF EXISTS health_event_logs CASCADE;
DROP TABLE IF EXISTS seizure_logs CASCADE;
DROP TABLE IF EXISTS therapy_logs CASCADE;
DROP TABLE IF EXISTS social_logs CASCADE;
DROP TABLE IF EXISTS sensory_logs CASCADE;
DROP TABLE IF EXISTS sleep_logs CASCADE;
DROP TABLE IF EXISTS weight_logs CASCADE;
DROP TABLE IF EXISTS diet_logs CASCADE;
DROP TABLE IF EXISTS speech_logs CASCADE;
DROP TABLE IF EXISTS bowel_logs CASCADE;
DROP TABLE IF EXISTS behavior_logs CASCADE;
DROP TABLE IF EXISTS medication_logs CASCADE;
DROP TABLE IF EXISTS medication_schedules CASCADE;
DROP TABLE IF EXISTS medications CASCADE;
DROP TABLE IF EXISTS medication_reference CASCADE;
DROP TABLE IF EXISTS child_conditions CASCADE;
DROP TABLE IF EXISTS children CASCADE;
DROP TABLE IF EXISTS family_memberships CASCADE;
DROP TABLE IF EXISTS families CASCADE;
DROP TABLE IF EXISTS users CASCADE;

DROP TYPE IF EXISTS correlation_status CASCADE;
DROP TYPE IF EXISTS correlation_type CASCADE;
DROP TYPE IF EXISTS appetite_level CASCADE;
DROP TYPE IF EXISTS sleep_quality CASCADE;
DROP TYPE IF EXISTS bristol_scale CASCADE;
DROP TYPE IF EXISTS mood_level CASCADE;
DROP TYPE IF EXISTS alert_status CASCADE;
DROP TYPE IF EXISTS alert_severity CASCADE;
DROP TYPE IF EXISTS log_status CASCADE;
DROP TYPE IF EXISTS medication_time_of_day CASCADE;
DROP TYPE IF EXISTS medication_frequency CASCADE;
DROP TYPE IF EXISTS family_role CASCADE;
DROP TYPE IF EXISTS user_status CASCADE;
