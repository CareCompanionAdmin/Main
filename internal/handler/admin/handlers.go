package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// ============================================================================
// SUPER ADMIN HANDLERS
// ============================================================================

func (h *Handler) ListAdminUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.adminRepo.ListAdminUsers(ctx)
	if err != nil {
		http.Error(w, "Failed to list admin users: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "list_admins", "admin", uuid.Nil, nil)
	respondJSON(w, users)
}

type CreateAdminRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
}

func (h *Handler) CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CreateAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.FirstName == "" {
		http.Error(w, "Email, password, and first name are required", http.StatusBadRequest)
		return
	}

	if !models.IsValidSystemRole(req.Role) {
		http.Error(w, "Invalid system role", http.StatusBadRequest)
		return
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	user, err := h.adminRepo.CreateAdminUser(ctx, req.Email, string(hash), req.FirstName, req.LastName, models.SystemRole(req.Role))
	if err != nil {
		http.Error(w, "Failed to create admin user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "create_admin", "user", user.ID, map[string]interface{}{"email": req.Email, "role": req.Role})
	respondJSON(w, user)
}

func (h *Handler) GetAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := h.adminRepo.GetUserByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	respondJSON(w, user)
}

type UpdateAdminRequest struct {
	Role string `json:"role"`
}

func (h *Handler) UpdateAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var req UpdateAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !models.IsValidSystemRole(req.Role) {
		http.Error(w, "Invalid system role", http.StatusBadRequest)
		return
	}

	if err := h.adminRepo.UpdateAdminRole(ctx, id, models.SystemRole(req.Role)); err != nil {
		http.Error(w, "Failed to update admin: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "update_admin_role", "user", id, map[string]interface{}{"new_role": req.Role})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) DeleteAdminUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Can't delete yourself
	claims := middleware.GetAuthClaims(ctx)
	if claims != nil && claims.UserID == id {
		http.Error(w, "Cannot remove your own admin role", http.StatusBadRequest)
		return
	}

	if err := h.adminRepo.RemoveAdminRole(ctx, id); err != nil {
		http.Error(w, "Failed to remove admin role: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "remove_admin_role", "user", id, nil)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) GetSystemMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metrics, err := h.adminRepo.GetCachedMetrics(ctx)
	if err != nil {
		http.Error(w, "Failed to get metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, metrics)
}

func (h *Handler) RefreshMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.adminRepo.RefreshMetrics(ctx); err != nil {
		http.Error(w, "Failed to refresh metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Also refresh CloudWatch metrics if service is available
	if h.cloudwatchService != nil {
		cwMetrics, err := h.cloudwatchService.GetMetrics(ctx)
		if err == nil && cwMetrics != nil {
			h.adminRepo.UpdateSystemHealthMetrics(ctx, cwMetrics.CPUUtilization, cwMetrics.DBStorageUtilization)
		}
	}

	h.logAction(r, "refresh_metrics", "system", uuid.Nil, nil)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := h.adminRepo.GetAllSettings(ctx)
	if err != nil {
		http.Error(w, "Failed to get settings: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, settings)
}

func (h *Handler) UpdateSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")
	if key == "" {
		http.Error(w, "Setting key is required", http.StatusBadRequest)
		return
	}

	var value interface{}
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(ctx)
	if err := h.adminRepo.UpdateSetting(ctx, key, value, claims.UserID); err != nil {
		http.Error(w, "Failed to update setting: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "update_setting", "system", uuid.Nil, map[string]interface{}{"key": key})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

type MaintenanceModeRequest struct {
	Enabled bool   `json:"enabled"`
	Message string `json:"message"`
}

func (h *Handler) ToggleMaintenanceMode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req MaintenanceModeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(ctx)
	value := map[string]interface{}{
		"enabled": req.Enabled,
		"message": req.Message,
	}
	if err := h.adminRepo.UpdateSetting(ctx, "maintenance_mode", value, claims.UserID); err != nil {
		http.Error(w, "Failed to update maintenance mode: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "toggle_maintenance", "system", uuid.Nil, map[string]interface{}{"enabled": req.Enabled})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := getIntParam(r, "page", 1)
	limit := getIntParam(r, "limit", 50)
	action := r.URL.Query().Get("action")
	adminIDStr := r.URL.Query().Get("admin_id")

	var adminID uuid.UUID
	if adminIDStr != "" {
		adminID, _ = uuid.Parse(adminIDStr)
	}

	entries, total, err := h.adminRepo.GetAuditLog(ctx, adminID, action, page, limit)
	if err != nil {
		http.Error(w, "Failed to get audit log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"entries": entries,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// ============================================================================
// SUPPORT HANDLERS
// ============================================================================

func (h *Handler) ListTickets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := getIntParam(r, "page", 1)
	limit := getIntParam(r, "limit", 20)
	status := r.URL.Query().Get("status")

	tickets, total, err := h.adminRepo.GetTickets(ctx, status, page, limit)
	if err != nil {
		http.Error(w, "Failed to list tickets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"tickets": tickets,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

func (h *Handler) GetOpenTicketCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.adminRepo.GetOpenTicketCount(r.Context())
	if err != nil {
		http.Error(w, "Failed to get ticket count: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, map[string]interface{}{
		"open_count": count,
	})
}

type CreateTicketRequest struct {
	UserID      string `json:"user_id,omitempty"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

func (h *Handler) CreateTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req CreateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Subject == "" || req.Description == "" {
		http.Error(w, "Subject and description are required", http.StatusBadRequest)
		return
	}

	var userID uuid.UUID
	if req.UserID != "" {
		userID, _ = uuid.Parse(req.UserID)
	}

	ticket, err := h.adminRepo.CreateTicket(ctx, userID, req.Subject, req.Description, req.Priority)
	if err != nil {
		http.Error(w, "Failed to create ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "create_ticket", "ticket", ticket.ID, nil)
	respondJSON(w, ticket)
}

func (h *Handler) GetTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	ticket, err := h.adminRepo.GetTicketByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if ticket == nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	respondJSON(w, ticket)
}

type UpdateTicketRequest struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

func (h *Handler) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	var req UpdateTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Status != "" {
		if err := h.adminRepo.UpdateTicketStatus(ctx, id, req.Status); err != nil {
			http.Error(w, "Failed to update ticket: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	h.logAction(r, "update_ticket", "ticket", id, map[string]interface{}{"status": req.Status})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

type AssignTicketRequest struct {
	AssigneeID string `json:"assignee_id"`
}

func (h *Handler) AssignTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	var req AssignTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	assigneeID, err := uuid.Parse(req.AssigneeID)
	if err != nil {
		http.Error(w, "Invalid assignee ID", http.StatusBadRequest)
		return
	}

	if err := h.adminRepo.AssignTicket(ctx, id, assigneeID); err != nil {
		http.Error(w, "Failed to assign ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "assign_ticket", "ticket", id, map[string]interface{}{"assignee_id": req.AssigneeID})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) ResolveTicket(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(ctx)
	if err := h.adminRepo.ResolveTicket(ctx, id, claims.UserID); err != nil {
		http.Error(w, "Failed to resolve ticket: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "resolve_ticket", "ticket", id, nil)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) GetTicketMessages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	messages, err := h.adminRepo.GetTicketMessages(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get messages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, messages)
}

type AddMessageRequest struct {
	Message    string `json:"message"`
	IsInternal bool   `json:"is_internal"`
}

func (h *Handler) AddTicketMessage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket ID", http.StatusBadRequest)
		return
	}

	var req AddMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Message is required", http.StatusBadRequest)
		return
	}

	claims := middleware.GetAuthClaims(ctx)
	if err := h.adminRepo.AddTicketMessage(ctx, id, claims.UserID, req.Message, req.IsInternal); err != nil {
		http.Error(w, "Failed to add message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query().Get("q")
	page := getIntParam(r, "page", 1)
	limit := getIntParam(r, "limit", 20)

	users, total, err := h.adminRepo.SearchUsers(ctx, query, page, limit)
	if err != nil {
		http.Error(w, "Failed to search users: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"users": users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := h.adminRepo.GetUserByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	respondJSON(w, user)
}

type UpdateUserStatusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) UpdateUserStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	var req UpdateUserStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	status := models.UserStatus(req.Status)
	if err := h.adminRepo.UpdateUserStatus(ctx, id, status); err != nil {
		http.Error(w, "Failed to update user status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "update_user_status", "user", id, map[string]interface{}{"status": req.Status})
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	// Generate a temporary password (in production, send reset email instead)
	tempPassword := uuid.New().String()[:12]
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to generate password", http.StatusInternalServerError)
		return
	}

	if err := h.adminRepo.ResetUserPassword(ctx, id, string(hash)); err != nil {
		http.Error(w, "Failed to reset password: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "reset_user_password", "user", id, nil)
	respondJSON(w, map[string]interface{}{
		"success":       true,
		"temp_password": tempPassword,
		"message":       "User must change password on next login",
	})
}

func (h *Handler) ResetUserMFA(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if err := h.adminRepo.ResetUserMFA(ctx, id); err != nil {
		http.Error(w, "Failed to reset MFA: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "reset_user_mfa", "user", id, nil)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}

func (h *Handler) ListFamilies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := getIntParam(r, "page", 1)
	limit := getIntParam(r, "limit", 20)

	families, total, err := h.adminRepo.ListFamilies(ctx, page, limit)
	if err != nil {
		http.Error(w, "Failed to list families: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"families": families,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

func (h *Handler) GetFamily(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid family ID", http.StatusBadRequest)
		return
	}

	family, err := h.adminRepo.GetFamilyByID(ctx, id)
	if err != nil {
		http.Error(w, "Failed to get family: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if family == nil {
		http.Error(w, "Family not found", http.StatusNotFound)
		return
	}

	respondJSON(w, family)
}

// ============================================================================
// MARKETING HANDLERS
// ============================================================================

func (h *Handler) GetMarketingDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metrics, err := h.adminRepo.GetCachedMetrics(ctx)
	if err != nil {
		http.Error(w, "Failed to get metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, metrics)
}

func (h *Handler) GetMarketingMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metrics, err := h.adminRepo.GetCachedMetrics(ctx)
	if err != nil {
		http.Error(w, "Failed to get metrics: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, metrics)
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

func (h *Handler) logAction(r *http.Request, action, targetType string, targetID uuid.UUID, details map[string]interface{}) {
	ctx := r.Context()
	claims := middleware.GetAuthClaims(ctx)
	if claims == nil {
		return
	}
	ip := r.RemoteAddr
	userAgent := r.UserAgent()
	h.adminRepo.LogAction(ctx, claims.UserID, action, targetType, targetID, details, ip, userAgent)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func getIntParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}
