package api

import (
	"fmt"
	stdlog "log"
	"net/http"
	"strconv"
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

// GetDatesWithLogs returns dates that have log entries
func (h *LogHandler) GetDatesWithLogs(w http.ResponseWriter, r *http.Request) {
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

	// Default to 30 days
	limit := 30
	dates, err := h.logService.GetDatesWithLogs(r.Context(), childID, limit)
	if err != nil {
		respondInternalError(w, "Failed to get dates with logs")
		return
	}

	respondOK(w, dates)
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
		stdlog.Printf("CreateBehaviorLog decode error: %v", err)
		respondBadRequest(w, "Invalid request body: "+err.Error())
		return
	}

	log, err := h.logService.CreateBehaviorLog(r.Context(), childID, userID, &req)
	if err != nil {
		stdlog.Printf("CreateBehaviorLog error: %v", err)
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

func (h *LogHandler) UpdateBehaviorLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetBehaviorLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Behavior log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateBehaviorLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.LogTime.String = req.LogTime
	existing.LogTime.Valid = req.LogTime != ""
	existing.TimeScope.String = req.TimeScope
	existing.TimeScope.Valid = req.TimeScope != ""
	existing.MoodLevel = req.MoodLevel
	existing.EnergyLevel = req.EnergyLevel
	existing.AnxietyLevel = req.AnxietyLevel
	existing.InterpersonalBehavior.String = req.InterpersonalBehavior
	existing.InterpersonalBehavior.Valid = req.InterpersonalBehavior != ""
	existing.Meltdowns = req.Meltdowns
	existing.StimmingEpisodes = req.StimmingEpisodes
	existing.StimmingLevel.String = req.StimmingLevel
	existing.StimmingLevel.Valid = req.StimmingLevel != ""
	existing.AggressionIncidents = req.AggressionIncidents
	existing.SelfInjuryIncidents = req.SelfInjuryIncidents
	existing.Triggers = req.Triggers
	existing.PositiveBehaviors = req.PositiveBehaviors
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateBehaviorLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update behavior log")
		return
	}

	respondOK(w, existing)
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

func (h *LogHandler) UpdateBowelLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetBowelLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Bowel log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateBowelLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.LogTime.String = req.LogTime
	existing.LogTime.Valid = req.LogTime != ""
	existing.BristolScale = req.BristolScale
	existing.HadAccident = req.HadAccident
	existing.PainLevel = req.PainLevel
	existing.BloodPresent = req.BloodPresent
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateBowelLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update bowel log")
		return
	}

	respondOK(w, existing)
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
		stdlog.Printf("Failed to create speech log: %v", err)
		respondInternalError(w, "Failed to create speech log: "+err.Error())
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

func (h *LogHandler) UpdateSpeechLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetSpeechLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Speech log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateSpeechLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.VerbalOutputLevel = req.VerbalOutputLevel
	existing.ClarityLevel = req.ClarityLevel
	existing.NewWords = req.NewWords
	existing.LostWords = req.LostWords
	existing.EcholaliaLevel = req.EcholaliaLevel
	existing.CommunicationAttempts = req.CommunicationAttempts
	existing.SuccessfulCommunications = req.SuccessfulCommunications
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateSpeechLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update speech log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteSpeechLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteSpeechLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete speech log")
		return
	}

	respondNoContent(w)
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

func (h *LogHandler) UpdateDietLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetDietLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Diet log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateDietLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.MealType.String = req.MealType
	existing.MealType.Valid = req.MealType != ""
	existing.MealTime.String = req.MealTime
	existing.MealTime.Valid = req.MealTime != ""
	existing.FoodsEaten = req.FoodsEaten
	existing.FoodsRefused = req.FoodsRefused
	existing.AppetiteLevel.String = req.AppetiteLevel
	existing.AppetiteLevel.Valid = req.AppetiteLevel != ""
	existing.WaterIntakeOz = req.WaterIntakeOz
	existing.SupplementsTaken = req.SupplementsTaken
	existing.NewFoodTried.String = req.NewFoodTried
	existing.NewFoodTried.Valid = req.NewFoodTried != ""
	existing.NewFoodAcceptance.String = req.NewFoodAcceptance
	existing.NewFoodAcceptance.Valid = req.NewFoodAcceptance != ""
	existing.AllergicReaction = req.AllergicReaction
	existing.ReactionDetails.String = req.ReactionDetails
	existing.ReactionDetails.Valid = req.ReactionDetails != ""
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateDietLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update diet log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteDietLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteDietLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete diet log")
		return
	}

	respondNoContent(w)
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

