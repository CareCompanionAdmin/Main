package web

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// WebHandlers handles web page rendering
type WebHandlers struct {
	services *service.Services
}

// NewWebHandlers creates web handlers
func NewWebHandlers(services *service.Services) *WebHandlers {
	return &WebHandlers{services: services}
}

// Home renders the home page
func (h *WebHandlers) Home(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Redirect to family dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Login renders the login page
func (h *WebHandlers) Login(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", nil)
}

// Register renders the register page
func (h *WebHandlers) Register(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "register", nil)
}

// Dashboard renders the main dashboard
func (h *WebHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())

	if familyID.String() == "00000000-0000-0000-0000-000000000000" {
		// No family, redirect to create one
		http.Redirect(w, r, "/family/new", http.StatusSeeOther)
		return
	}

	// Get family dashboard
	dashboard, err := h.services.Family.GetDashboard(r.Context(), familyID)
	if err != nil {
		renderError(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	// Get user's families for switch family dropdown
	families, err := h.services.User.GetUserFamilies(r.Context(), userID)
	if err != nil {
		families = nil
	}

	data := map[string]interface{}{
		"UserID":    userID,
		"FamilyID":  familyID,
		"Dashboard": dashboard,
		"FirstName": middleware.GetFirstName(r.Context()),
		"Families":  families,
	}

	renderTemplate(w, "dashboard", data)
}

// ChildDashboard renders a child's dashboard
func (h *WebHandlers) ChildDashboard(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	dashboard, err := h.services.Child.GetDashboard(r.Context(), childID)
	if err != nil {
		renderError(w, "Failed to load dashboard", http.StatusInternalServerError)
		return
	}

	// Get all children in the family for switch child dropdown
	allChildren, err := h.services.Child.GetByFamilyID(r.Context(), child.FamilyID)
	if err != nil {
		allChildren = nil
	}

	data := map[string]interface{}{
		"Child":       child,
		"Dashboard":   dashboard,
		"AllChildren": allChildren,
	}

	renderTemplate(w, "child_dashboard", data)
}

// DailyLogs renders the daily logs page
func (h *WebHandlers) DailyLogs(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	// Get user preferences for timezone
	userTimezone := "America/New_York" // Default timezone
	prefs, err := h.services.User.GetPreferences(r.Context(), userID)
	if err == nil && prefs != nil && prefs.Timezone != "" {
		userTimezone = prefs.Timezone
	}

	loc, locErr := time.LoadLocation(userTimezone)
	if locErr != nil {
		loc = time.Local
	}

	dateStr := r.URL.Query().Get("date")
	viewMode := r.URL.Query().Get("view")
	if viewMode == "" {
		viewMode = "daily"
	}

	date := time.Now().In(loc)
	if dateStr != "" {
		date, _ = time.ParseInLocation("2006-01-02", dateStr, loc)
	}

	var logs *models.DailyLogPage
	var startDate, endDate time.Time

	if viewMode == "weekly" {
		// Calculate week bounds (Monday 00:00 to Sunday 23:59)
		startDate, endDate = h.services.Log.GetWeekBounds(date, loc)
		logs, err = h.services.Log.GetLogsForDateRange(r.Context(), childID, startDate, endDate)
		if err != nil {
			renderError(w, "Failed to load logs", http.StatusInternalServerError)
			return
		}
		logs.ViewMode = "weekly"
		logs.Date = startDate
		logs.EndDate = endDate
	} else {
		logs, err = h.services.Log.GetDailyLogs(r.Context(), childID, date)
		if err != nil {
			renderError(w, "Failed to load logs", http.StatusInternalServerError)
			return
		}
		logs.ViewMode = "daily"
		startDate = date
		endDate = date
	}

	dueMeds, err := h.services.Medication.GetDueMedications(r.Context(), childID, date)
	if err != nil {
		dueMeds = nil
	}

	data := map[string]interface{}{
		"Child":          child,
		"Date":           startDate,
		"EndDate":        endDate,
		"ViewMode":       viewMode,
		"Logs":           logs,
		"MedicationsDue": dueMeds,
		"UserTimezone":   userTimezone,
	}

	renderTemplate(w, "daily_logs", data)
}

// Medications renders the medications page
func (h *WebHandlers) Medications(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	meds, err := h.services.Medication.GetByChildID(r.Context(), childID, true)
	if err != nil {
		renderError(w, "Failed to load medications", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Child":       child,
		"Medications": meds,
	}

	renderTemplate(w, "medications", data)
}

// Alerts renders the alerts page
func (h *WebHandlers) Alerts(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	// Get user preferences for timezone
	userTimezone := "America/New_York" // Default timezone
	prefs, err := h.services.User.GetPreferences(r.Context(), userID)
	if err == nil && prefs != nil && prefs.Timezone != "" {
		userTimezone = prefs.Timezone
	}

	alertsPage, err := h.services.Alert.GetAlertsPage(r.Context(), childID)
	if err != nil {
		renderError(w, "Failed to load alerts", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Child":        child,
		"AlertsPage":   alertsPage,
		"UserTimezone": userTimezone,
	}

	renderTemplate(w, "alerts", data)
}

// Insights renders the insights page
func (h *WebHandlers) Insights(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	insights, err := h.services.Correlation.GetInsightsPage(r.Context(), childID)
	if err != nil {
		renderError(w, "Failed to load insights", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Child":    child,
		"Insights": insights,
	}

	renderTemplate(w, "insights", data)
}

// ChildSettings renders the child settings page
func (h *WebHandlers) ChildSettings(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	conditions, err := h.services.Child.GetConditions(r.Context(), childID)
	if err != nil {
		conditions = nil
	}

	data := map[string]interface{}{
		"Child":      child,
		"Conditions": conditions,
	}

	renderTemplate(w, "child_settings", data)
}

// NewChild renders the new child form
func (h *WebHandlers) NewChild(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "new_child", nil)
}

// NewFamily renders the new family form
func (h *WebHandlers) NewFamily(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "new_family", nil)
}

// Settings renders the settings page
func (h *WebHandlers) Settings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	user, err := h.services.User.GetByID(r.Context(), userID)
	if err != nil {
		renderError(w, "Failed to load user", http.StatusInternalServerError)
		return
	}

	families, err := h.services.User.GetUserFamilies(r.Context(), userID)
	if err != nil {
		families = nil
	}

	data := map[string]interface{}{
		"User":     user,
		"Families": families,
	}

	renderTemplate(w, "settings", data)
}

// AlertAnalysis renders the full alert analysis page (Layer 3)
func (h *WebHandlers) AlertAnalysis(w http.ResponseWriter, r *http.Request) {
	childID, err := parseUUID(chi.URLParam(r, "childID"))
	if err != nil {
		renderError(w, "Invalid child ID", http.StatusBadRequest)
		return
	}

	alertID := chi.URLParam(r, "alertID")
	if alertID == "" {
		renderError(w, "Invalid alert ID", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	_, err = h.services.Child.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		renderError(w, "Access denied", http.StatusForbidden)
		return
	}

	data := map[string]interface{}{
		"AlertID": alertID,
		"ChildID": childID.String(),
	}

	renderTemplate(w, "alert_analysis", data)
}

// Chat renders the family chat page
func (h *WebHandlers) Chat(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())
	firstName := middleware.GetFirstName(r.Context())

	if familyID.String() == "00000000-0000-0000-0000-000000000000" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// Get threads for this user
	threads, err := h.services.Chat.GetThreads(r.Context(), familyID, userID)
	if err != nil {
		threads = nil
	}

	// Get family members for starting new threads
	members, err := h.services.Family.GetMembers(r.Context(), familyID)
	if err != nil {
		members = nil
	}

	// Get children for child-specific threads
	children, err := h.services.Child.GetByFamilyID(r.Context(), familyID)
	if err != nil {
		children = nil
	}

	data := map[string]interface{}{
		"UserID":    userID,
		"FamilyID":  familyID,
		"FirstName": firstName,
		"Threads":   threads,
		"Members":   members,
		"Children":  children,
	}

	renderTemplate(w, "chat", data)
}
