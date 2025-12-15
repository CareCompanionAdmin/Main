package api

import (
	"net/http"
	"time"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type LogHandler struct {
	logService   *service.LogService
	childService *service.ChildService
}

func NewLogHandler(logService *service.LogService, childService *service.ChildService) *LogHandler {
	return &LogHandler{
		logService:   logService,
		childService: childService,
	}
}

// GetDailyLogs returns all logs for a specific day
func (h *LogHandler) GetDailyLogs(w http.ResponseWriter, r *http.Request) {
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

	logs, err := h.logService.GetDailyLogs(r.Context(), childID, date)
	if err != nil {
		respondInternalError(w, "Failed to get daily logs")
		return
	}

	respondOK(w, logs)
}

// Behavior logs
func (h *LogHandler) CreateBehaviorLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateBehaviorLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateBehaviorLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create behavior log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetBehaviorLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetBehaviorLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get behavior logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) DeleteBehaviorLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteBehaviorLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete behavior log")
		return
	}

	respondNoContent(w)
}

// Bowel logs
func (h *LogHandler) CreateBowelLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateBowelLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateBowelLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create bowel log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetBowelLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetBowelLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get bowel logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) DeleteBowelLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteBowelLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete bowel log")
		return
	}

	respondNoContent(w)
}

// Speech logs
func (h *LogHandler) CreateSpeechLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateSpeechLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateSpeechLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create speech log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetSpeechLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetSpeechLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get speech logs")
		return
	}

	respondOK(w, logs)
}

// Diet logs
func (h *LogHandler) CreateDietLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateDietLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateDietLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create diet log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetDietLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetDietLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get diet logs")
		return
	}

	respondOK(w, logs)
}

// Weight logs
func (h *LogHandler) CreateWeightLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateWeightLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateWeightLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create weight log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetWeightLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, -3, 0) // Last 3 months for weight
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetWeightLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get weight logs")
		return
	}

	respondOK(w, logs)
}

// Sleep logs
func (h *LogHandler) CreateSleepLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateSleepLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateSleepLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create sleep log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetSleepLogs(w http.ResponseWriter, r *http.Request) {
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

	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetSleepLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get sleep logs")
		return
	}

	respondOK(w, logs)
}
