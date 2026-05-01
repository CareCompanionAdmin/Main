package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// BountyService runs the monthly rewards program: top 5 bug reports + top 5
// feature requests per calendar month, each receiving a 1-month free promo
// code. Submissions that don't make the cut can still get a "thanks anyway"
// canned message so submitters don't feel ignored.
type BountyService struct {
	repo            repository.BountyAwardRepository
	adminRepo       repository.AdminRepository
	email           *EmailService
	db              *sql.DB

	// Caps — kept as plain ints so they're easy to tweak later via env if
	// needed. Hard-coded for now per the agreed design (5 + 5 / month).
	bugCap     int
	featureCap int

	// Promo code defaults.
	rewardMonths   int           // how many free months per award
	rewardValidity time.Duration // how long the promo code stays redeemable
}

// NewBountyService wires everything. db is needed because we insert into
// promo_codes directly via raw SQL — there's no Promo create that takes a
// pre-shaped row with the bounty-specific fields we want.
func NewBountyService(
	repo repository.BountyAwardRepository,
	adminRepo repository.AdminRepository,
	email *EmailService,
	db *sql.DB,
) *BountyService {
	return &BountyService{
		repo:           repo,
		adminRepo:      adminRepo,
		email:          email,
		db:             db,
		bugCap:         5,
		featureCap:     5,
		rewardMonths:   1,
		rewardValidity: 90 * 24 * time.Hour,
	}
}

// CurrentMonth returns the first-of-month for "right now" in UTC. All
// admin-page queries and award rows use this to bucket per-cycle.
func CurrentMonth() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// EligibleCandidates returns the bug + feature pools for a given month.
// windowStart is current month minus 30 days so newly-resolved items at
// the very start of the month are still eligible.
func (s *BountyService) EligibleCandidates(ctx context.Context, awardMonth time.Time) ([]repository.BountyCandidate, []repository.BountyCandidate, error) {
	windowStart := awardMonth.AddDate(0, 0, -30)
	bugs, err := s.repo.EligibleBugs(ctx, windowStart, awardMonth)
	if err != nil {
		return nil, nil, fmt.Errorf("eligible bugs: %w", err)
	}
	feats, err := s.repo.EligibleFeatures(ctx, windowStart, awardMonth)
	if err != nil {
		return nil, nil, fmt.Errorf("eligible features: %w", err)
	}
	return bugs, feats, nil
}

// ListThisMonth returns awards already recorded for awardMonth.
func (s *BountyService) ListThisMonth(ctx context.Context, awardMonth time.Time) ([]repository.BountyAward, error) {
	return s.repo.ListAwards(ctx, awardMonth)
}

// History returns recent past awards for the audit-trail panel.
func (s *BountyService) History(ctx context.Context, limit int) ([]repository.BountyAward, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.HistoryAwards(ctx, limit)
}

// Caps returns the per-type monthly caps so the admin UI can show progress.
func (s *BountyService) Caps() (bug int, feature int) { return s.bugCap, s.featureCap }

// Select records a "this is one of the top 5" decision for one candidate.
// Generates a fresh single-use 1-month promo code locked to the recipient,
// posts a canned thank-you message on their original ticket, and emails them
// the code. Enforces the per-type monthly cap.
func (s *BountyService) Select(ctx context.Context, c repository.BountyCandidate, awardedBy uuid.UUID, notes string) (*repository.BountyAward, error) {
	awardMonth := CurrentMonth()

	// Cap check.
	taken, err := s.repo.CountSelected(ctx, awardMonth, c.Type)
	if err != nil {
		return nil, fmt.Errorf("count selected: %w", err)
	}
	cap := s.bugCap
	if c.Type == "feature" {
		cap = s.featureCap
	}
	if taken >= cap {
		return nil, fmt.Errorf("monthly cap reached for %s awards (%d of %d already selected this month)", c.Type, taken, cap)
	}

	// Issue promo code.
	promoID, code, err := s.issuePromoCode(ctx, c, awardedBy)
	if err != nil {
		return nil, fmt.Errorf("issue promo code: %w", err)
	}

	award := &repository.BountyAward{
		AwardMonth:      awardMonth,
		AwardType:       c.Type,
		Decision:        "selected",
		TicketID:        c.TicketID,
		RoadmapItemID:   c.RoadmapItemID,
		RecipientUserID: c.RecipientUserID,
		PromoCodeID:     models.NullUUID{UUID: promoID, Valid: true},
		AwardedBy:       awardedBy,
		Notes:           notes,
	}
	if err := s.repo.CreateAward(ctx, award); err != nil {
		// Promo code was created but award insert failed — deactivate the
		// orphan code so it can't be redeemed accidentally.
		_ = s.deactivatePromoCode(ctx, promoID, awardedBy, "bounty award insert failed: "+err.Error())
		return nil, fmt.Errorf("create award: %w", err)
	}

	// Post canned message + email. Best-effort — failures don't roll back the
	// award (the admin can resend manually if needed).
	s.notifyWinner(ctx, c, code, awardedBy)
	return award, nil
}

