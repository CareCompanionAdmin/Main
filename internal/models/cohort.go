package models

import (
	"time"

	"github.com/google/uuid"
)

// CohortDefinition defines a group of similar children for pattern analysis
type CohortDefinition struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description NullString `json:"description"`
	Criteria    JSONB     `json:"criteria"`
	MinMembers  int       `json:"min_members"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CohortCriteria defines matching parameters for cohort membership
type CohortCriteria struct {
	AgeRangeMin int      `json:"age_range_min"`
	AgeRangeMax int      `json:"age_range_max"`
	Conditions  []string `json:"conditions"`
	Medications []string `json:"medications"`
	Gender      string   `json:"gender,omitempty"`
}

// CohortMembership represents anonymous membership in a cohort
type CohortMembership struct {
	ID         uuid.UUID `json:"id"`
	CohortID   uuid.UUID `json:"cohort_id"`
	ChildHash  string    `json:"child_hash"` // SHA-256 hash for privacy
	MatchScore *float64  `json:"match_score"`
	JoinedAt   time.Time `json:"joined_at"`
}

// CohortPattern represents aggregated patterns observed in a cohort
type CohortPattern struct {
	ID           uuid.UUID `json:"id"`
	CohortID     uuid.UUID `json:"cohort_id"`
	PatternType  string    `json:"pattern_type"`
	InputFactor  string    `json:"input_factor"`
	OutputFactor string    `json:"output_factor"`

	FamiliesAffected        int       `json:"families_affected"`
	FamiliesTotal           int       `json:"families_total"`
	AvgCorrelation          *float64  `json:"avg_correlation"`
	StdDeviation            *float64  `json:"std_deviation"`
	ConfidenceIntervalLow   *float64  `json:"confidence_interval_low"`
	ConfidenceIntervalHigh  *float64  `json:"confidence_interval_high"`

	SimpleDescription   string     `json:"simple_description"`
	DetailedDescription NullString `json:"detailed_description"`

	FirstObservedAt NullTime  `json:"first_observed_at"`
	LastUpdatedAt   time.Time `json:"last_updated_at"`
	IsActive        bool      `json:"is_active"`
}

// CohortMatchResult shows how well a child matches a cohort
type CohortMatchResult struct {
	CohortID    uuid.UUID              `json:"cohort_id"`
	CohortName  string                 `json:"cohort_name"`
	MatchScore  float64                `json:"match_score"` // 0.0 - 1.0
	MemberCount int                    `json:"member_count"`
	Patterns    []CohortPatternDisplay `json:"patterns"`
}

// CohortPatternDisplay for UI
type CohortPatternDisplay struct {
	ID                  uuid.UUID `json:"id"`
	SimpleDescription   string    `json:"simple_description"`
	DetailedDescription string    `json:"detailed_description"`
	FamiliesAffected    int       `json:"families_affected"`
	FamiliesTotal       int       `json:"families_total"`
	Percentage          float64   `json:"percentage"`
	Confidence          float64   `json:"confidence"`
}

// Percentage returns the percentage of families affected
func (p CohortPattern) Percentage() float64 {
	if p.FamiliesTotal == 0 {
		return 0
	}
	return float64(p.FamiliesAffected) / float64(p.FamiliesTotal) * 100
}
