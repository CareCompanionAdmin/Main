package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// smithFixtures resolves a Smith Test Family child + member user on the dev DB,
// or skips the test if the fixtures are absent. Uses openTestDB from session_repo_test.go.
func smithFixtures(t *testing.T) (childID, userID uuid.UUID) {
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	if err := db.QueryRowContext(ctx, `SELECT id FROM app_users WHERE email='joe_parent1@test.com'`).Scan(&userID); err != nil {
		t.Skipf("test user missing: %v", err)
	}
	if err := db.QueryRowContext(ctx, `
		SELECT c.id FROM children c
		JOIN family_memberships fm ON fm.family_id=c.family_id
		WHERE fm.user_id=$1 LIMIT 1`, userID).Scan(&childID); err != nil {
		t.Skipf("test child missing: %v", err)
	}
	return childID, userID
}

func TestTreatmentChangeEffectiveDate_RoundTripAndEdit(t *testing.T) {
	childID, userID := smithFixtures(t)
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repository.NewTransparencyRepository(db)

	tc := &models.TreatmentChange{
		ChildID:         childID.String(),
		ChangeType:      models.ChangeTypeMedicationDoseChanged,
		SourceTable:     "medications",
		SourceID:        uuid.New().String(),
		ChangeSummary:   "TEST effective-date roundtrip",
		ChangedByUserID: userID.String(),
		EffectiveDate:   "2026-06-20",
	}
	if err := repo.CreateTreatmentChange(ctx, tc); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM treatment_changes WHERE id=$1`, tc.ID)

	if tc.EffectiveDate != "2026-06-20" {
		t.Fatalf("expected effective_date stamped 2026-06-20, got %q", tc.EffectiveDate)
	}

	// Reads on the original date include it.
	got, err := repo.GetTreatmentChangesByDate(ctx, userID.String(), childID.String(), "2026-06-20")
	if err != nil {
		t.Fatalf("get by date: %v", err)
	}
	if !containsChange(got, tc.ID) {
		t.Fatalf("expected change %s in 2026-06-20 results", tc.ID)
	}

	// Edit the date to two days earlier.
	if err := repo.UpdateTreatmentChangeEffectiveDate(ctx, userID.String(), tc.ID, "2026-06-18"); err != nil {
		t.Fatalf("update date: %v", err)
	}
	moved, _ := repo.GetTreatmentChangesByDate(ctx, userID.String(), childID.String(), "2026-06-18")
	if !containsChange(moved, tc.ID) {
		t.Fatalf("expected change in 2026-06-18 after edit")
	}
	stale, _ := repo.GetTreatmentChangesByDate(ctx, userID.String(), childID.String(), "2026-06-20")
	if containsChange(stale, tc.ID) {
		t.Fatalf("change should no longer appear on 2026-06-20 after edit")
	}

	// Authorization: an unrelated random user id cannot edit it.
	if err := repo.UpdateTreatmentChangeEffectiveDate(ctx, uuid.New().String(), tc.ID, "2026-06-15"); err == nil {
		t.Fatalf("expected error editing as a non-family user")
	}
}

func containsChange(list []models.TreatmentChange, id string) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestReconcileSchedules_PreservesLogLinkage(t *testing.T) {
	childID, userID := smithFixtures(t)
	db := openTestDB(t)
	defer db.Close()
	ctx := context.Background()
	repo := repository.NewMedicationRepo(db)

	med := &models.Medication{
		ChildID:    childID,
		Name:       "TEST-Reconcile-Med",
		Dosage:     "5",
		DosageUnit: "mg",
		Frequency:  models.MedicationFrequency("once_daily"),
	}
	if err := repo.Create(ctx, med); err != nil {
		t.Fatalf("create med: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM medications WHERE id=$1`, med.ID)

	// Morning schedule at 08:00.
	morning := &models.MedicationSchedule{MedicationID: med.ID, TimeOfDay: models.MedicationTimeOfDayMorning}
	morning.ScheduledTime.String, morning.ScheduledTime.Valid = "08:00", true
	if err := repo.CreateSchedule(ctx, morning); err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	originalScheduleID := morning.ID

	// Log today's morning dose bound to that schedule.
	today := time.Now()
	logRow := &models.MedicationLog{
		MedicationID: med.ID,
		ChildID:      childID,
		LogDate:      today,
		Status:       models.LogStatusTaken,
		LoggedBy:     userID,
	}
	logRow.ScheduleID.UUID, logRow.ScheduleID.Valid = originalScheduleID, true
	if err := repo.CreateLog(ctx, logRow); err != nil {
		t.Fatalf("create log: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM medication_logs WHERE id=$1`, logRow.ID)

	// User changes the morning time to 09:30 → reconcile (not recreate).
	desired := models.MedicationSchedule{MedicationID: med.ID, TimeOfDay: models.MedicationTimeOfDayMorning}
	desired.ScheduledTime.String, desired.ScheduledTime.Valid = "09:30", true
	if err := repo.ReconcileSchedules(ctx, med.ID, []models.MedicationSchedule{desired}); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	// The active morning schedule must keep its original id.
	scheds, err := repo.GetSchedules(ctx, med.ID)
	if err != nil {
		t.Fatalf("get schedules: %v", err)
	}
	if len(scheds) != 1 {
		t.Fatalf("expected exactly 1 active schedule, got %d", len(scheds))
	}
	if scheds[0].ID != originalScheduleID {
		t.Fatalf("schedule id changed (%s -> %s); log linkage would break", originalScheduleID, scheds[0].ID)
	}
	if got := scheds[0].ScheduledTime.String; got != "09:30:00" && got != "09:30" {
		t.Fatalf("expected updated time 09:30, got %q", got)
	}

	// Due meds for today must show the morning dose as already logged — no duplicate.
	due, err := repo.GetDueMedications(ctx, childID, today)
	if err != nil {
		t.Fatalf("get due: %v", err)
	}
	morningCount, loggedCount := 0, 0
	for _, d := range due {
		if d.Medication.ID == med.ID {
			morningCount++
			if d.IsLogged {
				loggedCount++
			}
		}
	}
	if morningCount != 1 {
		t.Fatalf("expected 1 due row for the med (no double-meds), got %d", morningCount)
	}
	if loggedCount != 1 {
		t.Fatalf("expected the single due row to show IsLogged=true, got %d logged", loggedCount)
	}
}
