package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/config"
	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type ChatHandler struct {
	chatService   *service.ChatService
	familyService *service.FamilyService
	storageConfig *config.StorageConfig
}

func NewChatHandler(chatService *service.ChatService, familyService *service.FamilyService, storageConfig *config.StorageConfig) *ChatHandler {
	// Ensure upload directory exists
	if storageConfig != nil && storageConfig.UploadDir != "" {
		os.MkdirAll(filepath.Join(storageConfig.UploadDir, "chat"), 0755)
	}
	return &ChatHandler{
		chatService:   chatService,
		familyService: familyService,
		storageConfig: storageConfig,
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

	// Require either message text or attachments
	if req.MessageText == "" && len(req.Attachments) == 0 {
		respondBadRequest(w, "Message content or attachment is required")
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

// GetParticipants returns participants for a thread
func (h *ChatHandler) GetParticipants(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	// Verify user is a participant
	thread, err := h.chatService.GetThread(r.Context(), threadID, userID)
	if err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to get thread")
		return
	}

	respondOK(w, thread.Participants)
}

// RemoveParticipant removes a user from a thread
func (h *ChatHandler) RemoveParticipant(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	participantID, err := parseUUID(chi.URLParam(r, "participantID"))
	if err != nil {
		respondBadRequest(w, "Invalid participant ID")
		return
	}

	requesterID := middleware.GetUserID(r.Context())

	if err := h.chatService.RemoveParticipant(r.Context(), threadID, participantID, requesterID); err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to remove participant")
		return
	}

	respondOK(w, map[string]string{"status": "removed"})
}

// UploadFile handles file uploads for chat messages
func (h *ChatHandler) UploadFile(w http.ResponseWriter, r *http.Request) {
	if h.storageConfig == nil {
		respondInternalError(w, "File uploads not configured")
		return
	}

	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	// Verify user is a participant
	_, err = h.chatService.GetThread(r.Context(), threadID, userID)
	if err != nil {
		if err == service.ErrNotParticipant {
			respondForbidden(w, "Access denied")
			return
		}
		respondInternalError(w, "Failed to verify access")
		return
	}

	// Parse multipart form with max file size
	if err := r.ParseMultipartForm(h.storageConfig.MaxFileSize); err != nil {
		respondBadRequest(w, "File too large or invalid form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		respondBadRequest(w, "No file provided")
		return
	}
	defer file.Close()

	// Validate file type
	allowedTypes := map[string]bool{
		"image/jpeg":      true,
		"image/png":       true,
		"image/gif":       true,
		"image/webp":      true,
		"application/pdf": true,
		"text/plain":      true,
	}
	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		respondBadRequest(w, "File type not allowed")
		return
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		// Try to get extension from content type
		switch contentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/gif":
			ext = ".gif"
		case "image/webp":
			ext = ".webp"
		case "application/pdf":
			ext = ".pdf"
		case "text/plain":
			ext = ".txt"
		}
	}

	filename := fmt.Sprintf("%s_%s%s", time.Now().Format("20060102150405"), uuid.New().String()[:8], ext)
	uploadPath := filepath.Join(h.storageConfig.UploadDir, "chat", filename)

	// Create destination file
	dst, err := os.Create(uploadPath)
	if err != nil {
		respondInternalError(w, "Failed to save file")
		return
	}
	defer dst.Close()

	// Copy file
	if _, err := io.Copy(dst, file); err != nil {
		respondInternalError(w, "Failed to save file")
		return
	}

	// Return file info
	attachment := map[string]interface{}{
		"filename":     header.Filename,
		"stored_name":  filename,
		"content_type": contentType,
		"size":         header.Size,
		"url":          fmt.Sprintf("/api/chat/files/%s", filename),
	}

	respondOK(w, attachment)
}

// ServeFile serves uploaded chat files
func (h *ChatHandler) ServeFile(w http.ResponseWriter, r *http.Request) {
	if h.storageConfig == nil {
		respondNotFound(w, "File not found")
		return
	}

	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "..") {
		respondBadRequest(w, "Invalid filename")
		return
	}

	filePath := filepath.Join(h.storageConfig.UploadDir, "chat", filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		respondNotFound(w, "File not found")
		return
	}

	// Determine content type
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".png":
		contentType = "image/png"
	case ".gif":
		contentType = "image/gif"
	case ".webp":
		contentType = "image/webp"
	case ".pdf":
		contentType = "application/pdf"
	case ".txt":
		contentType = "text/plain"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")

	http.ServeFile(w, r, filePath)
}

// DeleteThread deletes a chat thread (creator only)
func (h *ChatHandler) DeleteThread(w http.ResponseWriter, r *http.Request) {
	threadID, err := parseUUID(chi.URLParam(r, "threadID"))
	if err != nil {
		respondBadRequest(w, "Invalid thread ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	if err := h.chatService.DeleteThread(r.Context(), threadID, userID); err != nil {
		if err == service.ErrThreadNotFound {
			respondNotFound(w, "Thread not found")
			return
		}
		if err.Error() == "only the thread creator can delete the thread" {
			respondForbidden(w, err.Error())
			return
		}
		respondInternalError(w, "Failed to delete thread")
		return
	}

	respondOK(w, map[string]string{"status": "deleted"})
}
