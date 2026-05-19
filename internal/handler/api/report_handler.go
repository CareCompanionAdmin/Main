package api

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// signedURLTTL is how long a freshly-issued PDF URL stays valid. Short by
// design — the URL is minted right before opening in SFSafariViewController
// or Chrome Custom Tabs, where the user immediately sees the PDF.
const signedURLTTL = 10 * time.Minute

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

	rc, err := h.reportService.OpenPDF(r.Context(), report)
	if err != nil {
		respondNotFound(w, "Report file not available")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=\""+report.Title+".pdf\"")
	w.Header().Set("Content-Type", "application/pdf")
	if _, err := io.Copy(w, rc); err != nil {
		// Headers already written, so we can't change status. Log for observability.
		log.Printf("[REPORT] DownloadReport io.Copy failed for report %s: %v", report.ID, err)
	}
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

// GetSignedURL returns a short-lived HMAC-signed URL for the report PDF.
// SFSafariViewController and Chrome Custom Tabs don't share the WKWebView's
// localStorage or auth cookies, so they can't hit the JWT-protected endpoints
// directly. The signed URL is unauthenticated but tied to a specific report
// ID + expiry so it can't be reused or pivoted.
func (h *ReportHandler) GetSignedURL(w http.ResponseWriter, r *http.Request) {
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

	path, exp := h.reportService.SignedPDFURL(report.ID, signedURLTTL)
	respondOK(w, map[string]interface{}{
		"path":       path,
		"expires_at": exp.Format(time.RFC3339),
	})
}

// ServeSignedPDF streams a report PDF when the request carries a valid HMAC
// signature. No JWT required — the URL itself is the bearer credential, and
// it expires in minutes. Used by Capacitor Browser, AirPrint, and external
// share targets that can't carry our auth headers.
func (h *ReportHandler) ServeSignedPDF(w http.ResponseWriter, r *http.Request) {
	reportID, err := parseUUID(chi.URLParam(r, "reportID"))
	if err != nil {
		respondBadRequest(w, "Invalid report ID")
		return
	}

	expRaw := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if expRaw == "" || sig == "" {
		respondBadRequest(w, "Missing signature")
		return
	}

	expUnix, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		respondBadRequest(w, "Bad expiry")
		return
	}

	if err := h.reportService.VerifySignedPDF(reportID, expUnix, sig); err != nil {
		respondError(w, "Link expired or invalid", http.StatusForbidden)
		return
	}

	report, err := h.reportService.GetByID(r.Context(), reportID)
	if err != nil || report == nil {
		respondNotFound(w, "Report not found")
		return
	}

	rc, err := h.reportService.OpenPDF(r.Context(), report)
	if err != nil {
		respondNotFound(w, "Report file not available")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+report.Title+".pdf\"")
	if _, err := io.Copy(w, rc); err != nil {
		// Headers already written, so we can't change status. Log for observability.
		log.Printf("[REPORT] ServeSignedPDF io.Copy failed for report %s: %v", report.ID, err)
	}
}

// ServeReportPDF streams a report PDF from blob storage. Routed by reportID
// instead of filename so we don't have to look up storage_path by basename.
func (h *ReportHandler) ServeReportPDF(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == [16]byte{} {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
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

	// Same access check used elsewhere — the user must have access to the
	// child the report is about.
	if _, err := h.childService.VerifyChildAccess(r.Context(), report.ChildID, userID); err != nil {
		respondForbidden(w, "Access denied")
		return
	}

	rc, err := h.reportService.OpenPDF(r.Context(), report)
	if err != nil {
		respondNotFound(w, "Report file not available")
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=\""+report.Title+".pdf\"")
	if _, err := io.Copy(w, rc); err != nil {
		// Connection drop / write error; nothing useful to send back.
		return
	}
}
