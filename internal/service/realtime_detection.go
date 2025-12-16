package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// RealtimeDetectionService handles real-time pattern detection as events occur
type RealtimeDetectionService struct {
	correlationRepo repository.CorrelationRepository
	alertRepo       repository.AlertRepository
	childRepo       repository.ChildRepository
	medicationRepo  repository.MedicationRepository
	alertService    *AlertService
}

// NewRealtimeDetectionService creates a new realtime detection service
func NewRealtimeDetectionService(
	correlationRepo repository.CorrelationRepository,
	alertRepo repository.AlertRepository,
	childRepo repository.ChildRepository,
	medicationRepo repository.MedicationRepository,
	alertService *AlertService,
) *RealtimeDetectionService {
	return &RealtimeDetectionService{
		correlationRepo: correlationRepo,
		alertRepo:       alertRepo,
		childRepo:       childRepo,
		medicationRepo:  medicationRepo,
		alertService:    alertService,
	}
}

// DetectionResult represents the result of real-time pattern detection
type DetectionResult struct {
	Detected      bool                   `json:"detected"`
	PatternType   string                 `json:"pattern_type,omitempty"`
	Severity      models.AlertSeverity   `json:"severity,omitempty"`
	Title         string                 `json:"title,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	AlertCreated  bool                   `json:"alert_created,omitempty"`
	RelatedData   map[string]interface{} `json:"related_data,omitempty"`
}

// OnLogCreated is called when a new log entry is created - checks for patterns
func (s *RealtimeDetectionService) OnLogCreated(ctx context.Context, childID uuid.UUID, logType string, logData map[string]interface{}) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Check against established baselines
	baselineResult := s.checkBaselines(ctx, childID, logType, logData)
	if baselineResult != nil && baselineResult.Detected {
		return baselineResult, nil
	}

	// Check for known pattern triggers
	patternResult := s.checkPatternTriggers(ctx, childID, logType, logData)
	if patternResult != nil && patternResult.Detected {
		return patternResult, nil
	}

	// Check for concerning values based on log type
	switch logType {
	case "sleep":
		return s.checkSleepPattern(ctx, childID, logData)
	case "behavior":
		return s.checkBehaviorPattern(ctx, childID, logData)
	case "symptom":
		return s.checkSymptomPattern(ctx, childID, logData)
	case "meal":
		return s.checkMealPattern(ctx, childID, logData)
	}

	return result, nil
}

// OnMedicationMissed is called when a medication dose is missed
func (s *RealtimeDetectionService) OnMedicationMissed(ctx context.Context, childID uuid.UUID, medicationID uuid.UUID) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Get medication details
	medication, err := s.medicationRepo.GetByID(ctx, medicationID)
	if err != nil || medication == nil {
		return result, err
	}

	// Get child info
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil || child == nil {
		return result, err
	}

	// Check for pattern of missed doses
	missedPattern := s.checkMissedMedicationPattern(ctx, childID, medicationID)

	if missedPattern {
		result.Detected = true
		result.PatternType = "medication_adherence"
		result.Severity = models.AlertSeverityWarning
		result.Title = fmt.Sprintf("Medication Adherence Concern: %s", medication.Name)
		result.Description = fmt.Sprintf("Multiple missed doses of %s detected recently. This may affect treatment effectiveness.", medication.Name)
		result.Confidence = 0.85
		result.RelatedData["medication_name"] = medication.Name
		result.RelatedData["medication_id"] = medicationID.String()

		// Create alert
		alert := &models.Alert{
			ChildID:     childID,
			FamilyID:    child.FamilyID,
			AlertType:   models.AlertTypeMedicationAdherence,
			Severity:    result.Severity,
			Title:       result.Title,
			Description: result.Description,
			Data:        models.JSONB(result.RelatedData),
		}

		if err := s.alertRepo.Create(ctx, alert); err == nil {
			result.AlertCreated = true
		}
	}

	return result, nil
}

// checkBaselines compares new data against established baselines
func (s *RealtimeDetectionService) checkBaselines(ctx context.Context, childID uuid.UUID, logType string, logData map[string]interface{}) *DetectionResult {
	// Get established baselines for this child
	baselines, err := s.correlationRepo.GetBaselines(ctx, childID)
	if err != nil || len(baselines) == 0 {
		return nil
	}

	for _, baseline := range baselines {
		// Check if this baseline applies to this log type (MetricName contains log type)
		if !containsLogType(baseline.MetricName, logType) {
			continue
		}

		// Extract relevant value from log data based on metric name
		value := s.extractValueForBaseline(logData, baseline.MetricName)
		if value == 0 {
			continue
		}

		// Check if value deviates significantly from baseline
		deviation := calculateDeviation(value, baseline.BaselineValue, baseline.StdDeviation)

		if deviation > 2.0 { // More than 2 standard deviations
			result := &DetectionResult{
				Detected:    true,
				PatternType: "baseline_deviation",
				Confidence:  minFloat(0.5+(deviation*0.15), 0.95),
				RelatedData: map[string]interface{}{
					"baseline_metric":  baseline.MetricName,
					"expected_value":   baseline.BaselineValue,
					"actual_value":     value,
					"deviation_level":  deviation,
				},
			}

			if deviation > 3.0 {
				result.Severity = models.AlertSeverityWarning
				result.Title = fmt.Sprintf("Significant %s deviation detected", baseline.MetricName)
				result.Description = fmt.Sprintf("Today's %s value (%.1f) is significantly different from the established baseline (%.1f)",
					baseline.MetricName, value, baseline.BaselineValue)
			} else {
				result.Severity = models.AlertSeverityInfo
				result.Title = fmt.Sprintf("Notable %s change detected", baseline.MetricName)
				result.Description = fmt.Sprintf("Today's %s value (%.1f) differs from the usual baseline (%.1f)",
					baseline.MetricName, value, baseline.BaselineValue)
			}

			return result
		}
	}

	return nil
}

// checkPatternTriggers checks for known pattern precursors
func (s *RealtimeDetectionService) checkPatternTriggers(ctx context.Context, childID uuid.UUID, logType string, logData map[string]interface{}) *DetectionResult {
	// Get active patterns for this child
	patterns, err := s.correlationRepo.GetPatterns(ctx, childID, true) // activeOnly = true
	if err != nil || len(patterns) == 0 {
		return nil
	}

	for _, pattern := range patterns {
		// Check if this log type is a known trigger factor (input factor)
		if !s.isKnownTrigger(pattern, logType) {
			continue
		}

		// Check confidence threshold
		if pattern.ConfidenceScore < 0.6 {
			continue
		}

		description := ""
		if pattern.Description.Valid {
			description = pattern.Description.String
		}

		result := &DetectionResult{
			Detected:    true,
			PatternType: "pattern_trigger",
			Severity:    models.AlertSeverityInfo,
			Confidence:  pattern.ConfidenceScore,
			Title:       fmt.Sprintf("Pattern trigger detected: %s", pattern.InputFactor),
			Description: generateTriggerDescription(pattern, logType),
			RelatedData: map[string]interface{}{
				"pattern_id":       pattern.ID.String(),
				"trigger_type":     logType,
				"pattern_strength": pattern.ConfidenceScore,
				"description":      description,
			},
		}

		if pattern.ConfidenceScore > 0.8 {
			result.Severity = models.AlertSeverityWarning
		}

		return result
	}

	return nil
}

// checkSleepPattern checks for concerning sleep patterns
func (s *RealtimeDetectionService) checkSleepPattern(ctx context.Context, childID uuid.UUID, logData map[string]interface{}) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Extract sleep data
	duration, _ := logData["duration"].(float64)
	quality, _ := logData["quality"].(string)
	nightWakings, _ := logData["night_wakings"].(float64)

	// Check for concerning patterns
	var concerns []string

	if duration > 0 && duration < 6 {
		concerns = append(concerns, "insufficient sleep duration")
	}
	if quality == "poor" || quality == "very_poor" {
		concerns = append(concerns, "poor sleep quality")
	}
	if nightWakings >= 3 {
		concerns = append(concerns, "frequent night wakings")
	}

	if len(concerns) >= 2 {
		result.Detected = true
		result.PatternType = "sleep_concern"
		result.Severity = models.AlertSeverityInfo
		result.Title = "Sleep pattern concern"
		result.Description = fmt.Sprintf("Multiple sleep concerns detected: %v", concerns)
		result.Confidence = 0.7
		result.RelatedData["concerns"] = concerns
		result.RelatedData["duration"] = duration
		result.RelatedData["quality"] = quality
	}

	return result, nil
}

// checkBehaviorPattern checks for concerning behavior patterns
func (s *RealtimeDetectionService) checkBehaviorPattern(ctx context.Context, childID uuid.UUID, logData map[string]interface{}) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Extract behavior data
	intensity, _ := logData["intensity"].(float64)
	mood, _ := logData["mood"].(string)
	duration, _ := logData["duration"].(float64)

	// Check for concerning patterns
	if intensity >= 8 || (mood == "aggressive" || mood == "extremely_upset") || duration >= 60 {
		result.Detected = true
		result.PatternType = "behavior_concern"
		result.Severity = models.AlertSeverityWarning
		result.Title = "Significant behavior event"
		result.Description = "A significant behavior event has been logged that may warrant attention"
		result.Confidence = 0.75
		result.RelatedData["intensity"] = intensity
		result.RelatedData["mood"] = mood
		result.RelatedData["duration"] = duration

		if intensity >= 9 || duration >= 90 {
			result.Severity = models.AlertSeverityCritical
		}
	}

	return result, nil
}

// checkSymptomPattern checks for concerning symptom patterns
func (s *RealtimeDetectionService) checkSymptomPattern(ctx context.Context, childID uuid.UUID, logData map[string]interface{}) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Extract symptom data
	severity, _ := logData["severity"].(float64)
	symptomType, _ := logData["type"].(string)
	isNew, _ := logData["is_new"].(bool)

	// Check for concerning patterns
	if severity >= 7 || (isNew && severity >= 5) {
		result.Detected = true
		result.PatternType = "symptom_concern"
		result.Severity = models.AlertSeverityWarning
		result.Title = fmt.Sprintf("Symptom alert: %s", symptomType)
		result.Description = fmt.Sprintf("A %s symptom with severity %.0f/10 has been reported", symptomType, severity)
		result.Confidence = 0.8
		result.RelatedData["symptom_type"] = symptomType
		result.RelatedData["severity"] = severity
		result.RelatedData["is_new"] = isNew

		if severity >= 9 {
			result.Severity = models.AlertSeverityCritical
		}
	}

	return result, nil
}

// checkMealPattern checks for concerning meal patterns
func (s *RealtimeDetectionService) checkMealPattern(ctx context.Context, childID uuid.UUID, logData map[string]interface{}) (*DetectionResult, error) {
	result := &DetectionResult{
		Detected:    false,
		RelatedData: make(map[string]interface{}),
	}

	// Extract meal data
	appetite, _ := logData["appetite"].(string)
	reaction, _ := logData["reaction"].(string)

	// Check for concerning patterns
	if appetite == "none" || reaction == "vomiting" || reaction == "severe_reaction" {
		result.Detected = true
		result.PatternType = "meal_concern"
		result.Severity = models.AlertSeverityWarning
		result.Title = "Meal/eating concern"
		result.Description = "A meal-related concern has been logged that may need attention"
		result.Confidence = 0.7
		result.RelatedData["appetite"] = appetite
		result.RelatedData["reaction"] = reaction

		if reaction == "severe_reaction" {
			result.Severity = models.AlertSeverityCritical
		}
	}

	return result, nil
}

// checkMissedMedicationPattern checks if there's a pattern of missed medications
func (s *RealtimeDetectionService) checkMissedMedicationPattern(ctx context.Context, childID uuid.UUID, medicationID uuid.UUID) bool {
	// Get recent medication logs (last 7 days)
	since := time.Now().AddDate(0, 0, -7)
	logs, err := s.medicationRepo.GetLogsByMedicationSince(ctx, medicationID, since)
	if err != nil {
		return false
	}

	// Count missed doses
	missedCount := 0
	for _, log := range logs {
		if log.Status == models.LogStatusMissed {
			missedCount++
		}
	}

	// Flag if more than 2 missed doses in a week
	return missedCount >= 3
}

// Helper functions

func (s *RealtimeDetectionService) extractValueForBaseline(logData map[string]interface{}, dataKey string) float64 {
	if val, ok := logData[dataKey].(float64); ok {
		return val
	}
	if val, ok := logData[dataKey].(int); ok {
		return float64(val)
	}
	return 0
}

func (s *RealtimeDetectionService) isKnownTrigger(pattern models.FamilyPattern, logType string) bool {
	// Check if the pattern's input factor contains the log type
	return containsLogType(pattern.InputFactor, logType)
}

func calculateDeviation(value, baseline, stdDev float64) float64 {
	if stdDev == 0 {
		stdDev = 1 // Avoid division by zero
	}
	return absFloat(value-baseline) / stdDev
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func generateTriggerDescription(pattern models.FamilyPattern, logType string) string {
	description := pattern.OutputFactor
	if pattern.Description.Valid && pattern.Description.String != "" {
		description = pattern.Description.String
	}
	return fmt.Sprintf("Based on historical patterns, this %s event may be associated with %s. The pattern has been observed with %.0f%% confidence.",
		logType, description, pattern.ConfidenceScore*100)
}

func containsLogType(factor, logType string) bool {
	// Simple containment check - factor might be "sleep_duration" and logType "sleep"
	return len(factor) >= len(logType) && factor[:len(logType)] == logType
}
