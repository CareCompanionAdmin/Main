package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type ChatHandler struct {
	chatService   *service.ChatService
	familyService *service.FamilyService
}

func NewChatHandler(chatService *service.ChatService, familyService *service.FamilyService) *ChatHandler {
	return &ChatHandler{
		chatService:   chatService,
		familyService: familyService,
	}
}

// ListThreads returns all chat threads for the user's family
func (h *ChatHandler) ListThreads(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())
	userID := middleware.GetUserID(r.Context())

	threads, err := h.chatService.GetThreads(r.Context(), familyID, userID)
	if err != nil {
		respondInternalError(w, "Failed to get chat threads")
		return
	}

	respondOK(w, threads)
}

// CreateThread creates a new chat thread
func (h *ChatHandler) CreateThread(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	var req models.CreateThreadRequest

	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Title == "" {
		respondBadRequest(w, "Title is required")
		return
	}

	thread, err := h.chatService.CreateThread(r.Context(), familyID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create thread")
		return
	}

	respondCreated(w, thread)
}

// GetThread returns a specific thread with messages
func (h *ChatHandler) GetThread(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	thread, err := h.chatService.GetThread(r.Context(), threadID, userID)
	if err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		if err == service.ErrThreadNotFound {
			respondNotFound(w, "Thread not found")
			return
		}
		respondInternalError(w, "Failed to get thread")
		return
	}

	// Get messages
	messages, err := h.chatService.GetMessages(r.Context(), threadID, userID, 50, 0)
	if err != nil {
		messages = []models.ChatMessage{}
	}

	respondOK(w, map[string]interface{}{
		"thread":   thread,
		"messages": messages,
	})
}

// SendMessage sends a message to a thread
func (h *ChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	var req models.SendMessageRequest

	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.MessageText == "" {
		respondBadRequest(w, "Message content is required")
		return
	}

	message, err := h.chatService.SendMessage(r.Context(), threadID, userID, &req)
	if err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to send message")
		return
	}

	respondCreated(w, message)
}

// GetMessages returns messages for a thread with pagination
func (h *ChatHandler) GetMessages(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	messages, err := h.chatService.GetMessages(r.Context(), threadID, userID, limit, offset)
	if err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to get messages")
		return
	}

	respondOK(w, messages)
}

// AddParticipant adds a user to a thread
func (h *ChatHandler) AddParticipant(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	requesterID := middleware.GetUserID(r.Context())

	var req struct {
		UserID string `json:"user_id"`
	}

	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	userID, err := parseUUID(req.UserID)
	if err != nil {
		respondBadRequest(w, "Invalid user ID")
		return
	}

	if err := h.chatService.AddParticipant(r.Context(), threadID, userID, requesterID); err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to add participant")
		return
	}

	respondOK(w, map[string]string{"status": "added"})
}

// GetUnreadCount returns unread message counts
func (h *ChatHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	count, err := h.chatService.GetTotalUnreadCount(r.Context(), familyID, userID)
	if err != nil {
		respondInternalError(w, "Failed to get unread count")
		return
	}

	respondOK(w, map[string]int{"unread_count": count})
}
