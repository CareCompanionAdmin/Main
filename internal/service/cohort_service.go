package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

type CohortService struct {
	cohortRepo  repository.CohortRepository
	childRepo   repository.ChildRepository
	insightRepo repository.InsightRepository
}

func NewCohortService(
	cohortRepo repository.CohortRepository,
	childRepo repository.ChildRepository,
	insightRepo repository.InsightRepository,
) *CohortService {
	return &CohortService{
		cohortRepo:  cohortRepo,
		childRepo:   childRepo,
		insightRepo: insightRepo,
	}
}

// ChildProfile represents a child's characteristics for cohort matching
type ChildProfile struct {
	AgeYears   int
	Gender     string
	Conditions []string
}

// FindMatchingCohorts finds cohorts that match a child's profile
func (s *CohortService) FindMatchingCohorts(ctx context.Context, childID uuid.UUID) ([]models.CohortMatchResult, error) {
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, fmt.Errorf("child not found")
	}

	// Build child's profile
	profile := s.buildChildProfile(child)

	// Get all cohort definitions
	cohorts, err := s.cohortRepo.GetAllCohorts(ctx)
	if err != nil {
		return nil, err
	}

	var results []models.CohortMatchResult
	for _, cohort := range cohorts {
		criteria, err := s.parseCriteria(cohort.Criteria)
		if err != nil {
			continue
		}

		matchScore := s.calculateMatchScore(profile, criteria)

		if matchScore < 0.5 { // Minimum 50% match
			continue
		}

		// Get patterns for this cohort
		patterns, _ := s.cohortRepo.GetActivePatterns(ctx, cohort.ID)

		memberCount, _ := s.cohortRepo.GetMemberCount(ctx, cohort.ID)

		result := models.CohortMatchResult{
			CohortID:    cohort.ID,
			CohortName:  cohort.Name,
			MatchScore:  matchScore,
			MemberCount: memberCount,
			Patterns:    s.formatPatterns(patterns),
		}

		results = append(results, result)
	}

	return results, nil
}

// GetCohortInsights returns Tier 2 insights for a child
func (s *CohortService) GetCohortInsights(ctx context.Context, childID uuid.UUID) ([]models.Insight, error) {
	matches, err := s.FindMatchingCohorts(ctx, childID)
	if err != nil {
		return nil, err
	}

	var insights []models.Insight
	for _, match := range matches {
		for _, pattern := range match.Patterns {
			insight := models.Insight{
				ID:                pattern.ID,
				ChildID:           &childID,
				Tier:              models.TierCohortPattern,
				Category:          "cohort",
				Title:             fmt.Sprintf("Pattern from %d similar families", match.MemberCount),
				SimpleDescription: pattern.SimpleDescription,
				ConfidenceScore:   &pattern.Confidence,
				CohortSize:        &match.MemberCount,
				CohortMatchScore:  &match.MatchScore,
				IsActive:          true,
			}
			if pattern.DetailedDescription != "" {
				insight.DetailedDescription = models.NullString{}
				insight.DetailedDescription.String = pattern.DetailedDescription
				insight.DetailedDescription.Valid = true
			}
			insights = append(insights, insight)
		}
	}

	return insights, nil
}

// AddToCohort anonymously adds a child to matching cohorts
func (s *CohortService) AddToCohort(ctx context.Context, childID uuid.UUID) error {
	// Generate anonymous hash (cannot be reversed to find child)
	hash := s.generateAnonymousHash(childID)

	matches, err := s.FindMatchingCohorts(ctx, childID)
	if err != nil {
		return err
	}

	for _, match := range matches {
		if match.MatchScore >= 0.7 { // Strong match required
			s.cohortRepo.AddMember(ctx, match.CohortID, hash, match.MatchScore)
		}
	}

	return nil
}

