package service

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// ReportScheduler runs scheduled reports on a timer
type ReportScheduler struct {
	reportService *ReportService
}

// NewReportScheduler creates a new report scheduler
func NewReportScheduler(reportService *ReportService) *ReportScheduler {
	return &ReportScheduler{reportService: reportService}
}

// Start begins the scheduler loop, checking for due reports every minute
func (s *ReportScheduler) Start(ctx context.Context) {
	log.Println("Report scheduler started")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Report scheduler stopped")
			return
		case <-ticker.C:
			s.checkDueReports(ctx)
		}
	}
}

func (s *ReportScheduler) checkDueReports(ctx context.Context) {
	dueReports, err := s.reportService.reportRepo.GetDueScheduledReports(ctx)
	if err != nil {
		log.Printf("Report scheduler: failed to get due reports: %v", err)
		return
	}

	for _, sr := range dueReports {
		s.runScheduledReport(ctx, sr)
	}
}

func (s *ReportScheduler) runScheduledReport(ctx context.Context, sr models.ScheduledReport) {
	log.Printf("Report scheduler: running scheduled report %s (frequency: %s, child: %s)", sr.ID, sr.Frequency, sr.ChildID)

	// Compute date range based on frequency
	now := time.Now()
	var periodType string
	switch sr.Frequency {
	case "daily":
		periodType = "day"
	case "weekly":
		periodType = "week"
	case "monthly":
		periodType = "month"
	default:
		periodType = "week"
	}

	req := &models.GenerateReportRequest{
		PeriodType:  periodType,
		DataFilters: []string(sr.DataFilters),
	}

	report, err := s.reportService.GenerateReport(ctx, sr.ChildID, sr.FamilyID, sr.CreatedBy, req)
	if err != nil {
		log.Printf("Report scheduler: failed to generate report for schedule %s: %v", sr.ID, err)
		return
	}

	// Share with recipients via chat
	for _, recipientID := range sr.Recipients {
		if err := s.reportService.ShareViaChat(ctx, report, sr.CreatedBy, recipientID); err != nil {
			log.Printf("Report scheduler: failed to share report with %s: %v", recipientID, err)
		}
	}

	// Compute next run time
	nextRun := computeNextRun(sr.Frequency, now)
	if err := s.reportService.reportRepo.UpdateScheduledLastRun(ctx, sr.ID, nextRun); err != nil {
		log.Printf("Report scheduler: failed to update last run for %s: %v", sr.ID, err)
	}

	log.Printf("Report scheduler: completed report %s, next run at %s", sr.ID, nextRun.Format(time.RFC3339))
}

// computeNextRun calculates the next run time based on frequency
func computeNextRun(frequency string, from time.Time) time.Time {
	// Run at 7:00 AM the next cycle
	next := time.Date(from.Year(), from.Month(), from.Day(), 7, 0, 0, 0, from.Location())

	switch frequency {
	case "daily":
		next = next.AddDate(0, 0, 1)
	case "weekly":
		next = next.AddDate(0, 0, 7)
	case "monthly":
		next = next.AddDate(0, 1, 0)
	}

	return next
}

// ComputeFirstRun calculates the first run time for a new scheduled report
func ComputeFirstRun(frequency string) time.Time {
	return computeNextRun(frequency, time.Now())
}

// ShareViaChat shares a report PDF as a chat message
func (s *ReportService) ShareViaChat(ctx context.Context, report *models.Report, senderID, recipientID uuid.UUID) error {
	if !report.FilePath.Valid {
		return fmt.Errorf("report has no PDF file")
	}

	// Find or create a thread between sender and recipient
	threads, err := s.chatRepo.GetThreadsByFamily(ctx, report.FamilyID)
	if err != nil {
		return err
	}

	var threadID uuid.UUID
	for _, t := range threads {
		participants, _ := s.chatRepo.GetParticipants(ctx, t.ID)
		if len(participants) == 2 {
			hasS, hasR := false, false
			for _, p := range participants {
				if p.UserID == senderID { hasS = true }
				if p.UserID == recipientID { hasR = true }
			}
			if hasS && hasR {
				threadID = t.ID
				break
			}
		}
	}

	if threadID == uuid.Nil {
		// Create a new thread
		thread := &models.ChatThread{
			FamilyID:  report.FamilyID,
			CreatedBy: senderID,
		}
		if err := s.chatRepo.CreateThread(ctx, thread); err != nil {
			return err
		}
		threadID = thread.ID
		s.chatRepo.AddParticipant(ctx, threadID, senderID, "parent")
		s.chatRepo.AddParticipant(ctx, threadID, recipientID, "parent")
	}

	// Create message with attachment
	msg := &models.ChatMessage{
		ThreadID:    threadID,
		SenderID:    senderID,
		MessageText: fmt.Sprintf("Report: %s", report.Title),
		Attachments: models.Attachments{
			{
				Filename:    report.Title + ".pdf",
				StoredName:  filepath.Base(report.FilePath.String),
				ContentType: "application/pdf",
				URL:         "/api/reports/files/" + filepath.Base(report.FilePath.String),
			},
		},
	}

	return s.chatRepo.CreateMessage(ctx, msg)
}
