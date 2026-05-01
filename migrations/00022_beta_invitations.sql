-- 00022_beta_invitations.sql
--
-- Beta invitation tracking for the marketing-managed TestFlight beta program.
-- An admin (marketing or super_admin) enters an email; the system mails a
-- secret link to a hidden onboarding page; the user submits their Apple ID;
-- the system then registers them as an external tester in App Store Connect.
--
-- This is purely additive — no existing tables touched.

DO $$ BEGIN
    CREATE TYPE beta_invitation_status AS ENUM (
        'invited',              -- email sent, awaiting Apple ID
        'apple_id_collected',   -- user submitted Apple ID, ASC API call pending/failed
        'added_to_testflight',  -- successfully added to External Beta Testers group
        'error'                 -- ASC API or other failure (see notes)
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS beta_invitations (
    id                       UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email                    VARCHAR(255) NOT NULL,
    invited_by               UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token                    UUID NOT NULL DEFAULT uuid_generate_v4(),
    status                   beta_invitation_status NOT NULL DEFAULT 'invited',
    apple_id                 VARCHAR(255),
    apple_first_name         VARCHAR(100),
    apple_last_name          VARCHAR(100),
    invited_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    apple_id_collected_at    TIMESTAMPTZ,
    added_to_testflight_at   TIMESTAMPTZ,
    last_error               TEXT,
    notes                    TEXT,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_beta_invitations_token ON beta_invitations(token);
CREATE INDEX        IF NOT EXISTS idx_beta_invitations_email ON beta_invitations(LOWER(email));
CREATE INDEX        IF NOT EXISTS idx_beta_invitations_status ON beta_invitations(status);

-- updated_at trigger (mirrors pattern used elsewhere)
DROP TRIGGER IF EXISTS update_beta_invitations_updated_at ON beta_invitations;
CREATE TRIGGER update_beta_invitations_updated_at
    BEFORE UPDATE ON beta_invitations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
