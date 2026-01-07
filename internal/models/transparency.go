package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"time"
)

// JSONMap is a map type for JSONB columns
type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// JSONArray is an array type for JSONB array columns
type JSONArray []interface{}

func (j JSONArray) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONArray) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, j)
}

// CitationType represents the type of citation
type CitationType string

const (
	CitationTypeMedicationReference CitationType = "medication_reference"
	CitationTypeDrugInteraction     CitationType = "drug_interaction"
	CitationTypeGlobalCorrelation   CitationType = "global_correlation"
	CitationTypeDosageGuideline     CitationType = "dosage_guideline"
	CitationTypeBehavioralReference CitationType = "behavioral_reference"
	CitationTypeDietaryReference    CitationType = "dietary_reference"
)

// AuthorityType represents the type of authority for citations
type AuthorityType string

const (
	AuthorityTypeGovernment              AuthorityType = "government"
	AuthorityTypeMedicalJournal          AuthorityType = "medical_journal"
	AuthorityTypeProfessionalOrg         AuthorityType = "professional_organization"
	AuthorityTypeDrugManufacturer        AuthorityType = "drug_manufacturer"
	AuthorityTypeResearchInstitution     AuthorityType = "research_institution"
)

// FactorType represents the type of confidence factor
type FactorType string

const (
	FactorTypeGlobalMedical        FactorType = "global_medical"
	FactorTypeCohortPattern        FactorType = "cohort_pattern"
	FactorTypeFamilyHistory        FactorType = "family_history"
	FactorTypeTemporalProximity    FactorType = "temporal_proximity"
	FactorTypeAmplitude            FactorType = "amplitude"
	FactorTypeMedicationCriticality FactorType = "medication_criticality"
	FactorTypeClinicalValidation   FactorType = "clinical_validation"
)

// ChangeType represents the type of treatment change
type ChangeType string

const (
	ChangeTypeMedicationAdded           ChangeType = "medication_added"
	ChangeTypeMedicationDiscontinued    ChangeType = "medication_discontinued"
	ChangeTypeMedicationDoseChanged     ChangeType = "medication_dose_changed"
	ChangeTypeMedicationScheduleChanged ChangeType = "medication_schedule_changed"
	ChangeTypeMedicationSwitched        ChangeType = "medication_switched"
	ChangeTypeMedicationLogEdited       ChangeType = "medication_log_edited"
	ChangeTypeDietPlanStarted           ChangeType = "diet_plan_started"
	ChangeTypeDietPlanEnded             ChangeType = "diet_plan_ended"
	ChangeTypeConditionAdded            ChangeType = "condition_added"
	ChangeTypeConditionRemoved          ChangeType = "condition_removed"
)

// InterrogativeStatus represents the status of an interrogative prompt
type InterrogativeStatus string

const (
	InterrogativeStatusPending  InterrogativeStatus = "pending"
	InterrogativeStatusPrompted InterrogativeStatus = "prompted"
	InterrogativeStatusAnswered InterrogativeStatus = "answered"
	InterrogativeStatusSkipped  InterrogativeStatus = "skipped"
	InterrogativeStatusExpired  InterrogativeStatus = "expired"
)

// ChangeSource represents who initiated a treatment change
type ChangeSource string

const (
	ChangeSourceSelfDirected        ChangeSource = "self_directed"
	ChangeSourceProviderRecommended ChangeSource = "provider_recommended"
	ChangeSourceOtherFamilyMember   ChangeSource = "other_family_member"
	ChangeSourcePreferNotToSay      ChangeSource = "prefer_not_to_say"
)

// AnalysisRelation represents how a change relates to an analysis
type AnalysisRelation string

const (
	AnalysisRelationYesProviderAgreed AnalysisRelation = "yes_provider_agreed"
	AnalysisRelationPartiallyOneFactor AnalysisRelation = "partially_one_factor"
	AnalysisRelationNoDifferentReason  AnalysisRelation = "no_different_reason"
	AnalysisRelationNotSure            AnalysisRelation = "not_sure"
)

// ValidationStrength represents the strength of a clinical validation
type ValidationStrength string

const (
	ValidationStrengthStrong   ValidationStrength = "strong"
	ValidationStrengthModerate ValidationStrength = "moderate"
	ValidationStrengthWeak     ValidationStrength = "weak"
	ValidationStrengthNone     ValidationStrength = "none"
)

// ExportType represents how an alert was exported
type ExportType string

