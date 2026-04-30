package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrRoadmapTitleRequired = errors.New("title is required")
	ErrRoadmapInvalidStatus = errors.New("invalid status")
	ErrRoadmapInvalidPrio   = errors.New("invalid priority")
	ErrRoadmapInvalidSource = errors.New("invalid source")
	ErrRoadmapTicketAlready = errors.New("ticket has already been promoted to the roadmap")
	ErrRoadmapTicketWrongType = errors.New("only feature_request tickets can be promoted")
	ErrDuplicateSelf         = errors.New("a ticket cannot be a duplicate of itself")
	ErrDuplicateAlreadySet   = errors.New("ticket is already marked as a duplicate")
	ErrDuplicateTargetMissing = errors.New("duplicate target not found")
	ErrDuplicateTargetIsDup  = errors.New("the chosen target is itself a duplicate; pick the canonical one")
)

// validRoadmapStatuses, validRoadmapPriorities, validRoadmapSources mirror the
// Postgres enums; keep these in sync with migration 00019.
var (
	validRoadmapStatuses   = map[string]bool{"proposed": true, "planned": true, "in_progress": true, "in_dev": true, "in_prod": true, "cancelled": true}
	validRoadmapPriorities = map[string]bool{"p0": true, "p1": true, "p2": true, "p3": true}
	validRoadmapSources    = map[string]bool{"manual": true, "internal": true, "feature_request": true}
)

// RoadmapService coordinates roadmap CRUD plus the cross-table flows:
// promoting a feature_request ticket onto the roadmap (which closes the
// ticket and emails the requester) and marking an item live in dev / prod
// (which posts a follow-up ticket message + email and writes a version_log
// entry).
type RoadmapService struct {
	repo      repository.RoadmapRepository
	adminRepo repository.AdminRepository
	email     *EmailService
	db        *sql.DB
}

// NewRoadmapService constructs a RoadmapService. The raw *sql.DB is used only
// to write into version_log, which doesn't have a dedicated repo yet.
func NewRoadmapService(repo repository.RoadmapRepository, adminRepo repository.AdminRepository, email *EmailService, db *sql.DB) *RoadmapService {
	return &RoadmapService{repo: repo, adminRepo: adminRepo, email: email, db: db}
}

// CreateRoadmapInput is the payload accepted by Create / Update.
type CreateRoadmapInput struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Priority     string `json:"priority"`
	Source       string `json:"source"`
	NotifyOnDev  bool   `json:"notify_on_dev"`
	NotifyOnProd bool   `json:"notify_on_prod"`
}

// List returns all roadmap items, optionally filtered.
func (s *RoadmapService) List(ctx context.Context, status, priority, source string) ([]repository.RoadmapItem, error) {
	return s.repo.List(ctx, status, priority, source)
}

// Get returns a single roadmap item.
func (s *RoadmapService) Get(ctx context.Context, id uuid.UUID) (*repository.RoadmapItem, error) {
	return s.repo.GetByID(ctx, id)
}

// GetByTicketID returns the roadmap item promoted from a given support ticket,
// or nil if the ticket has not been promoted.
func (s *RoadmapService) GetByTicketID(ctx context.Context, ticketID uuid.UUID) (*repository.RoadmapItem, error) {
	return s.repo.GetByTicketID(ctx, ticketID)
}

// ListFollowers returns all users subscribed to release notifications for a
// given roadmap item.
func (s *RoadmapService) ListFollowers(ctx context.Context, id uuid.UUID) ([]repository.RoadmapFollower, error) {
	return s.repo.ListFollowers(ctx, id)
}

// Create inserts a manual or internal roadmap item. For feature_request items
// promoted from a ticket, callers should use AddFromTicket instead.
func (s *RoadmapService) Create(ctx context.Context, in CreateRoadmapInput, createdBy uuid.UUID) (*repository.RoadmapItem, error) {
	if in.Source == "feature_request" {
		return nil, errors.New("use AddFromTicket for feature_request items")
	}
	item, err := s.buildItem(in)
	if err != nil {
		return nil, err
	}
	item.CreatedBy = nullUUID(createdBy)
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, item.ID)
}

// Update modifies an existing roadmap item. Source / linked ticket / requester
// are immutable post-creation.
func (s *RoadmapService) Update(ctx context.Context, id uuid.UUID, in CreateRoadmapInput) (*repository.RoadmapItem, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, sql.ErrNoRows
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, ErrRoadmapTitleRequired
	}
	if !validRoadmapStatuses[in.Status] {
		return nil, ErrRoadmapInvalidStatus
	}
	if !validRoadmapPriorities[in.Priority] {
		return nil, ErrRoadmapInvalidPrio
	}
	existing.Title = strings.TrimSpace(in.Title)
	existing.Description = in.Description
	existing.Status = in.Status
	existing.Priority = in.Priority
	existing.NotifyOnDev = in.NotifyOnDev
	existing.NotifyOnProd = in.NotifyOnProd
	if err := s.repo.Update(ctx, existing); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, id)
}

