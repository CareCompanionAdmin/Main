package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/repository"
)

// BetaService orchestrates the marketing-managed beta opt-in flow:
//   1. Admin enters an email -> we create a beta_invitations row and email a
//      secret onboarding link.
//   2. User opens the link -> submits Apple ID + name on a public page.
//   3. We register them as an External Tester in App Store Connect and the
//      user receives Apple's TestFlight invite.
type BetaService struct {
	repo     repository.BetaInvitationRepository
	email    *EmailService
	asc      *AppStoreConnectService
	appURL   string // e.g. https://dev.mycarecompanion.com
	pdfURL   string // e.g. /static/docs/beta-onboarding.pdf
}

// NewBetaService wires the dependencies. asc may be nil if the App Store
// Connect API isn't configured — in that case the auto-add step is skipped
// and a log line tells an admin to add the tester manually.
func NewBetaService(repo repository.BetaInvitationRepository, email *EmailService, asc *AppStoreConnectService, appURL, pdfURL string) *BetaService {
	return &BetaService{
		repo:   repo,
		email:  email,
		asc:    asc,
		appURL: strings.TrimRight(appURL, "/"),
		pdfURL: pdfURL,
	}
}

// Invite creates a beta_invitations row and sends the onboarding email.
// invitedBy must be a real user (admin) for the audit trail.
func (s *BetaService) Invite(ctx context.Context, email string, invitedBy uuid.UUID, notes string) (*repository.BetaInvitation, error) {
	email = strings.TrimSpace(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("invalid email address: %w", err)
	}

	// Idempotent: if we've already invited this email, reuse the row so a
	// double-click on "Send invite" doesn't create dupes.
	if existing, err := s.repo.GetByEmail(ctx, email); err == nil && existing != nil {
		return existing, nil
	}

	inv := &repository.BetaInvitation{
		Email:     email,
		InvitedBy: invitedBy,
		Notes:     notes,
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		return nil, fmt.Errorf("create invitation: %w", err)
	}

	if err := s.sendInviteEmail(inv); err != nil {
		// Don't roll back the row — admin can resend. Just log.
		log.Printf("[BETA] invite created %s but email send failed: %v", inv.Email, err)
	}
	return inv, nil
}

// sendInviteEmail formats and sends the onboarding email with the secret link.
func (s *BetaService) sendInviteEmail(inv *repository.BetaInvitation) error {
	link := fmt.Sprintf("%s/beta/onboard/%s", s.appURL, inv.Token.String())
	subject := "You're invited to the My Care Companion beta"
	body := fmt.Sprintf(`<!doctype html>
<html><body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 580px; margin: 0 auto; padding: 24px; color: #1f2937; background: #ffffff;">
  <h1 style="font-size: 22px; color: #0f766e;">Welcome to the My Care Companion beta</h1>
  <p>Thanks for being open to trying My Care Companion before our public release.</p>
  <p>To finish joining the beta program, please open the secure onboarding page below. There you'll provide your Apple ID and download a short PDF with step-by-step instructions for installing the beta on your iPhone via TestFlight.</p>
  <p style="margin: 28px 0;">
    <a href="%s" style="background: #0f766e; color: #ffffff; padding: 12px 22px; border-radius: 999px; text-decoration: none; font-weight: 600;">Start beta onboarding &rarr;</a>
  </p>
  <p style="font-size: 13px; color: #6b7280;">If the button doesn't work, copy and paste this link into your browser:<br/><span style="word-break: break-all;">%s</span></p>
  <p style="font-size: 13px; color: #6b7280;">This is a one-time link tied to your email. Don't forward it — if you weren't expecting this, just ignore the message.</p>
  <p style="margin-top: 28px;">— The My Care Companion team</p>
</body></html>`, html.EscapeString(link), html.EscapeString(link))

	return s.email.SendEmail(inv.Email, subject, body)
}

// Resend re-issues the onboarding email for an existing invitation.
func (s *BetaService) Resend(ctx context.Context, id uuid.UUID) error {
	inv, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("lookup invitation: %w", err)
	}
	return s.sendInviteEmail(inv)
}

// CollectAppleID is called by the public onboarding page handler when the
// user submits the form. Stores the Apple ID, then attempts to register them
// in App Store Connect as an external tester. Returns the (refreshed)
// invitation so the page can show the right "what to expect next" message.
func (s *BetaService) CollectAppleID(ctx context.Context, token uuid.UUID, appleID, firstName, lastName string) (*repository.BetaInvitation, error) {
	appleID = strings.TrimSpace(appleID)
	firstName = strings.TrimSpace(firstName)
	lastName = strings.TrimSpace(lastName)

	if _, err := mail.ParseAddress(appleID); err != nil {
		return nil, errors.New("Apple ID must be a valid email address")
	}
	if firstName == "" || lastName == "" {
		return nil, errors.New("first and last name are required")
	}

	inv, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("invitation not found: %w", err)
	}

	if err := s.repo.UpdateAppleID(ctx, inv.ID, appleID, firstName, lastName); err != nil {
		return nil, fmt.Errorf("save Apple ID: %w", err)
	}
	// Refresh after the update.
	inv, _ = s.repo.GetByID(ctx, inv.ID)

	if !s.asc.IsConfigured() {
		log.Printf("[BETA] App Store Connect not configured — manual add needed for tester %s (%s %s) on invitation %s",
			appleID, firstName, lastName, inv.ID)
		return inv, nil
	}

	if err := s.asc.AddExternalTester(ctx, appleID, firstName, lastName); err != nil {
		_ = s.repo.MarkError(ctx, inv.ID, err.Error())
		log.Printf("[BETA] AddExternalTester failed for %s: %v", appleID, err)
		// Refresh once more so caller sees the error status.
		inv, _ = s.repo.GetByID(ctx, inv.ID)
		return inv, err
	}

	if err := s.repo.MarkAddedToTestFlight(ctx, inv.ID); err != nil {
		log.Printf("[BETA] mark added_to_testflight failed for %s: %v", inv.ID, err)
	}
	inv, _ = s.repo.GetByID(ctx, inv.ID)
	return inv, nil
}

// List returns all invitations for the admin UI.
func (s *BetaService) List(ctx context.Context) ([]repository.BetaInvitation, error) {
	return s.repo.List(ctx)
}

// GetByToken looks up an invitation by its onboarding token. Returns nil
// (and nil error) when not found, so callers can render a friendly 404
// without log noise.
func (s *BetaService) GetByToken(ctx context.Context, token uuid.UUID) (*repository.BetaInvitation, error) {
	inv, err := s.repo.GetByToken(ctx, token)
	if err != nil {
		// sql.ErrNoRows surfaces as an error from the repo; treat as not-found.
		return nil, nil
	}
	return inv, nil
}

// PDFURL exposes the onboarding doc location for the public page template.
func (s *BetaService) PDFURL() string { return s.pdfURL }

// ASCConfigured reports whether the App Store Connect API integration is
// wired up. False = invitations still go out, but new testers must be added
// to the TestFlight group manually.
func (s *BetaService) ASCConfigured() bool { return s.asc.IsConfigured() }