const (
	ExportTypePDFDownload     ExportType = "pdf_download"
	ExportTypeShareToProvider ExportType = "share_to_provider"
	ExportTypePrint           ExportType = "print"
)

// ShareMethod represents how an analysis was shared
type ShareMethod string

const (
	ShareMethodInApp ShareMethod = "in_app"
	ShareMethodEmail ShareMethod = "email"
)

// AnalysisView represents the view mode for analysis
type AnalysisView string

const (
	AnalysisViewParent   AnalysisView = "parent"
	AnalysisViewClinical AnalysisView = "clinical"
)

// AttachmentType represents the type of chat attachment
type AttachmentType string

const (
	AttachmentTypeAnalysisSnapshot AttachmentType = "analysis_snapshot"
	AttachmentTypePDFExport        AttachmentType = "pdf_export"
	AttachmentTypeImage            AttachmentType = "image"
	AttachmentTypeDocument         AttachmentType = "document"
)

// Cohort represents a group of similar children for pattern analysis
type Cohort struct {
	ID          string         `json:"id" db:"id"`
	Name        string         `json:"name" db:"name"`
	Description sql.NullString `json:"description" db:"description"`
	AgeRangeMin *int           `json:"age_range_min" db:"age_range_min"`
	AgeRangeMax *int           `json:"age_range_max" db:"age_range_max"`
	Diagnoses   []string       `json:"diagnoses" db:"diagnoses"`
	Medications []string       `json:"medications" db:"medications"`
	MemberCount int            `json:"member_count" db:"member_count"`
	IsActive    bool           `json:"is_active" db:"is_active"`
	CreatedAt   time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at" db:"updated_at"`
}

// Citation represents a reference to authoritative source
type Citation struct {
	ID                 string        `json:"id" db:"id"`
	CitationType       CitationType  `json:"citation_type" db:"citation_type"`
	SourceTable        string        `json:"source_table" db:"source_table"`
	SourceID           string        `json:"source_id" db:"source_id"`
	AuthorityName      string        `json:"authority_name" db:"authority_name"`
	AuthorityType      AuthorityType `json:"authority_type" db:"authority_type"`
	PublicationTitle   string        `json:"publication_title" db:"publication_title"`
	PublicationSection *string       `json:"publication_section" db:"publication_section"`
	PublicationDate    *time.Time    `json:"publication_date" db:"publication_date"`
	URL                *string       `json:"url" db:"url"`
	Excerpt            *string       `json:"excerpt" db:"excerpt"`
	RetrievedAt        time.Time     `json:"retrieved_at" db:"retrieved_at"`
	CreatedAt          time.Time     `json:"created_at" db:"created_at"`
}

// AlertConfidenceFactor represents a factor contributing to alert confidence
type AlertConfidenceFactor struct {
	ID                     string     `json:"id" db:"id"`
	AlertID                string     `json:"alert_id" db:"alert_id"`
	FactorOrder            int        `json:"factor_order" db:"factor_order"`
	FactorType             FactorType `json:"factor_type" db:"factor_type"`
	Description            string     `json:"description" db:"description"`
	Label                  *string    `json:"label" db:"label"`
	Icon                   *string    `json:"icon" db:"icon"`
	Score                  float64    `json:"score" db:"score"`
	Weight                 float64    `json:"weight" db:"weight"`
	Contribution           float64    `json:"contribution" db:"contribution"`
	CitationID             *string    `json:"citation_id" db:"citation_id"`
	CohortID               *string    `json:"cohort_id" db:"cohort_id"`
	FamilyPatternID        *string    `json:"family_pattern_id" db:"family_pattern_id"`
	CohortMatchCriteria    JSONMap    `json:"cohort_match_criteria" db:"cohort_match_criteria"`
	CohortSampleSize       *int       `json:"cohort_sample_size" db:"cohort_sample_size"`
	CohortConfirmationRate *float64   `json:"cohort_confirmation_rate" db:"cohort_confirmation_rate"`
	FamilyHistoryInstances JSONArray  `json:"family_history_instances" db:"family_history_instances"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`

	// Joined fields for display
	Citation      *Citation      `json:"citation,omitempty"`
	Cohort        *Cohort        `json:"cohort,omitempty"`
	FamilyPattern *FamilyPattern `json:"family_pattern,omitempty"`
}

