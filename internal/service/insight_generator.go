package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// InsightGenerator runs periodically to analyze data trends and create alerts
type InsightGenerator struct {
	alertService *AlertService
	logRepo      repository.LogRepository
	medRepo      repository.MedicationRepository
	alertRepo    repository.AlertRepository
	db           *sql.DB
	aiService    *AIInsightService
	lastAIRun    time.Time
	aiRunHour    int
}

// NewInsightGenerator creates a new insight generator
func NewInsightGenerator(
	alertService *AlertService,
	logRepo repository.LogRepository,
	medRepo repository.MedicationRepository,
	alertRepo repository.AlertRepository,
	db *sql.DB,
	aiService *AIInsightService,
	aiRunHour int,
) *InsightGenerator {
	return &InsightGenerator{
		alertService: alertService,
		logRepo:      logRepo,
		medRepo:      medRepo,
		alertRepo:    alertRepo,
		db:           db,
		aiService:    aiService,
		aiRunHour:    aiRunHour,
	}
}

// Start begins the insight generation loop
func (g *InsightGenerator) Start(ctx context.Context) {
	log.Println("Insight generator started")
	// Run immediately on startup
	g.generateAllInsights(ctx)

	// Also run AI analysis immediately on first startup if enabled
	if g.aiService != nil {
		log.Println("AI Insight generator enabled — running initial analysis")
		g.runAIAnalysis(ctx)
	}

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Insight generator stopped")
			return
		case <-ticker.C:
			g.generateAllInsights(ctx)

			// Run AI analysis daily at configured hour
			if g.shouldRunAI() {
				g.runAIAnalysis(ctx)
			}
		}
	}
}

// shouldRunAI checks if it's time for the daily AI analysis
func (g *InsightGenerator) shouldRunAI() bool {
	if g.aiService == nil {
		return false
	}
	now := time.Now()
	// Run at the configured hour, only if we haven't already run today
	if now.Hour() == g.aiRunHour && now.Format("2006-01-02") != g.lastAIRun.Format("2006-01-02") {
		return true
	}
	return false
}

// runAIAnalysis runs Claude AI analysis for all active children
func (g *InsightGenerator) runAIAnalysis(ctx context.Context) {
	children, err := g.getAllActiveChildren(ctx)
	if err != nil {
		log.Printf("AI Insights: failed to get children: %v", err)
		return
	}

	log.Printf("AI Insights: starting analysis for %d children", len(children))
	g.lastAIRun = time.Now()

	for _, child := range children {
		if err := g.aiService.AnalyzeChild(ctx, child); err != nil {
			log.Printf("AI Insights: error analyzing %s: %v", child.FirstName, err)
		}
	}

	log.Println("AI Insights: daily analysis complete")
}

func (g *InsightGenerator) generateAllInsights(ctx context.Context) {
	children, err := g.getAllActiveChildren(ctx)
	if err != nil {
		log.Printf("Insight generator: failed to get children: %v", err)
		return
	}

	for _, child := range children {
		g.analyzeChild(ctx, child)
	}
}

func (g *InsightGenerator) getAllActiveChildren(ctx context.Context) ([]models.Child, error) {
	rows, err := g.db.QueryContext(ctx, "SELECT id, family_id, first_name FROM children WHERE is_active = true")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var children []models.Child
	for rows.Next() {
		var c models.Child
		if err := rows.Scan(&c.ID, &c.FamilyID, &c.FirstName); err != nil {
			continue
		}
		children = append(children, c)
	}
	return children, rows.Err()
}

func (g *InsightGenerator) analyzeChild(ctx context.Context, child models.Child) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	threeDaysAgo := today.AddDate(0, 0, -3)
	sevenDaysAgo := today.AddDate(0, 0, -7)

	// Get recent logs
	logs, err := g.logRepo.GetLogsForDateRange(ctx, child.ID, threeDaysAgo, today)
	if err != nil {
		return
	}

	weekLogs, err := g.logRepo.GetLogsForDateRange(ctx, child.ID, sevenDaysAgo, today)
	if err != nil {
		weekLogs = logs // fallback
	}

	// Check mood trends
	g.checkMoodTrend(ctx, child, logs)

	// Check sleep deficit
	g.checkSleepDeficit(ctx, child, logs)

	// Check meltdown frequency
	g.checkMeltdownFrequency(ctx, child, logs)

	// Check medication adherence (weekly)
	g.checkMedicationAdherence(ctx, child, weekLogs)

	// Check missed medication streak
	g.checkMissedMedStreak(ctx, child, logs)
}

