package api

import (
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// SearchHandler handles global search endpoints
type SearchHandler struct {
	searchService *service.SearchService
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(searchService *service.SearchService) *SearchHandler {
	return &SearchHandler{searchService: searchService}
}

// Search handles GET /api/search?q=term
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	query := r.URL.Query().Get("q")
	if query == "" {
		respondOK(w, &models.SearchResponse{})
		return
	}

	results, err := h.searchService.Search(r.Context(), familyID, userID, query)
	if err != nil {
		respondInternalError(w, "Search failed")
		return
	}

	respondOK(w, results)
}