// CreateCohort creates a new cohort definition
func (s *CohortService) CreateCohort(ctx context.Context, name, description string, criteria models.CohortCriteria, minMembers int) (*models.CohortDefinition, error) {
	criteriaJSON, err := json.Marshal(criteria)
	if err != nil {
		return nil, err
	}

	var jsonbCriteria models.JSONB
	if err := json.Unmarshal(criteriaJSON, &jsonbCriteria); err != nil {
		return nil, err
	}

	cohort := &models.CohortDefinition{
		Name:       name,
		Criteria:   jsonbCriteria,
		MinMembers: minMembers,
	}
	if description != "" {
		cohort.Description.String = description
		cohort.Description.Valid = true
	}

	err = s.cohortRepo.CreateCohort(ctx, cohort)
	return cohort, err
}

// GetAllCohorts returns all active cohorts
func (s *CohortService) GetAllCohorts(ctx context.Context) ([]models.CohortDefinition, error) {
	return s.cohortRepo.GetAllCohorts(ctx)
}

// Helper functions

func (s *CohortService) buildChildProfile(child *models.Child) ChildProfile {
	profile := ChildProfile{
		Gender: child.Gender.String,
	}

	// Calculate age from date of birth
	if !child.DateOfBirth.IsZero() {
		years := time.Since(child.DateOfBirth).Hours() / (24 * 365.25)
		profile.AgeYears = int(years)
	}

	return profile
}

func (s *CohortService) parseCriteria(criteria models.JSONB) (models.CohortCriteria, error) {
	var c models.CohortCriteria
	data, err := json.Marshal(criteria)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(data, &c)
	return c, err
}

func (s *CohortService) calculateMatchScore(profile ChildProfile, criteria models.CohortCriteria) float64 {
	matchPoints := 0.0
	totalPoints := 0.0

	// Age matching (40% weight)
	if criteria.AgeRangeMin > 0 || criteria.AgeRangeMax > 0 {
		totalPoints += 40
		if profile.AgeYears >= criteria.AgeRangeMin && profile.AgeYears <= criteria.AgeRangeMax {
			matchPoints += 40
		} else {
			// Partial credit for close matches
			ageDiff := 0
			if profile.AgeYears < criteria.AgeRangeMin {
				ageDiff = criteria.AgeRangeMin - profile.AgeYears
			} else {
				ageDiff = profile.AgeYears - criteria.AgeRangeMax
			}
			if ageDiff <= 2 {
				matchPoints += 20 // 50% credit for within 2 years
			}
		}
	}

	// Gender matching (20% weight)
	if criteria.Gender != "" {
		totalPoints += 20
		if criteria.Gender == profile.Gender || criteria.Gender == "any" {
			matchPoints += 20
		}
	}

	// Condition matching (40% weight)
	if len(criteria.Conditions) > 0 && len(profile.Conditions) > 0 {
		totalPoints += 40
		matchedConditions := 0
		for _, required := range criteria.Conditions {
			for _, have := range profile.Conditions {
				if required == have {
					matchedConditions++
					break
				}
			}
		}
		conditionScore := float64(matchedConditions) / float64(len(criteria.Conditions)) * 40
		matchPoints += conditionScore
	}

	if totalPoints == 0 {
		return 1.0 // No criteria = universal match
	}

	return matchPoints / totalPoints
}

func (s *CohortService) formatPatterns(patterns []models.CohortPattern) []models.CohortPatternDisplay {
	var displays []models.CohortPatternDisplay
	for _, p := range patterns {
		confidence := 0.0
		if p.AvgCorrelation != nil {
			confidence = *p.AvgCorrelation
			if confidence < 0 {
				confidence = -confidence
			}
		}

		display := models.CohortPatternDisplay{
			ID:                p.ID,
			SimpleDescription: p.SimpleDescription,
			FamiliesAffected:  p.FamiliesAffected,
			FamiliesTotal:     p.FamiliesTotal,
			Percentage:        p.Percentage(),
			Confidence:        confidence,
		}
		if p.DetailedDescription.Valid {
			display.DetailedDescription = p.DetailedDescription.String
		}
		displays = append(displays, display)
	}
	return displays
}

func (s *CohortService) generateAnonymousHash(childID uuid.UUID) string {
	// Combine child ID with a salt to prevent rainbow table attacks
	salt := "carecompanion-cohort-v1-" // In production, use a secure secret
	data := salt + childID.String()
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
