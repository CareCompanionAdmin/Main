package api

import (
	"net/http"
	"time"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type ChildHandler struct {
	childService *service.ChildService
}

func NewChildHandler(childService *service.ChildService) *ChildHandler {
	return &ChildHandler{childService: childService}
}

// List returns all children for the current family
func (h *ChildHandler) List(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	children, err := h.childService.GetByFamilyID(r.Context(), familyID)
	if err != nil {
		respondInternalError(w, "Failed to get children")
		return
	}

	respondOK(w, children)
}

// Get returns a specific child
func (h *ChildHandler) Get(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.childService.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to get child")
		}
		return
	}

	respondOK(w, child)
}

// Create creates a new child
func (h *ChildHandler) Create(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	var req models.CreateChildRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.FirstName == "" {
		respondBadRequest(w, "First name is required")
		return
	}

	if req.DateOfBirth.IsZero() {
		respondBadRequest(w, "Date of birth is required")
		return
	}

	child, err := h.childService.Create(r.Context(), familyID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create child")
		return
	}

	respondCreated(w, child)
}

// Update updates a child
func (h *ChildHandler) Update(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to verify access")
		}
		return
	}

	var req models.UpdateChildRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	child, err := h.childService.Update(r.Context(), childID, &req)
	if err != nil {
		respondInternalError(w, "Failed to update child")
		return
	}

	respondOK(w, child)
}

// Delete soft deletes a child
func (h *ChildHandler) Delete(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to verify access")
		}
		return
	}

	if err := h.childService.Delete(r.Context(), childID); err != nil {
		respondInternalError(w, "Failed to delete child")
		return
	}

	respondNoContent(w)
}

// Dashboard returns the child's dashboard data
func (h *ChildHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to verify access")
		}
		return
	}

	// Get optional date parameter
	dateStr := r.URL.Query().Get("date")
	var date time.Time
	if dateStr != "" {
		date, err = parseDate(dateStr)
		if err != nil {
			respondBadRequest(w, "Invalid date format")
			return
		}
	} else {
		date = time.Now()
	}

	dashboard, err := h.childService.GetDashboardForDate(r.Context(), childID, date)
	if err != nil {
		respondInternalError(w, "Failed to get dashboard")
		return
	}

	respondOK(w, dashboard)
}

// AddCondition adds a condition to a child
func (h *ChildHandler) AddCondition(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to verify access")
		}
		return
	}

	var req struct {
		ConditionName string `json:"condition_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.ConditionName == "" {
		respondBadRequest(w, "Condition name is required")
		return
	}

	condition, err := h.childService.AddCondition(r.Context(), childID, req.ConditionName)
	if err != nil {
		respondInternalError(w, "Failed to add condition")
		return
	}

	respondCreated(w, condition)
}

// GetConditions returns a child's conditions
func (h *ChildHandler) GetConditions(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), childID, userID); err != nil {
		switch err {
		case service.ErrChildNotFound:
			respondNotFound(w, "Child not found")
		case service.ErrNotFamilyMember:
			respondForbidden(w, "Access denied")
		default:
			respondInternalError(w, "Failed to verify access")
		}
		return
	}

	conditions, err := h.childService.GetConditions(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to get conditions")
		return
	}

	respondOK(w, conditions)
}

// UpdateCondition updates a condition for a child
func (h *ChildHandler) UpdateCondition(w http.ResponseWriter, r *http.Request) {
	conditionID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid condition ID")
		return
	}

	var req struct {
		ConditionName string  `json:"condition_name"`
		ICDCode       *string `json:"icd_code,omitempty"`
		DiagnosedBy   *string `json:"diagnosed_by,omitempty"`
		Severity      *string `json:"severity,omitempty"`
		Notes         *string `json:"notes,omitempty"`
		IsActive      *bool   `json:"is_active,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.ConditionName == "" {
		respondBadRequest(w, "Condition name is required")
		return
	}

	condition := &models.ChildCondition{
		ID:            conditionID,
		ConditionName: req.ConditionName,
		IsActive:      true,
	}

	if req.ICDCode != nil && *req.ICDCode != "" {
		condition.ICDCode.Valid = true
		condition.ICDCode.String = *req.ICDCode
	}
	if req.DiagnosedBy != nil && *req.DiagnosedBy != "" {
		condition.DiagnosedBy.Valid = true
		condition.DiagnosedBy.String = *req.DiagnosedBy
	}
	if req.Severity != nil && *req.Severity != "" {
		condition.Severity.Valid = true
		condition.Severity.String = *req.Severity
	}
	if req.Notes != nil && *req.Notes != "" {
		condition.Notes.Valid = true
		condition.Notes.String = *req.Notes
	}
	if req.IsActive != nil {
		condition.IsActive = *req.IsActive
	}

	if err := h.childService.UpdateCondition(r.Context(), condition); err != nil {
		respondInternalError(w, "Failed to update condition")
		return
	}

	respondOK(w, condition)
}

// RemoveCondition removes a condition from a child
func (h *ChildHandler) RemoveCondition(w http.ResponseWriter, r *http.Request) {
	conditionID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid condition ID")
		return
	}

	if err := h.childService.RemoveCondition(r.Context(), conditionID); err != nil {
		respondInternalError(w, "Failed to remove condition")
		return
	}

	respondNoContent(w)
}