// ThanksAnyway records a "considered but not selected" decision. Posts a
// gentler canned message on the user's original ticket. No promo code, no
// cap enforcement (you can thank as many people as you want).
func (s *BountyService) ThanksAnyway(ctx context.Context, c repository.BountyCandidate, awardedBy uuid.UUID, notes string) (*repository.BountyAward, error) {
	award := &repository.BountyAward{
		AwardMonth:      CurrentMonth(),
		AwardType:       c.Type,
		Decision:        "thanks_anyway",
		TicketID:        c.TicketID,
		RoadmapItemID:   c.RoadmapItemID,
		RecipientUserID: c.RecipientUserID,
		AwardedBy:       awardedBy,
		Notes:           notes,
	}
	if err := s.repo.CreateAward(ctx, award); err != nil {
		return nil, fmt.Errorf("create thanks-anyway award: %w", err)
	}
	s.notifyRunnerUp(ctx, c, awardedBy)
	return award, nil
}

// ============================================================================
// internals
// ============================================================================

// issuePromoCode inserts a new row in promo_codes locked to the recipient
// and returns the code string + new uuid.
func (s *BountyService) issuePromoCode(ctx context.Context, c repository.BountyCandidate, createdBy uuid.UUID) (uuid.UUID, string, error) {
	code := generatePromoCode()
	name := truncateBountyName("Bounty: "+c.Subject, 100)
	expires := time.Now().UTC().Add(s.rewardValidity)

	var id uuid.UUID
	err := s.db.QueryRowContext(ctx, `
        INSERT INTO promo_codes (
            code, name, description,
            discount_type, discount_value, applies_to,
            specific_user_ids,
            max_total_uses, max_uses_per_user,
            duration_months,
            expires_at,
            campaign_name, campaign_source,
            is_active, created_by
        ) VALUES (
            $1, $2, $3,
            'free_months', $4, 'subscription',
            ARRAY[$5]::uuid[],
            1, 1,
            $4,
            $6,
            'bounty', 'bounty_program',
            TRUE, $7
        )
        RETURNING id
    `, code, name, "Awarded for selected bounty submission",
		s.rewardMonths,
		c.RecipientUserID,
		expires,
		createdBy,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, "", err
	}
	return id, code, nil
}

func (s *BountyService) deactivatePromoCode(ctx context.Context, id, by uuid.UUID, reason string) error {
	_, err := s.db.ExecContext(ctx, `
        UPDATE promo_codes
        SET is_active = FALSE,
            deactivated_at = NOW(),
            deactivated_by = $2,
            deactivation_reason = $3
        WHERE id = $1
    `, id, by, reason)
	return err
}

// notifyWinner posts a thank-you message on the user's source ticket and
// emails them the promo code. Best-effort.
func (s *BountyService) notifyWinner(ctx context.Context, c repository.BountyCandidate, code string, sender uuid.UUID) {
	var msg string
	if c.Type == "bug" {
		msg = "Great news — your bug report was selected as one of this month's most significant. " +
			"As a thank-you, we've credited your account with one free month of My Care Companion. " +
			"Use this promo code on your next billing cycle: " + code + ". " +
			"It's locked to your account and stays valid for 90 days."
	} else {
		msg = "Great news — the feature you requested (\"" + c.Subject + "\") was selected as one of this month's " +
			"most significant feature requests. As a thank-you we've credited your account with one free month of " +
			"My Care Companion. Use this promo code on your next billing cycle: " + code + ". " +
			"It's locked to your account and stays valid for 90 days."
	}

	if c.SourceTicketID.Valid {
		if err := s.adminRepo.AddTicketMessage(ctx, c.SourceTicketID.UUID, sender, msg, false); err != nil {
			log.Printf("[BOUNTY] post winner message on ticket %s failed: %v", c.SourceTicketID.UUID, err)
		}
	}

	if c.RecipientEmail != "" && s.email != nil && s.email.IsEnabled() {
		subject := "You earned a free month of My Care Companion"
		body := winnerEmailBody(c, code)
		if err := s.email.SendEmail(c.RecipientEmail, subject, body); err != nil {
			log.Printf("[BOUNTY] email winner %s failed: %v", c.RecipientEmail, err)
		}
	}
}

