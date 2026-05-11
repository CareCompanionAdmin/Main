package service

// ai_narrative_consent_service.go — manages opt-in consent for sending
// free-text fields to outbound LLM calls.
//
// Internal-AI Phase 3. The Phase 1 PHI stripper drops behavior notes,
// therapy progress notes, and health-event descriptions by default
// (the LLM never sees them). This service is the gate that lets a
// parent explicitly opt into having those fields included so the model
// can interpret narrative context.
//
// Default state for every user: not consented. Without consent the
// Phase 1 strip behavior continues unchanged. With consent AND the
// server-side feature flag AI_NARRATIVE_OPT_IN_AVAILABLE both true,
// free-text fields are included.
//
// Audit trail: every consent change writes a row to
// ai_narrative_consent_audit (table created in migration 00035) so we
// can demonstrate consent compliance if ever audited.
//
// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CurrentNarrativeDisclosureVersion is the version of the in-app disclosure
// text shown to users when they enable narrative analysis. Bump this when
// the disclosure changes materially — bumping forces existing consents to
// reset to false so users re-consent against the new text.
const CurrentNarrativeDisclosureVersion = 1

// CurrentNarrativeDisclosureText is the canonical disclosure shown in
// the Settings → AI Narrative Analysis modal. The SHA-256 of this exact
// text is stored alongside each user's consent so we can prove later
// which version they agreed to.
const CurrentNarrativeDisclosureText = `By enabling AI Narrative Analysis you agree that the following will be included in outbound calls to our AI analysis partner (currently AWS Bedrock running Anthropic's Claude model under our HIPAA-eligible BAA):

  • Free-text notes you write on behavior logs
  • Progress notes you write on therapy logs
  • Descriptions you write on health event logs

These narrative fields will be sent alongside the de-identified categorical and numerical log data that is already sent without consent. We will continue to strip first names, exact birthdates, specific medication names, and ICD codes before send. You can disable this at any time in Settings; previously-generated insights remain.`

// CurrentNarrativeDisclosureSHA returns the SHA-256 of the current
// disclosure text, used as the version key in the audit log.
func CurrentNarrativeDisclosureSHA() string {
	h := sha256.Sum256([]byte(CurrentNarrativeDisclosureText))
	return hex.EncodeToString(h[:])
}

// NarrativeConsent describes a user's current state.
type NarrativeConsent struct {
	UserID           uuid.UUID
	Enabled          bool
	ConsentedAt      time.Time
	Version          int
	DisclosureSHA    string
	NeedsReConsent   bool // true when stored version != CurrentNarrativeDisclosureVersion
	FeatureAvailable bool // true when AI_NARRATIVE_OPT_IN_AVAILABLE is set on this server
}

// AINarrativeConsentService manages reading and writing per-user consent
// state for the AI narrative opt-in path.
type AINarrativeConsentService struct {
	db               *sql.DB
	featureAvailable bool
}

func NewAINarrativeConsentService(db *sql.DB, featureAvailable bool) *AINarrativeConsentService {
	return &AINarrativeConsentService{
		db:               db,
		featureAvailable: featureAvailable,
	}
}

// FeatureAvailable returns whether the server-side feature flag is on.
// When false, the consent UI should be hidden and the AI service should
// treat all consents as effectively false.
func (s *AINarrativeConsentService) FeatureAvailable() bool {
	return s.featureAvailable
}

// GetConsent returns the current consent state for a user. If the user
// has no consent row (default state), Enabled is false.
//
// If the stored consent version is older than CurrentNarrativeDisclosureVersion,
// NeedsReConsent is true AND Enabled is reported as false (we don't trust
// an old-text consent for a new-text feature).
func (s *AINarrativeConsentService) GetConsent(ctx context.Context, userID uuid.UUID) (*NarrativeConsent, error) {
	c := &NarrativeConsent{
		UserID:           userID,
		FeatureAvailable: s.featureAvailable,
	}
	var (
		enabled      sql.NullBool
		consentedAt  sql.NullTime
		version      sql.NullInt64
		disclosureSHA sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT ai_narrative_consent_enabled,
		       ai_narrative_consent_at,
		       ai_narrative_consent_version,
		       ai_narrative_consent_disclosure_sha
		  FROM app_users
		 WHERE id = $1
	`, userID).Scan(&enabled, &consentedAt, &version, &disclosureSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get narrative consent: %w", err)
	}
	if enabled.Valid {
		c.Enabled = enabled.Bool
	}
	if consentedAt.Valid {
		c.ConsentedAt = consentedAt.Time
	}
	if version.Valid {
		c.Version = int(version.Int64)
	}
	if disclosureSHA.Valid {
		c.DisclosureSHA = disclosureSHA.String
	}
	if c.Enabled && c.Version != CurrentNarrativeDisclosureVersion {
		c.NeedsReConsent = true
		c.Enabled = false // stale consent doesn't authorize current send behavior
	}
	return c, nil
}

// SetConsent updates a user's consent state. `enabled=true` requires
// the feature to be available on this server. Always records an audit
// row regardless of whether the state actually changed.
func (s *AINarrativeConsentService) SetConsent(ctx context.Context, userID uuid.UUID, enabled bool, ipAddress, userAgent string) error {
	if enabled && !s.featureAvailable {
		return errors.New("narrative analysis is not available on this server")
	}

	disclosureSHA := CurrentNarrativeDisclosureSHA()
	now := time.Now()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE app_users
		   SET ai_narrative_consent_enabled = $1,
		       ai_narrative_consent_at = $2,
		       ai_narrative_consent_version = $3,
		       ai_narrative_consent_disclosure_sha = $4,
		       updated_at = $2
		 WHERE id = $5
	`, enabled, now, CurrentNarrativeDisclosureVersion, disclosureSHA, userID); err != nil {
		return fmt.Errorf("update consent: %w", err)
	}

	action := "disabled"
	if enabled {
		action = "enabled"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_narrative_consent_audit
		    (app_user_id, action, disclosure_version, disclosure_sha, ip_address, user_agent, occurred_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,'')::inet, NULLIF($6,''), $7)
	`, userID, action, CurrentNarrativeDisclosureVersion, disclosureSHA, ipAddress, userAgent, now); err != nil {
		return fmt.Errorf("audit insert: %w", err)
	}

	return tx.Commit()
}

// AllowsNarrative returns whether free-text fields may be included in
// outbound LLM calls for this user RIGHT NOW. Single source of truth
// for the AI service gate. Returns false if:
//   - the server feature flag is off, OR
//   - the user is not consented, OR
//   - the stored consent version is stale.
func (s *AINarrativeConsentService) AllowsNarrative(ctx context.Context, userID uuid.UUID) bool {
	if !s.featureAvailable {
		return false
	}
	c, err := s.GetConsent(ctx, userID)
	if err != nil {
		return false
	}
	return c.Enabled && !c.NeedsReConsent
}

// AllowsNarrativeForFamily returns whether the family's primary parent
// (families.created_by) has consented to narrative analysis. Used by
// the AI insight pipeline which operates on a child→family scope rather
// than on the per-user scope. If the family or its primary parent can't
// be resolved, returns false (safe default).
func (s *AINarrativeConsentService) AllowsNarrativeForFamily(ctx context.Context, familyID uuid.UUID) bool {
	if !s.featureAvailable {
		return false
	}
	var primaryUserID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
		SELECT created_by FROM families WHERE id = $1
	`, familyID).Scan(&primaryUserID)
	if err != nil {
		return false
	}
	return s.AllowsNarrative(ctx, primaryUserID)
}
