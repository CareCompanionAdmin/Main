package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
)

// ============================================================================
// JSON API
// ============================================================================

// ListBetaInvitations returns every beta invitation, newest first.
func (h *Handler) ListBetaInvitations(w http.ResponseWriter, r *http.Request) {
	if h.betaService == nil {
		http.Error(w, "Beta service unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := h.betaService.List(r.Context())
	if err != nil {
		http.Error(w, "List failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, items)
}

// CreateBetaInvitationRequest is the body of POST /admin/marketing/beta/invitations.
type CreateBetaInvitationRequest struct {
	Email string `json:"email"`
	Notes string `json:"notes,omitempty"`
}

// CreateBetaInvitation creates a new invitation row and sends the onboarding
// email. Idempotent: sending the same email twice returns the existing row.
func (h *Handler) CreateBetaInvitation(w http.ResponseWriter, r *http.Request) {
	if h.betaService == nil {
		http.Error(w, "Beta service unavailable", http.StatusServiceUnavailable)
		return
	}
	var req CreateBetaInvitationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	inv, err := h.betaService.Invite(r.Context(), req.Email, claims.UserID, req.Notes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.logAction(r, "create_beta_invitation", "beta_invitation", inv.ID, map[string]interface{}{"email": inv.Email})
	respondJSON(w, inv)
}

// ResendBetaInvitation re-issues the onboarding email for an existing invitation.
func (h *Handler) ResendBetaInvitation(w http.ResponseWriter, r *http.Request) {
	if h.betaService == nil {
		http.Error(w, "Beta service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	if err := h.betaService.Resend(r.Context(), id); err != nil {
		http.Error(w, "Resend failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "resend_beta_invitation", "beta_invitation", id, nil)
	respondJSON(w, map[string]string{"status": "sent"})
}

// ============================================================================
// UI PAGE
// ============================================================================

// BetaProgramPage renders the marketing-side beta invite admin UI.
func (h *Handler) BetaProgramPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID: claims.UserID, Email: claims.Email, FirstName: claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	var items interface{}
	ascReady := false
	if h.betaService != nil {
		items, _ = h.betaService.List(r.Context())
	}
	if h.betaService != nil {
		ascReady = h.betaService.ASCConfigured()
	}

	tmpl, err := parseTemplates("layout.html", "beta_program.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Beta Program",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"items":     items,
			"asc_ready": ascReady,
		},
	})
}

