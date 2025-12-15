package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
)

// parseUUID parses a UUID string
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// getChildIDFromURL extracts child_id from URL
func getChildIDFromURL(r *http.Request) (uuid.UUID, error) {
	return parseUUID(chi.URLParam(r, "childID"))
}

// getIDFromURL extracts id from URL
func getIDFromURL(r *http.Request) (uuid.UUID, error) {
	return parseUUID(chi.URLParam(r, "id"))
}

// parseDate parses a date string in YYYY-MM-DD format
func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// getDateFromQuery gets a date from query parameters with optional default
func getDateFromQuery(r *http.Request, key string, defaultVal time.Time) time.Time {
	dateStr := r.URL.Query().Get(key)
	if dateStr == "" {
		return defaultVal
	}
	date, err := parseDate(dateStr)
	if err != nil {
		return defaultVal
	}
	return date
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// respondOK writes a 200 OK JSON response
func respondOK(w http.ResponseWriter, data interface{}) {
	respondJSON(w, data, http.StatusOK)
}

// respondCreated writes a 201 Created JSON response
func respondCreated(w http.ResponseWriter, data interface{}) {
	respondJSON(w, data, http.StatusCreated)
}

// respondNoContent writes a 204 No Content response
func respondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// respondError writes an error JSON response
func respondError(w http.ResponseWriter, message string, statusCode int) {
	middleware.JSONError(w, message, statusCode)
}

// respondBadRequest writes a 400 Bad Request response
func respondBadRequest(w http.ResponseWriter, message string) {
	respondError(w, message, http.StatusBadRequest)
}

// respondNotFound writes a 404 Not Found response
func respondNotFound(w http.ResponseWriter, message string) {
	respondError(w, message, http.StatusNotFound)
}

// respondForbidden writes a 403 Forbidden response
func respondForbidden(w http.ResponseWriter, message string) {
	respondError(w, message, http.StatusForbidden)
}

// respondInternalError writes a 500 Internal Server Error response
func respondInternalError(w http.ResponseWriter, message string) {
	respondError(w, message, http.StatusInternalServerError)
}

// decodeJSON decodes JSON from request body
func decodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// SuccessResponse is a generic success response
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ListResponse is a response for list endpoints
type ListResponse struct {
	Data       interface{} `json:"data"`
	TotalCount int         `json:"total_count,omitempty"`
	Page       int         `json:"page,omitempty"`
	PageSize   int         `json:"page_size,omitempty"`
}
