package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrChildNotFound = errors.New("child not found")
)

type ChildService struct {
	childRepo  repository.ChildRepository
	familyRepo repository.FamilyRepository
}

func NewChildService(childRepo repository.ChildRepository, familyRepo repository.FamilyRepository) *ChildService {
	return &ChildService{
		childRepo:  childRepo,
		familyRepo: familyRepo,
	}
}

func (s *ChildService) Create(ctx context.Context, familyID uuid.UUID, req *models.CreateChildRequest) (*models.Child, error) {
	child := &models.Child{
		FamilyID:    familyID,
		FirstName:   req.FirstName,
		DateOfBirth: req.DateOfBirth,
	}
	child.LastName.String = req.LastName
	child.LastName.Valid = req.LastName != ""
	child.Gender.String = req.Gender
	child.Gender.Valid = req.Gender != ""
	child.Notes.String = req.Notes
	child.Notes.Valid = req.Notes != ""

	if err := s.childRepo.Create(ctx, child); err != nil {
		return nil, err
	}

	// Add conditions if provided
	for _, conditionName := range req.Conditions {
		condition := &models.ChildCondition{
			ChildID:       child.ID,
			ConditionName: conditionName,
		}
		if err := s.childRepo.AddCondition(ctx, condition); err != nil {
			return nil, err
		}
		child.Conditions = append(child.Conditions, *condition)
	}

	return child, nil
}

func (s *ChildService) GetByID(ctx context.Context, id uuid.UUID) (*models.Child, error) {
	child, err := s.childRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, ErrChildNotFound
	}
	return child, nil
}

func (s *ChildService) GetByFamilyID(ctx context.Context, familyID uuid.UUID) ([]models.Child, error) {
	return s.childRepo.GetByFamilyID(ctx, familyID)
}

func (s *ChildService) Update(ctx context.Context, childID uuid.UUID, req *models.UpdateChildRequest) (*models.Child, error) {
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, ErrChildNotFound
	}

	if req.FirstName != nil {
		child.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		child.LastName.String = *req.LastName
		child.LastName.Valid = *req.LastName != ""
	}
	if req.DateOfBirth != nil {
		child.DateOfBirth = *req.DateOfBirth
	}
	if req.Gender != nil {
		child.Gender.String = *req.Gender
		child.Gender.Valid = *req.Gender != ""
	}
	if req.Notes != nil {
		child.Notes.String = *req.Notes
		child.Notes.Valid = *req.Notes != ""
	}

	if err := s.childRepo.Update(ctx, child); err != nil {
		return nil, err
	}

	return child, nil
}

func (s *ChildService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.childRepo.Delete(ctx, id)
}

func (s *ChildService) AddCondition(ctx context.Context, childID uuid.UUID, conditionName string) (*models.ChildCondition, error) {
	condition := &models.ChildCondition{
		ChildID:       childID,
		ConditionName: conditionName,
	}
	if err := s.childRepo.AddCondition(ctx, condition); err != nil {
		return nil, err
	}
	return condition, nil
}

func (s *ChildService) GetConditions(ctx context.Context, childID uuid.UUID) ([]models.ChildCondition, error) {
	return s.childRepo.GetConditions(ctx, childID)
}

func (s *ChildService) UpdateCondition(ctx context.Context, condition *models.ChildCondition) error {
	return s.childRepo.UpdateCondition(ctx, condition)
}

func (s *ChildService) RemoveCondition(ctx context.Context, conditionID uuid.UUID) error {
	return s.childRepo.RemoveCondition(ctx, conditionID)
}

func (s *ChildService) GetDashboard(ctx context.Context, childID uuid.UUID) (*models.ChildDashboard, error) {
	return s.childRepo.GetDashboard(ctx, childID, time.Now())
}

func (s *ChildService) GetDashboardForDate(ctx context.Context, childID uuid.UUID, date time.Time) (*models.ChildDashboard, error) {
	return s.childRepo.GetDashboard(ctx, childID, date)
}

// VerifyChildAccess checks if a user has access to a child through family membership
func (s *ChildService) VerifyChildAccess(ctx context.Context, childID, userID uuid.UUID) (*models.Child, error) {
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil {
		return nil, err
	}
	if child == nil {
		return nil, ErrChildNotFound
	}

	// Check if user is a member of the child's family
	membership, err := s.familyRepo.GetMembership(ctx, child.FamilyID, userID)
	if err != nil {
		return nil, err
	}
	if membership == nil {
		return nil, ErrNotFamilyMember
	}

	return child, nil
}