func (h *LogHandler) UpdateWeightLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetWeightLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Weight log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateWeightLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.WeightLbs = req.WeightLbs
	existing.HeightInches = req.HeightInches
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateWeightLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update weight log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteWeightLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteWeightLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete weight log")
		return
	}

	respondNoContent(w)
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

func (h *LogHandler) UpdateSleepLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetSleepLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Sleep log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateSleepLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.Bedtime.String = req.Bedtime
	existing.Bedtime.Valid = req.Bedtime != ""
	existing.WakeTime.String = req.WakeTime
	existing.WakeTime.Valid = req.WakeTime != ""
	existing.TotalSleepMinutes = req.TotalSleepMinutes
	existing.NightWakings = req.NightWakings
	existing.SleepQuality.String = req.SleepQuality
	existing.SleepQuality.Valid = req.SleepQuality != ""
	existing.TookSleepAid = req.TookSleepAid
	existing.SleepAidName.String = req.SleepAidName
	existing.SleepAidName.Valid = req.SleepAidName != ""
	existing.Nightmares = req.Nightmares
	existing.BedWetting = req.BedWetting
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateSleepLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update sleep log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteSleepLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteSleepLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete sleep log")
		return
	}

	respondNoContent(w)
}

// Sensory logs
func (h *LogHandler) CreateSensoryLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateSensoryLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateSensoryLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create sensory log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetSensoryLogs(w http.ResponseWriter, r *http.Request) {
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

	logs, err := h.logService.GetSensoryLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get sensory logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) UpdateSensoryLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetSensoryLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Sensory log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateSensoryLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.LogTime.String = req.LogTime
	existing.LogTime.Valid = req.LogTime != ""
	existing.SensorySeekingBehaviors = models.StringArray(req.SensorySeekingBehaviors)
	existing.SensoryAvoidingBehaviors = models.StringArray(req.SensoryAvoidingBehaviors)
	existing.OverloadTriggers = models.StringArray(req.OverloadTriggers)
	existing.CalmingStrategiesUsed = models.StringArray(req.CalmingStrategiesUsed)
	existing.OverloadEpisodes = req.OverloadEpisodes
	existing.OverallRegulation = req.OverallRegulation
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateSensoryLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update sensory log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteSensoryLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteSensoryLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete sensory log")
		return
	}

	respondNoContent(w)
}

// Social logs
func (h *LogHandler) CreateSocialLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateSocialLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateSocialLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create social log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetSocialLogs(w http.ResponseWriter, r *http.Request) {
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

	logs, err := h.logService.GetSocialLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get social logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) UpdateSocialLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetSocialLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Social log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateSocialLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.EyeContactLevel = req.EyeContactLevel
	existing.SocialEngagementLevel = req.SocialEngagementLevel
	existing.PeerInteractions = req.PeerInteractions
	existing.PositiveInteractions = req.PositiveInteractions
	existing.Conflicts = req.Conflicts
	existing.ParallelPlayMinutes = req.ParallelPlayMinutes
	existing.CooperativePlayMinutes = req.CooperativePlayMinutes
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateSocialLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update social log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteSocialLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteSocialLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete social log")
		return
	}

	respondNoContent(w)
}

// Therapy logs
func (h *LogHandler) CreateTherapyLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateTherapyLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateTherapyLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create therapy log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetTherapyLogs(w http.ResponseWriter, r *http.Request) {
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
	startDate := endDate.AddDate(0, -1, 0) // Last month for therapy
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetTherapyLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get therapy logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) UpdateTherapyLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetTherapyLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Therapy log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateTherapyLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.TherapyType.String = req.TherapyType
	existing.TherapyType.Valid = req.TherapyType != ""
	existing.TherapistName.String = req.TherapistName
	existing.TherapistName.Valid = req.TherapistName != ""
	existing.DurationMinutes = req.DurationMinutes
	existing.GoalsWorkedOn = req.GoalsWorkedOn
	existing.ProgressNotes.String = req.ProgressNotes
	existing.ProgressNotes.Valid = req.ProgressNotes != ""
	existing.HomeworkAssigned.String = req.HomeworkAssigned
	existing.HomeworkAssigned.Valid = req.HomeworkAssigned != ""
	existing.ParentNotes.String = req.ParentNotes
	existing.ParentNotes.Valid = req.ParentNotes != ""

	if err := h.logService.UpdateTherapyLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update therapy log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteTherapyLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteTherapyLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete therapy log")
		return
	}

	respondNoContent(w)
}

