-- App Store Demo Account seed
--
-- Creates appreview@mycarecompanion.net with one child, 7 days of varied
-- logs, an active medication schedule, a complimentary subscription, and
-- one family-chat thread with sample messages.
--
-- Safe to run multiple times on dev (uses fixed UUIDs + ON CONFLICT DO NOTHING).
-- For prod: REQUIRES explicit user approval (touches families + subscriptions).
-- The bcrypt hash below was generated for password "MyCareReview2026!" via
-- Python's bcrypt (cost=10). Rotate the password by regenerating the hash.

BEGIN;

-- ---------------------------------------------------------------------------
-- Fixed UUIDs so re-runs are idempotent (no orphan duplicates if re-applied).
-- ---------------------------------------------------------------------------
\set reviewer_id       'a99e5e51-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set family_id         'a99e5e52-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set child_id          'a99e5e53-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set medication_id     'a99e5e54-d6b3-4a8a-9c5e-1d3c4e5f6a7b'
\set thread_id         'a99e5e55-d6b3-4a8a-9c5e-1d3c4e5f6a7b'

-- ---------------------------------------------------------------------------
-- Reviewer user (email is keyed UNIQUE so ON CONFLICT keeps re-runs safe).
-- ---------------------------------------------------------------------------
INSERT INTO app_users (id, email, password_hash, first_name, last_name, status, email_verified_at, timezone)
VALUES (
    :'reviewer_id',
    'appreview@mycarecompanion.net',
    '$2b$10$76YwSiQeywo5YT2JC4pB0eMWd3lXMla2I3oiGHLKOMPARF.5WKgJC',
    'App',
    'Reviewer',
    'active',
    NOW(),
    'America/Chicago'
)
ON CONFLICT (email) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    status        = 'active',
    email_verified_at = COALESCE(app_users.email_verified_at, NOW());

-- ---------------------------------------------------------------------------
-- Family. created_by points to the reviewer — that designates them primary.
-- ---------------------------------------------------------------------------
INSERT INTO families (id, name, created_by)
VALUES (:'family_id', 'Demo Family', :'reviewer_id')
ON CONFLICT (id) DO NOTHING;

