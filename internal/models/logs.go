package models

import (
	"time"

	"github.com/google/uuid"
)

// Behavior Log
type BehaviorLog struct {
	ID                  uuid.UUID   `json:"id"`
	ChildID             uuid.UUID   `json:"child_id"`
	LogDate             time.Time   `json:"log_date"`
	LogTime             NullString  `json:"log_time,omitempty"`
	MoodLevel           *int        `json:"mood_level,omitempty"`
	EnergyLevel         *int        `json:"energy_level,omitempty"`
	AnxietyLevel        *int        `json:"anxiety_level,omitempty"`
	Meltdowns           int         `json:"meltdowns"`
	StimmingEpisodes    int         `json:"stimming_episodes"`
	AggressionIncidents int         `json:"aggression_incidents"`
	SelfInjuryIncidents int         `json:"self_injury_incidents"`
	Triggers            StringArray `json:"triggers,omitempty"`
	PositiveBehaviors   StringArray `json:"positive_behaviors,omitempty"`
	Notes               NullString  `json:"notes,omitempty"`
	LoggedBy            uuid.UUID   `json:"logged_by"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
}

// Bowel Log
type BowelLog struct {
	ID           uuid.UUID  `json:"id"`
	ChildID      uuid.UUID  `json:"child_id"`
	LogDate      time.Time  `json:"log_date"`
	LogTime      NullString `json:"log_time,omitempty"`
	BristolScale *int       `json:"bristol_scale,omitempty"`
	HadAccident  bool       `json:"had_accident"`
	PainLevel    *int       `json:"pain_level,omitempty"`
	BloodPresent bool       `json:"blood_present"`
	Notes        NullString `json:"notes,omitempty"`
	LoggedBy     uuid.UUID  `json:"logged_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Speech Log
type SpeechLog struct {
	ID                       uuid.UUID   `json:"id"`
	ChildID                  uuid.UUID   `json:"child_id"`
	LogDate                  time.Time   `json:"log_date"`
	VerbalOutputLevel        *int        `json:"verbal_output_level,omitempty"`
	ClarityLevel             *int        `json:"clarity_level,omitempty"`
	NewWords                 StringArray `json:"new_words,omitempty"`
	LostWords                StringArray `json:"lost_words,omitempty"`
	EcholaliaLevel           *int        `json:"echolalia_level,omitempty"`
	CommunicationAttempts    *int        `json:"communication_attempts,omitempty"`
	SuccessfulCommunications *int        `json:"successful_communications,omitempty"`
	Notes                    NullString  `json:"notes,omitempty"`
	LoggedBy                 uuid.UUID   `json:"logged_by"`
	CreatedAt                time.Time   `json:"created_at"`
}

// Diet Log
type DietLog struct {
	ID               uuid.UUID   `json:"id"`
	ChildID          uuid.UUID   `json:"child_id"`
	LogDate          time.Time   `json:"log_date"`
	MealType         NullString  `json:"meal_type,omitempty"`
	MealTime         NullString  `json:"meal_time,omitempty"`
	FoodsEaten       StringArray `json:"foods_eaten,omitempty"`
	FoodsRefused     StringArray `json:"foods_refused,omitempty"`
	AppetiteLevel    NullString  `json:"appetite_level,omitempty"`
	WaterIntakeOz    *int        `json:"water_intake_oz,omitempty"`
	SupplementsTaken StringArray `json:"supplements_taken,omitempty"`
	NewFoodTried     NullString  `json:"new_food_tried,omitempty"`
	AllergicReaction bool        `json:"allergic_reaction"`
	ReactionDetails  NullString  `json:"reaction_details,omitempty"`
	Notes            NullString  `json:"notes,omitempty"`
	LoggedBy         uuid.UUID   `json:"logged_by"`
	CreatedAt        time.Time   `json:"created_at"`
}

// Weight Log
type WeightLog struct {
	ID           uuid.UUID  `json:"id"`
	ChildID      uuid.UUID  `json:"child_id"`
	LogDate      time.Time  `json:"log_date"`
	WeightLbs    *float64   `json:"weight_lbs,omitempty"`
	HeightInches *float64   `json:"height_inches,omitempty"`
	Notes        NullString `json:"notes,omitempty"`
	LoggedBy     uuid.UUID  `json:"logged_by"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Sleep Log
type SleepLog struct {
	ID                uuid.UUID  `json:"id"`
	ChildID           uuid.UUID  `json:"child_id"`
	LogDate           time.Time  `json:"log_date"`
	Bedtime           NullString `json:"bedtime,omitempty"`
	WakeTime          NullString `json:"wake_time,omitempty"`
	TotalSleepMinutes *int       `json:"total_sleep_minutes,omitempty"`
	NightWakings      int        `json:"night_wakings"`
	SleepQuality      NullString `json:"sleep_quality,omitempty"`
	TookSleepAid      bool       `json:"took_sleep_aid"`
	SleepAidName      NullString `json:"sleep_aid_name,omitempty"`
	Nightmares        bool       `json:"nightmares"`
	BedWetting        bool       `json:"bed_wetting"`
	Notes             NullString `json:"notes,omitempty"`
	LoggedBy          uuid.UUID  `json:"logged_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Sensory Log
type SensoryLog struct {
	ID                       uuid.UUID   `json:"id"`
	ChildID                  uuid.UUID   `json:"child_id"`
	LogDate                  time.Time   `json:"log_date"`
	LogTime                  NullString  `json:"log_time,omitempty"`
	SensorySeekingBehaviors  StringArray `json:"sensory_seeking_behaviors,omitempty"`
	SensoryAvoidingBehaviors StringArray `json:"sensory_avoiding_behaviors,omitempty"`
	OverloadTriggers         StringArray `json:"overload_triggers,omitempty"`
	CalmingStrategiesUsed    StringArray `json:"calming_strategies_used,omitempty"`
	OverloadEpisodes         int         `json:"overload_episodes"`
	OverallRegulation        *int        `json:"overall_regulation,omitempty"`
	Notes                    NullString  `json:"notes,omitempty"`
	LoggedBy                 uuid.UUID   `json:"logged_by"`
	CreatedAt                time.Time   `json:"created_at"`
}

// Social Log
type SocialLog struct {
	ID                     uuid.UUID  `json:"id"`
	ChildID                uuid.UUID  `json:"child_id"`
	LogDate                time.Time  `json:"log_date"`
	EyeContactLevel        *int       `json:"eye_contact_level,omitempty"`
	SocialEngagementLevel  *int       `json:"social_engagement_level,omitempty"`
	PeerInteractions       int        `json:"peer_interactions"`
	PositiveInteractions   int        `json:"positive_interactions"`
	Conflicts              int        `json:"conflicts"`
	ParallelPlayMinutes    *int       `json:"parallel_play_minutes,omitempty"`
	CooperativePlayMinutes *int       `json:"cooperative_play_minutes,omitempty"`
	Notes                  NullString `json:"notes,omitempty"`
	LoggedBy               uuid.UUID  `json:"logged_by"`
	CreatedAt              time.Time  `json:"created_at"`
}

// Therapy Log
type TherapyLog struct {
	ID               uuid.UUID   `json:"id"`
	ChildID          uuid.UUID   `json:"child_id"`
	LogDate          time.Time   `json:"log_date"`
	TherapyType      NullString  `json:"therapy_type,omitempty"`
	TherapistName    NullString  `json:"therapist_name,omitempty"`
	DurationMinutes  *int        `json:"duration_minutes,omitempty"`
	GoalsWorkedOn    StringArray `json:"goals_worked_on,omitempty"`
	ProgressNotes    NullString  `json:"progress_notes,omitempty"`
	HomeworkAssigned NullString  `json:"homework_assigned,omitempty"`
	ParentNotes      NullString  `json:"parent_notes,omitempty"`
	LoggedBy         uuid.UUID   `json:"logged_by"`
	CreatedAt        time.Time   `json:"created_at"`
}

// Seizure Log
type SeizureLog struct {
	ID                    uuid.UUID   `json:"id"`
	ChildID               uuid.UUID   `json:"child_id"`
	LogDate               time.Time   `json:"log_date"`
	LogTime               string      `json:"log_time"`
	SeizureType           NullString  `json:"seizure_type,omitempty"`
	DurationSeconds       *int        `json:"duration_seconds,omitempty"`
	Triggers              StringArray `json:"triggers,omitempty"`
	WarningSigns          StringArray `json:"warning_signs,omitempty"`
	PostIctalSymptoms     StringArray `json:"post_ictal_symptoms,omitempty"`
	RescueMedicationGiven bool        `json:"rescue_medication_given"`
	RescueMedicationName  NullString  `json:"rescue_medication_name,omitempty"`
	Called911             bool        `json:"called_911"`
	Notes                 NullString  `json:"notes,omitempty"`
	LoggedBy              uuid.UUID   `json:"logged_by"`
	CreatedAt             time.Time   `json:"created_at"`
}

// Health Event Log
type HealthEventLog struct {
	ID           uuid.UUID   `json:"id"`
	ChildID      uuid.UUID   `json:"child_id"`
	LogDate      time.Time   `json:"log_date"`
	EventType    NullString  `json:"event_type,omitempty"`
	Description  NullString  `json:"description,omitempty"`
	Symptoms     StringArray `json:"symptoms,omitempty"`
	TemperatureF *float64    `json:"temperature_f,omitempty"`
	ProviderName NullString  `json:"provider_name,omitempty"`
	Diagnosis    NullString  `json:"diagnosis,omitempty"`
	Treatment    NullString  `json:"treatment,omitempty"`
	FollowUpDate NullTime    `json:"follow_up_date,omitempty"`
	Notes        NullString  `json:"notes,omitempty"`
	LoggedBy     uuid.UUID   `json:"logged_by"`
	CreatedAt    time.Time   `json:"created_at"`
}

// Daily Log Page combines all logs for a day
type DailyLogPage struct {
	Child          Child           `json:"child"`
	Date           time.Time       `json:"date"`
	MedicationLogs []MedicationLog `json:"medication_logs"`
	MedicationsDue []MedicationDue `json:"medications_due"`
	BehaviorLogs   []BehaviorLog   `json:"behavior_logs"`
	BowelLogs      []BowelLog      `json:"bowel_logs"`
	SpeechLogs     []SpeechLog     `json:"speech_logs"`
	DietLogs       []DietLog       `json:"diet_logs"`
	WeightLogs     []WeightLog     `json:"weight_logs"`
	SleepLogs      []SleepLog      `json:"sleep_logs"`
}

// Request types for creating logs
type CreateBehaviorLogRequest struct {
	LogDate             time.Time `json:"log_date"`
	LogTime             string    `json:"log_time,omitempty"`
	MoodLevel           *int      `json:"mood_level,omitempty"`
	EnergyLevel         *int      `json:"energy_level,omitempty"`
	AnxietyLevel        *int      `json:"anxiety_level,omitempty"`
	Meltdowns           int       `json:"meltdowns"`
	StimmingEpisodes    int       `json:"stimming_episodes"`
	AggressionIncidents int       `json:"aggression_incidents"`
	SelfInjuryIncidents int       `json:"self_injury_incidents"`
	Triggers            []string  `json:"triggers,omitempty"`
	PositiveBehaviors   []string  `json:"positive_behaviors,omitempty"`
	Notes               string    `json:"notes,omitempty"`
}

type CreateBowelLogRequest struct {
	LogDate      time.Time `json:"log_date"`
	LogTime      string    `json:"log_time,omitempty"`
	BristolScale *int      `json:"bristol_scale,omitempty"`
	HadAccident  bool      `json:"had_accident"`
	PainLevel    *int      `json:"pain_level,omitempty"`
	BloodPresent bool      `json:"blood_present"`
	Notes        string    `json:"notes,omitempty"`
}

type CreateSpeechLogRequest struct {
	LogDate                  time.Time `json:"log_date"`
	VerbalOutputLevel        *int      `json:"verbal_output_level,omitempty"`
	ClarityLevel             *int      `json:"clarity_level,omitempty"`
	NewWords                 []string  `json:"new_words,omitempty"`
	LostWords                []string  `json:"lost_words,omitempty"`
	EcholaliaLevel           *int      `json:"echolalia_level,omitempty"`
	CommunicationAttempts    *int      `json:"communication_attempts,omitempty"`
	SuccessfulCommunications *int      `json:"successful_communications,omitempty"`
	Notes                    string    `json:"notes,omitempty"`
}

type CreateDietLogRequest struct {
	LogDate          time.Time `json:"log_date"`
	MealType         string    `json:"meal_type,omitempty"`
	MealTime         string    `json:"meal_time,omitempty"`
	FoodsEaten       []string  `json:"foods_eaten,omitempty"`
	FoodsRefused     []string  `json:"foods_refused,omitempty"`
	AppetiteLevel    string    `json:"appetite_level,omitempty"`
	WaterIntakeOz    *int      `json:"water_intake_oz,omitempty"`
	SupplementsTaken []string  `json:"supplements_taken,omitempty"`
	NewFoodTried     string    `json:"new_food_tried,omitempty"`
	AllergicReaction bool      `json:"allergic_reaction"`
	ReactionDetails  string    `json:"reaction_details,omitempty"`
	Notes            string    `json:"notes,omitempty"`
}

type CreateWeightLogRequest struct {
	LogDate      time.Time `json:"log_date"`
	WeightLbs    *float64  `json:"weight_lbs,omitempty"`
	HeightInches *float64  `json:"height_inches,omitempty"`
	Notes        string    `json:"notes,omitempty"`
}

type CreateSleepLogRequest struct {
	LogDate           time.Time `json:"log_date"`
	Bedtime           string    `json:"bedtime,omitempty"`
	WakeTime          string    `json:"wake_time,omitempty"`
	TotalSleepMinutes *int      `json:"total_sleep_minutes,omitempty"`
	NightWakings      int       `json:"night_wakings"`
	SleepQuality      string    `json:"sleep_quality,omitempty"`
	TookSleepAid      bool      `json:"took_sleep_aid"`
	SleepAidName      string    `json:"sleep_aid_name,omitempty"`
	Nightmares        bool      `json:"nightmares"`
	BedWetting        bool      `json:"bed_wetting"`
	Notes             string    `json:"notes,omitempty"`
}

type CreateSensoryLogRequest struct {
	LogDate                  time.Time `json:"log_date"`
	LogTime                  string    `json:"log_time,omitempty"`
	SensorySeekingBehaviors  []string  `json:"sensory_seeking_behaviors,omitempty"`
	SensoryAvoidingBehaviors []string  `json:"sensory_avoiding_behaviors,omitempty"`
	OverloadTriggers         []string  `json:"overload_triggers,omitempty"`
	CalmingStrategiesUsed    []string  `json:"calming_strategies_used,omitempty"`
	OverloadEpisodes         int       `json:"overload_episodes"`
	OverallRegulation        *int      `json:"overall_regulation,omitempty"`
	Notes                    string    `json:"notes,omitempty"`
}

type CreateSocialLogRequest struct {
	LogDate                time.Time `json:"log_date"`
	EyeContactLevel        *int      `json:"eye_contact_level,omitempty"`
	SocialEngagementLevel  *int      `json:"social_engagement_level,omitempty"`
	PeerInteractions       int       `json:"peer_interactions"`
	PositiveInteractions   int       `json:"positive_interactions"`
	Conflicts              int       `json:"conflicts"`
	ParallelPlayMinutes    *int      `json:"parallel_play_minutes,omitempty"`
	CooperativePlayMinutes *int      `json:"cooperative_play_minutes,omitempty"`
	Notes                  string    `json:"notes,omitempty"`
}

type CreateTherapyLogRequest struct {
	LogDate          time.Time `json:"log_date"`
	TherapyType      string    `json:"therapy_type,omitempty"`
	TherapistName    string    `json:"therapist_name,omitempty"`
	DurationMinutes  *int      `json:"duration_minutes,omitempty"`
	GoalsWorkedOn    []string  `json:"goals_worked_on,omitempty"`
	ProgressNotes    string    `json:"progress_notes,omitempty"`
	HomeworkAssigned string    `json:"homework_assigned,omitempty"`
	ParentNotes      string    `json:"parent_notes,omitempty"`
}

type CreateSeizureLogRequest struct {
	LogDate               time.Time `json:"log_date"`
	LogTime               string    `json:"log_time"`
	SeizureType           string    `json:"seizure_type,omitempty"`
	DurationSeconds       *int      `json:"duration_seconds,omitempty"`
	Triggers              []string  `json:"triggers,omitempty"`
	WarningSigns          []string  `json:"warning_signs,omitempty"`
	PostIctalSymptoms     []string  `json:"post_ictal_symptoms,omitempty"`
	RescueMedicationGiven bool      `json:"rescue_medication_given"`
	RescueMedicationName  string    `json:"rescue_medication_name,omitempty"`
	Called911             bool      `json:"called_911"`
	Notes                 string    `json:"notes,omitempty"`
}

type CreateHealthEventLogRequest struct {
	LogDate      time.Time  `json:"log_date"`
	EventType    string     `json:"event_type,omitempty"`
	Description  string     `json:"description,omitempty"`
	Symptoms     []string   `json:"symptoms,omitempty"`
	TemperatureF *float64   `json:"temperature_f,omitempty"`
	ProviderName string     `json:"provider_name,omitempty"`
	Diagnosis    string     `json:"diagnosis,omitempty"`
	Treatment    string     `json:"treatment,omitempty"`
	FollowUpDate *time.Time `json:"follow_up_date,omitempty"`
	Notes        string     `json:"notes,omitempty"`
}
