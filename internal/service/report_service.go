package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ReportService handles report generation, chart rendering, and PDF creation
type ReportService struct {
	reportRepo    repository.ReportRepository
	logRepo       repository.LogRepository
	childRepo     repository.ChildRepository
	chatRepo      repository.ChatRepository
	storage       BlobStorage
	signingSecret []byte
}

// NewReportService creates a new report service. signingSecret is used to
// HMAC-sign short-lived PDF URLs so SFSafariViewController / Custom Tabs
// can fetch the file without inheriting the WKWebView's JWT (cookies and
// localStorage don't cross that boundary on iOS or Android).
func NewReportService(reportRepo repository.ReportRepository, logRepo repository.LogRepository, childRepo repository.ChildRepository, chatRepo repository.ChatRepository, storage BlobStorage, signingSecret string) *ReportService {
	return &ReportService{
		reportRepo:    reportRepo,
		logRepo:       logRepo,
		childRepo:     childRepo,
		chatRepo:      chatRepo,
		storage:       storage,
		signingSecret: []byte(signingSecret),
	}
}

// SignedPDFURL returns a path with HMAC signature + expiry that ServeSignedPDF
// will accept without auth. TTL is short by design — the URL is meant to be
// minted just before opening in a system browser.
func (s *ReportService) SignedPDFURL(reportID uuid.UUID, ttl time.Duration) (path string, exp time.Time) {
	exp = time.Now().Add(ttl)
	expUnix := exp.Unix()
	mac := hmac.New(sha256.New, s.signingSecret)
	mac.Write([]byte(reportID.String() + "|" + strconv.FormatInt(expUnix, 10)))
	sig := hex.EncodeToString(mac.Sum(nil))
	path = fmt.Sprintf("/r/signed/%s?exp=%d&sig=%s", reportID.String(), expUnix, sig)
	return path, exp
}

// VerifySignedPDF confirms exp is in the future and sig matches reportID|exp
// signed with our secret. Returns nil on success.
func (s *ReportService) VerifySignedPDF(reportID uuid.UUID, expUnix int64, sig string) error {
	if time.Now().Unix() > expUnix {
		return fmt.Errorf("signed url expired")
	}
	mac := hmac.New(sha256.New, s.signingSecret)
	mac.Write([]byte(reportID.String() + "|" + strconv.FormatInt(expUnix, 10)))
	want := hex.EncodeToString(mac.Sum(nil))
	got, err := hex.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("bad signature encoding")
	}
	wantBytes, _ := hex.DecodeString(want)
	if !hmac.Equal(got, wantBytes) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