func (g *InsightGenerator) checkMoodTrend(ctx context.Context, child models.Child, logs *models.DailyLogPage) {
	if len(logs.BehaviorLogs) < 3 {
		return
	}

	dayMoods := make(map[string][]int)
	for _, bl := range logs.BehaviorLogs {
		if bl.MoodLevel == nil {
			continue
		}
		d := bl.LogDate.Format("2006-01-02")
		dayMoods[d] = append(dayMoods[d], *bl.MoodLevel)
	}

	if len(dayMoods) < 2 {
		return
	}

	type dayAvg struct {
		date string
		avg  float64
	}
	var avgs []dayAvg
	for d, moods := range dayMoods {
		sum := 0
		for _, m := range moods {
			sum += m
		}
		avgs = append(avgs, dayAvg{d, float64(sum) / float64(len(moods))})
	}

	for i := 0; i < len(avgs)-1; i++ {
		for j := i + 1; j < len(avgs); j++ {
			if avgs[i].date > avgs[j].date {
				avgs[i], avgs[j] = avgs[j], avgs[i]
			}
		}
	}

	if len(avgs) >= 2 {
		first := avgs[0].avg
		last := avgs[len(avgs)-1].avg
		diff := last - first

		if diff <= -1.5 {
			g.createAlertIfNew(ctx, child, "behavior_change", models.AlertSeverityWarning,
				fmt.Sprintf("Declining mood trend for %s", child.FirstName),
				fmt.Sprintf("Average mood has dropped from %.1f to %.1f over the past %d days. This may indicate increased stress or discomfort.",
					first, last, len(avgs)))
		} else if diff >= 1.5 {
			g.createAlertIfNew(ctx, child, "behavior_change", models.AlertSeverityInfo,
				fmt.Sprintf("Improving mood trend for %s", child.FirstName),
				fmt.Sprintf("Average mood has improved from %.1f to %.1f over the past %d days. Keep up the great work!",
					first, last, len(avgs)))
		}
	}
}

func (g *InsightGenerator) checkSleepDeficit(ctx context.Context, child models.Child, logs *models.DailyLogPage) {
	if len(logs.SleepLogs) < 2 {
		return
	}

	totalMinutes := 0
	count := 0
	for _, sl := range logs.SleepLogs {
		if sl.TotalSleepMinutes != nil && *sl.TotalSleepMinutes > 0 {
			totalMinutes += *sl.TotalSleepMinutes
			count++
		}
	}

	if count == 0 {
		return
	}

	avgHours := float64(totalMinutes) / float64(count) / 60.0
	if avgHours < 8.0 {
		g.createAlertIfNew(ctx, child, "sleep_pattern", models.AlertSeverityWarning,
			fmt.Sprintf("Sleep deficit detected for %s", child.FirstName),
			fmt.Sprintf("Average sleep over the past %d nights is only %.1f hours. Children typically need 9-11 hours. Consider reviewing bedtime routine.",
				count, avgHours))
	}
}

func (g *InsightGenerator) checkMeltdownFrequency(ctx context.Context, child models.Child, logs *models.DailyLogPage) {
	totalMeltdowns := 0
	for _, bl := range logs.BehaviorLogs {
		totalMeltdowns += bl.Meltdowns
	}

	if totalMeltdowns >= 3 {
		g.createAlertIfNew(ctx, child, "behavior_change", models.AlertSeverityWarning,
			fmt.Sprintf("Elevated meltdown frequency for %s", child.FirstName),
			fmt.Sprintf("%d meltdowns recorded in the past 3 days. Look for environmental triggers or schedule changes that may be contributing.",
				totalMeltdowns))
	}
}

func (g *InsightGenerator) checkMedicationAdherence(ctx context.Context, child models.Child, logs *models.DailyLogPage) {
	if len(logs.MedicationLogs) == 0 {
		return
	}

	taken := 0
	total := 0
	for _, ml := range logs.MedicationLogs {
		total++
		if ml.Status == "taken" {
			taken++
		}
	}

	if total == 0 {
		return
	}

	adherenceRate := float64(taken) / float64(total) * 100

	if adherenceRate >= 90 {
		g.createAlertIfNew(ctx, child, "medication_adherence", models.AlertSeverityInfo,
			fmt.Sprintf("Great medication adherence for %s!", child.FirstName),
			fmt.Sprintf("%.0f%% of medications taken this week (%d of %d doses). Excellent consistency!",
				adherenceRate, taken, total))
	}
}

func (g *InsightGenerator) checkMissedMedStreak(ctx context.Context, child models.Child, logs *models.DailyLogPage) {
	missedByDate := make(map[string]int)
	for _, ml := range logs.MedicationLogs {
		if ml.Status == "missed" {
			d := ml.LogDate.Format("2006-01-02")
			missedByDate[d]++
		}
	}

	consecutiveDays := 0
	now := time.Now()
	for i := 0; i < 3; i++ {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		if missedByDate[d] > 0 {
			consecutiveDays++
		} else {
			break
		}
	}

	if consecutiveDays >= 2 {
		g.createAlertIfNew(ctx, child, "medication_adherence", models.AlertSeverityWarning,
			fmt.Sprintf("Missed medications %d days in a row for %s", consecutiveDays, child.FirstName),
			fmt.Sprintf("Medications have been missed for %d consecutive days. Consistent medication timing is important for effectiveness.",
				consecutiveDays))
	}
}

func (g *InsightGenerator) createAlertIfNew(ctx context.Context, child models.Child, alertType string, severity models.AlertSeverity, title, description string) {
	since := time.Now().Add(-24 * time.Hour)
	existing, err := g.alertRepo.GetByChildIDAndTypeSince(ctx, child.ID, alertType, since)
	if err == nil && len(existing) > 0 {
		return
	}

	alert := &models.Alert{
		ChildID:     child.ID,
		FamilyID:    child.FamilyID,
		AlertType:   alertType,
		Severity:    severity,
		Status:      models.AlertStatusActive,
		Title:       title,
		Description: description,
		SourceType:  models.CorrelationTypeAutomatic,
	}

	if err := g.alertService.Create(ctx, alert); err != nil {
		log.Printf("Insight generator: failed to create alert for %s: %v", child.FirstName, err)
	}
}
