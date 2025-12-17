package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

type TransparencyService struct {
	repo      *repository.TransparencyRepository
	alertRepo repository.AlertRepository
	childRepo repository.ChildRepository
}

func NewTransparencyService(
	repo *repository.TransparencyRepository,
	alertRepo repository.AlertRepository,
	childRepo repository.ChildRepository,
) *TransparencyService {
	return &TransparencyService{
		repo:      repo,
		alertRepo: alertRepo,
		childRepo: childRepo,
	}
}

// GetFullAlertAnalysis retrieves complete transparency data for an alert (Layer 3)
func (s *TransparencyService) GetFullAlertAnalysis(ctx context.Context, alertID, userID string) (*models.FullAlertAnalysis, error) {
	alertUUID, err := uuid.Parse(alertID)
	if err != nil {
		return nil, fmt.Errorf("invalid alert ID: %w", err)
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Get the alert first
	alert, err := s.alertRepo.GetByID(ctx, alertUUID)
	if err != nil {
		return nil, fmt.Errorf("alert not found: %w", err)
	}
	if alert == nil {
		return nil, fmt.Errorf("alert not found")
	}

	// Verify user has access
	hasAccess, err := s.alertRepo.UserHasAccess(ctx, alertUUID, userUUID)
	if err != nil || !hasAccess {
		return nil, errors.New("access denied")
	}

	// Get child name
	child, err := s.childRepo.GetByID(ctx, alert.ChildID)
	childName := ""
	if err == nil && child != nil {
		childName = child.FirstName
	}

	// Get analysis details
	analysisDetails, err := s.repo.GetAnalysisDetails(ctx, alertID)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get analysis details: %w", err)
	}

	// Get confidence factors
	factors, err := s.repo.GetConfidenceFactors(ctx, alertID)
	if err != nil {
		return nil, fmt.Errorf("failed to get confidence factors: %w", err)
	}

	// Enrich factors with related data
	for i := range factors {
		if factors[i].CitationID != nil {
			citation, _ := s.repo.GetCitation(ctx, *factors[i].CitationID)
			factors[i].Citation = citation
		}
		if factors[i].CohortID != nil {
			cohort, _ := s.repo.GetCohort(ctx, *factors[i].CohortID)
			factors[i].Cohort = cohort
		}
	}

	// Get cohort matching
	cohortMatching, err := s.repo.GetCohortMatching(ctx, alertID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cohort matching: %w", err)
	}

	return &models.FullAlertAnalysis{
		Alert:             alert,
		AnalysisDetails:   analysisDetails,
		ConfidenceFactors: factors,
		CohortMatching:    cohortMatching,
		ChildName:         childName,
	}, nil
}

