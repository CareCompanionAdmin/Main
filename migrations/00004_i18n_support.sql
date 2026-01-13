-- Migration: 00004_i18n_support.sql
-- Date: 2026-01-13
-- Purpose: Add internationalization support fields for future multi-language support

-- Add language preference to users (default to English)
ALTER TABLE users ADD COLUMN IF NOT EXISTS language VARCHAR(10) DEFAULT 'en';

-- Add locale preference to families (default to English US)
ALTER TABLE families ADD COLUMN IF NOT EXISTS locale VARCHAR(10) DEFAULT 'en-US';

-- Create index for potential filtering by language
CREATE INDEX IF NOT EXISTS idx_users_language ON users(language);

-- Add comment for documentation
COMMENT ON COLUMN users.language IS 'User preferred language code (e.g., en, es, fr, de, zh)';
COMMENT ON COLUMN families.locale IS 'Family default locale code (e.g., en-US, es-MX, fr-FR)';

-- ============================================================================
-- ROLLBACK
-- ============================================================================
-- ALTER TABLE users DROP COLUMN IF EXISTS language;
-- ALTER TABLE families DROP COLUMN IF EXISTS locale;
-- DROP INDEX IF EXISTS idx_users_language;
