package models

import (
	"time"

	"github.com/google/uuid"
)

type MedicationReference struct {
	ID                uuid.UUID   `json:"id"`
	Name              string      `json:"name"`
	GenericName       NullString  `json:"generic_name,omitempty"`
	DrugClass         NullString  `json:"drug_class,omitempty"`
	CommonDosages     StringArray `json:"common_dosages,omitempty"`
	CommonSideEffects StringArray `json:"common_side_effects,omitempty"`
	Warnings          StringArray `json:"warnings,omitempty"`
	Interactions      JSONArray   `json:"interactions,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
}

type Medication struct {
	ID           uuid.UUID           `json:"id"`
	ChildID      uuid.UUID           `json:"child_id"`
	ReferenceID  NullUUID            `json:"reference_id,omitempty"`
	Name         string              `json:"name"`
	Dosage       string              `json:"dosage"`
	DosageUnit   string              `json:"dosage_unit"`
	Frequency    MedicationFrequency `json:"frequency"`
	Instructions NullString          `json:"instructions,omitempty"`
	Prescriber   NullString          `json:"prescriber,omitempty"`
	Pharmacy     NullString          `json:"pharmacy,omitempty"`
	StartDate    NullTime            `json:"start_date,omitempty"`
	EndDate      NullTime            `json:"end_date,omitempty"`
	IsActive     bool                `json:"is_active"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Schedules    []MedicationSchedule `json:"schedules,omitempty"`
}

type MedicationSchedule struct {
	ID            uuid.UUID           `json:"id"`
	MedicationID  uuid.UUID           `json:"medication_id"`
	TimeOfDay     MedicationTimeOfDay `json:"time_of_day"`
	ScheduledTime NullString          `json:"scheduled_time,omitempty"`
	DaysOfWeek    []int               `json:"days_of_week"`
	IsActive      bool                `json:"is_active"`
	CreatedAt     time.Time           `json:"created_at"`
}

type MedicationLog struct {
	ID            uuid.UUID  `json:"id"`
	MedicationID  uuid.UUID  `json:"medication_id"`
	ChildID       uuid.UUID  `json:"child_id"`
	ScheduleID    NullUUID   `json:"schedule_id,omitempty"`
	LogDate       time.Time  `json:"log_date"`
	ScheduledTime NullString `json:"scheduled_time,omitempty"`
	ActualTime    NullString `json:"actual_time,omitempty"`
	Status        LogStatus  `json:"status"`
	DosageGiven   NullString `json:"dosage_given,omitempty"`
	Notes         NullString `json:"notes,omitempty"`
	LoggedBy      uuid.UUID  `json:"logged_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type MedicationDue struct {
	Medication    Medication         `json:"medication"`
	Schedule      MedicationSchedule `json:"schedule"`
	IsLogged      bool               `json:"is_logged"`
	LoggedStatus  LogStatus          `json:"logged_status,omitempty"`
}

// Request types
type CreateMedicationRequest struct {
	Name         string              `json:"name"`
	Dosage       string              `json:"dosage"`
	DosageUnit   string              `json:"dosage_unit"`
	Frequency    MedicationFrequency `json:"frequency"`
	Instructions string              `json:"instructions,omitempty"`
	Prescriber   string              `json:"prescriber,omitempty"`
	Pharmacy     string              `json:"pharmacy,omitempty"`
	StartDate    *time.Time          `json:"start_date,omitempty"`
	Schedules    []CreateScheduleRequest `json:"schedules,omitempty"`
}

type CreateScheduleRequest struct {
	TimeOfDay     MedicationTimeOfDay `json:"time_of_day"`
	ScheduledTime string              `json:"scheduled_time,omitempty"`
	DaysOfWeek    []int               `json:"days_of_week,omitempty"`
}

type LogMedicationRequest struct {
	MedicationID uuid.UUID  `json:"medication_id"`
	ScheduleID   *uuid.UUID `json:"schedule_id,omitempty"`
	LogDate      time.Time  `json:"log_date"`
	Status       LogStatus  `json:"status"`
	ActualTime   string     `json:"actual_time,omitempty"`
	DosageGiven  string     `json:"dosage_given,omitempty"`
	Notes        string     `json:"notes,omitempty"`
}
