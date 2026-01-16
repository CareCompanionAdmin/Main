-- Migration: 00009_marketing_materials.sql
-- Marketing Materials Center: Brand configuration, assets, and social templates

-- Brand configuration (editable by super_admin)
CREATE TABLE brand_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Core Identity
    app_name VARCHAR(100) NOT NULL DEFAULT 'CareCompanion',
    tagline VARCHAR(255) NOT NULL DEFAULT 'Finally, One Place for Everything',
    mission_statement TEXT,

    -- Colors (hex values)
    primary_color VARCHAR(7) DEFAULT '#4F46E5',
    primary_light VARCHAR(7) DEFAULT '#6366F1',
    primary_dark VARCHAR(7) DEFAULT '#4338CA',
    secondary_color VARCHAR(7) DEFAULT '#10B981',
    secondary_dark VARCHAR(7) DEFAULT '#059669',
    accent_color VARCHAR(7) DEFAULT '#F59E0B',
    accent_dark VARCHAR(7) DEFAULT '#D97706',

    -- Typography
    heading_font VARCHAR(100) DEFAULT 'Inter',
    body_font VARCHAR(100) DEFAULT 'Inter',

    -- Voice & Tone
    brand_voice TEXT,
    writing_guidelines TEXT,

    -- Contact Info
    website_url VARCHAR(255) DEFAULT 'https://mycarecompanion.net',
    support_email VARCHAR(255) DEFAULT 'support@mycarecompanion.net',
    contact_phone VARCHAR(50),

    -- Social Media
    facebook_url VARCHAR(255),
    twitter_url VARCHAR(255),
    instagram_url VARCHAR(255),
    linkedin_url VARCHAR(255),

    -- Legal
    copyright_text VARCHAR(255),
    disclaimer_text TEXT,

    -- Timestamps
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by UUID REFERENCES users(id)
);

-- Insert default brand config
INSERT INTO brand_config (
    mission_statement,
    brand_voice,
    writing_guidelines,
    copyright_text
) VALUES (
    'CareCompanion helps parents and caregivers track and improve care for children with autism through AI-powered pattern analysis. We transform overwhelming daily chaos into actionable insights.',
    E'Compassionate and understanding - we know how hard this journey is.\nSupportive and encouraging - celebrating every small victory.\nKnowledgeable yet accessible - expert information in plain language.\nEmpowering - helping families take control of their care journey.',
    E'Use "you" and "your" to speak directly to families.\nLead with empathy before solutions.\nAvoid clinical jargon unless necessary.\nKeep sentences short and clear.\nEmphasize hope and progress.',
    '2025 CareCompanion. All rights reserved.'
);

-- Marketing material assets (generated files)
CREATE TABLE marketing_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Asset Info
    name VARCHAR(255) NOT NULL,
    description TEXT,
    asset_type VARCHAR(50) NOT NULL, -- 'logo', 'brochure', 'social_graphic', 'style_guide'
    format VARCHAR(20) NOT NULL, -- 'pdf', 'png', 'svg', 'jpg'

    -- Dimensions
    width_px INTEGER,
    height_px INTEGER,

    -- Storage
    file_path VARCHAR(500),
    file_size_bytes BIGINT,

    -- Generation
    is_auto_generated BOOLEAN DEFAULT TRUE,
    generation_template VARCHAR(100),
    last_generated_at TIMESTAMPTZ,

    -- Status
    is_active BOOLEAN DEFAULT TRUE,

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Social media template configurations
CREATE TABLE social_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Template Info
    name VARCHAR(100) NOT NULL,
    platform VARCHAR(50) NOT NULL, -- 'facebook', 'twitter', 'instagram', 'linkedin'
    template_type VARCHAR(50) NOT NULL, -- 'post', 'story', 'cover', 'profile'

    -- Dimensions
    width_px INTEGER NOT NULL,
    height_px INTEGER NOT NULL,

    -- Design Config (JSON)
    design_config JSONB DEFAULT '{}',

    -- Content placeholders
    headline_max_chars INTEGER,
    body_max_chars INTEGER,

    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Insert default social templates
INSERT INTO social_templates (name, platform, template_type, width_px, height_px, headline_max_chars, body_max_chars) VALUES
('Facebook Post', 'facebook', 'post', 1200, 630, 80, 250),
('Facebook Cover', 'facebook', 'cover', 820, 312, 50, 0),
('Instagram Post', 'instagram', 'post', 1080, 1080, 60, 200),
('Instagram Story', 'instagram', 'story', 1080, 1920, 40, 100),
('Twitter Post', 'twitter', 'post', 1200, 675, 70, 200),
('LinkedIn Post', 'linkedin', 'post', 1200, 627, 80, 250),
('LinkedIn Cover', 'linkedin', 'cover', 1584, 396, 50, 0);

-- Indexes
CREATE INDEX idx_marketing_assets_type ON marketing_assets(asset_type);
CREATE INDEX idx_marketing_assets_active ON marketing_assets(is_active) WHERE is_active = TRUE;
CREATE INDEX idx_social_templates_platform ON social_templates(platform);
CREATE INDEX idx_social_templates_active ON social_templates(is_active) WHERE is_active = TRUE;