// notifyRunnerUp posts the "thanks anyway, keep them coming" message.
func (s *BountyService) notifyRunnerUp(ctx context.Context, c repository.BountyCandidate, sender uuid.UUID) {
	var msg string
	if c.Type == "bug" {
		msg = "Thanks for taking the time to report this. We reviewed every bug report from this past month and " +
			"yours didn't make the top 5 this round, but it absolutely helped us understand what to prioritize. " +
			"Please keep them coming — every report makes the app better."
	} else {
		msg = "Thanks for sharing your idea. We reviewed every feature request from this past month and yours " +
			"didn't make the top 5 this round, but the team read it carefully and it's still on our list. " +
			"Please keep them coming — they shape where we go next."
	}

	if c.SourceTicketID.Valid {
		if err := s.adminRepo.AddTicketMessage(ctx, c.SourceTicketID.UUID, sender, msg, false); err != nil {
			log.Printf("[BOUNTY] post runner-up message on ticket %s failed: %v", c.SourceTicketID.UUID, err)
		}
	}

	if c.RecipientEmail != "" && s.email != nil && s.email.IsEnabled() {
		subject := "About your recent My Care Companion feedback"
		body := runnerUpEmailBody(c)
		if err := s.email.SendEmail(c.RecipientEmail, subject, body); err != nil {
			log.Printf("[BOUNTY] email runner-up %s failed: %v", c.RecipientEmail, err)
		}
	}
}

// generatePromoCode creates a human-friendly 13-char code in the form
// THANKS-XXXXXX where X's are crockford-base32-ish (A-Z, 2-9, no I/O/L/0/1).
func generatePromoCode() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// Should never happen; fall back to time-based randomness.
		return fmt.Sprintf("THANKS-%X", time.Now().UnixNano())
	}
	enc := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
	// strip ambiguous chars
	enc = strings.NewReplacer("I", "9", "O", "8", "L", "7", "0", "6", "1", "5").Replace(enc)
	if len(enc) > 8 {
		enc = enc[:8]
	}
	return "THANKS-" + enc
}

func truncateBountyName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func winnerEmailBody(c repository.BountyCandidate, code string) string {
	what := "your bug report"
	context := ""
	if c.Type == "feature" {
		what = "your feature request"
		context = " (\"" + html.EscapeString(c.Subject) + "\")"
	}
	return fmt.Sprintf(`<!doctype html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:580px;margin:0 auto;padding:24px;color:#1f2937;background:#ffffff;">
  <h1 style="font-size:22px;color:#0f766e;">A free month, on us.</h1>
  <p>Hi —</p>
  <p>%s%s was selected as one of the five most significant %ss filed this month. As a thank-you, we've credited your account with <strong>one free month</strong> of My Care Companion.</p>
  <div style="background:#ecfdf5;border:1px solid #a7f3d0;border-radius:10px;padding:18px 20px;margin:18px 0;">
    <p style="margin:0;font-size:13px;color:#065f46;text-transform:uppercase;letter-spacing:0.08em;font-weight:700;">Your promo code</p>
    <p style="margin:6px 0 0;font-size:24px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:700;color:#064e3b;">%s</p>
  </div>
  <p style="font-size:14px;color:#4b5563;">Apply it on your next billing cycle. The code is locked to your account and stays valid for 90 days.</p>
  <p style="font-size:14px;color:#4b5563;">Thank you for taking the time — feedback like yours is the reason the app keeps getting better.</p>
  <p style="margin-top:28px;">— The My Care Companion team</p>
</body></html>`,
		strings.Title(what), context, c.Type, html.EscapeString(code))
}

func runnerUpEmailBody(c repository.BountyCandidate) string {
	what := "bug report"
	if c.Type == "feature" {
		what = "feature request"
	}
	return fmt.Sprintf(`<!doctype html>
<html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;max-width:580px;margin:0 auto;padding:24px;color:#1f2937;background:#ffffff;">
  <h1 style="font-size:20px;color:#0f766e;">Thanks for the %s.</h1>
  <p>Hi —</p>
  <p>We reviewed every %s filed this past month and want to say thank you for taking the time. Yours didn't make this month's top five, but it absolutely helped us understand what to prioritize and is still on our list.</p>
  <p>Please keep them coming — feedback like yours is what shapes where we go next.</p>
  <p style="margin-top:28px;">— The My Care Companion team</p>
</body></html>`, what, what)
}

// errCapReached is reserved for callers that want to detect cap errors
// distinctly from other failures. Currently unused but exported for future
// admin UI logic.
var errCapReached = errors.New("monthly cap reached")
