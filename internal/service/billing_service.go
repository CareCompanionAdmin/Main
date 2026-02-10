package service

import (
	"context"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// BillingService handles family billing operations
type BillingService struct {
	billingRepo repository.BillingRepository
	childRepo   repository.ChildRepository
}

// NewBillingService creates a new billing service
func NewBillingService(billingRepo repository.BillingRepository, childRepo repository.ChildRepository) *BillingService {
	return &BillingService{
		billingRepo: billingRepo,
		childRepo:   childRepo,
	}
}

// GetFamilyBillingInfo returns the billing information for a family
func (s *BillingService) GetFamilyBillingInfo(ctx context.Context, familyID uuid.UUID) (*models.FamilyBillingInfo, error) {
	return s.billingRepo.GetFamilyBillingInfo(ctx, familyID)
}

// GetAvailablePlans returns all active subscription plans
func (s *BillingService) GetAvailablePlans(ctx context.Context) ([]models.SubscriptionPlan, error) {
	return s.billingRepo.GetActivePlans(ctx)
}

// CanAddChild checks if a family can add more children based on their plan limits
func (s *BillingService) CanAddChild(ctx context.Context, familyID uuid.UUID) (bool, error) {
	info, err := s.billingRepo.GetFamilyBillingInfo(ctx, familyID)
	if err != nil {
		return false, err
	}
	if info == nil {
		// No subscription found - default to allowing (for now)
		return true, nil
	}

	return info.CanAddMoreChildren, nil
}

// GetFamilySubscription returns the subscription details for a family
func (s *BillingService) GetFamilySubscription(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error) {
	return s.billingRepo.GetFamilySubscription(ctx, familyID)
}
