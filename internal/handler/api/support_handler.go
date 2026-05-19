package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// Maximum lengths for user-supplied free-text fields. Defense-in-depth against
// resource exhaustion and XSS payload bloat — render-side escaping is still
// required (templates should be audited separately).
const (
	maxTicketDescriptionLen = 50000
	maxTicketMessageLen     = 10000
)

// SupportHandler handles user-facing support ticket API endpoints
type SupportHandler struct {
	supportService *service.UserSupportService
	attachService  *service.TicketAttachmentService
}

// NewSupportHandler creates a new support handler
func NewSupportHandler(supportService *service.UserSupportService, attachService *service.TicketAttachmentService) *SupportHandler {
	return &SupportHandler{
		supportService: supportService,
		attachService:  attachService,
	}
}

// ListTickets returns all support tickets for the current user
func (h *SupportHandler) ListTickets(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	tickets, err := h.supportService.GetTickets(r.Context(), userID)
	if err != nil {
		respondInternalError(w, "Failed to get support tickets")
		return
	}

	respondOK(w, tickets)
}

// CreateTicket creates a new support ticket
func (h *SupportHandler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req service.CreateTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Subject == "" {
		respondBadRequest(w, "Subject is required")
		return
	}
	if req.Description == "" {
		respondBadRequest(w, "Description is required")
		return
	}
	if len(req.Description) > maxTicketDescriptionLen {
		respondBadRequest(w, "Description is too long")
		return
	}

	ticket, err := h.supportService.CreateTicket(r.Context(), userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create ticket")
		return
	}

	respondCreated(w, ticket)
}

// GetTicket returns a specific ticket with its messages
func (h *SupportHandler) GetTicket(w http.ResponseWriter, r *http.Request) {
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	ticket, messages, err := h.supportService.GetTicketWithMessages(r.Context(), ticketID, userID)
	if err != nil {
		if err == service.ErrTicketNotFound {
			respondNotFound(w, "Ticket not found")
			return
		}
		respondInternalError(w, "Failed to get ticket")
		return
	}

	respondOK(w, map[string]interface{}{
		"ticket":   ticket,
		"messages": messages,
	})
}