// AlertAnalysisDetails stores complete methodology for an alert
type AlertAnalysisDetails struct {
	ID                 string    `json:"id" db:"id"`
	AlertID            string    `json:"alert_id" db:"alert_id"`
	ParentView         JSONMap   `json:"parent_view" db:"parent_view"`
	ClinicalView       JSONMap   `json:"clinical_view" db:"clinical_view"`
	DataPointsUsed     *int      `json:"data_points_used" db:"data_points_used"`
	AnalysisWindowDays *int      `json:"analysis_window_days" db:"analysis_window_days"`
	AlgorithmVersion   *string   `json:"algorithm_version" db:"algorithm_version"`
	ProcessingTimeMs   *int      `json:"processing_time_ms" db:"processing_time_ms"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// AlertCohortMatching records how cohorts were matched for an alert
type AlertCohortMatching struct {
	ID                  string    `json:"id" db:"id"`
	AlertID             string    `json:"alert_id" db:"alert_id"`
	MatchedCohortID     string    `json:"matched_cohort_id" db:"matched_cohort_id"`
	CriteriaUsed        JSONMap   `json:"criteria_used" db:"criteria_used"`
	CriteriaExcluded    JSONMap   `json:"criteria_excluded" db:"criteria_excluded"`
	CohortSize          int       `json:"cohort_size" db:"cohort_size"`
	PatternPresentations int       `json:"pattern_presentations" db:"pattern_presentations"`
	PatternConfirmations int       `json:"pattern_confirmations" db:"pattern_confirmations"`
	PatternDenials      int       `json:"pattern_denials" db:"pattern_denials"`
	PatternNoResponse   int       `json:"pattern_no_response" db:"pattern_no_response"`
	ConfirmationRate    float64   `json:"confirmation_rate" db:"confirmation_rate"`
	ConfirmationTrend   JSONArray `json:"confirmation_trend" db:"confirmation_trend"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`

	// Joined field
	Cohort *Cohort `json:"cohort,omitempty"`
}

// TreatmentChange represents a change to a child's treatment
type TreatmentChange struct {
	ID                              string              `json:"id" db:"id"`
	ChildID                         string              `json:"child_id" db:"child_id"`
	ChangeType                      ChangeType          `json:"change_type" db:"change_type"`
	SourceTable                     string              `json:"source_table" db:"source_table"`
	SourceID                        string              `json:"source_id" db:"source_id"`
	PreviousValue                   JSONMap             `json:"previous_value" db:"previous_value"`
	NewValue                        JSONMap             `json:"new_value" db:"new_value"`
	ChangeSummary                   string              `json:"change_summary" db:"change_summary"`
	ChangedByUserID                 string              `json:"changed_by_user_id" db:"changed_by_user_id"`
	PotentiallyRelatedAlertID       *string             `json:"potentially_related_alert_id" db:"potentially_related_alert_id"`
	PotentiallyRelatedShareThreadID *string             `json:"potentially_related_share_thread_id" db:"potentially_related_share_thread_id"`
	DaysSinceAnalysisShared         *int                `json:"days_since_analysis_shared" db:"days_since_analysis_shared"`
	InterrogativeStatus             InterrogativeStatus `json:"interrogative_status" db:"interrogative_status"`
	InterrogativePromptedAt         *time.Time          `json:"interrogative_prompted_at" db:"interrogative_prompted_at"`
	InterrogativeAnsweredAt         *time.Time          `json:"interrogative_answered_at" db:"interrogative_answered_at"`
	CreatedAt                       time.Time           `json:"created_at" db:"created_at"`

	// Joined fields
	RelatedAlert *Alert `json:"related_alert,omitempty"`
}

// TreatmentChangeResponse represents a user's response to treatment change questions
type TreatmentChangeResponse struct {
	ID                 string            `json:"id" db:"id"`
	TreatmentChangeID  string            `json:"treatment_change_id" db:"treatment_change_id"`
	RespondedByUserID  string            `json:"responded_by_user_id" db:"responded_by_user_id"`
	ChangeSource       ChangeSource      `json:"change_source" db:"change_source"`
	RelatedToAnalysis  *AnalysisRelation `json:"related_to_analysis" db:"related_to_analysis"`
	ProviderUserID     *string           `json:"provider_user_id" db:"provider_user_id"`
	ProviderNameFreetext *string         `json:"provider_name_freetext" db:"provider_name_freetext"`
	Notes              *string           `json:"notes" db:"notes"`
	CreatedAt          time.Time         `json:"created_at" db:"created_at"`
}

// UserInteractionPreferences stores user preferences for interrogative prompts
type UserInteractionPreferences struct {
	ID                              string    `json:"id" db:"id"`
	UserID                          string    `json:"user_id" db:"user_id"`
	TreatmentChangePromptDelayHours int       `json:"treatment_change_prompt_delay_hours" db:"treatment_change_prompt_delay_hours"`
	InterrogativeQuietStart         *string   `json:"interrogative_quiet_start" db:"interrogative_quiet_start"`
	InterrogativeQuietEnd           *string   `json:"interrogative_quiet_end" db:"interrogative_quiet_end"`
	InterrogativePreferredDays      []int     `json:"interrogative_preferred_days" db:"interrogative_preferred_days"`
	BatchInterrogatives             bool      `json:"batch_interrogatives" db:"batch_interrogatives"`
	MaxInterrogativesPerDay         int       `json:"max_interrogatives_per_day" db:"max_interrogatives_per_day"`
	InterrogativeReminderHours      *int      `json:"interrogative_reminder_hours" db:"interrogative_reminder_hours"`
	MaxInterrogativeReminders       int       `json:"max_interrogative_reminders" db:"max_interrogative_reminders"`
	CreatedAt                       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                       time.Time `json:"updated_at" db:"updated_at"`
}

// AlertExport tracks exports and shares of alerts
type AlertExport struct {
	ID               string      `json:"id" db:"id"`
	AlertID          string      `json:"alert_id" db:"alert_id"`
	ExportedByUserID string      `json:"exported_by_user_id" db:"exported_by_user_id"`
	ExportType       ExportType  `json:"export_type" db:"export_type"`
	SharedWithUserID *string     `json:"shared_with_user_id" db:"shared_with_user_id"`
	SharedVia        *ShareMethod `json:"shared_via" db:"shared_via"`
	ViewMode         AnalysisView `json:"view_mode" db:"view_mode"`
	CreatedAt        time.Time   `json:"created_at" db:"created_at"`
}

// ChatAttachment represents an attachment to a chat message
type ChatAttachment struct {
	ID             string         `json:"id" db:"id"`
	MessageID      string         `json:"message_id" db:"message_id"`
	AttachmentType AttachmentType `json:"attachment_type" db:"attachment_type"`
	SnapshotData   JSONMap        `json:"snapshot_data" db:"snapshot_data"`
	FileName       *string        `json:"file_name" db:"file_name"`
	FileSizeBytes  *int           `json:"file_size_bytes" db:"file_size_bytes"`
	MimeType       *string        `json:"mime_type" db:"mime_type"`
	StoragePath    *string        `json:"storage_path" db:"storage_path"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
}