// Delete removes an item.
func (s *RoadmapService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

// AddFromTicket promotes a feature_request ticket onto the roadmap:
//  1. creates a roadmap_items row linked back to the ticket
//  2. posts the canned "your idea was AWESOME" message to the ticket thread
//  3. closes the ticket
//  4. emails the requester (best-effort; failures are logged, not fatal)
func (s *RoadmapService) AddFromTicket(ctx context.Context, ticketID, adminID uuid.UUID, priority string) (*repository.RoadmapItem, error) {
	ticket, err := s.adminRepo.GetTicketByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if ticket == nil {
		return nil, sql.ErrNoRows
	}
	if ticket.Type != "feature_request" {
		return nil, ErrRoadmapTicketWrongType
	}
	if existing, _ := s.repo.GetByTicketID(ctx, ticketID); existing != nil {
		return nil, ErrRoadmapTicketAlready
	}
	if !validRoadmapPriorities[priority] {
		priority = "p2"
	}

	item := &repository.RoadmapItem{
		Title:           ticket.Subject,
		Description:     ticket.Description,
		Status:          "planned",
		Priority:        priority,
		Source:          "feature_request",
		SourceTicketID:  models.NullUUID{UUID: ticket.ID, Valid: true},
		RequesterUserID: ticket.UserID,
		NotifyOnDev:     true,
		NotifyOnProd:    true,
		CreatedBy:       nullUUID(adminID),
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}

	// Enroll the original requester as the first follower, then enroll any
	// existing duplicates of this ticket so they get release pings too.
	if ticket.UserID.Valid {
		tid := ticket.ID
		if err := s.repo.AddFollower(ctx, item.ID, ticket.UserID.UUID, &tid, true, true); err != nil {
			log.Printf("[ROADMAP] enroll original requester as follower failed: %v", err)
		}
	}
	dups, _ := s.adminRepo.GetTicketDuplicates(ctx, ticket.ID)
	for _, d := range dups {
		if !d.UserID.Valid {
			continue
		}
		dt := d.ID
		if err := s.repo.AddFollower(ctx, item.ID, d.UserID.UUID, &dt, true, true); err != nil {
			log.Printf("[ROADMAP] enroll dup follower failed: %v", err)
		}
	}

	const cannedMsg = "Thank you for being such a valuable part of our community. " +
		"After review, our dev team decided your idea was AWESOME and was added to our roadmap. " +
		"This ticket will be marked as closed but you'll receive a message when your requested feature goes live."
	if err := s.adminRepo.AddTicketMessage(ctx, ticket.ID, adminID, cannedMsg, false); err != nil {
		log.Printf("[ROADMAP] add canned ticket message failed: %v", err)
	}
	if err := s.adminRepo.UpdateTicketStatus(ctx, ticket.ID, "closed"); err != nil {
		log.Printf("[ROADMAP] close ticket failed: %v", err)
	}

	// Best-effort email; don't block the promotion if SMTP is down.
	if ticket.UserEmail != "" && s.email != nil && s.email.IsEnabled() {
		subject := "Your feature request is on our roadmap!"
		body := emailWrap("Your idea made the roadmap", html.EscapeString(cannedMsg), "")
		if err := s.email.SendEmail(ticket.UserEmail, subject, body); err != nil {
			log.Printf("[ROADMAP] email requester failed: %v", err)
		}
	}

	return s.repo.GetByID(ctx, item.ID)
}

// MarkLiveDev sets dev_released_at, posts a notification message on the
// originating ticket (if any), emails the requester (if opted in), and writes
// a version_log entry. Repeated calls are no-ops on dev_released_at.
func (s *RoadmapService) MarkLiveDev(ctx context.Context, id, adminID uuid.UUID) (*repository.RoadmapItem, error) {
	return s.markLive(ctx, id, adminID, "dev")
}

// MarkLiveProd is the prod-side analogue.
func (s *RoadmapService) MarkLiveProd(ctx context.Context, id, adminID uuid.UUID) (*repository.RoadmapItem, error) {
	return s.markLive(ctx, id, adminID, "prod")
}

func (s *RoadmapService) markLive(ctx context.Context, id, adminID uuid.UUID, env string) (*repository.RoadmapItem, error) {
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, sql.ErrNoRows
	}

	alreadyReleased := false
	if env == "dev" {
		alreadyReleased = item.DevReleasedAt.Valid
		if err := s.repo.MarkDevReleased(ctx, id); err != nil {
			return nil, err
		}
	} else {
		alreadyReleased = item.ProdReleasedAt.Valid
		if err := s.repo.MarkProdReleased(ctx, id); err != nil {
			return nil, err
		}
	}

	// Don't double-notify on a re-press of the same button.
	if !alreadyReleased {
		s.notifyRequester(ctx, item, adminID, env)
		s.writeVersionLog(ctx, item, adminID, env)
	}

	return s.repo.GetByID(ctx, id)
}

