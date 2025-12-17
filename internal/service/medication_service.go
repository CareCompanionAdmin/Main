package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrMedicationNotFound = errors.New("medication not found")
)

type MedicationService struct {
	medRepo          repository.MedicationRepository
	transparencyRepo *repository.TransparencyRepository
}

func NewMedicationService(medRepo repository.MedicationRepository, transparencyRepo *repository.TransparencyRepository) *MedicationService {
	return &MedicationService{
		medRepo:          medRepo,
		transparencyRepo: transparencyRepo,
	}
}

func (s *MedicationService) Create(ctx context.Context, childID uuid.UUID, req *models.CreateMedicationRequest) (*models.Medication, error) {
	med := &models.Medication{
		ChildID:    childID,
		Name:       req.Name,
		Dosage:     req.Dosage,
		DosageUnit: req.DosageUnit,
		Frequency:  req.Frequency,
	}
	med.Instructions.String = req.Instructions
	med.Instructions.Valid = req.Instructions != ""
	med.Prescriber.String = req.Prescriber
	med.Prescriber.Valid = req.Prescriber != ""
	med.Pharmacy.String = req.Pharmacy
	med.Pharmacy.Valid = req.Pharmacy != ""

	if req.StartDate != nil {
		med.StartDate.Time = *req.StartDate
		med.StartDate.Valid = true
	}

	// Check if there's a reference medication
	ref, _ := s.medRepo.GetMedicationReference(ctx, req.Name)
	if ref != nil {
		med.ReferenceID = models.NullUUID{UUID: ref.ID, Valid: true}
	}

	if err := s.medRepo.Create(ctx, med); err != nil {
		return nil, err
	}

	// Create schedules if provided
	for _, schedReq := range req.Schedules {
		schedule := &models.MedicationSchedule{
			MedicationID: med.ID,
			TimeOfDay:    schedReq.TimeOfDay,
			DaysOfWeek:   schedReq.DaysOfWeek,
		}
		schedule.ScheduledTime.String = schedReq.ScheduledTime
		schedule.ScheduledTime.Valid = schedReq.ScheduledTime != ""
		if err := s.medRepo.CreateSchedule(ctx, schedule); err != nil {
			return nil, err
		}
		med.Schedules = append(med.Schedules, *schedule)
	}

	return med, nil
}

func (s *MedicationService) GetByID(ctx context.Context, id uuid.UUID) (*models.Medication, error) {
	med, err := s.medRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if med == nil {
		return nil, ErrMedicationNotFound
	}
	return med, nil
}

func (s *MedicationService) GetByChildID(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.Medication, error) {
	return s.medRepo.GetByChildID(ctx, childID, activeOnly)
}

func (s *MedicationService) Update(ctx context.Context, med *models.Medication) error {
	return s.medRepo.Update(ctx, med)
}

// UpdateWithTracking updates a medication and creates treatment change records for significant changes
func (s *MedicationService) UpdateWithTracking(ctx context.Context, oldMed *models.Medication, newMed *models.Medication, userID uuid.UUID) error {
	// Check for dosage changes
	if oldMed.Dosage != newMed.Dosage || oldMed.DosageUnit != newMed.DosageUnit {
		if s.transparencyRepo != nil {
			tc := &models.TreatmentChange{
				ChildID:         newMed.ChildID.String(),
				ChangeType:      models.ChangeTypeMedicationDoseChanged,
				SourceTable:     "medications",
				SourceID:        newMed.ID.String(),
				PreviousValue: models.JSONMap{
					"dosage":      oldMed.Dosage,
					"dosage_unit": oldMed.DosageUnit,
				},
				NewValue: models.JSONMap{
					"dosage":      newMed.Dosage,
					"dosage_unit": newMed.DosageUnit,
				},
				ChangeSummary:   fmt.Sprintf("Changed %s dosage from %s %s to %s %s", newMed.Name, oldMed.Dosage, oldMed.DosageUnit, newMed.Dosage, newMed.DosageUnit),
				ChangedByUserID: userID.String(),
			}
			if err := s.transparencyRepo.CreateTreatmentChange(ctx, tc); err != nil {
				// Log but don't fail the update
				fmt.Printf("Warning: Failed to create treatment change record: %v\n", err)
			}
		}
	}

	// Check for frequency changes
	if oldMed.Frequency != newMed.Frequency {
		if s.transparencyRepo != nil {
			tc := &models.TreatmentChange{
				ChildID:     newMed.ChildID.String(),
				ChangeType:  models.ChangeTypeMedicationScheduleChanged,
				SourceTable: "medications",
				SourceID:    newMed.ID.String(),
				PreviousValue: models.JSONMap{
					"frequency": oldMed.Frequency,
				},
				NewValue: models.JSONMap{
					"frequency": newMed.Frequency,
				},
				ChangeSummary:   fmt.Sprintf("Changed %s frequency from %s to %s", newMed.Name, oldMed.Frequency, newMed.Frequency),
				ChangedByUserID: userID.String(),
			}
			if err := s.transparencyRepo.CreateTreatmentChange(ctx, tc); err != nil {
				fmt.Printf("Warning: Failed to create treatment change record: %v\n", err)
			}
		}
	}

	return s.medRepo.Update(ctx, newMed)
}

func (s *MedicationService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.medRepo.Delete(ctx, id)
}

func (s *MedicationService) Discontinue(ctx context.Context, id uuid.UUID) error {
	return s.DiscontinueWithTracking(ctx, id, uuid.Nil)
}