// FullAlertAnalysis combines all transparency data for an alert
type FullAlertAnalysis struct {
	Alert             *Alert                   `json:"alert"`
	AnalysisDetails   *AlertAnalysisDetails    `json:"analysis_details"`
	ConfidenceFactors []AlertConfidenceFactor  `json:"confidence_factors"`
	CohortMatching    []AlertCohortMatching    `json:"cohort_matching"`
	ChildName         string                   `json:"child_name"`
}

// ConfidenceBreakdown is a simplified view of confidence factors for Layer 2
type ConfidenceBreakdown struct {
	OverallConfidence float64                `json:"overall_confidence"`
	Factors           []ConfidenceFactorSummary `json:"factors"`
}

// ConfidenceFactorSummary is a summary of a confidence factor
type ConfidenceFactorSummary struct {
	Icon         string  `json:"icon"`
	Label        string  `json:"label"`
	Percentage   float64 `json:"percentage"`
	Description  string  `json:"description"`
	FactorType   string  `json:"factor_type"`
}

// TreatmentChangePromptResponse is the response for treatment change prompts
type TreatmentChangePromptResponse struct {
	ChangeSource      ChangeSource      `json:"change_source"`
	RelatedToAnalysis *AnalysisRelation `json:"related_to_analysis,omitempty"`
	ProviderName      string            `json:"provider_name,omitempty"`
	Notes             string            `json:"notes,omitempty"`
}

// PendingInterrogative represents a pending treatment change question
type PendingInterrogative struct {
	TreatmentChange *TreatmentChange `json:"treatment_change"`
	RelatedAlert    *Alert           `json:"related_alert,omitempty"`
	ChildName       string           `json:"child_name"`
}
