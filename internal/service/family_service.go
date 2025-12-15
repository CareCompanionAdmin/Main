package service

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrFamilyNotFound   = errors.New("family not found")
	ErrNotFamilyMember  = errors.New("not a member of this family")
	ErrInsufficientRole = errors.New("insufficient role for this action")
)

type FamilyService struct {
	familyRepo repository.FamilyRepository
	childRepo  repository.ChildRepository
}

func NewFamilyService(familyRepo repository.FamilyRepository, childRepo repository.ChildRepository) *FamilyService {
	return &FamilyService{
		familyRepo: familyRepo,
		childRepo:  childRepo,
	}
}

func (s *FamilyService) Create(ctx context.Context, name string, creatorID uuid.UUID) (*models.Family, error) {
	family := &models.Family{
		Name: name,
	}

	if err := s.familyRepo.Create(ctx, family); err != nil {
		return nil, err
	}

	// Add creator as parent
	membership := &models.FamilyMembership{
		FamilyID: family.ID,
		UserID:   creatorID,
		Role:     models.FamilyRoleParent,
	}
	if err := s.familyRepo.AddMember(ctx, membership); err != nil {
		return nil, err
	}

	return family, nil
}

func (s *FamilyService) GetByID(ctx context.Context, id uuid.UUID) (*models.Family, error) {
	return s.familyRepo.GetByID(ctx, id)
}

func (s *FamilyService) Update(ctx context.Context, family *models.Family) error {
	return s.familyRepo.Update(ctx, family)
}

func (s *FamilyService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.familyRepo.Delete(ctx, id)
}

func (s *FamilyService) GetMembers(ctx context.Context, familyID uuid.UUID) ([]models.FamilyMembership, error) {
	return s.familyRepo.GetMembers(ctx, familyID)
}

func (s *FamilyService) GetMembership(ctx context.Context, familyID, userID uuid.UUID) (*models.FamilyMembership, error) {
	return s.familyRepo.GetMembership(ctx, familyID, userID)
}

type InviteMemberRequest struct {
	Email string            `json:"email"`
	Role  models.FamilyRole `json:"role"`
}

func (s *FamilyService) AddMember(ctx context.Context, familyID, userID uuid.UUID, role models.FamilyRole) error {
	membership := &models.FamilyMembership{
		FamilyID: familyID,
		UserID:   userID,
		Role:     role,
	}
	return s.familyRepo.AddMember(ctx, membership)
}

func (s *FamilyService) RemoveMember(ctx context.Context, familyID, userID uuid.UUID) error {
	return s.familyRepo.RemoveMember(ctx, familyID, userID)
}

func (s *FamilyService) UpdateMemberRole(ctx context.Context, familyID, userID uuid.UUID, role models.FamilyRole) error {
	return s.familyRepo.UpdateMemberRole(ctx, familyID, userID, role)
}

func (s *FamilyService) GetChildren(ctx context.Context, familyID uuid.UUID) ([]models.Child, error) {
	return s.childRepo.GetByFamilyID(ctx, familyID)
}

// VerifyMembership checks if a user is a member of a family
func (s *FamilyService) VerifyMembership(ctx context.Context, familyID, userID uuid.UUID) (*models.FamilyMembership, error) {
	membership, err := s.familyRepo.GetMembership(ctx, familyID, userID)
	if err != nil {
		return nil, err
	}
	if membership == nil {
		return nil, ErrNotFamilyMember
	}
	return membership, nil
}

// VerifyRole checks if a user has at least the specified role in a family
func (s *FamilyService) VerifyRole(ctx context.Context, familyID, userID uuid.UUID, requiredRole models.FamilyRole) error {
	membership, err := s.VerifyMembership(ctx, familyID, userID)
	if err != nil {
		return err
	}

	if !hasRequiredRole(membership.Role, requiredRole) {
		return ErrInsufficientRole
	}
	return nil
}

// hasRequiredRole checks if the actual role meets the required role level
func hasRequiredRole(actual, required models.FamilyRole) bool {
	roleHierarchy := map[models.FamilyRole]int{
		models.FamilyRoleParent:          3,
		models.FamilyRoleMedicalProvider: 2,
		models.FamilyRoleCaregiver:       1,
	}

	return roleHierarchy[actual] >= roleHierarchy[required]
}

// FamilyDashboard represents the family overview
type FamilyDashboard struct {
	Family   models.Family            `json:"family"`
	Members  []models.FamilyMembership `json:"members"`
	Children []models.Child           `json:"children"`
}

func (s *FamilyService) GetDashboard(ctx context.Context, familyID uuid.UUID) (*FamilyDashboard, error) {
	family, err := s.familyRepo.GetByID(ctx, familyID)
	if err != nil {
		return nil, err
	}
	if family == nil {
		return nil, ErrFamilyNotFound
	}

	members, err := s.familyRepo.GetMembers(ctx, familyID)
	if err != nil {
		return nil, err
	}

	children, err := s.childRepo.GetByFamilyID(ctx, familyID)
	if err != nil {
		return nil, err
	}

	return &FamilyDashboard{
		Family:   *family,
		Members:  members,
		Children: children,
	}, nil
}
