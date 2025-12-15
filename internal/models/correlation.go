package models

import (
	"time"

	"github.com/google/uuid"
)

type ChildBaseline struct {
	ID            uuid.UUID `json:"id"`
	ChildID       uuid.UUID `json:"child_id"`
	MetricName    string    `json:"metric_name"`
	BaselineValue float64   `json:"baseline_value"`
	StdDeviation  float64   `json:"std_deviation"`
	SampleSize    int       `json:"sample_size"`
	CalculatedAt  time.Time `json:"calculated_at"`
	ValidUntil    NullTime  `json:"valid_until,omitempty"`
}

type CorrelationRequest struct {
	ID             uuid.UUID         `json:"id"`
	ChildID        uuid.UUID         `json:"child_id"`
	RequestedBy    uuid.UUID         `json:"requested_by"`
	Status         CorrelationStatus `json:"status"`
	InputFactors   StringArray       `json:"input_factors,omitempty"`
	OutputFactors  StringArray       `json:"output_factors,omitempty"`
	DateRangeStart NullTime          `json:"date_range_start,omitempty"`
	DateRangeEnd   NullTime          `json:"date_range_end,omitempty"`
	Results        JSONB             `json:"results,omitempty"`
	StartedAt      NullTime          `json:"started_at,omitempty"`
	CompletedAt    NullTime          `json:"completed_at,omitempty"`
	ErrorMessage   NullString        `json:"error_message,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

type FamilyPattern struct {
	ID                  uuid.UUID  `json:"id"`
	ChildID             uuid.UUID  `json:"child_id"`
	PatternType         string     `json:"pattern_type"`
	InputFactor         string     `json:"input_factor"`
	OutputFactor        string     `json:"output_factor"`
	CorrelationStrength float64    `json:"correlation_strength"`
	ConfidenceScore     float64    `json:"confidence_score"`
	SampleSize          int        `json:"sample_size"`
	LagHours            int        `json:"lag_hours"`
	Description         NullString `json:"description,omitempty"`
	SupportingData      JSONB      `json:"supporting_data,omitempty"`
	FirstDetectedAt     NullTime   `json:"first_detected_at,omitempty"`
	LastConfirmedAt     NullTime   `json:"last_confirmed_at,omitempty"`
	TimesConfirmed      int        `json:"times_confirmed"`
	IsActive            bool       `json:"is_active"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type ClinicalValidation struct {
	ID                   uuid.UUID  `json:"id"`
	PatternID            NullUUID   `json:"pattern_id,omitempty"`
	AlertID              NullUUID   `json:"alert_id,omitempty"`
	ChildID              uuid.UUID  `json:"child_id"`
	ProviderUserID       NullUUID   `json:"provider_user_id,omitempty"`
	ValidationType       NullString `json:"validation_type,omitempty"`
	TreatmentChanged     bool       `json:"treatment_changed"`
	TreatmentDescription NullString `json:"treatment_description,omitempty"`
	ParentConfirmed      *bool      `json:"parent_confirmed,omitempty"`
	ParentConfirmedAt    NullTime   `json:"parent_confirmed_at,omitempty"`
	ValidationStrength   *float64   `json:"validation_strength,omitempty"`
	ExpiresAt            NullTime   `json:"expires_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

// Insights page data
type InsightsPage struct {
	Child              Child               `json:"child"`
	Patterns           []FamilyPattern     `json:"patterns"`
	RecentCorrelations []CorrelationRequest `json:"recent_correlations"`
	Baselines          []ChildBaseline     `json:"baselines"`
}

// Request types
type CreateCorrelationRequest struct {
	InputFactors   []string   `json:"input_factors"`
	OutputFactors  []string   `json:"output_factors"`
	DateRangeStart *time.Time `json:"date_range_start,omitempty"`
	DateRangeEnd   *time.Time `json:"date_range_end,omitempty"`
}

// DataPoint represents a single data point for correlation analysis
type DataPoint struct {
	Date  time.Time `json:"date"`
	Value float64   `json:"value"`
}
