package api

import (
	"encoding/json"
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"

	"github.com/go-chi/chi/v5"
)

type TransparencyHandler struct {
	service *service.TransparencyService
}

func NewTransparencyHandler(s *service.TransparencyService) *TransparencyHandler {
	return &TransparencyHandler{service: s}
}

// GetAlertAnalysis returns full analysis details for an alert (Layer 3)
// GET /api/alerts/{alertID}/analysis
func (h *TransparencyHandler) GetAlertAnalysis(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertID")
	userID := middleware.GetUserID(r.Context()).String()

	analysis, err := h.service.GetFullAlertAnalysis(r.Context(), alertID, userID)
	if err != nil {
		if err.Error() == "access denied" {
			respondError(w, "Access denied", http.StatusForbidden)
			return
		}
		respondError(w, "Alert not found", http.StatusNotFound)
		return
	}

	respondJSON(w, analysis, http.StatusOK)
}

// GetConfidenceFactors returns confidence breakdown for an alert (Layer 2)
// GET /api/alerts/{alertID}/confidence-factors
func (h *TransparencyHandler) GetConfidenceFactors(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertID")
	userID := middleware.GetUserID(r.Context()).String()

	breakdown, err := h.service.GetConfidenceBreakdown(r.Context(), alertID, userID)
	if err != nil {
		if err.Error() == "access denied" {
			respondError(w, "Access denied", http.StatusForbidden)
			return
		}
		respondError(w, "Alert not found", http.StatusNotFound)
		return
	}

	respondJSON(w, breakdown, http.StatusOK)
}

// SubmitAlertFeedback records user feedback on an alert
// POST /api/alerts/{alertID}/feedback
func (h *TransparencyHandler) SubmitAlertFeedback(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertID")
	userID := middleware.GetUserID(r.Context()).String()

	var req models.AlertFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.service.RecordAlertFeedback(r.Context(), alertID, userID, &req)
	if err != nil {
		if err.Error() == "access denied" {
			respondError(w, "Access denied", http.StatusForbidden)
			return
		}
		respondError(w, "Failed to record feedback", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// ExportAlert creates an export record for an alert
// POST /api/alerts/{alertID}/export
func (h *TransparencyHandler) ExportAlert(w http.ResponseWriter, r *http.Request) {
	alertID := chi.URLParam(r, "alertID")
	userID := middleware.GetUserID(r.Context()).String()

	var req struct {
		ExportType string `json:"export_type"`
		ViewMode   string `json:"view_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	exportType := models.ExportType(req.ExportType)
	viewMode := models.AnalysisView(req.ViewMode)
	if viewMode == "" {
		viewMode = models.AnalysisViewParent
	}

	export, err := h.service.ExportAlert(r.Context(), alertID, userID, exportType, viewMode)
	if err != nil {
		if err.Error() == "access denied" {
			respondError(w, "Access denied", http.StatusForbidden)
			return
		}
		respondError(w, "Failed to create export", http.StatusInternalServerError)
		return
	}

	respondJSON(w, export, http.StatusCreated)
}

// GetPendingInterrogatives returns pending treatment change questions
// GET /api/treatment-changes/pending-questions
func (h *TransparencyHandler) GetPendingInterrogatives(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context()).String()

	pending, err := h.service.GetPendingInterrogatives(r.Context(), userID)
	if err != nil {
		respondError(w, "Failed to get pending questions", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"pending": pending,
		"count":   len(pending),
	}, http.StatusOK)
}

// RespondToTreatmentChange records a response to a treatment change question
// POST /api/treatment-changes/{changeID}/respond
func (h *TransparencyHandler) RespondToTreatmentChange(w http.ResponseWriter, r *http.Request) {
	changeID := chi.URLParam(r, "changeID")
	userID := middleware.GetUserID(r.Context()).String()

	var req models.TreatmentChangePromptResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err := h.service.RespondToTreatmentChange(r.Context(), changeID, userID, &req)
	if err != nil {
		respondError(w, "Failed to record response", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// InteractionAlertRequest is the request body for creating an interaction alert
type InteractionAlertRequest struct {
	ChildID        string                   `json:"child_id"`
	MedicationID   string                   `json:"medication_id"`
	MedicationName string                   `json:"medication_name"`
	Interactions   []map[string]interface{} `json:"interactions"`
}

// CreateInteractionAlert creates a pending interaction alert for the dashboard
// POST /api/interaction-alerts
func (h *TransparencyHandler) CreateInteractionAlert(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context()).String()

	var req InteractionAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build interaction description
	var interactionDesc string
	for i, interaction := range req.Interactions {
		drug1, _ := interaction["drug1"].(string)
		drug2, _ := interaction["drug2"].(string)
		desc, _ := interaction["description"].(string)

		otherDrug := drug1
		if drug1 == req.MedicationName {
			otherDrug = drug2
		}

		if i > 0 {
			interactionDesc += "; "
		}
		interactionDesc += otherDrug + ": " + desc
	}

	// Create a treatment change entry for the interaction alert
	tc := &models.TreatmentChange{
		ChildID:             req.ChildID,
		ChangeType:          models.ChangeType("interaction_alert"),
		SourceTable:         "medications",
		SourceID:            req.MedicationID,
		ChangeSummary:       "You added " + req.MedicationName + " which may interact with other medications. " + interactionDesc,
		ChangedByUserID:     userID,
		InterrogativeStatus: models.InterrogativeStatusPending,
	}

	if err := h.service.CreateTreatmentChange(r.Context(), tc); err != nil {
		respondError(w, "Failed to create interaction alert", http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// GetInteractionPreferences returns user interaction preferences
// GET /api/users/me/interaction-preferences
func (h *TransparencyHandler) GetInteractionPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context()).String()

	prefs, err := h.service.GetUserInteractionPreferences(r.Context(), userID)
	if err != nil {
		respondError(w, "Failed to get preferences", http.StatusInternalServerError)
		return
	}

	respondJSON(w, prefs, http.StatusOK)
}

// UpdateInteractionPreferences updates user interaction preferences
// PUT /api/users/me/interaction-preferences
func (h *TransparencyHandler) UpdateInteractionPreferences(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context()).String()

	var prefs models.UserInteractionPreferences
	if err := json.NewDecoder(r.Body).Decode(&prefs); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	prefs.UserID = userID

	err := h.service.UpdateUserInteractionPreferences(r.Context(), &prefs)
	if err != nil {
		respondError(w, "Failed to update preferences", http.StatusInternalServerError)
		return
	}

	respondJSON(w, prefs, http.StatusOK)
}