// GenerateReport creates a report with PDF
func (s *ReportService) GenerateReport(ctx context.Context, childID, familyID, userID uuid.UUID, req *models.GenerateReportRequest) (*models.Report, error) {
	startDate, endDate, err := computeDateRange(req)
	if err != nil {
		return nil, err
	}

	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil || child == nil {
		return nil, fmt.Errorf("child not found")
	}

	title := fmt.Sprintf("%s - %s Report (%s to %s)",
		child.FirstName, strings.Title(req.PeriodType),
		startDate.Format("Jan 2"), endDate.Format("Jan 2, 2006"))

	report := &models.Report{
		ChildID:     childID,
		FamilyID:    familyID,
		CreatedBy:   userID,
		Title:       title,
		ReportType:  "on_demand",
		PeriodType:  req.PeriodType,
		StartDate:   startDate,
		EndDate:     endDate,
		DataFilters: models.StringArray(req.DataFilters),
	}

	if err := s.reportRepo.Create(ctx, report); err != nil {
		return nil, fmt.Errorf("failed to create report record: %w", err)
	}

	// Get log data for the date range
	logs, err := s.logRepo.GetLogsForDateRange(ctx, childID, startDate, endDate)
	if err != nil {
		s.reportRepo.UpdateError(ctx, report.ID, err.Error())
		return nil, fmt.Errorf("failed to get log data: %w", err)
	}

	// Aggregate chart data
	log.Printf("Report data for %s: behavior=%d sleep=%d diet=%d med=%d bowel=%d sensory=%d social=%d therapy=%d speech=%d seizure=%d weight=%d health=%d",
		child.FirstName,
		len(logs.BehaviorLogs), len(logs.SleepLogs), len(logs.DietLogs), len(logs.MedicationLogs),
		len(logs.BowelLogs), len(logs.SensoryLogs), len(logs.SocialLogs), len(logs.TherapyLogs),
		len(logs.SpeechLogs), len(logs.SeizureLogs), len(logs.WeightLogs), len(logs.HealthEventLogs))
	chartData := s.aggregateChartData(logs, req.DataFilters, startDate, endDate)

	// Generate PDF
	driver, storagePath, fileSize, err := s.generatePDF(ctx, report.ID, child, startDate, endDate, req.DataFilters, chartData, logs)
	if err != nil {
		s.reportRepo.UpdateError(ctx, report.ID, err.Error())
		return nil, fmt.Errorf("failed to generate PDF: %w", err)
	}

	if err := s.reportRepo.UpdateStatus(ctx, report.ID, "completed", driver, storagePath, fileSize); err != nil {
		return nil, err
	}

	report.Status = "completed"
	report.StorageDriver = models.NullString{NullString: sql.NullString{String: driver, Valid: true}}
	report.StoragePath = models.NullString{NullString: sql.NullString{String: storagePath, Valid: true}}
	sz := fileSize
	report.FileSize = &sz
	return report, nil
}

// GetViewData returns chart data and logs for the HTML view
func (s *ReportService) GetViewData(ctx context.Context, report *models.Report) (*models.ReportChartData, error) {
	child, _ := s.childRepo.GetByID(ctx, report.ChildID)
	childName := ""
	if child != nil {
		childName = child.FirstName
	}

	logs, err := s.logRepo.GetLogsForDateRange(ctx, report.ChildID, report.StartDate, report.EndDate)
	if err != nil {
		return nil, err
	}

	chartData := s.aggregateChartData(logs, []string(report.DataFilters), report.StartDate, report.EndDate)

	return &models.ReportChartData{
		ReportID:  report.ID,
		ChildName: childName,
		StartDate: report.StartDate.Format("2006-01-02"),
		EndDate:   report.EndDate.Format("2006-01-02"),
		Charts:    chartData,
		Logs:      logs,
	}, nil
}

// OpenPDF returns a reader for the report's PDF, picking the driver from
// the row. Caller must close the reader.
func (s *ReportService) OpenPDF(ctx context.Context, report *models.Report) (io.ReadCloser, error) {
	if !report.StoragePath.Valid {
		return nil, fmt.Errorf("report has no stored PDF")
	}
	return s.storage.Open(ctx, report.StoragePath.String)
}

// ListReports returns past reports for a child
func (s *ReportService) ListReports(ctx context.Context, childID uuid.UUID) ([]models.Report, error) {
	return s.reportRepo.GetByChildID(ctx, childID, 50)
}

// GetByID returns a report by ID
func (s *ReportService) GetByID(ctx context.Context, id uuid.UUID) (*models.Report, error) {
	return s.reportRepo.GetByID(ctx, id)
}

// DeleteReport deletes a report record
func (s *ReportService) DeleteReport(ctx context.Context, id uuid.UUID) error {
	report, err := s.reportRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if report != nil && report.StoragePath.Valid {
		if err := s.storage.Delete(ctx, report.StoragePath.String); err != nil {
			log.Printf("[REPORT] storage delete failed for %s: %v", id, err)
		}
	}
	return s.reportRepo.Delete(ctx, id)
}

