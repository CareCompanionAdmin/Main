package service

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
)

// TestBuildProfileContext_NoPHILeaks constructs a realistic child profile
// with identifying data and asserts that the de-identified output the
// LLM sees does NOT contain any of the original identifiers. This is the
// canary test for Phase 1 — if anyone changes buildProfileContext in a
// way that leaks PHI again, this fails loudly.
func TestBuildProfileContext_NoPHILeaks(t *testing.T) {
	dob := time.Date(2018, 3, 15, 0, 0, 0, 0, time.UTC) // realistic 7-8yo
	child := models.Child{
		ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		FamilyID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		FirstName:   "Sarah",
		LastName:    models.NullString{NullString: sql.NullString{String: "Smith", Valid: true}},
		DateOfBirth: dob,
		Gender:      models.NullString{NullString: sql.NullString{String: "female", Valid: true}},
		IsActive:    true,
	}
	conditions := []models.ChildCondition{
		{
			ID:            uuid.New(),
			ChildID:       child.ID,
			ConditionName: "Autism Spectrum Disorder",
			ICDCode:       models.NullString{NullString: sql.NullString{String: "F84.0", Valid: true}},
			Severity:      models.NullString{NullString: sql.NullString{String: "moderate", Valid: true}},
			IsActive:      true,
		},
		{
			ID:            uuid.New(),
			ChildID:       child.ID,
			ConditionName: "ADHD",
			ICDCode:       models.NullString{NullString: sql.NullString{String: "F90.0", Valid: true}},
			IsActive:      true,
		},
	}
	medications := []models.Medication{
		{Name: "Risperdal", Dosage: "0.5", DosageUnit: "mg", IsActive: true},
		{Name: "Adderall", Dosage: "10", DosageUnit: "mg", IsActive: true},
		{Name: "Melatonin", Dosage: "3", DosageUnit: "mg", IsActive: true},
	}

	svc := &AIInsightService{config: &config.ClaudeConfig{LookbackDays: 7}}
	out := svc.buildProfileContext(child, conditions, medications)
	t.Logf("De-identified profile context:\n%s", out)

	// Identifying values that MUST NOT appear in the output.
	forbidden := []string{
		"Sarah", "Smith",
		"2018-03-15", "2018", "March", "Mar",
		"F84.0", "F90.0",
		"Risperdal", "risperdal", "risperidone",
		"Adderall", "adderall", "amphetamine",
		"0.5", "10mg", "10 mg", "3mg", // dosages
	}
	for _, f := range forbidden {
		if strings.Contains(out, f) {
			t.Errorf("forbidden PHI %q leaked into output: %q", f, out)
		}
	}

	// De-identified replacements that SHOULD appear.
	// Note: DOB 2018-03-15 → age 8 in May 2026 → band "8-9y".
	required := []string{
		NamePlaceholder,
		"8-9y",            // age band — child is 8 in May 2026
		"autism-spectrum", // F84.0
		"adhd",            // F90.0
		"antipsychotic",   // Risperdal
		"stimulant",       // Adderall
		"melatonin",       // melatonin
	}
	for _, r := range required {
		if !strings.Contains(out, r) {
			t.Errorf("expected de-identified token %q missing from output: %q", r, out)
		}
	}
}

// TestBuildLogContext_NoPHILeaks_NoFreeText constructs realistic log
// data with free-text fields and asserts the free text never appears
// in the outbound prompt.
func TestBuildLogContext_NoPHILeaks_NoFreeText(t *testing.T) {
	child := models.Child{
		ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		FirstName: "Sarah",
	}

	now := time.Now()
	mood := 7
	energy := 5
	anxiety := 8
	sleepMin := 540
	bristol := 4

	identifyingNote := "Had a birthday party at grandma's house in Dallas — that's why mood dropped"
	identifyingTherapyNote := "Tantrum during OT session at Childcare Center. Therapist Jenny noted regression."
	identifyingHealthEvent := "Pediatrician Dr. Williams said Sarah needs allergy testing next week."

	logs := &models.DailyLogPage{
		BehaviorLogs: []models.BehaviorLog{
			{
				LogDate:             now.AddDate(0, 0, -3),
				MoodLevel:           &mood,
				EnergyLevel:         &energy,
				AnxietyLevel:        &anxiety,
				Meltdowns:           2,
				StimmingEpisodes:    5,
				AggressionIncidents: 1,
				Notes:               models.NullString{NullString: sql.NullString{String: identifyingNote, Valid: true}},
			},
		},
		SleepLogs: []models.SleepLog{
			{
				LogDate:           now.AddDate(0, 0, -1),
				TotalSleepMinutes: &sleepMin,
				NightWakings:      2,
				Nightmares:        true,
			},
		},
		MedicationLogs: []models.MedicationLog{
			{LogDate: now.AddDate(0, 0, -1), MedicationName: "Risperdal", Status: "taken"},
			{LogDate: now.AddDate(0, 0, -2), MedicationName: "Adderall", Status: "missed"},
			{LogDate: now.AddDate(0, 0, -3), MedicationName: "Melatonin", Status: "missed"},
		},
		TherapyLogs: []models.TherapyLog{
			{
				LogDate:         now.AddDate(0, 0, -5),
				TherapyType:     models.NullString{NullString: sql.NullString{String: "OT", Valid: true}},
				DurationMinutes: intPtr(45),
				ProgressNotes:   models.NullString{NullString: sql.NullString{String: identifyingTherapyNote, Valid: true}},
			},
		},
		HealthEventLogs: []models.HealthEventLog{
			{
				LogDate:     now.AddDate(0, 0, -2),
				EventType:   models.NullString{NullString: sql.NullString{String: "doctor_visit", Valid: true}},
				Description: models.NullString{NullString: sql.NullString{String: identifyingHealthEvent, Valid: true}},
			},
		},
		BowelLogs: []models.BowelLog{
			{LogDate: now.AddDate(0, 0, -1), BristolScale: &bristol, HadAccident: false},
		},
	}

	svc := &AIInsightService{config: &config.ClaudeConfig{LookbackDays: 7}}
	out := svc.buildLogContext(child, logs)
	t.Logf("De-identified log context:\n%s", out)

	forbidden := []string{
		"Sarah", "Smith",
		"grandma", "Dallas", "birthday party",
		"Jenny", "Childcare Center", "Tantrum",
		"Williams", "allergy testing",
		"Risperdal", "Adderall", "Melatonin",
	}
	for _, f := range forbidden {
		if strings.Contains(out, f) {
			t.Errorf("forbidden PHI %q leaked into log context: %q", f, out)
		}
	}

	// Risperdal was logged "taken" so its class doesn't appear in the
	// missed-meds aggregation (only missed Adderall + Melatonin do).
	required := []string{
		NamePlaceholder,
		"Day-",        // relative day labels
		"mood=7/10",   // numerical data preserved
		"meltdowns=2", // numerical data preserved
		"stimulant",   // missed Adderall → class
		"melatonin",   // missed Melatonin → class
	}
	for _, r := range required {
		if !strings.Contains(out, r) {
			t.Errorf("expected de-identified token %q missing from log context: %q", r, out)
		}
	}

	// Behavior log "notes" field must NOT be present at all.
	if strings.Contains(out, "notes=") {
		t.Errorf("free-text notes leaked into output — should be dropped: %q", out)
	}
}

func intPtr(i int) *int { return &i }
