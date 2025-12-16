package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Custom types for nullable fields
type NullString struct {
	sql.NullString
}

func (ns NullString) MarshalJSON() ([]byte, error) {
	if ns.Valid {
		return json.Marshal(ns.String)
	}
	return json.Marshal(nil)
}

func (ns *NullString) UnmarshalJSON(data []byte) error {
	var s *string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s != nil {
		ns.Valid = true
		ns.String = *s
	} else {
		ns.Valid = false
	}
	return nil
}

type NullTime struct {
	sql.NullTime
}

func (nt NullTime) MarshalJSON() ([]byte, error) {
	if nt.Valid {
		return json.Marshal(nt.Time)
	}
	return json.Marshal(nil)
}

func (nt *NullTime) UnmarshalJSON(data []byte) error {
	var t *time.Time
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	if t != nil {
		nt.Valid = true
		nt.Time = *t
	} else {
		nt.Valid = false
	}
	return nil
}

type NullUUID struct {
	UUID  uuid.UUID
	Valid bool
}

func (nu NullUUID) Value() (driver.Value, error) {
	if !nu.Valid {
		return nil, nil
	}
	return nu.UUID.String(), nil
}

func (nu *NullUUID) Scan(value interface{}) error {
	if value == nil {
		nu.UUID, nu.Valid = uuid.Nil, false
		return nil
	}
	nu.Valid = true
	switch v := value.(type) {
	case []byte:
		return nu.UUID.UnmarshalText(v)
	case string:
		return nu.UUID.UnmarshalText([]byte(v))
	}
	return nil
}

func (nu NullUUID) MarshalJSON() ([]byte, error) {
	if nu.Valid {
		return json.Marshal(nu.UUID)
	}
	return json.Marshal(nil)
}

// JSONB type for PostgreSQL JSONB columns
type JSONB map[string]interface{}

func (j JSONB) Value() (driver.Value, error) {
	return json.Marshal(j)
}

func (j *JSONB) Scan(value interface{}) error {
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

// StringArray for PostgreSQL TEXT[] columns
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return "{" + stringArrayToString(a) + "}", nil
}

func stringArrayToString(arr []string) string {
	result := ""
	for i, s := range arr {
		if i > 0 {
			result += ","
		}
		result += "\"" + s + "\""
	}
	return result
}

func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	str := string(bytes)
	if str == "{}" {
		*a = []string{}
		return nil
	}
	str = str[1 : len(str)-1]
	*a = parseStringArray(str)
	return nil
}

func parseStringArray(s string) []string {
	if s == "" {
		return []string{}
	}
	var result []string
	var current string
	inQuote := false
	for _, c := range s {
		switch c {
		case '"':
			inQuote = !inQuote
		case ',':
			if !inQuote {
				result = append(result, current)
				current = ""
			} else {
				current += string(c)
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// UUIDArray for PostgreSQL UUID[] columns
type UUIDArray []uuid.UUID

func (a UUIDArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	strs := make([]string, len(a))
	for i, u := range a {
		strs[i] = u.String()
	}
	return "{" + stringArrayToString(strs) + "}", nil
}

func (a *UUIDArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	str := string(bytes)
	if str == "{}" {
		*a = []uuid.UUID{}
		return nil
	}
	str = str[1 : len(str)-1]
	strs := parseStringArray(str)
	result := make([]uuid.UUID, len(strs))
	for i, s := range strs {
		u, err := uuid.Parse(s)
		if err != nil {
			return err
		}
		result[i] = u
	}
	*a = result
	return nil
}

// Enums
type UserStatus string

const (
	UserStatusActive              UserStatus = "active"
	UserStatusInactive            UserStatus = "inactive"
	UserStatusSuspended           UserStatus = "suspended"
	UserStatusPendingVerification UserStatus = "pending_verification"
)

type FamilyRole string

const (
	FamilyRoleParent          FamilyRole = "parent"
	FamilyRoleCaregiver       FamilyRole = "caregiver"
	FamilyRoleMedicalProvider FamilyRole = "medical_provider"
)

type MedicationFrequency string

const (
	MedicationFrequencyOnceDaily       MedicationFrequency = "once_daily"
	MedicationFrequencyTwiceDaily      MedicationFrequency = "twice_daily"
	MedicationFrequencyThreeTimesDaily MedicationFrequency = "three_times_daily"
	MedicationFrequencyFourTimesDaily  MedicationFrequency = "four_times_daily"
	MedicationFrequencyAsNeeded        MedicationFrequency = "as_needed"
	MedicationFrequencyWeekly          MedicationFrequency = "weekly"
	MedicationFrequencyCustom          MedicationFrequency = "custom"
)

type MedicationTimeOfDay string

const (
	MedicationTimeOfDayMorning       MedicationTimeOfDay = "morning"
	MedicationTimeOfDayAfternoon     MedicationTimeOfDay = "afternoon"
	MedicationTimeOfDayEvening       MedicationTimeOfDay = "evening"
	MedicationTimeOfDayNight         MedicationTimeOfDay = "night"
	MedicationTimeOfDayWithBreakfast MedicationTimeOfDay = "with_breakfast"
	MedicationTimeOfDayWithLunch     MedicationTimeOfDay = "with_lunch"
	MedicationTimeOfDayWithDinner    MedicationTimeOfDay = "with_dinner"
	MedicationTimeOfDayBedtime       MedicationTimeOfDay = "bedtime"
)

type LogStatus string

const (
	LogStatusTaken   LogStatus = "taken"
	LogStatusMissed  LogStatus = "missed"
	LogStatusSkipped LogStatus = "skipped"
	LogStatusPartial LogStatus = "partial"
)

type AlertSeverity string

const (
	AlertSeverityInfo     AlertSeverity = "info"
	AlertSeverityWarning  AlertSeverity = "warning"
	AlertSeverityCritical AlertSeverity = "critical"
)

type AlertStatus string

const (
	AlertStatusActive       AlertStatus = "active"
	AlertStatusAcknowledged AlertStatus = "acknowledged"
	AlertStatusResolved     AlertStatus = "resolved"
	AlertStatusDismissed    AlertStatus = "dismissed"
)

type CorrelationType string

const (
	CorrelationTypeGlobalMedical  CorrelationType = "global_medical"
	CorrelationTypeCohortPattern  CorrelationType = "cohort_pattern"
	CorrelationTypeFamilySpecific CorrelationType = "family_specific"
	CorrelationTypeAutomatic      CorrelationType = "automatic"
)

type CorrelationStatus string

const (
	CorrelationStatusPending    CorrelationStatus = "pending"
	CorrelationStatusProcessing CorrelationStatus = "processing"
	CorrelationStatusCompleted  CorrelationStatus = "completed"
	CorrelationStatusFailed     CorrelationStatus = "failed"
)
