package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// ReportHandler handles report API endpoints
type ReportHandler struct {
	reportService *service.ReportService
	childService  *service.ChildService
}

// NewReportHandler creates a new report handler
func NewReportHandler(reportService *service.ReportService, childService *service.ChildService) *ReportHandler {
	return &ReportHandler{
		reportService: reportService,
		childService:  childService,
	}
}

// GenerateReport creates an on-demand report
func (h *ReportHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.childService.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.GenerateReportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if len(req.DataFilters) == 0 {
		respondBadRequest(w, "At least one data filter is required")
		return
	}

	report, err := h.reportService.GenerateReport(r.Context(), childID, child.FamilyID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to generate report: "+err.Error())
		return
	}

	// Also return chart data so the UI can render immediately
	viewData, viewErr := h.reportService.GetViewData(r.Context(), report)
	if viewErr != nil {
		// Still return the report even if view data fails
		respondCreated(w, report)
		return
	}

	respondCreated(w, map[string]interface{}{
		"report": report,
		"view":   viewData,
	})
}

// ListReports returns past reports for a child
func (h *ReportHandler) ListReports(w http.ResponseWriter, r *http.Request) {
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

	reports, err := h.reportService.ListReports(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to list reports")
		return
	}

	respondOK(w, reports)
}

// GetReport returns a single report's metadata
func (h *ReportHandler) GetReport(w http.ResponseWriter, r *http.Request) {
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

	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	respondOK(w, report)
}

// DownloadReport streams the PDF file
func (h *ReportHandler) DownloadReport(w http.ResponseWriter, r *http.Request) {
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

	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	fp := h.reportService.GetFilePath(report)
	if fp == "" {
		respondNotFound(w, "Report file not available")
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=\""+report.Title+".pdf\"")
	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, fp)
}

// ViewReportData returns chart data for the HTML view
func (h *ReportHandler) ViewReportData(w http.ResponseWriter, r *http.Request) {
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

	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	viewData, err := h.reportService.GetViewData(r.Context(), report)
	if err != nil {
		respondInternalError(w, "Failed to get report data")
		return
	}

	respondOK(w, viewData)
}

// ShareReport shares a report via chat
func (h *ReportHandler) ShareReport(w http.ResponseWriter, r *http.Request) {
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

	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	var req models.ShareReportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	if err := h.reportService.ShareViaChat(r.Context(), report, userID, req.RecipientID); err != nil {
		respondInternalError(w, "Failed to share report: "+err.Error())
		return
	}

	respondOK(w, SuccessResponse{Success: true, Message: "Report shared via chat"})
}

// DeleteReport deletes a report and its PDF file
func (h *ReportHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
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

	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	// Delete the PDF file
	if report.FilePath.Valid {
		os.Remove(report.FilePath.String)
	}

	if err := h.reportService.DeleteReport(r.Context(), reportID); err != nil {
		respondInternalError(w, "Failed to delete report")
		return
	}

	respondNoContent(w)
}

// CreateSchedule creates a new scheduled report
func (h *ReportHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	childID, err := getChildIDFromURL(r)
	if err != nil {
		respondBadRequest(w, "Invalid child ID")
		return
	}

	userID := middleware.GetUserID(r.Context())
	child, err := h.childService.VerifyChildAccess(r.Context(), childID, userID)
	if err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	var req models.CreateScheduledReportRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	sr, err := h.reportService.CreateSchedule(r.Context(), childID, child.FamilyID, userID, &req)
	if err != nil {
		respondInternalError(w, "Failed to create schedule: "+err.Error())
		return
	}

	respondCreated(w, sr)
}

// ListSchedules returns scheduled reports for a child
func (h *ReportHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
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

	schedules, err := h.reportService.ListSchedules(r.Context(), childID)
	if err != nil {
		respondInternalError(w, "Failed to list schedules")
		return
	}

	respondOK(w, schedules)
}

// DeleteSchedule removes a scheduled report
func (h *ReportHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
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

	scheduleID, err := parseUUID(chi.URLParam(r, "scheduleID"))
	if err != nil {
		respondBadRequest(w, "Invalid schedule ID")
		return
	}

	if err := h.reportService.DeleteSchedule(r.Context(), scheduleID); err != nil {
		respondInternalError(w, "Failed to delete schedule")
		return
	}

	respondNoContent(w)
}

// ServeReportFile serves a report PDF file
func (h *ReportHandler) ServeReportFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" || strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		respondBadRequest(w, "Invalid filename")
		return
	}

	// Check auth
	userID := middleware.GetUserID(r.Context())
	if userID == [16]byte{} {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fp := filepath.Join("uploads", "reports", filename)
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		respondNotFound(w, "File not found")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	http.ServeFile(w, r, fp)
}