// CreateSchedule creates a new scheduled report
func (s *ReportService) CreateSchedule(ctx context.Context, childID, familyID, userID uuid.UUID, req *models.CreateScheduledReportRequest) (*models.ScheduledReport, error) {
	sr := &models.ScheduledReport{
		ChildID:     childID,
		FamilyID:    familyID,
		CreatedBy:   userID,
		Frequency:   req.Frequency,
		DataFilters: models.StringArray(req.DataFilters),
		Recipients:  models.UUIDArray(req.Recipients),
		NextRunAt:   ComputeFirstRun(req.Frequency),
	}

	if err := s.reportRepo.CreateScheduled(ctx, sr); err != nil {
		return nil, err
	}
	return sr, nil
}

// ListSchedules returns scheduled reports for a child
func (s *ReportService) ListSchedules(ctx context.Context, childID uuid.UUID) ([]models.ScheduledReport, error) {
	return s.reportRepo.GetScheduledByChildID(ctx, childID)
}

// DeleteSchedule deactivates a scheduled report
func (s *ReportService) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return s.reportRepo.DeleteScheduled(ctx, id)
}

// aggregateChartData processes log data into chart-friendly series
func (s *ReportService) aggregateChartData(logs *models.DailyLogPage, filters []string, startDate, endDate time.Time) map[string][]models.ChartDataPoint {
	charts := make(map[string][]models.ChartDataPoint)
	filterSet := make(map[string]bool)
	for _, f := range filters {
		filterSet[f] = true
	}

	if filterSet["behavior"] && len(logs.BehaviorLogs) > 0 {
		charts["Mood Level"] = aggregateByDay(logs.BehaviorLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.BehaviorLog) {
				d := l.LogDate.Format("2006-01-02")
				if l.MoodLevel != nil { m[d] = append(m[d], float64(*l.MoodLevel)) }
			}
			return m
		}, "avg")
		charts["Meltdowns"] = aggregateByDay(logs.BehaviorLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.BehaviorLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], float64(l.Meltdowns))
			}
			return m
		}, "sum")
	}

	if filterSet["sleep"] && len(logs.SleepLogs) > 0 {
		charts["Sleep Hours"] = aggregateByDay(logs.SleepLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.SleepLog) {
				d := l.LogDate.Format("2006-01-02")
				mins := 0; if l.TotalSleepMinutes != nil { mins = *l.TotalSleepMinutes }; hours := float64(mins) / 60.0
				m[d] = append(m[d], hours)
			}
			return m
		}, "sum")
	}

	if filterSet["diet"] && len(logs.DietLogs) > 0 {
		charts["Meals Logged"] = aggregateByDay(logs.DietLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.DietLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	if filterSet["medication"] && len(logs.MedicationLogs) > 0 {
		charts["Medications Logged"] = aggregateByDay(logs.MedicationLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.MedicationLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	if filterSet["bowel"] && len(logs.BowelLogs) > 0 {
		charts["Bowel - Bristol Scale"] = aggregateByDay(logs.BowelLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.BowelLog) {
				d := l.LogDate.Format("2006-01-02")
				if l.BristolScale != nil { m[d] = append(m[d], float64(*l.BristolScale)) }
			}
			return m
		}, "avg")
	}

	if filterSet["sensory"] && len(logs.SensoryLogs) > 0 {
		charts["Sensory Overload Level"] = aggregateByDay(logs.SensoryLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.SensoryLog) {
				d := l.LogDate.Format("2006-01-02")
				if l.OverallRegulation != nil { m[d] = append(m[d], float64(*l.OverallRegulation)) }
			}
			return m
		}, "avg")
	}

	if filterSet["social"] && len(logs.SocialLogs) > 0 {
		charts["Social Interactions"] = aggregateByDay(logs.SocialLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.SocialLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	if filterSet["therapy"] && len(logs.TherapyLogs) > 0 {
		charts["Therapy Sessions"] = aggregateByDay(logs.TherapyLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.TherapyLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	if filterSet["seizure"] && len(logs.SeizureLogs) > 0 {
		charts["Seizure Events"] = aggregateByDay(logs.SeizureLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.SeizureLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	if filterSet["speech"] && len(logs.SpeechLogs) > 0 {
		charts["Speech - Verbal Level"] = aggregateByDay(logs.SpeechLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.SpeechLog) {
				d := l.LogDate.Format("2006-01-02")
				if l.VerbalOutputLevel != nil { m[d] = append(m[d], float64(*l.VerbalOutputLevel)) }
			}
			return m
		}, "avg")
	}

	if filterSet["weight"] && len(logs.WeightLogs) > 0 {
		charts["Weight"] = aggregateByDay(logs.WeightLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.WeightLog) {
				d := l.LogDate.Format("2006-01-02")
				if l.WeightLbs != nil { m[d] = append(m[d], *l.WeightLbs) }
			}
			return m
		}, "avg")
	}

	if filterSet["health_event"] && len(logs.HealthEventLogs) > 0 {
		charts["Health Events"] = aggregateByDay(logs.HealthEventLogs, startDate, endDate, func(items interface{}) map[string][]float64 {
			m := make(map[string][]float64)
			for _, l := range items.([]models.HealthEventLog) {
				d := l.LogDate.Format("2006-01-02")
				m[d] = append(m[d], 1)
			}
			return m
		}, "sum")
	}

	return charts
}

// aggregateByDay is a generic helper that groups values by day and applies sum or avg
func aggregateByDay(items interface{}, startDate, endDate time.Time, extractor func(interface{}) map[string][]float64, method string) []models.ChartDataPoint {
	dayValues := extractor(items)

	var points []models.ChartDataPoint
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		vals := dayValues[dateStr]
		var value float64
		if len(vals) > 0 {
			if method == "avg" {
				sum := 0.0
				for _, v := range vals {
					sum += v
				}
				value = sum / float64(len(vals))
			} else {
				for _, v := range vals {
					value += v
				}
			}
		}
		points = append(points, models.ChartDataPoint{
			Date:  dateStr,
			Value: math.Round(value*100) / 100,
		})
	}
	return points
}

