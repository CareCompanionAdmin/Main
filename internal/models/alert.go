package models

import (
	"time"

	"github.com/google/uuid"
)

type Alert struct {
	ID              uuid.UUID       `json:"id"`
	ChildID         uuid.UUID       `json:"child_id"`
	FamilyID        uuid.UUID       `json:"family_id"`
	AlertType       string          `json:"alert_type"`
	Severity        AlertSeverity   `json:"severity"`
	Status          AlertStatus     `json:"status"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Data            JSONB           `json:"data,omitempty"`
	CorrelationID   NullUUID        `json:"correlation_id,omitempty"`
	SourceType      CorrelationType `json:"source_type,omitempty"`
	ConfidenceScore *float64        `json:"confidence_score,omitempty"`
	DateRangeStart  NullTime        `json:"date_range_start,omitempty"`
	DateRangeEnd    NullTime        `json:"date_range_end,omitempty"`
	AcknowledgedBy  NullUUID        `json:"acknowledged_by,omitempty"`
	AcknowledgedAt  NullTime        `json:"acknowledged_at,omitempty"`
	ResolvedBy      NullUUID        `json:"resolved_by,omitempty"`
	ResolvedAt      NullTime        `json:"resolved_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Common alert types
const (
	AlertTypeMedicationAdherence = "medication_adherence"
	AlertTypeBehaviorChange      = "behavior_change"
	AlertTypeWeightChange        = "weight_change"
	AlertTypeSleepPattern        = "sleep_pattern"
	AlertTypePatternDiscovered   = "pattern_discovered"
	AlertTypeMissedLog           = "missed_log"
)

type AlertFeedback struct {
	ID           uuid.UUID  `json:"id"`
	AlertID      uuid.UUID  `json:"alert_id"`
	UserID       uuid.UUID  `json:"user_id"`
	WasHelpful   *bool      `json:"was_helpful,omitempty"`
	FeedbackText NullString `json:"feedback_text,omitempty"`
	ActionTaken  NullString `json:"action_taken,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type AlertsPage struct {
	Child        Child       `json:"child"`
	ActiveAlerts []Alert     `json:"active_alerts"`
	RecentAlerts []Alert     `json:"recent_alerts"`
	AlertStats   AlertStats  `json:"alert_stats"`
}

type AlertStats struct {
	TotalActive    int `json:"total_active"`
	TotalThisWeek  int `json:"total_this_week"`
	TotalThisMonth int `json:"total_this_month"`
	CriticalCount  int `json:"critical_count"`
	WarningCount   int `json:"warning_count"`
	InfoCount      int `json:"info_count"`
}

type AlertFeedbackRequest struct {
	WasHelpful   *bool  `json:"was_helpful,omitempty"`
	FeedbackText string `json:"feedback_text,omitempty"`
	ActionTaken  string `json:"action_taken,omitempty"`
}

// AlertTypeStats tracks statistics for a specific alert type
type AlertTypeStats struct {
	Total           int `json:"total"`
	Acknowledged    int `json:"acknowledged"`
	Resolved        int `json:"resolved"`
	Dismissed       int `json:"dismissed"`
	HelpfulFeedback int `json:"helpful_feedback"`
}

// AlertEffectiveness tracks how well alerts perform
type AlertEffectiveness struct {
	AlertType          string  `json:"alert_type"`
	TotalGenerated     int     `json:"total_generated"`
	Acknowledged       int     `json:"acknowledged"`
	HelpfulCount       int     `json:"helpful_count"`
	DismissedCount     int     `json:"dismissed_count"`
	EffectivenessScore float64 `json:"effectiveness_score"`
}

// AlertGenerationData contains data for smart alert generation
type AlertGenerationData struct {
	Type          string
	Severity      AlertSeverity
	Title         string
	Description   string
	Data          JSONB
	Confidence    float64
	FamilyID      uuid.UUID
	CorrelationID *uuid.UUID
}
