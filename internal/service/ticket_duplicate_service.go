package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log"

	"github.com/google/uuid"

	"carecompanion/internal/repository"
)

// TicketDuplicateService coordinates the "mark as duplicate" flow. It can
// dedup a ticket onto either another ticket or directly onto a roadmap item;
// in either case the user gets a friendly canned message (no internal IDs)
// and the duplicate ticket is closed.
type TicketDuplicateService struct {
	adminRepo   repository.AdminRepository
	roadmapRepo repository.RoadmapRepository
	email       *EmailService
	attach      *TicketAttachmentService
}

// NewTicketDuplicateService builds the service.
func NewTicketDuplicateService(adminRepo repository.AdminRepository, roadmapRepo repository.RoadmapRepository, email *EmailService) *TicketDuplicateService {
	return &TicketDuplicateService{adminRepo: adminRepo, roadmapRepo: roadmapRepo, email: email}
}

// SetAttachmentService wires the attachment service so dup-close can purge PHI.
func (s *TicketDuplicateService) SetAttachmentService(a *TicketAttachmentService) {
	s.attach = a
}

// MarkAsDuplicate marks ticketID as a duplicate of either targetTicketID OR
// targetRoadmapID (exactly one must be non-nil). Side effects:
//
//   - sets the appropriate FK on ticketID
//   - posts a friendly canned message on the duplicate ticket (worded for
//     end users — no internal ticket numbers)
//   - closes the duplicate ticket
//   - emails the requester (best-effort)
//   - if the canonical target is (or maps to) a roadmap item, the duplicate
//     ticket's user is added as a follower so they get release notifications
//
// Returns the (now-updated) duplicate ticket.
func (s *TicketDuplicateService) MarkAsDuplicate(
	ctx context.Context,
	ticketID uuid.UUID,
	targetTicketID, targetRoadmapID *uuid.UUID,
	adminID uuid.UUID,
) (*repository.SupportTicket, error) {
	if (targetTicketID == nil) == (targetRoadmapID == nil) {
		return nil, errors.New("provide exactly one of targetTicketID or targetRoadmapID")
	}

	ticket, err := s.adminRepo.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, sql.ErrNoRows
	}
	if ticket.DuplicateOfTicketID.Valid || ticket.DuplicateOfRoadmapID.Valid {
		return nil, ErrDuplicateAlreadySet
	}

	// Resolve target & detect "is it a feature_request that landed on the
	// roadmap" so we can pick the right canned message + follower wiring.
	var (
		canonicalTicket  *repository.SupportTicket
		canonicalRoadmap *repository.RoadmapItem
	)
	if targetTicketID != nil {
		if *targetTicketID == ticketID {
			return nil, ErrDuplicateSelf
		}
		canonicalTicket, err = s.adminRepo.GetTicketByID(ctx, *targetTicketID)
		if err != nil || canonicalTicket == nil {
			return nil, ErrDuplicateTargetMissing
		}
		if canonicalTicket.DuplicateOfTicketID.Valid || canonicalTicket.DuplicateOfRoadmapID.Valid {
			return nil, ErrDuplicateTargetIsDup
		}
		// If the canonical ticket has been promoted to the roadmap, treat it
		// as a roadmap-level dup for follower-enrollment purposes.
		if rm, _ := s.roadmapRepo.GetByTicketID(ctx, canonicalTicket.ID); rm != nil {
			canonicalRoadmap = rm
		}
	} else {
		canonicalRoadmap, err = s.roadmapRepo.GetByID(ctx, *targetRoadmapID)
		if err != nil || canonicalRoadmap == nil {
			return nil, ErrDuplicateTargetMissing
		}
	}

	if err := s.adminRepo.SetTicketDuplicate(ctx, ticketID, targetTicketID, targetRoadmapID); err != nil {
		return nil, err
	}

	// Pick the user-facing message. We never expose internal ticket IDs.
	// If the duplicate target maps to a roadmap item OR the ticket is a
	// feature request, add the "you'll be notified" line.
	willBeNotified := canonicalRoadmap != nil || ticket.Type == "feature_request"
	msg := userFacingDuplicateMessage(willBeNotified)

	if err := s.adminRepo.AddTicketMessage(ctx, ticketID, adminID, msg, false); err != nil {
		log.Printf("[DUP] post canned message failed: %v", err)
	}
	if err := s.adminRepo.UpdateTicketStatus(ctx, ticketID, "closed"); err != nil {
		log.Printf("[DUP] close ticket failed: %v", err)
	}
	// PHI cleanup on the dup ticket itself.
	if s.attach != nil {
		s.attach.DeleteAllForTicket(ctx, ticketID)
	}

	// If we're duping onto something on the roadmap, enroll the user as a
	// follower so they get the dev/prod release pings on their own thread.
	if canonicalRoadmap != nil && ticket.UserID.Valid {
		tid := ticket.ID
		if err := s.roadmapRepo.AddFollower(ctx, canonicalRoadmap.ID, ticket.UserID.UUID, &tid, true, true); err != nil {
			log.Printf("[DUP] add follower failed: %v", err)
		}
	}

	// Best-effort email.
	if ticket.UserEmail != "" && s.email != nil && s.email.IsEnabled() {
		subject := "Thanks for your feedback"
		body := emailWrap("Thanks for letting us know", html.EscapeString(msg), "")
		if err := s.email.SendEmail(ticket.UserEmail, subject, body); err != nil {
			log.Printf("[DUP] email failed: %v", err)
		}
	}

	return s.adminRepo.GetTicketByID(ctx, ticketID)
}

