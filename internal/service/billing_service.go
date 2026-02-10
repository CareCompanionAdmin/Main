package service

import (
	"context"
	"log"

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
		// No subscription found - all families should have been enrolled by migration 00011.
		// Log this as a warning so we can investigate, but allow the action to avoid
		// blocking users during alpha. This should be changed to return false once
		// subscription enrollment is enforced on family creation.
		log.Printf("[BILLING] WARNING: No subscription found for family %s - allowing child addition by default", familyID)
		return true, nil
	}

	return info.CanAddMoreChildren, nil
}

// GetFamilySubscription returns the subscription details for a family
func (s *BillingService) GetFamilySubscription(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error) {
	return s.billingRepo.GetFamilySubscription(ctx, familyID)
}
