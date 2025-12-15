package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrPasswordMismatch = errors.New("current password is incorrect")
)

type UserService struct {
	userRepo   repository.UserRepository
	familyRepo repository.FamilyRepository
}

func NewUserService(userRepo repository.UserRepository, familyRepo repository.FamilyRepository) *UserService {
	return &UserService{
		userRepo:   userRepo,
		familyRepo: familyRepo,
	}
}

func (s *UserService) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *UserService) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	return s.userRepo.GetByEmail(ctx, email)
}

func (s *UserService) Update(ctx context.Context, user *models.User) error {
	return s.userRepo.Update(ctx, user)
}

func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req *models.UpdateProfileRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Phone != nil {
		user.Phone.String = *req.Phone
		user.Phone.Valid = *req.Phone != ""
	}

	return s.userRepo.Update(ctx, user)
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *UserService) ChangePassword(ctx context.Context, userID uuid.UUID, req *ChangePasswordRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrUserNotFound
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		return ErrPasswordMismatch
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.PasswordHash = string(hashedPassword)
	return s.userRepo.Update(ctx, user)
}

func (s *UserService) GetUserFamilies(ctx context.Context, userID uuid.UUID) ([]models.FamilyMembership, error) {
	return s.familyRepo.GetUserFamilies(ctx, userID)
}

func (s *UserService) GetUserWithFamilies(ctx context.Context, userID uuid.UUID) (*models.UserWithFamilies, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrUserNotFound
	}

	memberships, err := s.familyRepo.GetUserFamilies(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &models.UserWithFamilies{
		User:     *user,
		Families: memberships,
	}, nil
}

func (s *UserService) Deactivate(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.UpdateStatus(ctx, userID, models.UserStatusInactive)
}

func (s *UserService) Reactivate(ctx context.Context, userID uuid.UUID) error {
	return s.userRepo.UpdateStatus(ctx, userID, models.UserStatusActive)
}
