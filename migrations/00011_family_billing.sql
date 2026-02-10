-- 00011_family_billing.sql
-- Transition from user-based to family-based billing
-- New plans: Single Child ($10/month, 1 child) and Family ($15/month, unlimited)

-- ============================================================================
-- Step 1: Create family_subscriptions table
-- ============================================================================

CREATE TABLE IF NOT EXISTS family_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id UUID NOT NULL UNIQUE REFERENCES families(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES subscription_plans(id),
    status subscription_status NOT NULL DEFAULT 'active',
    current_period_start TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    trial_end TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    stripe_subscription_id VARCHAR(255),
    stripe_customer_id VARCHAR(255),
    promo_code_id UUID REFERENCES promo_codes(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Create indexes for common queries
CREATE INDEX IF NOT EXISTS idx_family_subscriptions_family_id ON family_subscriptions(family_id);
CREATE INDEX IF NOT EXISTS idx_family_subscriptions_plan_id ON family_subscriptions(plan_id);
CREATE INDEX IF NOT EXISTS idx_family_subscriptions_status ON family_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_family_subscriptions_period_end ON family_subscriptions(current_period_end);

-- ============================================================================
-- Step 2: Deactivate all existing subscription plans
-- ============================================================================

UPDATE subscription_plans SET is_active = FALSE;

-- ============================================================================
-- Step 3: Insert the two new plans
-- ============================================================================

-- Single Child Plan: $10/month, max 1 child
INSERT INTO subscription_plans (
    id,
    name,
    description,
    price_cents,
    billing_interval,
    features,
    max_children,
    max_family_members,
    is_active,
    created_at,
    updated_at
) VALUES (
    gen_random_uuid(),
    'Single Child',
    'Perfect for families with one child. Full access to all CareCompanion features.',
    1000,  -- $10.00
    'monthly',
    '{"unlimited_logs": true, "medication_tracking": true, "behavior_tracking": true, "insights": true, "chat": true, "alerts": true}'::jsonb,
    1,
    10,
    TRUE,
    NOW(),
    NOW()
);

-- Family Plan: $15/month, unlimited children
INSERT INTO subscription_plans (
    id,
    name,
    description,
    price_cents,
    billing_interval,
    features,
    max_children,
    max_family_members,
    is_active,
    created_at,
    updated_at
) VALUES (
    gen_random_uuid(),
    'Family',
    'For families with multiple children. Unlimited children with full access to all features.',
    1500,  -- $15.00
    'monthly',
    '{"unlimited_logs": true, "medication_tracking": true, "behavior_tracking": true, "insights": true, "chat": true, "alerts": true, "unlimited_children": true}'::jsonb,
    -1,  -- -1 means unlimited
    10,
    TRUE,
    NOW(),
    NOW()
);

-- ============================================================================
-- Step 4: Assign Family plan to all existing families
-- ============================================================================

INSERT INTO family_subscriptions (
    family_id,
    plan_id,
    status,
    current_period_start,
    current_period_end
)
SELECT
    f.id,
    (SELECT id FROM subscription_plans WHERE name = 'Family' AND is_active = TRUE LIMIT 1),
    'active',
    NOW(),
    '2026-12-31 23:59:59'::timestamptz
FROM families f
WHERE NOT EXISTS (
    SELECT 1 FROM family_subscriptions fs WHERE fs.family_id = f.id
);

-- ============================================================================
-- Step 5: Add trigger for updated_at
-- ============================================================================

CREATE OR REPLACE FUNCTION update_family_subscriptions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_family_subscriptions_updated_at ON family_subscriptions;
CREATE TRIGGER trigger_family_subscriptions_updated_at
    BEFORE UPDATE ON family_subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_family_subscriptions_updated_at();