// GetConfidenceBreakdown retrieves simplified confidence breakdown (Layer 2)
func (s *TransparencyService) GetConfidenceBreakdown(ctx context.Context, alertID, userID string) (*models.ConfidenceBreakdown, error) {
	alertUUID, err := uuid.Parse(alertID)
	if err != nil {
		return nil, fmt.Errorf("invalid alert ID: %w", err)
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Get the alert first
	alert, err := s.alertRepo.GetByID(ctx, alertUUID)
	if err != nil {
		return nil, fmt.Errorf("alert not found: %w", err)
	}
	if alert == nil {
		return nil, fmt.Errorf("alert not found")
	}

	// Verify user has access
	hasAccess, err := s.alertRepo.UserHasAccess(ctx, alertUUID, userUUID)
	if err != nil || !hasAccess {
		return nil, errors.New("access denied")
	}

	// Get confidence factors
	factors, err := s.repo.GetConfidenceFactors(ctx, alertID)
	if err != nil {
		return nil, fmt.Errorf("failed to get confidence factors: %w", err)
	}

	// Build summary
	breakdown := &models.ConfidenceBreakdown{
		OverallConfidence: 0,
		Factors:           make([]models.ConfidenceFactorSummary, 0, len(factors)),
	}

	if alert.ConfidenceScore != nil {
		breakdown.OverallConfidence = *alert.ConfidenceScore * 100
	}

	for _, f := range factors {
		// Prefer database values, fall back to derived values
		icon := getFactorIcon(f.FactorType)
		if f.Icon != nil && *f.Icon != "" {
			icon = *f.Icon
		}
		label := getFactorLabel(f.FactorType)
		if f.Label != nil && *f.Label != "" {
			label = *f.Label
		}

		breakdown.Factors = append(breakdown.Factors, models.ConfidenceFactorSummary{
			Icon:        icon,
			Label:       label,
			Percentage:  f.Contribution * 100,
			Description: f.Description,
			FactorType:  string(f.FactorType),
		})
	}

	return breakdown, nil
}

// RecordAlertFeedback records user feedback on an alert
func (s *TransparencyService) RecordAlertFeedback(ctx context.Context, alertID, userID string, req *models.AlertFeedbackRequest) error {
	alertUUID, err := uuid.Parse(alertID)
	if err != nil {
		return fmt.Errorf("invalid alert ID: %w", err)
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Verify user has access
	hasAccess, err := s.alertRepo.UserHasAccess(ctx, alertUUID, userUUID)
	if err != nil || !hasAccess {
		return errors.New("access denied")
	}

	wasHelpful := false
	if req.WasHelpful != nil {
		wasHelpful = *req.WasHelpful
	}
	return s.repo.CreateAlertFeedback(ctx, alertID, userID, wasHelpful, req.FeedbackText, req.ActionTaken)
}

// ExportAlert records an export of an alert
func (s *TransparencyService) ExportAlert(ctx context.Context, alertID, userID string, exportType models.ExportType, viewMode models.AnalysisView) (*models.AlertExport, error) {
	alertUUID, err := uuid.Parse(alertID)
	if err != nil {
		return nil, fmt.Errorf("invalid alert ID: %w", err)
	}
	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Verify user has access
	hasAccess, err := s.alertRepo.UserHasAccess(ctx, alertUUID, userUUID)
	if err != nil || !hasAccess {
		return nil, errors.New("access denied")
	}

	export := &models.AlertExport{
		AlertID:          alertID,
		ExportedByUserID: userID,
		ExportType:       exportType,
		ViewMode:         viewMode,
	}

	err = s.repo.CreateAlertExport(ctx, export)
	if err != nil {
		return nil, err
	}

	return export, nil
}

// GetPendingInterrogatives retrieves pending treatment change questions
func (s *TransparencyService) GetPendingInterrogatives(ctx context.Context, userID string) ([]models.PendingInterrogative, error) {
	changes, err := s.repo.GetPendingInterrogatives(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]models.PendingInterrogative, 0, len(changes))
	for _, tc := range changes {
		pending := models.PendingInterrogative{
			TreatmentChange: &tc,
		}

		// Get child name
		if childUUID, err := uuid.Parse(tc.ChildID); err == nil {
			if child, err := s.childRepo.GetByID(ctx, childUUID); err == nil && child != nil {
				pending.ChildName = child.FirstName
			}
		}

		// Get related alert if exists
		if tc.PotentiallyRelatedAlertID != nil {
			if alertUUID, err := uuid.Parse(*tc.PotentiallyRelatedAlertID); err == nil {
				alert, _ := s.alertRepo.GetByID(ctx, alertUUID)
				pending.RelatedAlert = alert
			}
		}

		result = append(result, pending)
	}

	return result, nil
}

// RespondToTreatmentChange records a response to a treatment change question
func (s *TransparencyService) RespondToTreatmentChange(ctx context.Context, treatmentChangeID, userID string, req *models.TreatmentChangePromptResponse) error {
	// Create response
	resp := &models.TreatmentChangeResponse{
		TreatmentChangeID: treatmentChangeID,
		RespondedByUserID: userID,
		ChangeSource:      req.ChangeSource,
		RelatedToAnalysis: req.RelatedToAnalysis,
	}

	if req.ProviderName != "" {
		resp.ProviderNameFreetext = &req.ProviderName
	}
	if req.Notes != "" {
		resp.Notes = &req.Notes
	}

	err := s.repo.CreateTreatmentChangeResponse(ctx, resp)
	if err != nil {
		return err
	}

	// Update treatment change status
	return s.repo.UpdateTreatmentChangeStatus(ctx, treatmentChangeID, models.InterrogativeStatusAnswered)
}

// GetUserInteractionPreferences retrieves user preferences
func (s *TransparencyService) GetUserInteractionPreferences(ctx context.Context, userID string) (*models.UserInteractionPreferences, error) {
	return s.repo.GetUserInteractionPreferences(ctx, userID)
}

// UpdateUserInteractionPreferences updates user preferences
func (s *TransparencyService) UpdateUserInteractionPreferences(ctx context.Context, prefs *models.UserInteractionPreferences) error {
	return s.repo.UpsertUserInteractionPreferences(ctx, prefs)
}

// Helper functions

func getFactorIcon(factorType models.FactorType) string {
	switch factorType {
	case models.FactorTypeGlobalMedical:
		return "📚"
	case models.FactorTypeFamilyHistory:
		return "📋"
	case models.FactorTypeCohortPattern:
		return "👥"
	case models.FactorTypeTemporalProximity:
		return "⏱️"
	case models.FactorTypeClinicalValidation:
		return "✅"
	default:
		return "📊"
	}
}

func getFactorLabel(factorType models.FactorType) string {
	switch factorType {
	case models.FactorTypeGlobalMedical:
		return "Medical Reference"
	case models.FactorTypeFamilyHistory:
		return "Your Child's History"
	case models.FactorTypeCohortPattern:
		return "Similar Families"
	case models.FactorTypeTemporalProximity:
		return "Timing Match"
	case models.FactorTypeClinicalValidation:
		return "Clinically Validated"
	case models.FactorTypeAmplitude:
		return "Change Magnitude"
	case models.FactorTypeMedicationCriticality:
		return "Medication Importance"
	default:
		return "Analysis Factor"
	}
}

// GetConfidenceLevel returns the confidence level category (for UI coloring)
func GetConfidenceLevel(score float64) string {
	if score >= 0.7 {
		return "high"
	} else if score >= 0.4 {
		return "medium"
	}
	return "low"
}

// GetConfidenceColor returns the color class for a confidence score
func GetConfidenceColor(score float64) string {
	if score >= 0.7 {
		return "green"
	} else if score >= 0.4 {
		return "yellow"
	}
	return "red"
}

// GetFilledDots returns the number of filled dots for confidence meter (out of 5)
func GetFilledDots(score float64) int {
	return int(score * 5)
}
