package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// ListErrorLogs returns paginated error logs with filtering
// By default, only returns errors from logged-in users and infrastructure
func (h *Handler) ListErrorLogs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	errorType := r.URL.Query().Get("error_type")

	var acknowledged *bool
	if ack := r.URL.Query().Get("acknowledged"); ack != "" {
		val := ack == "true"
		acknowledged = &val
	}

	// Parse source filter - can be comma-separated list
	var sources []models.ErrorSource
	if sourceParam := r.URL.Query().Get("source"); sourceParam != "" {
		for _, s := range strings.Split(sourceParam, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				sources = append(sources, models.ErrorSource(s))
			}
		}
	}

	// Check if include_noise is set to show all errors
	includeNoise := r.URL.Query().Get("include_noise") == "true"

	logs, total, err := h.adminRepo.GetErrorLogs(r.Context(), page, limit, errorType, acknowledged, sources, includeNoise)
	if err != nil {
		http.Error(w, "Failed to fetch error logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get source counts for the UI
	sourceCounts, _ := h.adminRepo.GetErrorLogSourceCounts(r.Context())

	response := map[string]interface{}{
		"logs":          logs,
		"total":         total,
		"page":          page,
		"limit":         limit,
		"source_counts": sourceCounts,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetErrorLog returns a single error log by ID
func (h *Handler) GetErrorLog(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid error log ID", http.StatusBadRequest)
		return
	}

	log, err := h.adminRepo.GetErrorLogByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to fetch error log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if log == nil {
		http.Error(w, "Error log not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(log)
}

// AcknowledgeErrorLog marks an error log as acknowledged
func (h *Handler) AcknowledgeErrorLog(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid error log ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.adminRepo.AcknowledgeErrorLog(r.Context(), id, userID, req.Notes); err != nil {
		http.Error(w, "Failed to acknowledge error log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "acknowledged"})
}

// AcknowledgeErrorLogsBulk marks multiple error logs as acknowledged
func (h *Handler) AcknowledgeErrorLogsBulk(w http.ResponseWriter, r *http.Request) {
	var req models.BulkAcknowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		http.Error(w, "No IDs provided", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.adminRepo.AcknowledgeErrorLogsBulk(r.Context(), req.IDs, userID, req.Notes); err != nil {
		http.Error(w, "Failed to acknowledge error logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "acknowledged",
		"count":  len(req.IDs),
	})
}

// DeleteErrorLog soft-deletes an error log
func (h *Handler) DeleteErrorLog(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid error log ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.adminRepo.DeleteErrorLog(r.Context(), id, userID); err != nil {
		http.Error(w, "Failed to delete error log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// DeleteErrorLogsBulk soft-deletes multiple error logs
func (h *Handler) DeleteErrorLogsBulk(w http.ResponseWriter, r *http.Request) {
	var req models.BulkDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.IDs) == 0 {
		http.Error(w, "No IDs provided", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.adminRepo.DeleteErrorLogsBulk(r.Context(), req.IDs, userID); err != nil {
		http.Error(w, "Failed to delete error logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "deleted",
		"count":  len(req.IDs),
	})
}

// CreateTicketFromError creates a support ticket from an error log
func (h *Handler) CreateTicketFromError(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid error log ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Priority string `json:"priority"`
		Notes    string `json:"notes"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	userID := middleware.GetUserID(r.Context())
	ticket, err := h.adminRepo.CreateTicketFromError(r.Context(), id, userID, req.Priority, req.Notes)
	if err != nil {
		http.Error(w, "Failed to create ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ticket)
}

// GetUnacknowledgedErrorCount returns the count of unacknowledged errors
func (h *Handler) GetUnacknowledgedErrorCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.adminRepo.GetUnacknowledgedErrorCount(r.Context())
	if err != nil {
		http.Error(w, "Failed to get error count: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"unacknowledged_count": count})
}