// DiscontinueWithTracking discontinues a medication and creates a treatment change record
func (s *MedicationService) DiscontinueWithTracking(ctx context.Context, id uuid.UUID, userID uuid.UUID) error {
	med, err := s.medRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if med == nil {
		return ErrMedicationNotFound
	}

	// Create treatment change record before discontinuing
	if s.transparencyRepo != nil && userID != uuid.Nil {
		tc := &models.TreatmentChange{
			ChildID:     med.ChildID.String(),
			ChangeType:  models.ChangeTypeMedicationDiscontinued,
			SourceTable: "medications",
			SourceID:    med.ID.String(),
			PreviousValue: models.JSONMap{
				"is_active":   true,
				"name":        med.Name,
				"dosage":      med.Dosage,
				"dosage_unit": med.DosageUnit,
				"frequency":   med.Frequency,
			},
			NewValue: models.JSONMap{
				"is_active": false,
				"end_date":  time.Now().Format("2006-01-02"),
			},
			ChangeSummary:   fmt.Sprintf("Discontinued %s (%s %s, %s)", med.Name, med.Dosage, med.DosageUnit, med.Frequency),
			ChangedByUserID: userID.String(),
		}
		if err := s.transparencyRepo.CreateTreatmentChange(ctx, tc); err != nil {
			fmt.Printf("Warning: Failed to create treatment change record: %v\n", err)
		}
	}

	med.IsActive = false
	med.EndDate.Time = time.Now()
	med.EndDate.Valid = true
	return s.medRepo.Update(ctx, med)
}

// Schedule management
func (s *MedicationService) AddSchedule(ctx context.Context, medicationID uuid.UUID, req *models.CreateScheduleRequest) (*models.MedicationSchedule, error) {
	schedule := &models.MedicationSchedule{
		MedicationID: medicationID,
		TimeOfDay:    req.TimeOfDay,
		DaysOfWeek:   req.DaysOfWeek,
	}
	schedule.ScheduledTime.String = req.ScheduledTime
	schedule.ScheduledTime.Valid = req.ScheduledTime != ""
	if err := s.medRepo.CreateSchedule(ctx, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

func (s *MedicationService) GetSchedules(ctx context.Context, medicationID uuid.UUID) ([]models.MedicationSchedule, error) {
	return s.medRepo.GetSchedules(ctx, medicationID)
}

func (s *MedicationService) UpdateSchedule(ctx context.Context, schedule *models.MedicationSchedule) error {
	return s.medRepo.UpdateSchedule(ctx, schedule)
}

func (s *MedicationService) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return s.medRepo.DeleteSchedule(ctx, id)
}

// Logging
func (s *MedicationService) LogMedication(ctx context.Context, childID, loggedBy uuid.UUID, req *models.LogMedicationRequest) (*models.MedicationLog, error) {
	log := &models.MedicationLog{
		MedicationID: req.MedicationID,
		ChildID:      childID,
		LogDate:      req.LogDate,
		Status:       req.Status,
		LoggedBy:     loggedBy,
	}
	log.ActualTime.String = req.ActualTime
	log.ActualTime.Valid = req.ActualTime != ""
	log.DosageGiven.String = req.DosageGiven
	log.DosageGiven.Valid = req.DosageGiven != ""
	log.Notes.String = req.Notes
	log.Notes.Valid = req.Notes != ""

	if req.ScheduleID != nil {
		log.ScheduleID.UUID = *req.ScheduleID
		log.ScheduleID.Valid = true
	}

	if err := s.medRepo.CreateLog(ctx, log); err != nil {
		return nil, err
	}

	return log, nil
}

func (s *MedicationService) GetLogs(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error) {
	return s.medRepo.GetLogs(ctx, childID, startDate, endDate)
}

func (s *MedicationService) GetLogsByMedication(ctx context.Context, medicationID uuid.UUID, startDate, endDate time.Time) ([]models.MedicationLog, error) {
	return s.medRepo.GetLogsByMedication(ctx, medicationID, startDate, endDate)
}

func (s *MedicationService) UpdateLog(ctx context.Context, log *models.MedicationLog) error {
	return s.medRepo.UpdateLog(ctx, log)
}

// Due medications
func (s *MedicationService) GetDueMedications(ctx context.Context, childID uuid.UUID, date time.Time) ([]models.MedicationDue, error) {
	return s.medRepo.GetDueMedications(ctx, childID, date)
}

func (s *MedicationService) GetTodaysDueMedications(ctx context.Context, childID uuid.UUID) ([]models.MedicationDue, error) {
	return s.medRepo.GetDueMedications(ctx, childID, time.Now())
}

// Reference data
func (s *MedicationService) SearchMedicationReferences(ctx context.Context, query string) ([]models.MedicationReference, error) {
	return s.medRepo.SearchMedicationReferences(ctx, query)
}

func (s *MedicationService) GetMedicationReference(ctx context.Context, name string) (*models.MedicationReference, error) {
	return s.medRepo.GetMedicationReference(ctx, name)
}

// Adherence calculation
func (s *MedicationService) CalculateAdherence(ctx context.Context, childID uuid.UUID, startDate, endDate time.Time) (float64, error) {
	logs, err := s.medRepo.GetLogs(ctx, childID, startDate, endDate)
	if err != nil {
		return 0, err
	}

	if len(logs) == 0 {
		return 100, nil // No medications logged = 100% adherence (nothing to take)
	}

	taken := 0
	for _, log := range logs {
		if log.Status == models.LogStatusTaken {
			taken++
		}
	}

	return float64(taken) / float64(len(logs)) * 100, nil
}
