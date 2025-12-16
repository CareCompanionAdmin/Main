package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

type InsightService struct {
	insightRepo     repository.InsightRepository
	correlationRepo repository.CorrelationRepository
	childRepo       repository.ChildRepository
}

func NewInsightService(
	insightRepo repository.InsightRepository,
	correlationRepo repository.CorrelationRepository,
	childRepo repository.ChildRepository,
) *InsightService {
	return &InsightService{
		insightRepo:     insightRepo,
		correlationRepo: correlationRepo,
		childRepo:       childRepo,
	}
}

// GetInsightsForChild returns all insights for a child, organized by tier
func (s *InsightService) GetInsightsForChild(ctx context.Context, childID uuid.UUID) (*InsightsResponse, error) {
	// Get child info
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, fmt.Errorf("child not found")
	}

	response := &InsightsResponse{
		Child: child,
	}

	// Get Tier 3: Personal patterns (child-specific)
	personalInsights, err := s.insightRepo.GetByChildID(ctx, childID, tierPtr(models.TierFamilySpecific), true)
	if err != nil {
		return nil, err
	}
	response.PersonalInsights = personalInsights

	// Get Tier 2: Cohort patterns (would come from cohort service - placeholder for now)
	// In the future, this would call the cohort service to get matching cohort insights
	cohortInsights, err := s.insightRepo.GetByChildID(ctx, childID, tierPtr(models.TierCohortPattern), true)
	if err != nil {
		return nil, err
	}
	response.CohortInsights = cohortInsights

	// Get Tier 1: Medical knowledge (global insights related to child's medications/conditions)
	// For now, get all global insights. In future, filter by child's medications/conditions
	globalInsights, err := s.insightRepo.GetGlobalInsights(ctx, "")
	if err != nil {
		return nil, err
	}
	response.MedicalInsights = globalInsights

	return response, nil
}

// CreateInsightFromPattern creates a Tier 3 insight from a detected pattern
func (s *InsightService) CreateInsightFromPattern(ctx context.Context, pattern *models.FamilyPattern) (*models.Insight, error) {
	// Get child for family_id
	child, err := s.childRepo.GetByID(ctx, pattern.ChildID)
	if err != nil || child == nil {
		return nil, err
	}

	insight := &models.Insight{
		ChildID:  &pattern.ChildID,
		FamilyID: &child.FamilyID,
		Tier:     models.TierFamilySpecific,
		Category: categorizePattern(pattern),

		Title:               generatePatternTitle(pattern),
		SimpleDescription:   generateSimpleDescription(pattern),
		DetailedDescription: models.NullString{NullString: sql.NullString{Valid: true, String: generateDetailedDescription(pattern)}},

		ConfidenceScore:     &pattern.ConfidenceScore,
		SampleSize:          &pattern.SampleSize,
		CorrelationStrength: &pattern.CorrelationStrength,

		InputFactors:   models.StringArray{pattern.InputFactor},
		OutputFactors:  models.StringArray{pattern.OutputFactor},
		LagHours:       &pattern.LagHours,
		DataPointCount: &pattern.SampleSize,

		PatternID: models.NullUUID{UUID: pattern.ID, Valid: true},
		IsActive:  true,
	}

	if pattern.FirstDetectedAt.Valid {
		insight.DateRangeStart = pattern.FirstDetectedAt
	}
	if pattern.LastConfirmedAt.Valid {
		insight.DateRangeEnd = pattern.LastConfirmedAt
	}

	// Upsert to handle pattern updates
	err = s.insightRepo.Upsert(ctx, insight)
	if err != nil {
		return nil, err
	}

	return insight, nil
}

// CreateMedicalInsight creates a Tier 1 global medical insight
func (s *InsightService) CreateMedicalInsight(ctx context.Context, req *CreateMedicalInsightRequest) (*models.Insight, error) {
	insight := &models.Insight{
		Tier:                models.TierGlobalMedical,
		Category:            req.Category,
		Title:               req.Title,
		SimpleDescription:   req.SimpleDescription,
		DetailedDescription: models.NullString{NullString: sql.NullString{Valid: req.DetailedDescription != "", String: req.DetailedDescription}},
		ConfidenceScore:     float64Ptr(1.0), // Tier 1 is established knowledge
		SourceIDs:           req.SourceIDs,
		IsActive:            true,
	}

	err := s.insightRepo.Create(ctx, insight)
	if err != nil {
		return nil, err
	}

	return insight, nil
}

// GetTopInsights returns the most significant insights for a child
func (s *InsightService) GetTopInsights(ctx context.Context, childID uuid.UUID, limit int) ([]models.Insight, error) {
	insights, err := s.insightRepo.GetByChildID(ctx, childID, nil, true)
	if err != nil {
		return nil, err
	}

	// Return top N by confidence
	if limit > 0 && len(insights) > limit {
		insights = insights[:limit]
	}

	return insights, nil
}