// Seizure logs
func (h *LogHandler) CreateSeizureLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateSeizureLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateSeizureLog(r.Context(), childID, userID, &req)
	if err != nil {
		stdlog.Printf("Failed to create seizure log: %v", err)
		respondInternalError(w, "Failed to create seizure log: "+err.Error())
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetSeizureLogs(w http.ResponseWriter, r *http.Request) {
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
	startDate := endDate.AddDate(0, -1, 0) // Last month for seizures
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetSeizureLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get seizure logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) UpdateSeizureLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetSeizureLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Seizure log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateSeizureLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.LogTime = req.LogTime
	existing.SeizureType.String = req.SeizureType
	existing.SeizureType.Valid = req.SeizureType != ""
	existing.DurationSeconds = req.DurationSeconds
	existing.Triggers = models.StringArray(req.Triggers)
	existing.WarningSigns = models.StringArray(req.WarningSigns)
	existing.PostIctalSymptoms = models.StringArray(req.PostIctalSymptoms)
	existing.RescueMedicationGiven = req.RescueMedicationGiven
	existing.RescueMedicationName.String = req.RescueMedicationName
	existing.RescueMedicationName.Valid = req.RescueMedicationName != ""
	existing.Called911 = req.Called911
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateSeizureLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update seizure log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteSeizureLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteSeizureLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete seizure log")
		return
	}

	respondNoContent(w)
}

// Health event logs
func (h *LogHandler) CreateHealthEventLog(w http.ResponseWriter, r *http.Request) {
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

	var req models.CreateHealthEventLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	log, err := h.logService.CreateHealthEventLog(r.Context(), childID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create health event log")
		return
	}

	respondCreated(w, log)
}

func (h *LogHandler) GetHealthEventLogs(w http.ResponseWriter, r *http.Request) {
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
	startDate := endDate.AddDate(0, -3, 0) // Last 3 months for health events
	startDate = getDateFromQuery(r, "start_date", startDate)
	endDate = getDateFromQuery(r, "end_date", endDate)

	logs, err := h.logService.GetHealthEventLogs(r.Context(), childID, startDate, endDate)
	if err != nil {
		respondInternalError(w, "Failed to get health event logs")
		return
	}

	respondOK(w, logs)
}

func (h *LogHandler) UpdateHealthEventLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	existing, err := h.logService.GetHealthEventLogByID(r.Context(), logID)
	if err != nil || existing == nil {
		respondNotFound(w, "Health event log not found")
		return
	}

	userID := middleware.GetUserID(r.Context())
	if _, err := h.childService.VerifyChildAccess(r.Context(), existing.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateHealthEventLogRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	existing.EventType.String = req.EventType
	existing.EventType.Valid = req.EventType != ""
	existing.Description.String = req.Description
	existing.Description.Valid = req.Description != ""
	existing.Symptoms = req.Symptoms
	existing.TemperatureF = req.TemperatureF
	existing.ProviderName.String = req.ProviderName
	existing.ProviderName.Valid = req.ProviderName != ""
	existing.Diagnosis.String = req.Diagnosis
	existing.Diagnosis.Valid = req.Diagnosis != ""
	existing.Treatment.String = req.Treatment
	existing.Treatment.Valid = req.Treatment != ""
	if req.FollowUpDate != nil {
		existing.FollowUpDate.Time = *req.FollowUpDate
		existing.FollowUpDate.Valid = true
	} else {
		existing.FollowUpDate.Valid = false
	}
	existing.Notes.String = req.Notes
	existing.Notes.Valid = req.Notes != ""

	if err := h.logService.UpdateHealthEventLog(r.Context(), existing); err != nil {
		respondInternalError(w, "Failed to update health event log")
		return
	}

	respondOK(w, existing)
}

func (h *LogHandler) DeleteHealthEventLog(w http.ResponseWriter, r *http.Request) {
	logID, err := getIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid log ID")
		return
	}

	if err := h.logService.DeleteHealthEventLog(r.Context(), logID); err != nil {
		respondInternalError(w, "Failed to delete health event log")
		return
	}

	respondNoContent(w)
}

// QuickSummaryResponse represents the response for quick summary
type QuickSummaryResponse struct {
	Category    string       `json:"category"`
	TimeRange   string       `json:"time_range"`
	Days        []DaySummary `json:"days"`
	PeriodStart string       `json:"period_start"`
	PeriodEnd   string       `json:"period_end"`
	PeriodLabel string       `json:"period_label"`
	Offset      int          `json:"offset"`
}