// computeDateRange calculates start and end dates from the request
func computeDateRange(req *models.GenerateReportRequest) (time.Time, time.Time, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Normalize legacy / alternate period aliases so older app builds (which
	// sent e.g. "last_30_days") don't 500. The canonical values are
	// day / week / month / custom. (#112392)
	switch req.PeriodType {
	case "today", "daily", "last_24_hours", "last_day":
		req.PeriodType = "day"
	case "weekly", "last_7_days", "last_week", "7_days":
		req.PeriodType = "week"
	case "monthly", "last_30_days", "last_month", "30_days":
		req.PeriodType = "month"
	}

	switch req.PeriodType {
	case "day":
		return today, today, nil
	case "week":
		return today.AddDate(0, 0, -6), today, nil
	case "month":
		return today.AddDate(0, 0, -29), today, nil
	case "custom":
		start, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date: %w", err)
		}
		end, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date: %w", err)
		}
		if end.Before(start) {
			return time.Time{}, time.Time{}, fmt.Errorf("end_date must be after start_date")
		}
		return start, end, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period_type: %s", req.PeriodType)
	}
}

// renderChartImage creates a PNG chart image using fogleman/gg
func renderChartImage(series []models.ChartDataPoint, title string, width, height int) ([]byte, error) {
	dc := gg.NewContext(width, height)

	// Background
	dc.SetColor(color.White)
	dc.Clear()

	// Margins
	marginLeft := 60.0
	marginRight := 20.0
	marginTop := 40.0
	marginBottom := 50.0
	chartW := float64(width) - marginLeft - marginRight
	chartH := float64(height) - marginTop - marginBottom

	// Title
	dc.SetColor(color.RGBA{55, 65, 81, 255})
	dc.DrawStringAnchored(title, float64(width)/2, 20, 0.5, 0.5)

	if len(series) == 0 {
		dc.DrawStringAnchored("No data", float64(width)/2, float64(height)/2, 0.5, 0.5)
		var buf bytes.Buffer
		png.Encode(&buf, dc.Image())
		return buf.Bytes(), nil
	}

	// Find value range
	maxVal := 0.0
	for _, p := range series {
		if p.Value > maxVal {
			maxVal = p.Value
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}
	maxVal *= 1.1 // 10% headroom

	// Draw grid lines
	dc.SetColor(color.RGBA{229, 231, 235, 255})
	gridLines := 5
	for i := 0; i <= gridLines; i++ {
		y := marginTop + chartH - (float64(i)/float64(gridLines))*chartH
		dc.DrawLine(marginLeft, y, marginLeft+chartW, y)
		dc.Stroke()

		// Y-axis labels
		val := (float64(i) / float64(gridLines)) * maxVal
		dc.SetColor(color.RGBA{107, 114, 128, 255})
		dc.DrawStringAnchored(fmt.Sprintf("%.0f", val), marginLeft-10, y, 1, 0.5)
		dc.SetColor(color.RGBA{229, 231, 235, 255})
	}

	// Draw axes
	dc.SetColor(color.RGBA{156, 163, 175, 255})
	dc.SetLineWidth(1)
	dc.DrawLine(marginLeft, marginTop, marginLeft, marginTop+chartH)
	dc.DrawLine(marginLeft, marginTop+chartH, marginLeft+chartW, marginTop+chartH)
	dc.Stroke()

	n := len(series)
	barWidth := chartW / float64(n)

	// Draw bars and x-axis labels
	indigo := color.RGBA{79, 70, 229, 200}
	for i, p := range series {
		x := marginLeft + float64(i)*barWidth + barWidth*0.15
		bw := barWidth * 0.7
		barH := (p.Value / maxVal) * chartH
		y := marginTop + chartH - barH

		dc.SetColor(indigo)
		dc.DrawRectangle(x, y, bw, barH)
		dc.Fill()

		// X-axis label (show every Nth label to avoid overlap)
		labelInterval := 1
		if n > 14 {
			labelInterval = n / 7
		} else if n > 7 {
			labelInterval = 2
		}
		if i%labelInterval == 0 || i == n-1 {
			t, _ := time.Parse("2006-01-02", p.Date)
			label := t.Format("1/2")
			dc.SetColor(color.RGBA{107, 114, 128, 255})
			dc.DrawStringAnchored(label, x+bw/2, marginTop+chartH+15, 0.5, 0)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, dc.Image())
	return buf.Bytes(), nil
}

// generatePDF creates a PDF report with charts and detail tables
func (s *ReportService) generatePDF(ctx context.Context, reportID uuid.UUID, child *models.Child, startDate, endDate time.Time, filters []string, chartData map[string][]models.ChartDataPoint, logs *models.DailyLogPage) (driver string, storagePath string, size int64, err error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)

	// Medical disclaimer footer on every page (App Store guideline 1.4.1).
	pdf.SetFooterFunc(func() {
		pdf.SetY(-15)
		pdf.SetFont("Helvetica", "I", 7)
		pdf.SetTextColor(120, 113, 108)
		pdf.MultiCell(0, 3.2,
			"MyCareCompanion is a tracking and journaling tool, not a medical device. "+
				"The data and patterns in this report are observations of your logged entries, not medical advice. "+
				"Consult your child's healthcare provider for clinical decisions. In an emergency, call 911.",
			"", "C", false)
	})

	// Cover page
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 24)
	pdf.SetTextColor(79, 70, 229)
	pdf.Ln(30)
	pdf.CellFormat(0, 15, "MyCareCompanion", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 14)
	pdf.SetTextColor(107, 114, 128)
	pdf.CellFormat(0, 10, "Care Report", "", 1, "C", false, 0, "")
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(55, 65, 81)
	pdf.CellFormat(0, 12, child.FirstName, "", 1, "C", false, 0, "")
	pdf.Ln(5)
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetTextColor(107, 114, 128)
	pdf.CellFormat(0, 8, fmt.Sprintf("%s to %s", startDate.Format("January 2, 2006"), endDate.Format("January 2, 2006")), "", 1, "C", false, 0, "")
	pdf.Ln(5)
	pdf.CellFormat(0, 8, fmt.Sprintf("Generated: %s", time.Now().Format("January 2, 2006 3:04 PM")), "", 1, "C", false, 0, "")

	// Data sections - sorted for consistent ordering
	sortedCharts := make([]string, 0, len(chartData))
	for k := range chartData {
		sortedCharts = append(sortedCharts, k)
	}
	sort.Strings(sortedCharts)

	for _, chartTitle := range sortedCharts {
		series := chartData[chartTitle]

		pdf.AddPage()

		// Section header
		pdf.SetFont("Helvetica", "B", 16)
		pdf.SetTextColor(79, 70, 229)
		pdf.CellFormat(0, 10, chartTitle, "", 1, "L", false, 0, "")
		pdf.SetDrawColor(79, 70, 229)
		pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
		pdf.Ln(5)

		// Render and embed chart image
		chartPNG, err := renderChartImage(series, chartTitle, 700, 300)
		if err == nil && len(chartPNG) > 0 {
			reader := bytes.NewReader(chartPNG)
			imgName := fmt.Sprintf("chart_%s", chartTitle)
			pdf.RegisterImageOptionsReader(imgName, fpdf.ImageOptions{ImageType: "PNG"}, reader)
			pdf.ImageOptions(imgName, 10, pdf.GetY(), 190, 0, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
			pdf.Ln(85)
		}
	}

	// Detail tables for each filter
	filterSet := make(map[string]bool)
	for _, f := range filters {
		filterSet[f] = true
	}

	if filterSet["behavior"] && len(logs.BehaviorLogs) > 0 {
		addDetailPage(pdf, "Behavior Log Details", []string{"Date", "Mood", "Energy", "Meltdowns", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.BehaviorLogs {
				mood, energy := "--", "--"
				if l.MoodLevel != nil { mood = fmt.Sprintf("%d/10", *l.MoodLevel) }
				if l.EnergyLevel != nil { energy = fmt.Sprintf("%d/10", *l.EnergyLevel) }
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), mood, energy,
					fmt.Sprintf("%d", l.Meltdowns), truncate(l.Notes.String, 40),
				})
			}
			return rows
		})
	}

	if filterSet["sleep"] && len(logs.SleepLogs) > 0 {
		addDetailPage(pdf, "Sleep Log Details", []string{"Date", "Bed Time", "Wake Time", "Total Hrs", "Quality"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.SleepLogs {
				mins := 0; if l.TotalSleepMinutes != nil { mins = *l.TotalSleepMinutes }
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), l.Bedtime.String, l.WakeTime.String,
					fmt.Sprintf("%.1f", float64(mins)/60), l.SleepQuality.String,
				})
			}
			return rows
		})
	}

	if filterSet["diet"] && len(logs.DietLogs) > 0 {
		addDetailPage(pdf, "Diet Log Details", []string{"Date", "Meal", "Foods", "Appetite", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.DietLogs {
				foods := strings.Join([]string(l.FoodsEaten), ", ")
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), l.MealType.String,
					truncate(foods, 30), l.AppetiteLevel.String, truncate(l.Notes.String, 25),
				})
			}
			return rows
		})
	}

	if filterSet["medication"] && len(logs.MedicationLogs) > 0 {
		addDetailPage(pdf, "Medication Log Details", []string{"Date", "Status", "Time", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.MedicationLogs {
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), string(l.Status),
					l.ActualTime.String, truncate(l.Notes.String, 40),
				})
			}
			return rows
		})
	}

	if filterSet["bowel"] && len(logs.BowelLogs) > 0 {
		addDetailPage(pdf, "Bowel Log Details", []string{"Date", "Bristol", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.BowelLogs {
				bristol := "--"; if l.BristolScale != nil { bristol = fmt.Sprintf("%d", *l.BristolScale) }
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), bristol, truncate(l.Notes.String, 50),
				})
			}
			return rows
		})
	}

	if filterSet["sensory"] && len(logs.SensoryLogs) > 0 {
		addDetailPage(pdf, "Sensory Log Details", []string{"Date", "Regulation", "Triggers", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.SensoryLogs {
				reg := "--"; if l.OverallRegulation != nil { reg = fmt.Sprintf("%d/5", *l.OverallRegulation) }
				triggers := strings.Join([]string(l.OverloadTriggers), ", ")
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), reg, truncate(triggers, 30), truncate(l.Notes.String, 30),
				})
			}
			return rows
		})
	}

	if filterSet["therapy"] && len(logs.TherapyLogs) > 0 {
		addDetailPage(pdf, "Therapy Log Details", []string{"Date", "Type", "Duration", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.TherapyLogs {
				dur := "--"; if l.DurationMinutes != nil { dur = fmt.Sprintf("%d min", *l.DurationMinutes) }
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), l.TherapyType.String, dur, truncate(l.ProgressNotes.String, 35),
				})
			}
			return rows
		})
	}

	if filterSet["seizure"] && len(logs.SeizureLogs) > 0 {
		addDetailPage(pdf, "Seizure Log Details", []string{"Date", "Type", "Duration", "Notes"}, func() [][]string {
			var rows [][]string
			for _, l := range logs.SeizureLogs {
				dur := "--"; if l.DurationSeconds != nil { dur = fmt.Sprintf("%d sec", *l.DurationSeconds) }
				rows = append(rows, []string{
					l.LogDate.Format("01/02"), l.SeizureType.String, dur, truncate(l.Notes.String, 40),
				})
			}
			return rows
		})
	}

	// Render to a temp file, then hand the bytes to BlobStorage. Temp is
	// removed on the way out — never persisted on the EC2 instance.
	tmp, err := os.CreateTemp("", "report-*.pdf")
	if err != nil {
		return "", "", 0, fmt.Errorf("create temp PDF: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()
	if err := pdf.Output(tmp); err != nil {
		return "", "", 0, fmt.Errorf("write PDF: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, fmt.Errorf("seek temp PDF: %w", err)
	}
	info, err := tmp.Stat()
	if err != nil {
		return "", "", 0, err
	}
	size = info.Size()

	filename := fmt.Sprintf("%s.pdf", uuid.New().String())
	storagePath, _, err = s.storage.Save(ctx, reportID.String(), filename, "application/pdf", tmp)
	if err != nil {
		// Best-effort cleanup of any partial object the driver may have
		// left behind mid-upload. S3 multipart uploads can leave an
		// orphan object if Save errored after the first part was
		// committed; localfs drivers may have a half-written file.
		// Both are storage cost / quota leaks if not cleaned up.
		if storagePath != "" {
			if delErr := s.storage.Delete(context.Background(), storagePath); delErr != nil {
				log.Printf("[REPORT] partial-upload cleanup failed for %s: %v (original upload error: %v)", storagePath, delErr, err)
			}
		}
		return "", "", 0, fmt.Errorf("upload PDF: %w", err)
	}
	return s.storage.Driver(), storagePath, size, nil
}

// addDetailPage adds a detail table page to the PDF
func addDetailPage(pdf *fpdf.Fpdf, title string, headers []string, getRows func() [][]string) {
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(55, 65, 81)
	pdf.CellFormat(0, 10, title, "", 1, "L", false, 0, "")
	pdf.Ln(3)

	// Table header
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(243, 244, 246)
	pdf.SetTextColor(55, 65, 81)
	colWidth := 190.0 / float64(len(headers))
	for _, h := range headers {
		pdf.CellFormat(colWidth, 7, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	// Table rows
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(75, 85, 99)
	rows := getRows()
	for _, row := range rows {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetFillColor(243, 244, 246)
			pdf.SetTextColor(55, 65, 81)
			for _, h := range headers {
				pdf.CellFormat(colWidth, 7, h, "1", 0, "C", true, 0, "")
			}
			pdf.Ln(-1)
			pdf.SetFont("Helvetica", "", 8)
			pdf.SetTextColor(75, 85, 99)
		}
		for _, cell := range row {
			pdf.CellFormat(colWidth, 6, cell, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