// AddMessage adds a reply to a ticket
func (h *SupportHandler) AddMessage(w http.ResponseWriter, r *http.Request) {
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	var req struct {
		Message string `json:"message"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Message == "" {
		respondBadRequest(w, "Message is required")
		return
	}
	if len(req.Message) > maxTicketMessageLen {
		respondBadRequest(w, "Message is too long")
		return
	}

	if err := h.supportService.AddReply(r.Context(), ticketID, userID, req.Message); err != nil {
		if err == service.ErrTicketNotFound {
			respondNotFound(w, "Ticket not found")
			return
		}
		respondInternalError(w, "Failed to add message")
		return
	}

	respondOK(w, map[string]string{"status": "sent"})
}

// MarkRead marks a ticket as read
func (h *SupportHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	if err := h.supportService.MarkTicketRead(r.Context(), ticketID, userID); err != nil {
		respondInternalError(w, "Failed to mark ticket as read")
		return
	}

	respondOK(w, map[string]string{"status": "read"})
}

// ============================================================================
// ATTACHMENTS
// ============================================================================

// UploadAttachment accepts a multipart upload of one file under field name
// "file". On success the persisted attachment record is returned.
//
// Optional `kind` form field — set to "recording" for in-browser screen+mic
// recordings so the UI can label them appropriately.
func (h *SupportHandler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		respondInternalError(w, "Attachment service unavailable")
		return
	}
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}
	userID := middleware.GetUserID(r.Context())

	// Verify ownership before accepting bytes — cheap, and avoids spending
	// time uploading something we'd reject anyway.
	if _, err := h.supportService.GetTicketByID(r.Context(), ticketID, userID); err != nil {
		respondNotFound(w, "Ticket not found")
		return
	}

	// Cap the multipart body. ParseMultipartForm reads up to maxMemory in
	// memory; the rest spills to temp files. Add a safety margin (1 MB) over
	// the configured per-file cap so headers fit.
	maxBytes := h.attachService.MaxBytes() + 1*1024*1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(8 * 1024 * 1024); err != nil {
		respondBadRequest(w, "Upload too large or malformed")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "Missing file field")
		return
	}
	defer file.Close()

	// Sanitize the user-supplied filename. filepath.Base strips any path
	// components (e.g. "../../etc/passwd" -> "passwd") so a malicious upload
	// can't influence the storage path or escape the attachments directory.
	safeName := filepath.Base(header.Filename)
	if safeName == "." || safeName == "/" || safeName == `\` {
		safeName = "upload"
	}

	att, err := h.attachService.Upload(r.Context(), service.UploadInput{
		TicketID:     ticketID,
		UploaderID:   userID,
		Filename:     safeName,
		ContentType:  header.Header.Get("Content-Type"),
		Body:         file,
		KindOverride: r.FormValue("kind"),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAttachmentTooBig),
			errors.Is(err, service.ErrAttachmentTypeNotAllowed),
			errors.Is(err, service.ErrAttachmentLimitReached):
			respondBadRequest(w, err.Error())
		default:
			respondInternalError(w, "Upload failed: "+err.Error())
		}
		return
	}
	respondCreated(w, att)
}

// ListAttachments returns the attachments visible to the calling user for a
// ticket they own.
func (h *SupportHandler) ListAttachments(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		respondInternalError(w, "Attachment service unavailable")
		return
	}
	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}
	userID := middleware.GetUserID(r.Context())
	if _, err := h.supportService.GetTicketByID(r.Context(), ticketID, userID); err != nil {
		respondNotFound(w, "Ticket not found")
		return
	}
	atts, err := h.attachService.List(r.Context(), ticketID)
	if err != nil {
		respondInternalError(w, "Failed to list attachments")
		return
	}
	respondOK(w, atts)
}

// FetchAttachment streams the file bytes for a ticket the user owns.
func (h *SupportHandler) FetchAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		respondInternalError(w, "Attachment service unavailable")
		return
	}
	attID, err := parseUUID(chi.URLParam(r, "attachmentID"))
	if err != nil {
		respondBadRequest(w, "Invalid attachment ID")
		return
	}
	userID := middleware.GetUserID(r.Context())
	body, att, err := h.attachService.FetchForUser(r.Context(), attID, userID)
	if err != nil {
		if errors.Is(err, service.ErrAttachmentForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrAttachmentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		respondInternalError(w, "Failed to open attachment")
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", "inline; filename=\""+att.OriginalName+"\"")
	if att.SizeBytes > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(att.SizeBytes, 10))
	}
	_, _ = io.Copy(w, body)
}

// DeleteAttachment lets a user remove their own attachment from their ticket.
func (h *SupportHandler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		respondInternalError(w, "Attachment service unavailable")
		return
	}
	attID, err := parseUUID(chi.URLParam(r, "attachmentID"))
	if err != nil {
		respondBadRequest(w, "Invalid attachment ID")
		return
	}
	userID := middleware.GetUserID(r.Context())
	if err := h.attachService.DeleteByOwner(r.Context(), attID, userID); err != nil {
		if errors.Is(err, service.ErrAttachmentForbidden) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if errors.Is(err, service.ErrAttachmentNotFound) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		respondInternalError(w, "Delete failed")
		return
	}
	respondOK(w, map[string]string{"status": "deleted"})
}

// GetAttachmentLimits exposes the current per-file size cap and per-ticket
// count cap so the UI can show them in the consent modal.
func (h *SupportHandler) GetAttachmentLimits(w http.ResponseWriter, r *http.Request) {
	if h.attachService == nil {
		respondInternalError(w, "Attachment service unavailable")
		return
	}
	respondOK(w, map[string]interface{}{
		"max_bytes":      h.attachService.MaxBytes(),
		"max_per_ticket": h.attachService.MaxPerTicket(),
	})
}

// GetUnread returns whether the user has unread support messages
func (h *SupportHandler) GetUnread(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	hasUnread, err := h.supportService.HasUnreadSupportMessages(r.Context(), userID)
	if err != nil {
		respondInternalError(w, "Failed to check unread status")
		return
	}

	count, err := h.supportService.GetUnreadTicketCount(r.Context(), userID)
	if err != nil {
		count = 0
	}

	respondOK(w, map[string]interface{}{
		"has_unread":   hasUnread,
		"unread_count": count,
	})
}

// Reopen flips a resolved or closed ticket back to open and stamps
// reopened_at. Validates ownership server-side. Returns 404 when the
// ticket doesn't belong to the caller, 409 when it exists but is
// already in an open state.
func (h *SupportHandler) Reopen(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	ticketID, err := parseUUID(chi.URLParam(r, "ticketID"))
	if err != nil {
		respondBadRequest(w, "Invalid ticket ID")
		return
	}

	err = h.supportService.ReopenTicket(r.Context(), ticketID, userID)
	if err == nil {
		respondOK(w, map[string]interface{}{"status": "open"})
		return
	}
	switch {
	case errors.Is(err, service.ErrTicketNotFound):
		respondNotFound(w, "Ticket not found")
	case errors.Is(err, service.ErrTicketNotReopenable):
		respondError(w, "Ticket is already open or in progress", http.StatusConflict)
	default:
		respondInternalError(w, "Failed to reopen ticket")
	}
}
