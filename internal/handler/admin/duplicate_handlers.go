package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// ioCopy is a small alias so the handler signature reads cleanly.
var ioCopy = io.Copy

// markDuplicateRequest is the JSON body accepted by the mark-duplicate
// endpoint. Exactly one of TargetTicketID / TargetRoadmapID must be set.
type markDuplicateRequest struct {
	TargetTicketID  string `json:"target_ticket_id,omitempty"`
	TargetRoadmapID string `json:"target_roadmap_id,omitempty"`
}

// MarkTicketDuplicate handles POST /api/admin/support/tickets/{id}/mark-duplicate.
// Available to support + super_admin (it lives under the support route group).
func (h *Handler) MarkTicketDuplicate(w http.ResponseWriter, r *http.Request) {
	if h.dupService == nil {
		http.Error(w, "Duplicate service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket id", http.StatusBadRequest)
		return
	}
	var body markDuplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	var (
		ticketTarget  *uuid.UUID
		roadmapTarget *uuid.UUID
	)
	if body.TargetTicketID != "" {
		t, err := uuid.Parse(body.TargetTicketID)
		if err != nil {
			http.Error(w, "Invalid target_ticket_id", http.StatusBadRequest)
			return
		}
		ticketTarget = &t
	}
	if body.TargetRoadmapID != "" {
		t, err := uuid.Parse(body.TargetRoadmapID)
		if err != nil {
			http.Error(w, "Invalid target_roadmap_id", http.StatusBadRequest)
			return
		}
		roadmapTarget = &t
	}

	claims := middleware.GetAuthClaims(r.Context())
	updated, err := h.dupService.MarkAsDuplicate(r.Context(), id, ticketTarget, roadmapTarget, claims.UserID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrDuplicateSelf),
			errors.Is(err, service.ErrDuplicateAlreadySet),
			errors.Is(err, service.ErrDuplicateTargetIsDup):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, service.ErrDuplicateTargetMissing):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	h.logAction(r, "mark_ticket_duplicate", "ticket", id, map[string]interface{}{
		"target_ticket_id":  body.TargetTicketID,
		"target_roadmap_id": body.TargetRoadmapID,
	})
	respondJSON(w, updated)
}

// SearchDuplicateTargets handles GET /api/admin/support/duplicate-targets?q=...&limit=10
// returning matching tickets and roadmap items for the picker autocomplete.
func (h *Handler) SearchDuplicateTargets(w http.ResponseWriter, r *http.Request) {
	if h.dupService == nil {
		http.Error(w, "Duplicate service unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query().Get("q")
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 25 {
			limit = n
		}
	}
	res, err := h.dupService.SearchDupTargets(r.Context(), q, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, res)
}

// ListTicketAttachments returns the attachments on a ticket for admin view.
func (h *Handler) ListTicketAttachments(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		http.Error(w, "Attachment service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket id", http.StatusBadRequest)
		return
	}
	atts, err := h.attachService.List(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, atts)
}

// FetchTicketAttachment streams an attachment for admin view (no ownership
// check; auth is enforced by the support / super_admin middleware).
func (h *Handler) FetchTicketAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		http.Error(w, "Attachment service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid attachment id", http.StatusBadRequest)
		return
	}
	body, att, err := h.attachService.FetchForAdmin(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to open attachment: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", "inline; filename=\""+att.OriginalName+"\"")
	if att.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))
	}
	_, _ = ioCopy(w, body)
}

// ListTicketDuplicates handles GET /api/admin/support/tickets/{id}/duplicates
// returning all tickets that point to this one as their canonical.
func (h *Handler) ListTicketDuplicates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	dups, err := h.adminRepo.GetTicketDuplicates(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, dups)
}
