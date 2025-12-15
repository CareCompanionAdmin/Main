package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrAlertNotFound = errors.New("alert not found")
)

type AlertService struct {
	alertRepo repository.AlertRepository
	childRepo repository.ChildRepository
}

func NewAlertService(alertRepo repository.AlertRepository, childRepo repository.ChildRepository) *AlertService {
	return &AlertService{
		alertRepo: alertRepo,
		childRepo: childRepo,
	}
}

func (s *AlertService) Create(ctx context.Context, alert *models.Alert) error {
	return s.alertRepo.Create(ctx, alert)
}

func (s *AlertService) GetByID(ctx context.Context, id uuid.UUID) (*models.Alert, error) {
	alert, err := s.alertRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if alert == nil {
		return nil, ErrAlertNotFound
	}
	return alert, nil
}

func (s *AlertService) GetByChildID(ctx context.Context, childID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error) {
	return s.alertRepo.GetByChildID(ctx, childID, status)
}

func (s *AlertService) GetActiveAlerts(ctx context.Context, childID uuid.UUID) ([]models.Alert, error) {
	status := models.AlertStatusActive
	return s.alertRepo.GetByChildID(ctx, childID, &status)
}

func (s *AlertService) GetByFamilyID(ctx context.Context, familyID uuid.UUID, status *models.AlertStatus) ([]models.Alert, error) {
	return s.alertRepo.GetByFamilyID(ctx, familyID, status)
}

func (s *AlertService) Update(ctx context.Context, alert *models.Alert) error {
	return s.alertRepo.Update(ctx, alert)
}

func (s *AlertService) Acknowledge(ctx context.Context, alertID, userID uuid.UUID) error {
	return s.alertRepo.Acknowledge(ctx, alertID, userID)
}

func (s *AlertService) Resolve(ctx context.Context, alertID, userID uuid.UUID) error {
	return s.alertRepo.Resolve(ctx, alertID, userID)
}

func (s *AlertService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.alertRepo.Delete(ctx, id)
}

func (s *AlertService) CreateFeedback(ctx context.Context, alertID, userID uuid.UUID, req *models.AlertFeedbackRequest) (*models.AlertFeedback, error) {
	feedback := &models.AlertFeedback{
		AlertID:    alertID,
		UserID:     userID,
		WasHelpful: req.WasHelpful,
	}
	feedback.FeedbackText.String = req.FeedbackText
	feedback.FeedbackText.Valid = req.FeedbackText != ""
	feedback.ActionTaken.String = req.ActionTaken
	feedback.ActionTaken.Valid = req.ActionTaken != ""

	if err := s.alertRepo.CreateFeedback(ctx, feedback); err != nil {
		return nil, err
	}
	return feedback, nil
}

func (s *AlertService) GetFeedback(ctx context.Context, alertID uuid.UUID) ([]models.AlertFeedback, error) {
	return s.alertRepo.GetFeedback(ctx, alertID)
}

func (s *AlertService) GetStats(ctx context.Context, childID uuid.UUID) (*models.AlertStats, error) {
	return s.alertRepo.GetStats(ctx, childID)
}

func (s *AlertService) GetAlertsPage(ctx context.Context, childID uuid.UUID) (*models.AlertsPage, error) {
	return s.alertRepo.GetAlertsPage(ctx, childID)
}

// Alert creation helpers
func (s *AlertService) CreateMedicationAdherenceAlert(ctx context.Context, childID, familyID uuid.UUID, adherenceRate float64) error {
	severity := models.AlertSeverityWarning
	if adherenceRate < 50 {
		severity = models.AlertSeverityCritical
	}

	alert := &models.Alert{
		ChildID:     childID,
		FamilyID:    familyID,
		AlertType:   models.AlertTypeMedicationAdherence,
		Severity:    severity,
		Title:       "Low Medication Adherence",
		Description: "Medication adherence has dropped below acceptable levels. Please review medication logs.",
		Data: models.JSONB{
			"adherence_rate": adherenceRate,
		},
	}

	return s.alertRepo.Create(ctx, alert)
}

func (s *AlertService) CreateBehaviorChangeAlert(ctx context.Context, childID, familyID uuid.UUID, changeType string, details map[string]interface{}) error {
	alert := &models.Alert{
		ChildID:     childID,
		FamilyID:    familyID,
		AlertType:   models.AlertTypeBehaviorChange,
		Severity:    models.AlertSeverityWarning,
		Title:       "Significant Behavior Change Detected",
		Description: "A significant change in behavior patterns has been detected.",
		Data:        models.JSONB(details),
	}

	return s.alertRepo.Create(ctx, alert)
}

func (s *AlertService) CreatePatternDiscoveredAlert(ctx context.Context, childID, familyID uuid.UUID, pattern *models.FamilyPattern) error {
	var confidenceScore *float64
	cs := pattern.ConfidenceScore
	confidenceScore = &cs

	alert := &models.Alert{
		ChildID:         childID,
		FamilyID:        familyID,
		AlertType:       models.AlertTypePatternDiscovered,
		Severity:        models.AlertSeverityInfo,
		Title:           "New Pattern Discovered",
		Description:     pattern.Description.String,
		ConfidenceScore: confidenceScore,
		SourceType:      models.CorrelationTypeAutomatic,
		Data: models.JSONB{
			"pattern_id":           pattern.ID,
			"input_factor":         pattern.InputFactor,
			"output_factor":        pattern.OutputFactor,
			"correlation_strength": pattern.CorrelationStrength,
			"lag_hours":            pattern.LagHours,
		},
	}

	return s.alertRepo.Create(ctx, alert)
}

func (s *AlertService) CreateMissedLogAlert(ctx context.Context, childID, familyID uuid.UUID, logTypes []string) error {
	alert := &models.Alert{
		ChildID:     childID,
		FamilyID:    familyID,
		AlertType:   models.AlertTypeMissedLog,
		Severity:    models.AlertSeverityInfo,
		Title:       "Daily Logs Not Completed",
		Description: "Some daily logs have not been recorded today.",
		Data: models.JSONB{
			"missed_log_types": logTypes,
		},
	}

	return s.alertRepo.Create(ctx, alert)
}