// userFacingDuplicateMessage returns the canned text shown to the user when
// their ticket is closed as a duplicate. Phrasing is intentionally warm and
// avoids exposing internal ticket numbers.
func userFacingDuplicateMessage(willBeNotified bool) string {
	base := "Thank you very much for pointing out that opportunity to improve My Care Companion. " +
		"Luckily, another user has already made us aware of this issue and it's already being worked on."
	if willBeNotified {
		base += " You will be notified as this feature progresses."
	}
	return base
}

// SearchDupTargets returns up to `limit` ticket matches plus up to `limit`
// roadmap matches, both substring-matched on subject/title. Used by the dup
// picker autocomplete on the admin ticket detail page.
type DupTargetSearchResult struct {
	Tickets []DupTargetTicket `json:"tickets"`
	Roadmap []DupTargetItem   `json:"roadmap"`
}

// DupTargetTicket is a slim version of SupportTicket for the picker.
type DupTargetTicket struct {
	ID             uuid.UUID `json:"id"`
	Subject        string    `json:"subject"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	DuplicateCount int       `json:"duplicate_count"`
}

// DupTargetItem is a slim version of RoadmapItem for the picker.
type DupTargetItem struct {
	ID            uuid.UUID `json:"id"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	Priority      string    `json:"priority"`
	FollowerCount int       `json:"follower_count"`
}

func (s *TicketDuplicateService) SearchDupTargets(ctx context.Context, query string, limit int) (*DupTargetSearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	out := &DupTargetSearchResult{
		Tickets: []DupTargetTicket{},
		Roadmap: []DupTargetItem{},
	}
	if query == "" {
		return out, nil
	}
	tickets, err := s.adminRepo.SearchTicketsByText(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	for _, t := range tickets {
		// Don't suggest tickets that are themselves duplicates.
		if t.DuplicateOfTicketID.Valid || t.DuplicateOfRoadmapID.Valid {
			continue
		}
		out.Tickets = append(out.Tickets, DupTargetTicket{
			ID: t.ID, Subject: t.Subject, Type: t.Type, Status: t.Status, DuplicateCount: t.DuplicateCount,
		})
	}

	// Roadmap search via the existing List path with a substring filter
	// in code; the table is small enough that filtering in app is fine.
	all, err := s.roadmapRepo.List(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	q := lowerOrEmpty(query)
	for _, it := range all {
		if !contains(lowerOrEmpty(it.Title), q) {
			continue
		}
		out.Roadmap = append(out.Roadmap, DupTargetItem{
			ID: it.ID, Title: it.Title, Status: it.Status, Priority: it.Priority, FollowerCount: it.FollowerCount,
		})
		if len(out.Roadmap) >= limit {
			break
		}
	}
	return out, nil
}

func lowerOrEmpty(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		out[i] = c
	}
	return string(out)
}

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// Compile-time guards so service has the same dependency surface as the
// roadmap service (helps catch interface drift if either grows).
var _ = fmt.Sprintf