// notifyRequester posts a follow-up ticket message and (if SMTP is up) sends
// an email to every follower of the roadmap item that has the relevant
// notify_on_* flag enabled. All sends are best-effort — failures are logged.
//
// The item-level NotifyOn* flags act as a global gate; per-follower flags
// allow opt-out by individual users.
func (s *RoadmapService) notifyRequester(ctx context.Context, item *repository.RoadmapItem, adminID uuid.UUID, env string) {
	envLabel := "dev"
	envFriendly := "our development environment for testing"
	itemAllowed := item.NotifyOnDev
	if env == "prod" {
		envLabel = "prod"
		envFriendly = "production"
		itemAllowed = item.NotifyOnProd
	}
	if !itemAllowed {
		return
	}

	followers, err := s.repo.ListFollowers(ctx, item.ID)
	if err != nil {
		log.Printf("[ROADMAP] %s release: list followers failed: %v", envLabel, err)
		return
	}

	msgTmpl := "Heads up — your requested feature \"%s\" is now live in %s. Thanks again for the great idea!"
	for _, f := range followers {
		// Per-follower opt-out.
		if env == "dev" && !f.NotifyOnDev {
			continue
		}
		if env == "prod" && !f.NotifyOnProd {
			continue
		}
		msg := fmt.Sprintf(msgTmpl, item.Title, envFriendly)
		if f.SourceTicketID.Valid {
			if err := s.adminRepo.AddTicketMessage(ctx, f.SourceTicketID.UUID, adminID, msg, false); err != nil {
				log.Printf("[ROADMAP] %s release: post ticket message to %s failed: %v", envLabel, f.UserEmail, err)
			}
		}
		if f.UserEmail != "" && s.email != nil && s.email.IsEnabled() {
			subject := fmt.Sprintf("Your feature is live in %s: %s", envFriendly, item.Title)
			body := emailWrap(
				fmt.Sprintf("Your feature is live in %s", envFriendly),
				html.EscapeString(msg),
				"",
			)
			if err := s.email.SendEmail(f.UserEmail, subject, body); err != nil {
				log.Printf("[ROADMAP] %s release: email %s failed: %v", envLabel, f.UserEmail, err)
			}
		}
	}
}

// writeVersionLog drops a row into version_log so the existing
// /admin/version-log UI surfaces this release.
func (s *RoadmapService) writeVersionLog(ctx context.Context, item *repository.RoadmapItem, adminID uuid.UUID, env string) {
	const q = `
        INSERT INTO version_log (id, environment, entry_type, title, description, created_by, created_at, updated_at)
        VALUES (gen_random_uuid(), $1, 'feature', $2, $3, $4, NOW(), NOW())
    `
	if _, err := s.db.ExecContext(ctx, q, env, item.Title, item.Description, adminID); err != nil {
		log.Printf("[ROADMAP] write version_log (%s) failed: %v", env, err)
	}
}

func (s *RoadmapService) buildItem(in CreateRoadmapInput) (*repository.RoadmapItem, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, ErrRoadmapTitleRequired
	}
	if in.Status == "" {
		in.Status = "proposed"
	}
	if in.Priority == "" {
		in.Priority = "p2"
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	if !validRoadmapStatuses[in.Status] {
		return nil, ErrRoadmapInvalidStatus
	}
	if !validRoadmapPriorities[in.Priority] {
		return nil, ErrRoadmapInvalidPrio
	}
	if !validRoadmapSources[in.Source] {
		return nil, ErrRoadmapInvalidSource
	}
	return &repository.RoadmapItem{
		Title:        title,
		Description:  in.Description,
		Status:       in.Status,
		Priority:     in.Priority,
		Source:       in.Source,
		NotifyOnDev:  in.NotifyOnDev,
		NotifyOnProd: in.NotifyOnProd,
	}, nil
}

func nullUUID(id uuid.UUID) models.NullUUID {
	if id == uuid.Nil {
		return models.NullUUID{}
	}
	return models.NullUUID{UUID: id, Valid: true}
}

// emailWrap renders a minimal branded HTML email shell. heading/body should
// already be HTML-safe (callers escape user content).
func emailWrap(heading, bodyHTML, footer string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><body style="font-family:Arial,sans-serif;color:#1f2937;max-width:600px;margin:0 auto;padding:24px;">`)
	b.WriteString(`<h2 style="color:#4f46e5;">`)
	b.WriteString(html.EscapeString(heading))
	b.WriteString(`</h2><p style="line-height:1.5;">`)
	b.WriteString(bodyHTML)
	b.WriteString(`</p>`)
	if footer != "" {
		b.WriteString(`<p style="color:#6b7280;font-size:12px;margin-top:24px;">`)
		b.WriteString(html.EscapeString(footer))
		b.WriteString(`</p>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}
