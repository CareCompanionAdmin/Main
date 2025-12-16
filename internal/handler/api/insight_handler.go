package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

type InsightHandler struct {
	insightService *service.InsightService
	childService   *service.ChildService
}

func NewInsightHandler(insightService *service.InsightService, childService *service.ChildService) *InsightHandler {
	return &InsightHandler{
		insightService: insightService,
		childService:   childService,
	}
}

// GetInsightsByTier returns insights organized by tier for a child
func (h *InsightHandler) GetInsightsByTier(w http.ResponseWriter, r *http.Request) {
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

	insights, err := h.insightService.GetInsightsForChild(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get insights")
		return
	}

	respondOK(w, insights)
}

// GetTopInsights returns the most significant insights for a child
func (h *InsightHandler) GetTopInsights(w http.ResponseWriter, r *http.Request) {
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

	// Default to top 5
	limit := 5
	insights, err := h.insightService.GetTopInsights(r.Context(), childID, limit)
	if err != nil {
		respondInternalError(w, "Failed to get top insights")
		return
	}

	respondOK(w, insights)
}

// ValidateInsight records user or clinical validation of an insight
func (h *InsightHandler) ValidateInsight(w http.ResponseWriter, r *http.Request) {
	insightID, err := parseUUID(chi.URLParam(r, "insightID"))
	if err != nil {
		respondBadRequest(w, "Invalid insight ID")
		return
	}

	var req struct {
		Clinical bool `json:"clinical"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if err := h.insightService.ValidateInsight(r.Context(), insightID, req.Clinical); err != nil {
		respondInternalError(w, "Failed to validate insight")
		return
	}

	respondOK(w, map[string]string{"status": "validated"})
}

// CreateMedicalInsight creates a Tier 1 global medical insight (admin only)
func (h *InsightHandler) CreateMedicalInsight(w http.ResponseWriter, r *http.Request) {
	var req service.CreateMedicalInsightRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Title == "" || req.SimpleDescription == "" {
		respondBadRequest(w, "Title and description are required")
		return
	}

	insight, err := h.insightService.CreateMedicalInsight(r.Context(), &req)
	if err != nil {
		respondInternalError(w, "Failed to create medical insight")
		return
	}

	respondCreated(w, insight)
}
