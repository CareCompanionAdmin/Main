package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ValidationService handles clinical validation with implicit tracking
type ValidationService struct {
	correlationRepo repository.CorrelationRepository
	insightRepo     repository.InsightRepository
	medicationRepo  repository.MedicationRepository
}

// NewValidationService creates a new validation service
func NewValidationService(
	correlationRepo repository.CorrelationRepository,
	insightRepo repository.InsightRepository,
	medicationRepo repository.MedicationRepository,
) *ValidationService {
	return &ValidationService{
		correlationRepo: correlationRepo,
		insightRepo:     insightRepo,
		medicationRepo:  medicationRepo,
	}
}

// DetectTreatmentChange checks if a medication change followed an insight/alert
// This creates implicit validation records when treatment changes correlate with insights
func (s *ValidationService) DetectTreatmentChange(ctx context.Context, childID uuid.UUID, change models.MedicationChange) error {
	// Look for recent insights/alerts that might have prompted this change
	lookbackPeriod := 7 * 24 * time.Hour // 7 days
	since := time.Now().Add(-lookbackPeriod)

	// Find related insights
	insights, err := s.insightRepo.GetByChildIDSince(ctx, childID, since)
	if err != nil {
		return err
	}

	for _, insight := range insights {
		if s.isChangeRelatedToInsight(change, insight) {
			// Create implicit validation record
			strength := 0.7 // Implicit = moderate strength
			validation := &models.ClinicalValidation{
				ID:       uuid.New(),
				ChildID:  childID,
				InsightID: models.NullUUID{
					UUID:  insight.ID,
					Valid: true,
				},
				ValidationType: models.NullString{
					NullString: sql.NullString{
						String: "implicit_treatment_change",
						Valid:  true,
					},
				},
				TreatmentChanged: true,
				TreatmentDescription: models.NullString{
					NullString: sql.NullString{
						String: change.Description,
						Valid:  true,
					},
				},
				ValidationStrength: &strength,
				CreatedAt:          time.Now(),
			}

			if err := s.correlationRepo.CreateValidation(ctx, validation); err != nil {
				continue // Don't fail on individual validation errors
			}

			// Update insight validation count
			s.insightRepo.IncrementValidation(ctx, insight.ID)
		}
	}

	// Also check patterns
	patterns, err := s.correlationRepo.GetPatterns(ctx, childID, true)
	if err != nil {
		return err
	}

	for _, pattern := range patterns {
		if s.isChangeRelatedToPattern(change, pattern) {
			strength := 0.7
			validation := &models.ClinicalValidation{
				ID:       uuid.New(),
				ChildID:  childID,
				PatternID: models.NullUUID{
					UUID:  pattern.ID,
					Valid: true,
				},
				ValidationType: models.NullString{
					NullString: sql.NullString{
						String: "implicit_treatment_change",
						Valid:  true,
					},
				},
				TreatmentChanged: true,
				TreatmentDescription: models.NullString{
					NullString: sql.NullString{
						String: change.Description,
						Valid:  true,
					},
				},
				ValidationStrength: &strength,
				CreatedAt:          time.Now(),
			}

			s.correlationRepo.CreateValidation(ctx, validation)
			s.correlationRepo.IncrementPatternValidation(ctx, pattern.ID)
		}
	}

	return nil
}

// RecordParentConfirmation records when a parent confirms an insight was helpful
func (s *ValidationService) RecordParentConfirmation(ctx context.Context, insightID, userID uuid.UUID, wasHelpful bool) error {
	strength := validationStrength(wasHelpful)
	now := time.Now()

	validation := &models.ClinicalValidation{
		ID: uuid.New(),
		InsightID: models.NullUUID{
			UUID:  insightID,
			Valid: true,
		},
		ValidationType: models.NullString{
			NullString: sql.NullString{
				String: "parent_confirmation",
				Valid:  true,
			},
		},
		ParentConfirmed: &wasHelpful,
		ParentConfirmedAt: models.NullTime{
			NullTime: sql.NullTime{
				Time:  now,
				Valid: true,
			},
		},
		ValidationStrength: &strength,
		CreatedAt:          now,
	}

	// Get insight to find child ID
	insight, err := s.insightRepo.GetByID(ctx, insightID)
	if err != nil {
		return err
	}
	if insight != nil && insight.ChildID != nil {
		validation.ChildID = *insight.ChildID
	}

	if err := s.correlationRepo.CreateValidation(ctx, validation); err != nil {
		return err
	}

	// Update insight based on feedback
	if wasHelpful {
		s.insightRepo.IncrementValidation(ctx, insightID)
	}

	return nil
}

// RecordProviderValidation records when a medical provider validates an insight
func (s *ValidationService) RecordProviderValidation(ctx context.Context, insightID, providerUserID uuid.UUID, input models.ProviderValidationInput) error {
	strength := 1.0 // Provider validation = strongest

	validation := &models.ClinicalValidation{
		ID: uuid.New(),
		InsightID: models.NullUUID{
			UUID:  insightID,
			Valid: true,
		},
		ProviderUserID: models.NullUUID{
			UUID:  providerUserID,
			Valid: true,
		},
		ValidationType: models.NullString{
			NullString: sql.NullString{
				String: "provider_validation",
				Valid:  true,
			},
		},
		TreatmentChanged: input.TreatmentChanged,
		TreatmentDescription: models.NullString{
			NullString: sql.NullString{
				String: input.TreatmentDescription,
				Valid:  input.TreatmentDescription != "",
			},
		},
		ValidationStrength: &strength,
		CreatedAt:          time.Now(),
	}

	// Get insight to find child ID
	insight, err := s.insightRepo.GetByID(ctx, insightID)
	if err != nil {
		return err
	}
	if insight != nil && insight.ChildID != nil {
		validation.ChildID = *insight.ChildID
	}

	if err := s.correlationRepo.CreateValidation(ctx, validation); err != nil {
		return err
	}

	// Mark insight as clinically validated
	s.insightRepo.SetClinicallyValidated(ctx, insightID)

	return nil
}

// GetValidationStats returns validation statistics for a child
func (s *ValidationService) GetValidationStats(ctx context.Context, childID uuid.UUID) (*models.ValidationStats, error) {
	return s.correlationRepo.GetValidationStats(ctx, childID)
}

// Helper methods

func (s *ValidationService) isChangeRelatedToInsight(change models.MedicationChange, insight models.Insight) bool {
	// Check if the insight mentioned the medication
	if containsIgnoreCase(insight.SimpleDescription, change.MedicationName) {
		return true
	}
	if insight.DetailedDescription.Valid && containsIgnoreCase(insight.DetailedDescription.String, change.MedicationName) {
		return true
	}
	return false
}

func (s *ValidationService) isChangeRelatedToPattern(change models.MedicationChange, pattern models.FamilyPattern) bool {
	if containsIgnoreCase(pattern.InputFactor, change.MedicationName) {
		return true
	}
	if containsIgnoreCase(pattern.OutputFactor, change.MedicationName) {
		return true
	}
	return false
}

func validationStrength(wasHelpful bool) float64 {
	if wasHelpful {
		return 0.8
	}
	return -0.3 // Negative feedback reduces confidence
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