type DaySummary struct {
	Date      string  `json:"date"`
	DayOfWeek string  `json:"day_of_week"`
	Score     float64 `json:"score"` // 0-100, where 100 is best
	HasData   bool    `json:"has_data"`
	Details   string  `json:"details,omitempty"`
}

// GetQuickSummary returns a summary of logs for a category over a time range
func (h *LogHandler) GetQuickSummary(w http.ResponseWriter, r *http.Request) {
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

	category := r.URL.Query().Get("category")
	if category == "" {
		category = "behavior"
	}

	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "weekly"
	}

	// Parse offset parameter (negative = past, positive = future)
	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	// Calculate date range
	now := time.Now()
	var startDate, endDate time.Time
	var days []DaySummary
	var periodLabel string

	switch timeRange {
	case "daily":
		// Apply offset (each offset unit = 1 day)
		baseDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		startDate = baseDate.AddDate(0, 0, offset)
		endDate = startDate.Add(24 * time.Hour)
		days = make([]DaySummary, 1)
		days[0] = DaySummary{
			Date:      startDate.Format("2006-01-02"),
			DayOfWeek: startDate.Weekday().String(),
		}
		// Format: "Monday, Jan 13, 2026"
		periodLabel = startDate.Format("Monday, Jan 2, 2006")
	case "weekly":
		// Start from Sunday of current week, then apply offset (each offset unit = 1 week)
		daysUntilSunday := int(now.Weekday())
		baseDate := time.Date(now.Year(), now.Month(), now.Day()-daysUntilSunday, 0, 0, 0, 0, now.Location())
		startDate = baseDate.AddDate(0, 0, offset*7)
		endDate = startDate.Add(7 * 24 * time.Hour)
		days = make([]DaySummary, 7)
		for i := 0; i < 7; i++ {
			d := startDate.Add(time.Duration(i) * 24 * time.Hour)
			days[i] = DaySummary{
				Date:      d.Format("2006-01-02"),
				DayOfWeek: d.Weekday().String()[:3],
			}
		}
		// Format: "Jan 5 - 11, 2026" or "Dec 29, 2025 - Jan 4, 2026"
		endDisplay := endDate.AddDate(0, 0, -1) // Last day of week
		if startDate.Month() == endDisplay.Month() {
			periodLabel = fmt.Sprintf("%s %d - %d, %d", startDate.Format("Jan"), startDate.Day(), endDisplay.Day(), startDate.Year())
		} else if startDate.Year() == endDisplay.Year() {
			periodLabel = fmt.Sprintf("%s %d - %s %d, %d", startDate.Format("Jan"), startDate.Day(), endDisplay.Format("Jan"), endDisplay.Day(), startDate.Year())
		} else {
			periodLabel = fmt.Sprintf("%s %d, %d - %s %d, %d", startDate.Format("Jan"), startDate.Day(), startDate.Year(), endDisplay.Format("Jan"), endDisplay.Day(), endDisplay.Year())
		}
	case "monthly":
		// Apply offset (each offset unit = 1 month)
		baseDate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		startDate = baseDate.AddDate(0, offset, 0)
		endDate = startDate.AddDate(0, 1, 0)
		numDays := int(endDate.Sub(startDate).Hours() / 24)
		days = make([]DaySummary, numDays)
		for i := 0; i < numDays; i++ {
			d := startDate.Add(time.Duration(i) * 24 * time.Hour)
			days[i] = DaySummary{
				Date:      d.Format("2006-01-02"),
				DayOfWeek: d.Weekday().String()[:3],
			}
		}
		// Format: "January 2026"
		periodLabel = startDate.Format("January 2006")
	default:
		respondBadRequest(w, "Invalid time range")
		return
	}

	// Get summary data from service
	summaryData, err := h.logService.GetQuickSummary(r.Context(), childID, category, startDate, endDate)
	if err != nil {
		stdlog.Printf("Error getting quick summary: %v", err)
		respondInternalError(w, "Failed to get summary")
		return
	}

	// Map the data to days
	for i := range days {
		if data, ok := summaryData[days[i].Date]; ok {
			days[i].Score = data.Score
			days[i].HasData = data.HasData
			days[i].Details = data.Details
		}
	}

	response := QuickSummaryResponse{
		Category:    category,
		TimeRange:   timeRange,
		Days:        days,
		PeriodStart: startDate.Format("2006-01-02"),
		PeriodEnd:   endDate.AddDate(0, 0, -1).Format("2006-01-02"),
		PeriodLabel: periodLabel,
		Offset:      offset,
	}

	respondOK(w, response)
}
