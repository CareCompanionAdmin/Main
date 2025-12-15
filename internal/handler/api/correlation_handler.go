package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type CorrelationHandler struct {
	correlationService *service.CorrelationService
	childService       *service.ChildService
}

func NewCorrelationHandler(correlationService *service.CorrelationService, childService *service.ChildService) *CorrelationHandler {
	return &CorrelationHandler{
		correlationService: correlationService,
		childService:       childService,
	}
}

// GetInsights returns the insights page data
func (h *CorrelationHandler) GetInsights(w http.ResponseWriter, r *http.Request) {
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

	insights, err := h.correlationService.GetInsightsPage(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get insights")
		return
	}

	respondOK(w, insights)
}

// CreateCorrelationRequest creates a new correlation analysis request
func (h *CorrelationHandler) CreateCorrelationRequest(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateCorrelationRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if len(req.InputFactors) == 0 || len(req.OutputFactors) == 0 {
		respondBadRequest(w, "Input and output factors are required")
		return
	}

	correlation, err := h.correlationService.CreateCorrelationRequest(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create correlation request")
		return
	}

	// Run correlation in background (in production, use a job queue)
	go h.correlationService.RunCorrelation(r.Context(), correlation.ID)

	respondCreated(w, correlation)
}

// GetCorrelationRequest returns a specific correlation request
func (h *CorrelationHandler) GetCorrelationRequest(w http.ResponseWriter, r *http.Request) {
	correlationID, err := parseUUID(chi.URLParam(r, "correlationID"))
	if err != nil {
		respondBadRequest(w, "Invalid correlation ID")
		return
	}

	correlation, err := h.correlationService.GetCorrelationRequest(r.Context(), correlationID)
	if err != nil {
		switch err {
		case service.ErrCorrelationNotFound:
			respondNotFound(w, "Correlation request not found")
		default:
			respondInternalError(w, "Failed to get correlation request")
		}
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), correlation.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	respondOK(w, correlation)
}

// ListCorrelationRequests returns correlation requests for a child
func (h *CorrelationHandler) ListCorrelationRequests(w http.ResponseWriter, r *http.Request) {
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
	var status *models.CorrelationStatus
	statusStr := r.URL.Query().Get("status")
	if statusStr != "" {
		s := models.CorrelationStatus(statusStr)
		status = &s
	}

	correlations, err := h.correlationService.GetCorrelationRequests(r.Context(), childID, status)
	if err != nil {
		respondInternalError(w, "Failed to get correlation requests")
		return
	}

	respondOK(w, correlations)
}

// GetPatterns returns active patterns for a child
func (h *CorrelationHandler) GetPatterns(w http.ResponseWriter, r *http.Request) {
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
	patterns, err := h.correlationService.GetPatterns(r.Context(), childID, activeOnly)
	if err != nil {
		respondInternalError(w, "Failed to get patterns")
		return
	}

	respondOK(w, patterns)
}

// GetPattern returns a specific pattern
func (h *CorrelationHandler) GetPattern(w http.ResponseWriter, r *http.Request) {
	patternID, err := parseUUID(chi.URLParam(r, "patternID"))
	if err != nil {
		respondBadRequest(w, "Invalid pattern ID")
		return
	}

	pattern, err := h.correlationService.GetPattern(r.Context(), patternID)
	if err != nil {
		switch err {
		case service.ErrPatternNotFound:
			respondNotFound(w, "Pattern not found")
		default:
			respondInternalError(w, "Failed to get pattern")
		}
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), pattern.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	respondOK(w, pattern)
}

// DeletePattern deactivates a pattern
func (h *CorrelationHandler) DeletePattern(w http.ResponseWriter, r *http.Request) {
	patternID, err := parseUUID(chi.URLParam(r, "patternID"))
	if err != nil {
		respondBadRequest(w, "Invalid pattern ID")
		return
	}

	if err := h.correlationService.DeletePattern(r.Context(), patternID); err != nil {
		respondInternalError(w, "Failed to delete pattern")
		return
	}

	respondNoContent(w)
}

// GetBaselines returns baselines for a child
func (h *CorrelationHandler) GetBaselines(w http.ResponseWriter, r *http.Request) {
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

	baselines, err := h.correlationService.GetBaselines(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get baselines")
		return
	}

	respondOK(w, baselines)
}

// RecalculateBaselines triggers baseline recalculation
func (h *CorrelationHandler) RecalculateBaselines(w http.ResponseWriter, r *http.Request) {
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

	baselines, err := h.correlationService.CalculateBaselines(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to calculate baselines")
		return
	}

	respondOK(w, baselines)
}

// GetValidations returns clinical validations for a child
func (h *CorrelationHandler) GetValidations(w http.ResponseWriter, r *http.Request) {
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

	validations, err := h.correlationService.GetValidations(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get validations")
		return
	}

	respondOK(w, validations)
}

// CreateValidation creates a clinical validation
func (h *CorrelationHandler) CreateValidation(w http.ResponseWriter, r *http.Request) {
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

	var validation models.ClinicalValidation
	if err := decodeJSON(r, &validation); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	validation.ChildID = childID
	validation.ProviderUserID = models.NullUUID{UUID: userID, Valid: true}

	if err := h.correlationService.CreateValidation(r.Context(), &validation); err != nil {
		respondInternalError(w, "Failed to create validation")
		return
	}

	respondCreated(w, validation)
}

// GetTopPatterns returns top patterns by correlation strength
func (h *CorrelationHandler) GetTopPatterns(w http.ResponseWriter, r *http.Request) {
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

	patterns, err := h.correlationService.GetTopPatterns(r.Context(), childID, 5)
	if err != nil {
		respondInternalError(w, "Failed to get top patterns")
		return
	}

	respondOK(w, patterns)
}
