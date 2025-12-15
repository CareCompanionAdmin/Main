package models

import (
	"time"

	"github.com/google/uuid"
)

type Child struct {
	ID          uuid.UUID        `json:"id"`
	FamilyID    uuid.UUID        `json:"family_id"`
	FirstName   string           `json:"first_name"`
	LastName    NullString       `json:"last_name,omitempty"`
	DateOfBirth time.Time        `json:"date_of_birth"`
	Gender      NullString       `json:"gender,omitempty"`
	PhotoURL    NullString       `json:"photo_url,omitempty"`
	Notes       NullString       `json:"notes,omitempty"`
	Settings    JSONB            `json:"settings"`
	IsActive    bool             `json:"is_active"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	Conditions  []ChildCondition `json:"conditions,omitempty"`
}

func (c *Child) Age() int {
	now := time.Now()
	years := now.Year() - c.DateOfBirth.Year()
	if now.YearDay() < c.DateOfBirth.YearDay() {
		years--
	}
	return years
}

func (c *Child) FullName() string {
	if c.LastName.Valid {
		return c.FirstName + " " + c.LastName.String
	}
	return c.FirstName
}

type ChildCondition struct {
	ID            uuid.UUID  `json:"id"`
	ChildID       uuid.UUID  `json:"child_id"`
	ConditionName string     `json:"condition_name"`
	ICDCode       NullString `json:"icd_code,omitempty"`
	DiagnosedDate NullTime   `json:"diagnosed_date,omitempty"`
	DiagnosedBy   NullString `json:"diagnosed_by,omitempty"`
	Severity      NullString `json:"severity,omitempty"`
	Notes         NullString `json:"notes,omitempty"`
	IsActive      bool       `json:"is_active"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Request types
type CreateChildRequest struct {
	FirstName   string    `json:"first_name"`
	LastName    string    `json:"last_name,omitempty"`
	DateOfBirth time.Time `json:"date_of_birth"`
	Gender      string    `json:"gender,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Conditions  []string  `json:"conditions,omitempty"`
}

type UpdateChildRequest struct {
	FirstName   *string    `json:"first_name,omitempty"`
	LastName    *string    `json:"last_name,omitempty"`
	DateOfBirth *time.Time `json:"date_of_birth,omitempty"`
	Gender      *string    `json:"gender,omitempty"`
	Notes       *string    `json:"notes,omitempty"`
}

// Dashboard types
type ChildDashboard struct {
	Child          Child           `json:"child"`
	TodayLogs      DailyLogSummary `json:"today_logs"`
	ActiveAlerts   []Alert         `json:"active_alerts"`
	MedicationsDue []MedicationDue `json:"medications_due"`
	RecentPatterns []FamilyPattern `json:"recent_patterns"`
	WeekSummary    WeekSummary     `json:"week_summary"`
}

type DailyLogSummary struct {
	Date              time.Time `json:"date"`
	HasMedicationLog  bool      `json:"has_medication_log"`
	HasBehaviorLog    bool      `json:"has_behavior_log"`
	HasBowelLog       bool      `json:"has_bowel_log"`
	HasSleepLog       bool      `json:"has_sleep_log"`
	MedicationsTaken  int       `json:"medications_taken"`
	MedicationsTotal  int       `json:"medications_total"`
	MoodLevel         *int      `json:"mood_level,omitempty"`
	EnergyLevel       *int      `json:"energy_level,omitempty"`
	Meltdowns         int       `json:"meltdowns"`
	SleepMinutes      *int      `json:"sleep_minutes,omitempty"`
}

type WeekSummary struct {
	DaysLogged          int     `json:"days_logged"`
	AverageMood         float64 `json:"average_mood"`
	MedicationAdherence float64 `json:"medication_adherence"`
	AlertCount          int     `json:"alert_count"`
}
