package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// AlertIntelligenceService handles smart alert generation and feedback learning
type AlertIntelligenceService struct {
	alertRepo       repository.AlertRepository
	correlationRepo repository.CorrelationRepository
	insightRepo     repository.InsightRepository
}

// NewAlertIntelligenceService creates a new alert intelligence service
func NewAlertIntelligenceService(
	alertRepo repository.AlertRepository,
	correlationRepo repository.CorrelationRepository,
	insightRepo repository.InsightRepository,
) *AlertIntelligenceService {
	return &AlertIntelligenceService{
		alertRepo:       alertRepo,
		correlationRepo: correlationRepo,
		insightRepo:     insightRepo,
	}
}

// ShouldGenerateAlert uses historical feedback to decide if an alert is worth generating
func (s *AlertIntelligenceService) ShouldGenerateAlert(ctx context.Context, childID uuid.UUID, alertType string, confidence float64) bool {
	// Get historical effectiveness for this alert type and child
	effectiveness, _ := s.GetAlertEffectiveness(ctx, childID, alertType)

	if effectiveness == nil {
		// No history - generate if confidence is high enough
		return confidence >= 0.6
	}

	// Adjust threshold based on historical effectiveness
	baseThreshold := 0.6
	adjustedThreshold := baseThreshold

	if effectiveness.EffectivenessScore < 0.3 {
		// Alerts of this type are often dismissed - raise threshold
		adjustedThreshold = 0.8
	} else if effectiveness.EffectivenessScore > 0.7 {
		// Alerts of this type are helpful - lower threshold
		adjustedThreshold = 0.5
	}

	return confidence >= adjustedThreshold
}

// GetAlertEffectiveness calculates how effective a certain alert type has been
func (s *AlertIntelligenceService) GetAlertEffectiveness(ctx context.Context, childID uuid.UUID, alertType string) (*models.AlertEffectiveness, error) {
	stats, err := s.alertRepo.GetStatsByType(ctx, childID, alertType)
	if err != nil {
		return nil, err
	}

	effectiveness := &models.AlertEffectiveness{
		AlertType:      alertType,
		TotalGenerated: stats.Total,
		Acknowledged:   stats.Acknowledged,
		HelpfulCount:   stats.HelpfulFeedback,
		DismissedCount: stats.Dismissed,
	}

	// Calculate effectiveness score
	if stats.Total > 0 {
		// Weight: acknowledged (0.3) + helpful feedback (0.5) - dismissed penalty (0.2)
		score := float64(stats.Acknowledged)*0.3/float64(stats.Total) +
			float64(stats.HelpfulFeedback)*0.5/float64(stats.Total) -
			float64(stats.Dismissed)*0.2/float64(stats.Total)
		effectiveness.EffectivenessScore = clamp(score, 0, 1)
	}

	return effectiveness, nil
}

// ProcessFeedback learns from user feedback on alerts
func (s *AlertIntelligenceService) ProcessFeedback(ctx context.Context, alertID, userID uuid.UUID, feedback *models.AlertFeedbackRequest) error {
	// Record feedback
	fb := &models.AlertFeedback{
		ID:           uuid.New(),
		AlertID:      alertID,
		UserID:       userID,
		WasHelpful:   feedback.WasHelpful,
		FeedbackText: models.NullString{NullString: toNullString(feedback.FeedbackText)},
		ActionTaken:  models.NullString{NullString: toNullString(feedback.ActionTaken)},
		CreatedAt:    time.Now(),
	}

	if err := s.alertRepo.CreateFeedback(ctx, fb); err != nil {
		return err
	}

	// Get the alert to find related insight/pattern
	alert, _ := s.alertRepo.GetByID(ctx, alertID)
	if alert == nil {
		return nil
	}

	// If feedback is positive and alert was based on a correlation, strengthen it
	if feedback.WasHelpful != nil && *feedback.WasHelpful {
		if alert.CorrelationID.Valid {
			s.correlationRepo.IncrementPatternValidation(ctx, alert.CorrelationID.UUID)
		}
	}

	return nil
}

// GenerateSmartAlert creates an alert only if it's likely to be useful
func (s *AlertIntelligenceService) GenerateSmartAlert(ctx context.Context, childID uuid.UUID, alertData models.AlertGenerationData) (*models.Alert, error) {
	// Check if we should generate this alert
	if !s.ShouldGenerateAlert(ctx, childID, alertData.Type, alertData.Confidence) {
		return nil, nil // Don't generate
	}

	// Check for duplicate/similar recent alerts
	if s.hasSimilarRecentAlert(ctx, childID, alertData) {
		return nil, nil // Avoid alert fatigue
	}

	alert := &models.Alert{
		ID:              uuid.New(),
		ChildID:         childID,
		FamilyID:        alertData.FamilyID,
		AlertType:       alertData.Type,
		Severity:        alertData.Severity,
		Status:          models.AlertStatusActive,
		Title:           alertData.Title,
		Description:     alertData.Description,
		Data:            alertData.Data,
		ConfidenceScore: &alertData.Confidence,
		CreatedAt:       time.Now(),
	}

	if alertData.CorrelationID != nil {
		alert.CorrelationID = models.NullUUID{UUID: *alertData.CorrelationID, Valid: true}
	}

	if err := s.alertRepo.Create(ctx, alert); err != nil {
		return nil, err
	}

	return alert, nil
}

// GetAlertEffectivenessReport returns effectiveness metrics for all alert types for a child
func (s *AlertIntelligenceService) GetAlertEffectivenessReport(ctx context.Context, childID uuid.UUID) ([]models.AlertEffectiveness, error) {
	// Common alert types to report on
	alertTypes := []string{
		"pattern_detected",
		"medication_reminder",
		"behavior_alert",
		"health_alert",
		"correlation_insight",
	}

	var report []models.AlertEffectiveness
	for _, alertType := range alertTypes {
		effectiveness, err := s.GetAlertEffectiveness(ctx, childID, alertType)
		if err != nil {
			continue
		}
		if effectiveness.TotalGenerated > 0 {
			report = append(report, *effectiveness)
		}
	}

	return report, nil
}

func (s *AlertIntelligenceService) hasSimilarRecentAlert(ctx context.Context, childID uuid.UUID, data models.AlertGenerationData) bool {
	// Check for alerts of same type in last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	recentAlerts, _ := s.alertRepo.GetByChildIDAndTypeSince(ctx, childID, data.Type, since)
	return len(recentAlerts) > 0
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