// ValidateInsight records validation of an insight
func (s *InsightService) ValidateInsight(ctx context.Context, insightID uuid.UUID, clinical bool) error {
	if clinical {
		return s.insightRepo.SetClinicallyValidated(ctx, insightID)
	}
	return s.insightRepo.IncrementValidation(ctx, insightID)
}

// Helper functions
func categorizePattern(pattern *models.FamilyPattern) string {
	switch {
	case containsAny(pattern.InputFactor, "medication", "med_"):
		return "medication"
	case containsAny(pattern.InputFactor, "sleep", "night"):
		return "sleep"
	case containsAny(pattern.InputFactor, "diet", "food"):
		return "diet"
	case containsAny(pattern.InputFactor, "behavior", "mood", "meltdown"):
		return "behavior"
	default:
		return "general"
	}
}

func generatePatternTitle(pattern *models.FamilyPattern) string {
	input := humanizeFactor(pattern.InputFactor)
	output := humanizeFactor(pattern.OutputFactor)

	if pattern.CorrelationStrength > 0 {
		return fmt.Sprintf("%s linked to higher %s", input, output)
	}
	return fmt.Sprintf("%s linked to lower %s", input, output)
}

func generateSimpleDescription(pattern *models.FamilyPattern) string {
	direction := "increases"
	if pattern.CorrelationStrength < 0 {
		direction = "decreases"
	}

	strength := "slightly"
	absCorr := math.Abs(pattern.CorrelationStrength)
	if absCorr > 0.7 {
		strength = "strongly"
	} else if absCorr > 0.5 {
		strength = "moderately"
	}

	lagText := ""
	if pattern.LagHours > 0 {
		if pattern.LagHours >= 24 {
			days := pattern.LagHours / 24
			lagText = fmt.Sprintf(" within %d day(s)", days)
		} else {
			lagText = fmt.Sprintf(" within %d hours", pattern.LagHours)
		}
	}

	input := humanizeFactor(pattern.InputFactor)
	output := humanizeFactor(pattern.OutputFactor)

	return fmt.Sprintf("%s %s %s %s%s (based on %d observations)",
		input, strength, direction, output, lagText, pattern.SampleSize)
}

func generateDetailedDescription(pattern *models.FamilyPattern) string {
	strength := "weak"
	absCorr := math.Abs(pattern.CorrelationStrength)
	if absCorr > 0.7 {
		strength = "strong"
	} else if absCorr > 0.5 {
		strength = "moderate"
	}

	return fmt.Sprintf(
		"Statistical analysis found a %s correlation (r=%.2f) between %s and %s "+
			"with an optimal lag of %d hours. This pattern was observed across %d data points "+
			"and has been confirmed %d time(s).",
		strength,
		pattern.CorrelationStrength,
		pattern.InputFactor,
		pattern.OutputFactor,
		pattern.LagHours,
		pattern.SampleSize,
		pattern.TimesConfirmed,
	)
}

func humanizeFactor(factor string) string {
	names := map[string]string{
		"medication_taken":      "Taking medication",
		"medication_missed":     "Missing medication",
		"medication_adherence":  "Medication adherence",
		"sleep_minutes":         "Sleep duration",
		"sleep_quality":         "Sleep quality",
		"night_wakings":         "Night wakings",
		"mood":                  "mood",
		"energy":                "energy",
		"anxiety":               "anxiety",
		"meltdowns":             "meltdowns",
		"stimming":              "stimming episodes",
		"bristol_scale":         "bowel regularity",
		"bowel_count":           "bowel frequency",
		"water_intake":          "water intake",
		"aggression_incidents":  "aggressive behavior",
	}
	if name, ok := names[factor]; ok {
		return name
	}
	return factor
}

func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

func tierPtr(t models.InsightTier) *models.InsightTier {
	return &t
}

func float64Ptr(f float64) *float64 {
	return &f
}

// Response types
type InsightsResponse struct {
	Child            *models.Child     `json:"child"`
	MedicalInsights  []models.Insight  `json:"medical_insights"`  // Tier 1
	CohortInsights   []models.Insight  `json:"cohort_insights"`   // Tier 2
	PersonalInsights []models.Insight  `json:"personal_insights"` // Tier 3
}

type CreateMedicalInsightRequest struct {
	Category            string          `json:"category"`
	Title               string          `json:"title"`
	SimpleDescription   string          `json:"simple_description"`
	DetailedDescription string          `json:"detailed_description"`
	SourceIDs           models.UUIDArray `json:"source_ids"`
}
