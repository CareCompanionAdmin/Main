package api

import (
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// BillingHandler handles billing-related API endpoints
type BillingHandler struct {
	billingService *service.BillingService
}

// NewBillingHandler creates a new billing handler
func NewBillingHandler(billingService *service.BillingService) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
	}
}

// GetFamilyBilling returns the billing information for the current family
// GET /api/family/billing
func (h *BillingHandler) GetFamilyBilling(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	info, err := h.billingService.GetFamilyBillingInfo(r.Context(), familyID)
	if err != nil {
		respondInternalError(w, "Failed to get billing information")
		return
	}
	if info == nil {
		respondNotFound(w, "No subscription found for this family")
		return
	}

	respondOK(w, info)
}

// GetPlans returns all available subscription plans
// GET /api/billing/plans
func (h *BillingHandler) GetPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := h.billingService.GetAvailablePlans(r.Context())
	if err != nil {
		respondInternalError(w, "Failed to get subscription plans")
		return
	}

	respondOK(w, plans)
}

// CanAddChild checks if the current family can add more children
// GET /api/family/billing/can-add-child
func (h *BillingHandler) CanAddChild(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())

	canAdd, err := h.billingService.CanAddChild(r.Context(), familyID)
	if err != nil {
		respondInternalError(w, "Failed to check child limit")
		return
	}

	respondOK(w, map[string]bool{"can_add_child": canAdd})
}
