package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

type MedicationHandler struct {
	medService      *service.MedicationService
	childService    *service.ChildService
	drugDBService   *service.DrugDatabaseService
	insightService  *service.InsightService
}

func NewMedicationHandler(medService *service.MedicationService, childService *service.ChildService, drugDBService *service.DrugDatabaseService, insightService *service.InsightService) *MedicationHandler {
	return &MedicationHandler{
		medService:     medService,
		childService:   childService,
		drugDBService:  drugDBService,
		insightService: insightService,
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

// MedicationCreateResponse includes the medication and any detected interactions
type MedicationCreateResponse struct {
	Medication   *models.Medication           `json:"medication"`
	Interactions []service.InteractionWarning `json:"interactions,omitempty"`
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

	// Check for interactions with existing medications
	var interactions []service.InteractionWarning
	if h.drugDBService != nil {
		existingMeds, err := h.medService.GetByChildID(r.Context(), childID, true)
		if err == nil && len(existingMeds) > 0 {
			// Build list of all medication names (excluding the one just added)
			var medNames []string
			for _, m := range existingMeds {
				if m.ID != med.ID {
					medNames = append(medNames, m.Name)
				}
			}
			// Add the new medication name
			medNames = append(medNames, med.Name)

			// Check for interactions
			interactions, _ = h.drugDBService.CheckInteractions(r.Context(), medNames)

			// Filter to only show interactions involving the new medication
			var relevantInteractions []service.InteractionWarning
			for _, interaction := range interactions {
				if interaction.Drug1 == med.Name || interaction.Drug2 == med.Name {
					relevantInteractions = append(relevantInteractions, interaction)
				}
			}
			interactions = relevantInteractions
		}
	}

	response := MedicationCreateResponse{
		Medication:   med,
		Interactions: interactions,
	}

	respondCreated(w, response)
}

// Update updates a medication
func (h *MedicationHandler) Update(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	// Get the existing medication to track changes
	oldMed, err := h.medService.GetByID(r.Context(), medID)
	if err != nil {
		respondNotFound(w, "Medication not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), oldMed.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	// Create a copy to apply updates to
	newMed := *oldMed
	if err := decodeJSON(r, &newMed); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	// Use UpdateWithTracking to create treatment change records
	if err := h.medService.UpdateWithTracking(r.Context(), oldMed, &newMed, userID); err != nil {
		respondInternalError(w, "Failed to update medication")
		return
	}

	respondOK(w, &newMed)
}

// Delete discontinues a medication
func (h *MedicationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	// Get the medication to verify access
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

	// Use DiscontinueWithTracking to create treatment change record
	if err := h.medService.DiscontinueWithTracking(r.Context(), medID, userID); err != nil {
		respondInternalError(w, "Failed to discontinue medication")
		return
	}

	respondNoContent(w)
}

// DiscontinueRequest represents the discontinuation request body
type DiscontinueRequest struct {
	Reason     string `json:"reason"`
	ReasonText string `json:"reason_text"`
}

// Discontinue handles medication discontinuation with reason tracking
func (h *MedicationHandler) Discontinue(w http.ResponseWriter, r *http.Request) {
	medID, err := parseUUID(chi.URLParam(r, "medID"))
	if err != nil {
		respondBadRequest(w, "Invalid medication ID")
		return
	}

	// Get the medication to verify access
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

	var req DiscontinueRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Reason == "" {
		respondBadRequest(w, "Discontinuation reason is required")
		return
	}

	// Use the service to handle discontinuation with reason
	deleted, err := h.medService.DiscontinueWithReason(r.Context(), medID, userID, req.Reason, req.ReasonText)
	if err != nil {
		log.Printf("Failed to discontinue medication: %v", err)
		respondInternalError(w, "Failed to discontinue medication")
		return
	}

	respondOK(w, map[string]interface{}{
		"success": true,
		"deleted": deleted,
	})
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
		log.Printf("GetDueMedications error: %v", err)
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
		fmt.Printf("Error decoding medication log request: %v\n", err)
		respondBadRequest(w, fmt.Sprintf("Invalid request body: %v", err))
		return
	}

	// Validate required fields
	if req.MedicationID == uuid.Nil {
		respondBadRequest(w, "medication_id is required")
		return
	}
	if req.Status == "" {
		respondBadRequest(w, "status is required")
		return
	}

	log, err := h.medService.LogMedication(r.Context(), childID, userID, &req)
	if err != nil {
		fmt.Printf("Error logging medication: %v, request: %+v, childID: %v, userID: %v\n", err, req, childID, userID)
		respondInternalError(w, fmt.Sprintf("Failed to log medication: %v", err))
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

// UpdateLog updates a medication log entry
func (h *MedicationHandler) UpdateLog(w http.ResponseWriter, r *http.Request) {
	logID, err := parseUUID(chi.URLParam(r, "logID"))
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	// Get the existing log
	existingLog, err := h.medService.GetLogByID(r.Context(), logID)
	if err != nil || existingLog == nil {
		respondNotFound(w, "Medication log not found")
		return
	}

	// Store old values for audit trail
	oldLog := *existingLog

	// Verify access to the child
	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existingLog.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	// Parse update request
	var req struct {
		Status      models.LogStatus `json:"status"`
		ActualTime  string           `json:"actual_time,omitempty"`
		DosageGiven string           `json:"dosage_given,omitempty"`
		Notes       string           `json:"notes,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	// Update the log fields
	existingLog.Status = req.Status
	existingLog.ActualTime.String = req.ActualTime
	existingLog.ActualTime.Valid = req.ActualTime != ""
	existingLog.DosageGiven.String = req.DosageGiven
	existingLog.DosageGiven.Valid = req.DosageGiven != ""
	existingLog.Notes.String = req.Notes
	existingLog.Notes.Valid = req.Notes != ""

	// Update with tracking for audit log
	if err := h.medService.UpdateLogWithTracking(r.Context(), &oldLog, existingLog, userID); err != nil {
		log.Printf("UpdateLog error: %v", err)
		respondInternalError(w, "Failed to update medication log")
		return
	}

	respondOK(w, existingLog)
}

// DeleteLog deletes a medication log
func (h *MedicationHandler) DeleteLog(w http.ResponseWriter, r *http.Request) {
	logID, err := parseUUID(chi.URLParam(r, "logID"))
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	// Get the existing log to verify access
	existingLog, err := h.medService.GetLogByID(r.Context(), logID)
	if err != nil || existingLog == nil {
		respondNotFound(w, "Medication log not found")
		return
	}

	// Verify access to the child
	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existingLog.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	if err := h.medService.DeleteLog(r.Context(), logID); err != nil {
		log.Printf("DeleteLog error: %v", err)
		respondInternalError(w, "Failed to delete medication log")
		return
	}

	respondOK(w, map[string]string{"message": "Medication log deleted"})
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

// SearchReferences searches medication references using FDA database
func (h *MedicationHandler) SearchReferences(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondBadRequest(w, "Search query required")
		return
	}

	// Try FDA API first for comprehensive results
	fdaResults, err := h.drugDBService.SearchDrugs(r.Context(), query)
	if err == nil && len(fdaResults) > 0 {
		// Convert FDA results to a format compatible with the frontend
		type SearchResult struct {
			Name        string   `json:"name"`
			GenericName string   `json:"generic_name,omitempty"`
			BrandNames  []string `json:"brand_names,omitempty"`
		}
		results := make([]SearchResult, len(fdaResults))
		for i, r := range fdaResults {
			results[i] = SearchResult{
				Name:        r.Name,
				GenericName: r.GenericName,
				BrandNames:  r.BrandNames,
			}
		}
		respondOK(w, results)
		return
	}

	// Fall back to local database if FDA API fails
	refs, err := h.medService.SearchMedicationReferences(r.Context(), query)
	if err != nil {
		respondInternalError(w, "Failed to search medications")
		return
	}

	respondOK(w, refs)
}

// ValidateDrug validates a medication name against the FDA database
func (h *MedicationHandler) ValidateDrug(w http.ResponseWriter, r *http.Request) {
	drugName := r.URL.Query().Get("name")
	if drugName == "" {
		respondBadRequest(w, "Drug name required")
		return
	}

	result, err := h.drugDBService.ValidateMedication(r.Context(), drugName)
	if err != nil {
		respondInternalError(w, "Failed to validate medication")
		return
	}

	respondOK(w, result)
}

// GetDrugInfo gets detailed drug information from the FDA database
func (h *MedicationHandler) GetDrugInfo(w http.ResponseWriter, r *http.Request) {
	drugName := r.URL.Query().Get("name")
	if drugName == "" {
		respondBadRequest(w, "Drug name required")
		return
	}

	dosage := r.URL.Query().Get("dosage")

	info, err := h.drugDBService.LookupDrugWithDosage(r.Context(), drugName, dosage)
	if err != nil {
		respondInternalError(w, "Failed to get drug information")
		return
	}

	// DailyMed images are from a government API and don't need proxying
	// The image URL is passed through directly

	respondOK(w, info)
}

// ProxyDrugImage proxies drug images to avoid hotlink blocking
func (h *MedicationHandler) ProxyDrugImage(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		http.Error(w, "URL required", http.StatusBadRequest)
		return
	}

	// Create request to fetch the image
	req, err := http.NewRequestWithContext(r.Context(), "GET", imageURL, nil)
	if err != nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Set headers to appear as a browser request
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	req.Header.Set("Referer", "https://www.drugs.com/")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to fetch image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	// Copy content type and image data
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	io.Copy(w, resp.Body)
}

// CheckInteractions checks for drug interactions among a child's medications
func (h *MedicationHandler) CheckInteractions(w http.ResponseWriter, r *http.Request) {
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

	// Get active medications for child
	medications, err := h.medService.GetByChildID(r.Context(), childID, true)
	if err != nil {
		respondInternalError(w, "Failed to get medications")
		return
	}

	// Extract medication names
	var drugNames []string
	for _, med := range medications {
		drugNames = append(drugNames, med.Name)
	}

	// Check interactions
	warnings, err := h.drugDBService.CheckInteractions(r.Context(), drugNames)
	if err != nil {
		respondInternalError(w, "Failed to check interactions")
		return
	}

	respondOK(w, map[string]interface{}{
		"medication_count":    len(drugNames),
		"medications":         drugNames,
		"interactions":        warnings,
		"interaction_count":   len(warnings),
		"has_major_warnings":  hasMajorWarnings(warnings),
	})
}

// GetMedicalInsights gets Tier 1 medical insights for a child's medications
func (h *MedicationHandler) GetMedicalInsights(w http.ResponseWriter, r *http.Request) {
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

	// Get active medications for child
	medications, err := h.medService.GetByChildID(r.Context(), childID, true)
	if err != nil {
		respondInternalError(w, "Failed to get medications")
		return
	}

	var allInsights []models.Insight
	for _, med := range medications {
		insights, err := h.drugDBService.GetTier1Insights(r.Context(), med.Name)
		if err != nil {
			continue // Skip failed lookups
		}
		allInsights = append(allInsights, insights...)
	}

	respondOK(w, map[string]interface{}{
		"medication_count": len(medications),
		"insights":         allInsights,
		"insight_count":    len(allInsights),
	})
}

func hasMajorWarnings(warnings []service.InteractionWarning) bool {
	for _, w := range warnings {
		if w.Severity == "major" {
			return true
		}
	}
	return false
}

// GetHistory returns medication change history for a child
func (h *MedicationHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
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

	history, err := h.medService.GetMedicationHistory(r.Context(), childID)
	if err != nil {
		log.Printf("GetMedicationHistory error: %v", err)
		respondInternalError(w, "Failed to get medication history")
		return
	}

	if history == nil {
		history = []repository.MedicationHistoryEntry{}
	}

	respondOK(w, history)
}