INSERT INTO family_memberships (family_id, user_id, role, accepted_at, is_active)
VALUES (:'family_id', :'reviewer_id', 'parent', NOW(), TRUE)
ON CONFLICT (family_id, user_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Child: Alex, age 7 (DOB 2018-09-12).
-- ---------------------------------------------------------------------------
INSERT INTO children (id, family_id, first_name, last_name, date_of_birth, gender, notes, is_active)
VALUES (
    :'child_id',
    :'family_id',
    'Alex',
    'Demo',
    '2018-09-12',
    'nonbinary',
    'Sample child profile for App Store review.',
    TRUE
)
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Medication: methylphenidate 10mg, twice daily.
-- ---------------------------------------------------------------------------
INSERT INTO medications (id, child_id, name, dosage, dosage_unit, frequency, instructions, prescriber, start_date, is_active)
VALUES (
    :'medication_id',
    :'child_id',
    'Methylphenidate',
    '10',
    'mg',
    'twice_daily',
    'Take one tablet with breakfast and one tablet with afternoon snack.',
    'Dr. Sample Pediatrician',
    CURRENT_DATE - INTERVAL '60 days',
    TRUE
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO medication_schedules (medication_id, time_of_day, scheduled_time)
SELECT :'medication_id', 'morning', TIME '07:30'
WHERE NOT EXISTS (
    SELECT 1 FROM medication_schedules WHERE medication_id = :'medication_id' AND time_of_day = 'morning'
);
INSERT INTO medication_schedules (medication_id, time_of_day, scheduled_time)
SELECT :'medication_id', 'afternoon', TIME '15:30'
WHERE NOT EXISTS (
    SELECT 1 FROM medication_schedules WHERE medication_id = :'medication_id' AND time_of_day = 'afternoon'
);

-- ---------------------------------------------------------------------------
-- Comp'd subscription on the Single Child plan. The 'comped' status was
-- added in migration 00024 specifically for accounts like this.
-- ---------------------------------------------------------------------------
-- comped_by FKs to admin_users (not app_users) — leave NULL when seeded
-- via SQL. A real admin comp goes through the admin UI which fills this in.
INSERT INTO family_subscriptions (family_id, plan_id, status, current_period_start, current_period_end, comp_reason, comp_until)
SELECT
    :'family_id',
    (SELECT id FROM subscription_plans WHERE name = 'Single Child' AND is_active = TRUE LIMIT 1),
    'comped',
    NOW(),
    NOW() + INTERVAL '5 years',
    'App Store reviewer demo account',
    NOW() + INTERVAL '5 years'
ON CONFLICT (family_id) DO UPDATE SET
    status = 'comped',
    comp_reason = EXCLUDED.comp_reason,
    comp_until = EXCLUDED.comp_until,
    current_period_end = EXCLUDED.current_period_end;

-- ---------------------------------------------------------------------------
-- 7 days of behavior logs — varied mood/energy/anxiety, occasional meltdowns,
-- a mix of triggers and positive behaviors. Dates count back from today.
-- ---------------------------------------------------------------------------
INSERT INTO behavior_logs (child_id, log_date, log_time, mood_level, energy_level, anxiety_level, meltdowns, stimming_episodes, triggers, positive_behaviors, notes, logged_by)
SELECT :'child_id', CURRENT_DATE - i, TIME '18:00',
    -- mood: cycle good/hard days
    CASE (i % 7) WHEN 0 THEN 8 WHEN 1 THEN 6 WHEN 2 THEN 4 WHEN 3 THEN 7 WHEN 4 THEN 9 WHEN 5 THEN 5 ELSE 7 END,
    CASE (i % 7) WHEN 0 THEN 7 WHEN 1 THEN 5 WHEN 2 THEN 3 WHEN 3 THEN 6 WHEN 4 THEN 8 WHEN 5 THEN 4 ELSE 6 END,
    CASE (i % 7) WHEN 0 THEN 3 WHEN 1 THEN 5 WHEN 2 THEN 7 WHEN 3 THEN 4 WHEN 4 THEN 2 WHEN 5 THEN 6 ELSE 4 END,
    CASE (i % 7) WHEN 2 THEN 1 WHEN 5 THEN 1 ELSE 0 END,
    CASE (i % 7) WHEN 0 THEN 1 WHEN 1 THEN 2 WHEN 2 THEN 4 WHEN 3 THEN 2 WHEN 4 THEN 1 WHEN 5 THEN 3 ELSE 2 END,
    CASE (i % 7) WHEN 2 THEN ARRAY['loud_noise', 'change_in_routine'] WHEN 5 THEN ARRAY['transition'] ELSE ARRAY[]::text[] END,
    CASE (i % 7) WHEN 0 THEN ARRAY['shared_toy', 'used_words'] WHEN 4 THEN ARRAY['flexibility', 'cooperation'] ELSE ARRAY['eye_contact']::text[] END,
    CASE (i % 7) WHEN 2 THEN 'Hard afternoon — loud cafeteria triggered meltdown. Calmed with sensory break.' WHEN 4 THEN 'Great day. Adapted well to schedule change.' ELSE NULL END,
    :'reviewer_id'
FROM generate_series(0, 6) AS i
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 7 days of sleep logs.
-- ---------------------------------------------------------------------------
INSERT INTO sleep_logs (child_id, log_date, bedtime, wake_time, total_sleep_minutes, night_wakings, sleep_quality, notes, logged_by, time_scope)
SELECT :'child_id', CURRENT_DATE - i,
    TIME '20:30',
    TIME '06:45',
    -- ~10h with some variation
    CASE (i % 7) WHEN 2 THEN 540 WHEN 5 THEN 555 ELSE 615 END,
    CASE (i % 7) WHEN 2 THEN 2 WHEN 5 THEN 1 ELSE 0 END,
    CASE (i % 7) WHEN 0 THEN 'good'::sleep_quality WHEN 2 THEN 'poor'::sleep_quality WHEN 4 THEN 'excellent'::sleep_quality WHEN 5 THEN 'fair'::sleep_quality ELSE 'good'::sleep_quality END,
    CASE (i % 7) WHEN 2 THEN 'Restless — woke twice asking about tomorrow''s field trip.' ELSE NULL END,
    :'reviewer_id', 'day'
FROM generate_series(0, 6) AS i
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- Chat thread + two sample messages from the reviewer's own account so the
-- thread is non-empty when they tap into Chat.
-- ---------------------------------------------------------------------------
INSERT INTO chat_threads (id, family_id, title, created_by, thread_type)
VALUES (:'thread_id', :'family_id', 'Care team', :'reviewer_id', 'general')
ON CONFLICT (id) DO NOTHING;

INSERT INTO chat_participants (thread_id, user_id, role)
VALUES (:'thread_id', :'reviewer_id', 'parent')
ON CONFLICT DO NOTHING;

INSERT INTO chat_messages (thread_id, sender_id, message_text, created_at)
SELECT :'thread_id', :'reviewer_id', 'Welcome to MyCareCompanion! This is your family''s shared chat for coordinating care.', NOW() - INTERVAL '2 days'
WHERE NOT EXISTS (
    SELECT 1 FROM chat_messages WHERE thread_id = :'thread_id' AND sender_id = :'reviewer_id' LIMIT 1
);

INSERT INTO chat_messages (thread_id, sender_id, message_text, created_at)
SELECT :'thread_id', :'reviewer_id', 'Try logging today''s behavior in the daily logs view, then check Insights to see how patterns surface over time.', NOW() - INTERVAL '1 day'
WHERE (SELECT COUNT(*) FROM chat_messages WHERE thread_id = :'thread_id') < 2;

COMMIT;

SELECT 'Demo account seeded: appreview@mycarecompanion.net / MyCareReview2026!' AS result;
