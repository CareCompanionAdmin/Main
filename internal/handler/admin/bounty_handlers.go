package admin

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

// ============================================================================
// JSON API
// ============================================================================

// ListBountyCandidates returns this month's eligible bug + feature candidates
// plus what's already been awarded so the admin UI can show progress against
// the per-type cap.
func (h *Handler) ListBountyCandidates(w http.ResponseWriter, r *http.Request) {
	if h.bountyService == nil {
		http.Error(w, "Bounty service unavailable", http.StatusServiceUnavailable)
		return
	}
	month := service.CurrentMonth()
	bugs, feats, err := h.bountyService.EligibleCandidates(r.Context(), month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	awards, _ := h.bountyService.ListThisMonth(r.Context(), month)
	bugCap, featureCap := h.bountyService.Caps()

	respondJSON(w, map[string]interface{}{
		"month":       month,
		"bugs":        bugs,
		"features":    feats,
		"awards":      awards,
		"bug_cap":     bugCap,
		"feature_cap": featureCap,
	})
}

// SelectBountyRequest is the body of POST /admin/marketing/bounty/select.
// The candidate fields are echoed back from the client (it just received
// them from /bounty/candidates) so we don't have to re-query everything
// to recover the SourceTicketID.
type SelectBountyRequest struct {
	Type            string  `json:"type"`              // "bug" | "feature"
	TicketID        *string `json:"ticket_id,omitempty"`
	RoadmapItemID   *string `json:"roadmap_item_id,omitempty"`
	RecipientUserID string  `json:"recipient_user_id"`
	RecipientEmail  string  `json:"recipient_email,omitempty"`
	Subject         string  `json:"subject"`
	SourceTicketID  *string `json:"source_ticket_id,omitempty"`
	Notes           string  `json:"notes,omitempty"`
}

func (h *Handler) SelectBountyCandidate(w http.ResponseWriter, r *http.Request) {
	if h.bountyService == nil {
		http.Error(w, "Bounty service unavailable", http.StatusServiceUnavailable)
		return
	}
	cand, err := decodeBountyCandidate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	award, err := h.bountyService.Select(r.Context(), cand, claims.UserID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.logAction(r, "bounty_select", "bounty_award", award.ID, map[string]interface{}{
		"type":      cand.Type,
		"recipient": cand.RecipientEmail,
		"subject":   cand.Subject,
	})
	respondJSON(w, award)
}

func (h *Handler) ThanksAnywayBountyCandidate(w http.ResponseWriter, r *http.Request) {
	if h.bountyService == nil {
		http.Error(w, "Bounty service unavailable", http.StatusServiceUnavailable)
		return
	}
	cand, err := decodeBountyCandidate(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	award, err := h.bountyService.ThanksAnyway(r.Context(), cand, claims.UserID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "bounty_thanks_anyway", "bounty_award", award.ID, map[string]interface{}{
		"type":      cand.Type,
		"recipient": cand.RecipientEmail,
		"subject":   cand.Subject,
	})
	respondJSON(w, award)
}

func decodeBountyCandidate(r *http.Request) (repository.BountyCandidate, error) {
	var req SelectBountyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return repository.BountyCandidate{}, err
	}
	if req.Type != "bug" && req.Type != "feature" {
		return repository.BountyCandidate{}, errBadRequest("type must be 'bug' or 'feature'")
	}
	uid, err := uuid.Parse(req.RecipientUserID)
	if err != nil {
		return repository.BountyCandidate{}, errBadRequest("recipient_user_id must be a valid UUID")
	}
	c := repository.BountyCandidate{
		Type:            req.Type,
		RecipientUserID: uid,
		RecipientEmail:  req.RecipientEmail,
		Subject:         req.Subject,
	}
	if req.TicketID != nil && *req.TicketID != "" {
		id, err := uuid.Parse(*req.TicketID)
		if err != nil {
			return repository.BountyCandidate{}, errBadRequest("ticket_id must be a valid UUID")
		}
		c.TicketID = models.NullUUID{UUID: id, Valid: true}
	}
	if req.RoadmapItemID != nil && *req.RoadmapItemID != "" {
		id, err := uuid.Parse(*req.RoadmapItemID)
		if err != nil {
			return repository.BountyCandidate{}, errBadRequest("roadmap_item_id must be a valid UUID")
		}
		c.RoadmapItemID = models.NullUUID{UUID: id, Valid: true}
	}
	if req.SourceTicketID != nil && *req.SourceTicketID != "" {
		id, err := uuid.Parse(*req.SourceTicketID)
		if err != nil {
			return repository.BountyCandidate{}, errBadRequest("source_ticket_id must be a valid UUID")
		}
		c.SourceTicketID = models.NullUUID{UUID: id, Valid: true}
	}
	return c, nil
}

type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }
func errBadRequest(msg string) error     { return &badRequestError{msg: msg} }

// ============================================================================
// UI PAGE
// ============================================================================

func (h *Handler) BountyProgramPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID: claims.UserID, Email: claims.Email, FirstName: claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}
	if h.bountyService == nil {
		http.Error(w, "Bounty service unavailable", http.StatusServiceUnavailable)
		return
	}

	month := service.CurrentMonth()
	bugs, feats, err := h.bountyService.EligibleCandidates(r.Context(), month)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	awards, _ := h.bountyService.ListThisMonth(r.Context(), month)
	history, _ := h.bountyService.History(r.Context(), 25)
	bugCap, featureCap := h.bountyService.Caps()

	// Per-type counts of "selected" awards this month.
	var selBugs, selFeats int
	for _, a := range awards {
		if a.Decision != "selected" {
			continue
		}
		if a.AwardType == "bug" {
			selBugs++
		} else {
			selFeats++
		}
	}

	tmpl, err := parseTemplates("layout.html", "bounty_program.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Bounty Program",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"month":         month,
			"bugs":          bugs,
			"features":      feats,
			"awards":        awards,
			"history":       history,
			"bug_cap":       bugCap,
			"feature_cap":   featureCap,
			"bugs_selected": selBugs,
			"feats_selected": selFeats,
		},
	})
}
