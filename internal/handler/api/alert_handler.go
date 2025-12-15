package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type AlertHandler struct {
	alertService *service.AlertService
	childService *service.ChildService
}

func NewAlertHandler(alertService *service.AlertService, childService *service.ChildService) *AlertHandler {
	return &AlertHandler{
		alertService: alertService,
		childService: childService,
	}
}

// List returns alerts for a child
func (h *AlertHandler) List(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	// Optional status filter
	var status *models.AlertStatus
	statusStr := r.URL.Query().Get("status")
	if statusStr != "" {
		s := models.AlertStatus(statusStr)
		status = &s
	}

	alerts, err := h.alertService.GetByChildID(r.Context(), childID, status)
	if err != nil {
		respondInternalError(w, "Failed to get alerts")
		return
	}

	respondOK(w, alerts)
}

// Get returns a specific alert
func (h *AlertHandler) Get(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(chi.URLParam(r, "alertID"))
	if err != nil {
		respondBadRequest(w, "Invalid alert ID")
		return
	}

	alert, err := h.alertService.GetByID(r.Context(), alertID)
	if err != nil {
		switch err {
		case service.ErrAlertNotFound:
			respondNotFound(w, "Alert not found")
		default:
			respondInternalError(w, "Failed to get alert")
		}
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), alert.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	respondOK(w, alert)
}

// Acknowledge marks an alert as acknowledged
func (h *AlertHandler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(chi.URLParam(r, "alertID"))
	if err != nil {
		respondBadRequest(w, "Invalid alert ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	if err := h.alertService.Acknowledge(r.Context(), alertID, userID); err != nil {
		respondInternalError(w, "Failed to acknowledge alert")
		return
	}

	respondOK(w, map[string]string{"message": "Alert acknowledged"})
}

// Resolve marks an alert as resolved
func (h *AlertHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(chi.URLParam(r, "alertID"))
	if err != nil {
		respondBadRequest(w, "Invalid alert ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	if err := h.alertService.Resolve(r.Context(), alertID, userID); err != nil {
		respondInternalError(w, "Failed to resolve alert")
		return
	}

	respondOK(w, map[string]string{"message": "Alert resolved"})
}

// CreateFeedback creates feedback for an alert
func (h *AlertHandler) CreateFeedback(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(chi.URLParam(r, "alertID"))
	if err != nil {
		respondBadRequest(w, "Invalid alert ID")
		return
	}

	userID := middleware.GetUserID(r.Context())

	var req models.AlertFeedbackRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	feedback, err := h.alertService.CreateFeedback(r.Context(), alertID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create feedback")
		return
	}

	respondCreated(w, feedback)
}

// GetFeedback returns feedback for an alert
func (h *AlertHandler) GetFeedback(w http.ResponseWriter, r *http.Request) {
	alertID, err := parseUUID(chi.URLParam(r, "alertID"))
	if err != nil {
		respondBadRequest(w, "Invalid alert ID")
		return
	}

	feedback, err := h.alertService.GetFeedback(r.Context(), alertID)
	if err != nil {
		respondInternalError(w, "Failed to get feedback")
		return
	}

	respondOK(w, feedback)
}

// GetStats returns alert statistics for a child
func (h *AlertHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	stats, err := h.alertService.GetStats(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get stats")
		return
	}

	respondOK(w, stats)
}

// GetAlertsPage returns the alerts page data
func (h *AlertHandler) GetAlertsPage(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	page, err := h.alertService.GetAlertsPage(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get alerts page")
		return
	}

	respondOK(w, page)
}
