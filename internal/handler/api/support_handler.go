package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// SupportHandler handles user-facing support ticket API endpoints
type SupportHandler struct {
	supportService *service.UserSupportService
}

// NewSupportHandler creates a new support handler
func NewSupportHandler(supportService *service.UserSupportService) *SupportHandler {
	return &SupportHandler{
		supportService: supportService,
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
