-- 00035_ai_narrative_consent.sql
--
-- Internal-AI Phase 3: opt-in consent for narrative analysis.
--
-- The Phase 1 PHI stripping ensures no identifying content leaves the
-- server by default. This migration adds the consent infrastructure for
-- parents who explicitly opt into a richer experience: free-text fields
-- (behavior notes, therapy progress notes, health-event descriptions)
-- are included in outbound LLM calls so the model can interpret
-- narrative context that pure numerical data misses.
--
-- Defaults: OFF for every existing app_user. Feature is also gated by
-- a server-side env var AI_NARRATIVE_OPT_IN_AVAILABLE which stays false
-- in prod until Phase 5 (Bedrock + BAA + privacy docs all aligned).
--
-- See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

BEGIN;

-- Per-user consent state on app_users (the parent-facing user kind from
-- migration 00032). Default false so existing users start without consent.
ALTER TABLE app_users
  ADD COLUMN ai_narrative_consent_enabled        boolean        NOT NULL DEFAULT false,
  ADD COLUMN ai_narrative_consent_at             timestamptz,
  ADD COLUMN ai_narrative_consent_version        integer,
  ADD COLUMN ai_narrative_consent_disclosure_sha text;

COMMENT ON COLUMN app_users.ai_narrative_consent_enabled IS
  'Whether the user has opted into having free-text fields included in outbound LLM calls. Default false; remains false until user explicitly toggles on in Settings.';
COMMENT ON COLUMN app_users.ai_narrative_consent_at IS
  'Timestamp of the most recent consent state change (either enable or disable).';
COMMENT ON COLUMN app_users.ai_narrative_consent_version IS
  'Disclosure version the user accepted. Bumped when we materially change what gets sent or to whom — forces re-consent.';
COMMENT ON COLUMN app_users.ai_narrative_consent_disclosure_sha IS
  'SHA-256 of the exact disclosure text the user saw when they accepted. Lets us prove which disclosure they signed even if we later edit it.';

-- Append-only audit log of consent transitions. Every enable/disable
-- creates a new row regardless of repeats so we can audit changes-of-mind.
CREATE TABLE ai_narrative_consent_audit (
  id              uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
  app_user_id     uuid NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  action          text NOT NULL CHECK (action IN ('enabled', 'disabled', 'reset_due_to_version_bump')),
  disclosure_version integer,
  disclosure_sha     text,
  ip_address      inet,
  user_agent      text,
  occurred_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_narrative_consent_audit_user_time
  ON ai_narrative_consent_audit(app_user_id, occurred_at DESC);

COMMENT ON TABLE ai_narrative_consent_audit IS
  'Append-only history of AI narrative consent toggle events. Used to demonstrate consent compliance if ever audited and to detect users with frequent toggling who might need a UX nudge.';

COMMIT;
