package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type MedicationHandler struct {
	medService   *service.MedicationService
	childService *service.ChildService
}

func NewMedicationHandler(medService *service.MedicationService, childService *service.ChildService) *MedicationHandler {
	return &MedicationHandler{
		medService:   medService,
		childService: childService,
	}
}

// List returns all medications for a child
func (h *MedicationHandler) List(w http.ResponseWriter, r *http.Request) {
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

	activeOnly := r.URL.Query().Get("active_only") != "false"
	medications, err := h.medService.GetByChildID(r.Context(), childID, activeOnly)
	if err != nil {
		respondInternalError(w, "Failed to get medications")
		return
	}

	respondOK(w, medications)
}

// Get returns a specific medication
func (h *MedicationHandler) Get(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	med, err := h.medService.GetByID(r.Context(), medID)
	if err != nil {
		switch err {
		case service.ErrMedicationNotFound:
			respondNotFound(w, "Medication not found")
		default:
			respondInternalError(w, "Failed to get medication")
		}
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), med.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	respondOK(w, med)
}

// Create creates a new medication
func (h *MedicationHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateMedicationRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Name == "" || req.Dosage == "" || req.DosageUnit == "" || req.Frequency == "" {
		respondBadRequest(w, "Name, dosage, dosage unit, and frequency are required")
		return
	}

	med, err := h.medService.Create(r.Context(), childID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create medication")
		return
	}

	respondCreated(w, med)
}

// Update updates a medication
func (h *MedicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	med, err := h.medService.GetByID(r.Context(), medID)
	if err != nil {
		respondNotFound(w, "Medication not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), med.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	if err := decodeJSON(r, med); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if err := h.medService.Update(r.Context(), med); err != nil {
		respondInternalError(w, "Failed to update medication")
		return
	}

	respondOK(w, med)
}

// Delete discontinues a medication
func (h *MedicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	if err := h.medService.Discontinue(r.Context(), medID); err != nil {
		respondInternalError(w, "Failed to discontinue medication")
		return
	}

	respondNoContent(w)
}

// GetDue returns medications due for today
func (h *MedicationHandler) GetDue(w http.ResponseWriter, r *http.Request) {
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

	dateStr := r.URL.Query().Get("date")
	date := time.Now()
	if dateStr != "" {
		date, err = parseDate(dateStr)
		if err != nil {
			respondBadRequest(w, "Invalid date format")
			return
		}
	}

	dueMeds, err := h.medService.GetDueMedications(r.Context(), childID, date)
	if err != nil {
		respondInternalError(w, "Failed to get due medications")
		return
	}

	respondOK(w, dueMeds)
}

// Log logs a medication
func (h *MedicationHandler) Log(w http.ResponseWriter, r *http.Request) {
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

	var req models.LogMedicationRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.medService.LogMedication(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to log medication")
		return
	}

	respondCreated(w, log)
}

// GetLogs returns medication logs
func (h *MedicationHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
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

	// Default to last 7 days
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)

	startStr := r.URL.Query().Get("start_date")
	if startStr != "" {
		startDate, _ = parseDate(startStr)
	}

	endStr := r.URL.Query().Get("end_date")
	if endStr != "" {
		endDate, _ = parseDate(endStr)
	}

	logs, err := h.medService.GetLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get logs")
		return
	}

	respondOK(w, logs)
}

// GetAdherence returns medication adherence rate
func (h *MedicationHandler) GetAdherence(w http.ResponseWriter, r *http.Request) {
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

	// Default to last 30 days
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	adherence, err := h.medService.CalculateAdherence(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to calculate adherence")
		return
	}

	respondOK(w, map[string]interface{}{
		"adherence_rate": adherence,
		"start_date":     startDate,
		"end_date":       endDate,
	})
}

// SearchReferences searches medication references
func (h *MedicationHandler) SearchReferences(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondBadRequest(w, "Search query required")
		return
	}

	refs, err := h.medService.SearchMedicationReferences(r.Context(), query)
	if err != nil {
		respondInternalError(w, "Failed to search medications")
		return
	}

	respondOK(w, refs)
}
