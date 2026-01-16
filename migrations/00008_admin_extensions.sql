-- +goose Up
-- Admin Portal Extensions: Status, Errors, Financials, Promo Codes

-- ============================================================================
-- 1. Error Logs Enhancement (add acknowledgement tracking)
-- ============================================================================

ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS acknowledged_at TIMESTAMPTZ;
ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS acknowledged_by UUID REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS acknowledged_notes TEXT;
ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS is_deleted BOOLEAN DEFAULT FALSE;
ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
ALTER TABLE error_logs ADD COLUMN IF NOT EXISTS deleted_by UUID REFERENCES users(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_error_logs_acknowledged ON error_logs(acknowledged_at) WHERE acknowledged_at IS NULL AND is_deleted = FALSE;
CREATE INDEX IF NOT EXISTS idx_error_logs_not_deleted ON error_logs(created_at DESC) WHERE is_deleted = FALSE;

-- ============================================================================
-- 2. Billing Enums
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE billing_interval AS ENUM ('monthly', 'yearly', 'lifetime');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE subscription_status AS ENUM ('active', 'cancelled', 'expired', 'past_due', 'trialing', 'paused');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE payment_status AS ENUM ('pending', 'succeeded', 'failed', 'refunded', 'partially_refunded');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE payment_type AS ENUM ('subscription', 'one_time');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE promo_discount_type AS ENUM ('percentage', 'fixed_amount', 'free_trial_days', 'free_months');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE promo_applies_to AS ENUM ('subscription', 'one_time', 'both');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- ============================================================================
-- 3. Subscription Plans
-- ============================================================================

CREATE TABLE IF NOT EXISTS subscription_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price_cents INTEGER NOT NULL,
    billing_interval billing_interval NOT NULL,
    features JSONB DEFAULT '[]',
    max_children INTEGER DEFAULT 5,
    max_family_members INTEGER DEFAULT 10,
    is_active BOOLEAN DEFAULT TRUE,
    stripe_price_id VARCHAR(100),
    stripe_product_id VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscription_plans_active ON subscription_plans(is_active);

-- ============================================================================
-- 4. User Subscriptions
-- ============================================================================

CREATE TABLE IF NOT EXISTS user_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES subscription_plans(id),
    status subscription_status DEFAULT 'active',
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    trial_end TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    stripe_subscription_id VARCHAR(100),
    stripe_customer_id VARCHAR(100),
    promo_code_id UUID,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_subscriptions_user ON user_subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_status ON user_subscriptions(status);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_period_end ON user_subscriptions(current_period_end);
CREATE INDEX IF NOT EXISTS idx_user_subscriptions_stripe ON user_subscriptions(stripe_subscription_id);

-- ============================================================================
-- 5. Payments
-- ============================================================================

CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    payment_type payment_type NOT NULL DEFAULT 'subscription',
    amount_cents INTEGER NOT NULL,
    currency VARCHAR(3) DEFAULT 'USD',
    status payment_status DEFAULT 'pending',
    payment_method VARCHAR(50),
    stripe_payment_intent_id VARCHAR(100),
    stripe_invoice_id VARCHAR(100),
    description TEXT,
    promo_code_id UUID,
    discount_amount_cents INTEGER DEFAULT 0,
    refund_amount_cents INTEGER DEFAULT 0,
    refunded_at TIMESTAMPTZ,
    failure_reason TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_user ON payments(user_id);
CREATE INDEX IF NOT EXISTS idx_payments_subscription ON payments(subscription_id);
CREATE INDEX IF NOT EXISTS idx_payments_created ON payments(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_stripe ON payments(stripe_payment_intent_id);

-- ============================================================================
-- 6. Promo Codes
-- ============================================================================

CREATE TABLE IF NOT EXISTS promo_codes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Basic Info
    code VARCHAR(50) NOT NULL,
    name VARCHAR(100) NOT NULL,
    description TEXT,

    -- Discount Configuration
    discount_type promo_discount_type NOT NULL,
    discount_value DECIMAL(10,2) NOT NULL,
    max_discount_cents INTEGER,
    applies_to promo_applies_to DEFAULT 'both',

    -- Plan Restrictions
    applies_to_plans UUID[],
    applies_to_billing_intervals billing_interval[],
    minimum_purchase_cents INTEGER DEFAULT 0,

    -- User Eligibility
    new_users_only BOOLEAN DEFAULT FALSE,
    existing_users_only BOOLEAN DEFAULT FALSE,
    specific_user_ids UUID[],
    specific_email_domains TEXT[],

    -- Usage Limits
    max_total_uses INTEGER,
    max_uses_per_user INTEGER DEFAULT 1,
    current_total_uses INTEGER DEFAULT 0,

    -- Time Constraints
    starts_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,

    -- Duration (for recurring discounts)
    duration_months INTEGER, -- NULL=one-time, -1=forever, positive=N months

    -- Stacking Rules
    is_stackable BOOLEAN DEFAULT FALSE,
    stackable_with_codes UUID[],

    -- Campaign Tracking
    campaign_name VARCHAR(100),
    campaign_source VARCHAR(50),
    affiliate_id UUID,

    -- Financial Tracking
    total_discount_given_cents BIGINT DEFAULT 0,
    total_revenue_attributed_cents BIGINT DEFAULT 0,

    -- Status
    is_active BOOLEAN DEFAULT TRUE,
    deactivated_at TIMESTAMPTZ,
    deactivated_by UUID REFERENCES users(id) ON DELETE SET NULL,
    deactivation_reason TEXT,

    -- Metadata
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    CONSTRAINT promo_codes_code_unique UNIQUE (code)
);

CREATE INDEX IF NOT EXISTS idx_promo_codes_code ON promo_codes(UPPER(code));
CREATE INDEX IF NOT EXISTS idx_promo_codes_active ON promo_codes(is_active, expires_at);
CREATE INDEX IF NOT EXISTS idx_promo_codes_campaign ON promo_codes(campaign_name);

-- ============================================================================
-- 7. Promo Code Usages
-- ============================================================================

CREATE TABLE IF NOT EXISTS promo_code_usages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    promo_code_id UUID NOT NULL REFERENCES promo_codes(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    subscription_id UUID REFERENCES user_subscriptions(id) ON DELETE SET NULL,
    payment_id UUID REFERENCES payments(id) ON DELETE SET NULL,
    discount_applied_cents INTEGER NOT NULL,
    used_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_promo_code_usages_code ON promo_code_usages(promo_code_id);
CREATE INDEX IF NOT EXISTS idx_promo_code_usages_user ON promo_code_usages(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_code_usages_unique ON promo_code_usages(promo_code_id, user_id, subscription_id) WHERE subscription_id IS NOT NULL;

-- Add FK from user_subscriptions and payments to promo_codes
ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS fk_subscription_promo_code;
ALTER TABLE user_subscriptions ADD CONSTRAINT fk_subscription_promo_code
    FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE SET NULL;

ALTER TABLE payments DROP CONSTRAINT IF EXISTS fk_payment_promo_code;
ALTER TABLE payments ADD CONSTRAINT fk_payment_promo_code
    FOREIGN KEY (promo_code_id) REFERENCES promo_codes(id) ON DELETE SET NULL;

-- ============================================================================
-- 8. Revenue Tracking
-- ============================================================================

CREATE TABLE IF NOT EXISTS daily_revenue_snapshots (
    id SERIAL PRIMARY KEY,
    snapshot_date DATE NOT NULL UNIQUE,
    total_revenue_cents BIGINT DEFAULT 0,
    new_subscriptions INTEGER DEFAULT 0,
    cancelled_subscriptions INTEGER DEFAULT 0,
    upgrades INTEGER DEFAULT 0,
    downgrades INTEGER DEFAULT 0,
    refunds_cents BIGINT DEFAULT 0,
    promo_discounts_cents BIGINT DEFAULT 0,
    calculated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_daily_revenue_date ON daily_revenue_snapshots(snapshot_date DESC);

CREATE TABLE IF NOT EXISTS expected_revenue_calendar (
    id SERIAL PRIMARY KEY,
    expected_date DATE NOT NULL,
    subscription_id UUID NOT NULL REFERENCES user_subscriptions(id) ON DELETE CASCADE,
    expected_amount_cents INTEGER NOT NULL,
    plan_name VARCHAR(100),
    is_renewal BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_expected_revenue_date ON expected_revenue_calendar(expected_date);
CREATE UNIQUE INDEX IF NOT EXISTS idx_expected_revenue_unique ON expected_revenue_calendar(expected_date, subscription_id);

-- ============================================================================
-- 9. Insert default subscription plans
-- ============================================================================

INSERT INTO subscription_plans (name, description, price_cents, billing_interval, features, max_children, max_family_members)
VALUES
    ('Free Trial', 'Try CareCompanion free for 30 days', 0, 'monthly', '["Basic logging", "1 child profile", "2 family members"]', 1, 2),
    ('Basic Monthly', 'Essential features for families', 999, 'monthly', '["Unlimited logging", "3 child profiles", "5 family members", "Basic insights"]', 3, 5),
    ('Premium Monthly', 'Full featured plan', 1999, 'monthly', '["Unlimited logging", "10 child profiles", "Unlimited family members", "Advanced insights", "Priority support"]', 10, 100),
    ('Basic Yearly', 'Essential features - save 17%', 9990, 'yearly', '["Unlimited logging", "3 child profiles", "5 family members", "Basic insights"]', 3, 5),
    ('Premium Yearly', 'Full featured - save 17%', 19990, 'yearly', '["Unlimited logging", "10 child profiles", "Unlimited family members", "Advanced insights", "Priority support"]', 10, 100)
ON CONFLICT DO NOTHING;

-- +goose Down

DROP TABLE IF EXISTS expected_revenue_calendar;
DROP TABLE IF EXISTS daily_revenue_snapshots;
DROP TABLE IF EXISTS promo_code_usages;

ALTER TABLE payments DROP CONSTRAINT IF EXISTS fk_payment_promo_code;
ALTER TABLE user_subscriptions DROP CONSTRAINT IF EXISTS fk_subscription_promo_code;

DROP TABLE IF EXISTS promo_codes;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS user_subscriptions;
DROP TABLE IF EXISTS subscription_plans;

DROP TYPE IF EXISTS promo_applies_to;
DROP TYPE IF EXISTS promo_discount_type;
DROP TYPE IF EXISTS payment_type;
DROP TYPE IF EXISTS payment_status;
DROP TYPE IF EXISTS subscription_status;
DROP TYPE IF EXISTS billing_interval;

ALTER TABLE error_logs DROP COLUMN IF EXISTS acknowledged_at;
ALTER TABLE error_logs DROP COLUMN IF EXISTS acknowledged_by;
ALTER TABLE error_logs DROP COLUMN IF EXISTS acknowledged_notes;
ALTER TABLE error_logs DROP COLUMN IF EXISTS is_deleted;
ALTER TABLE error_logs DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE error_logs DROP COLUMN IF EXISTS deleted_by;

DROP INDEX IF EXISTS idx_error_logs_acknowledged;
DROP INDEX IF EXISTS idx_error_logs_not_deleted;
