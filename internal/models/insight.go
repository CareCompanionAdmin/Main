package models

import (
	"time"

	"github.com/google/uuid"
)

// InsightTier represents the three tiers of insight transparency
type InsightTier int

const (
	// TierGlobalMedical - Established medical knowledge
	// Source: Drug databases, medical literature, FDA data
	// Example: "Adderall commonly causes decreased appetite (FDA-documented side effect)"
	TierGlobalMedical InsightTier = 1

	// TierCohortPattern - Patterns observed across similar children
	// Source: Anonymized, aggregated data from CareCompanion users
	// Example: "3 of 5 families with similar profiles report improved sleep after removing red dye"
	TierCohortPattern InsightTier = 2

	// TierFamilySpecific - Patterns specific to THIS child
	// Source: This family's logged data only
	// Example: "Matthew's mood drops 2 points on average within 24 hours of missing Adderall"
	TierFamilySpecific InsightTier = 3
)

// String returns the string representation of the tier
func (t InsightTier) String() string {
	switch t {
	case TierGlobalMedical:
		return "medical"
	case TierCohortPattern:
		return "cohort"
	case TierFamilySpecific:
		return "personal"
	default:
		return "unknown"
	}
}

// InsightSource tracks external sources for Tier 1 medical knowledge
type InsightSource struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`        // "FDA Label", "DailyMed", "PubMed"
	SourceType  string     `json:"source_type"` // "drug_label", "interaction", "study"
	ExternalID  NullString `json:"external_id,omitempty"`
	URL         NullString `json:"url,omitempty"`
	Data        JSONB      `json:"data,omitempty"`
	RetrievedAt NullTime   `json:"retrieved_at,omitempty"`
	ExpiresAt   NullTime   `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Insight represents insights from all three tiers
type Insight struct {
	ID       uuid.UUID   `json:"id"`
	ChildID  *uuid.UUID  `json:"child_id,omitempty"`  // NULL for Tier 1 global insights
	FamilyID *uuid.UUID  `json:"family_id,omitempty"` // NULL for Tier 1
	Tier     InsightTier `json:"tier"`
	Category string      `json:"category"` // medication, behavior, diet, sleep, etc.

	// The insight content
	Title               string     `json:"title"`
	SimpleDescription   string     `json:"simple_description"`   // Parent-friendly
	DetailedDescription NullString `json:"detailed_description"` // Clinical/advanced view

	// Statistical backing
	ConfidenceScore     *float64 `json:"confidence_score,omitempty"`     // 0.0 - 1.0
	SampleSize          *int     `json:"sample_size,omitempty"`
	CorrelationStrength *float64 `json:"correlation_strength,omitempty"` // -1.0 to 1.0
	PValue              *float64 `json:"p_value,omitempty"`              // Statistical significance

	// Tier 2 specific: Cohort info (anonymized)
	CohortCriteria   JSONB    `json:"cohort_criteria,omitempty"`
	CohortSize       *int     `json:"cohort_size,omitempty"`
	CohortMatchScore *float64 `json:"cohort_match_score,omitempty"`

	// Tier 3 specific: Family pattern data
	InputFactors   StringArray `json:"input_factors,omitempty"`
	OutputFactors  StringArray `json:"output_factors,omitempty"`
	LagHours       *int        `json:"lag_hours,omitempty"`
	DataPointCount *int        `json:"data_point_count,omitempty"`
	DateRangeStart NullTime    `json:"date_range_start,omitempty"`
	DateRangeEnd   NullTime    `json:"date_range_end,omitempty"`

	// Source tracking
	SourceIDs UUIDArray `json:"source_ids,omitempty"` // References to insight_sources
	PatternID NullUUID  `json:"pattern_id,omitempty"` // Reference to family_patterns

	// Validation status
	ClinicallyValidated bool     `json:"clinically_validated"`
	ValidationCount     int      `json:"validation_count"`
	LastValidatedAt     NullTime `json:"last_validated_at,omitempty"`

	// Display
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Populated on fetch (not stored in DB)
	Sources []InsightSource `json:"sources,omitempty"`
}

// TierDisplay provides UI-friendly tier information
type TierDisplay struct {
	Tier        InsightTier `json:"tier"`
	Label       string      `json:"label"`
	Icon        string      `json:"icon"`
	Color       string      `json:"color"`
	Description string      `json:"description"`
}

// GetTierDisplay returns UI-friendly tier information
func GetTierDisplay(tier InsightTier) TierDisplay {
	switch tier {
	case TierGlobalMedical:
		return TierDisplay{
			Tier:        tier,
			Label:       "Medical Knowledge",
			Icon:        "🏥",
			Color:       "blue",
			Description: "Established medical information from clinical sources",
		}
	case TierCohortPattern:
		return TierDisplay{
			Tier:        tier,
			Label:       "Community Pattern",
			Icon:        "👥",
			Color:       "purple",
			Description: "Pattern observed in families with similar children",
		}
	case TierFamilySpecific:
		return TierDisplay{
			Tier:        tier,
			Label:       "Personal Pattern",
			Icon:        "👤",
			Color:       "green",
			Description: "Pattern specific to your child based on your logs",
		}
	default:
		return TierDisplay{}
	}
}

// ConfidencePercent returns confidence as a percentage
func (i Insight) ConfidencePercent() int {
	if i.ConfidenceScore == nil {
		return 0
	}
	return int(*i.ConfidenceScore * 100)
}

// GetTierDisplay returns the TierDisplay for this insight
func (i Insight) GetTierDisplay() TierDisplay {
	return GetTierDisplay(i.Tier)
}

// Request types
type CreateInsightRequest struct {
	ChildID             *uuid.UUID  `json:"child_id,omitempty"`
	Tier                InsightTier `json:"tier"`
	Category            string      `json:"category"`
	Title               string      `json:"title"`
	SimpleDescription   string      `json:"simple_description"`
	DetailedDescription string      `json:"detailed_description,omitempty"`
	ConfidenceScore     *float64    `json:"confidence_score,omitempty"`
	PatternID           *uuid.UUID  `json:"pattern_id,omitempty"`
}
