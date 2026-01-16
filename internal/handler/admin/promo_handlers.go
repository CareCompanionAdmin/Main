package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// ListPromoCodes returns paginated list of promo codes
func (h *Handler) ListPromoCodes(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	activeOnly := r.URL.Query().Get("active_only") == "true"
	search := r.URL.Query().Get("search")

	codes, total, err := h.adminRepo.ListPromoCodes(r.Context(), page, limit, activeOnly, search)
	if err != nil {
		http.Error(w, "Failed to fetch promo codes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"promo_codes": codes,
		"total":       total,
		"page":        page,
		"limit":       limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetPromoCode returns a single promo code by ID
func (h *Handler) GetPromoCode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid promo code ID", http.StatusBadRequest)
		return
	}

	code, err := h.adminRepo.GetPromoCodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to fetch promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if code == nil {
		http.Error(w, "Promo code not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(code)
}

// CreatePromoCodeRequest represents the request body for creating a promo code
type CreatePromoCodeRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Discount Configuration
	DiscountType     string  `json:"discount_type"`
	DiscountValue    float64 `json:"discount_value"`
	MaxDiscountCents *int    `json:"max_discount_cents,omitempty"`
	AppliesTo        string  `json:"applies_to"`

	// Plan Restrictions
	AppliesToPlans            []string `json:"applies_to_plans,omitempty"`
	AppliesToBillingIntervals []string `json:"applies_to_billing_intervals,omitempty"`
	MinimumPurchaseCents      int      `json:"minimum_purchase_cents"`

	// User Eligibility
	NewUsersOnly         bool     `json:"new_users_only"`
	ExistingUsersOnly    bool     `json:"existing_users_only"`
	SpecificUserIDs      []string `json:"specific_user_ids,omitempty"`
	SpecificEmailDomains []string `json:"specific_email_domains,omitempty"`

	// Usage Limits
	MaxTotalUses   *int `json:"max_total_uses,omitempty"`
	MaxUsesPerUser int  `json:"max_uses_per_user"`

	// Time Constraints
	StartsAt  string `json:"starts_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`

	// Duration
	DurationMonths *int `json:"duration_months,omitempty"`

	// Stacking
	IsStackable        bool     `json:"is_stackable"`
	StackableWithCodes []string `json:"stackable_with_codes,omitempty"`

	// Campaign
	CampaignName   string `json:"campaign_name,omitempty"`
	CampaignSource string `json:"campaign_source,omitempty"`
}

// CreatePromoCode creates a new promo code
func (h *Handler) CreatePromoCode(w http.ResponseWriter, r *http.Request) {
	var req CreatePromoCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Code == "" {
		http.Error(w, "Code is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.DiscountType == "" {
		http.Error(w, "Discount type is required", http.StatusBadRequest)
		return
	}
	if req.DiscountValue <= 0 {
		http.Error(w, "Discount value must be positive", http.StatusBadRequest)
		return
	}

	// Check if code already exists
	existing, err := h.adminRepo.GetPromoCodeByCode(r.Context(), req.Code)
	if err != nil {
		http.Error(w, "Failed to check existing code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing != nil {
		http.Error(w, "Promo code already exists", http.StatusConflict)
		return
	}

	userID := middleware.GetUserID(r.Context())

	// Build the promo code model
	promo := &models.PromoCode{
		Code:                 strings.ToUpper(req.Code),
		Name:                 req.Name,
		DiscountType:         models.PromoDiscountType(req.DiscountType),
		DiscountValue:        req.DiscountValue,
		MaxDiscountCents:     req.MaxDiscountCents,
		MinimumPurchaseCents: req.MinimumPurchaseCents,
		NewUsersOnly:         req.NewUsersOnly,
		ExistingUsersOnly:    req.ExistingUsersOnly,
		MaxTotalUses:         req.MaxTotalUses,
		MaxUsesPerUser:       req.MaxUsesPerUser,
		DurationMonths:       req.DurationMonths,
		IsStackable:          req.IsStackable,
		CreatedBy:            models.NullUUID{UUID: userID, Valid: true},
	}

	// Handle optional description
	if req.Description != "" {
		promo.Description = models.NullString{sql.NullString{String: req.Description, Valid: true}}
	}

	// Handle applies_to
	if req.AppliesTo != "" {
		promo.AppliesTo = models.PromoAppliesTo(req.AppliesTo)
	} else {
		promo.AppliesTo = models.PromoAppliesToBoth
	}

	// Handle campaign fields
	if req.CampaignName != "" {
		promo.CampaignName = models.NullString{sql.NullString{String: req.CampaignName, Valid: true}}
	}
	if req.CampaignSource != "" {
		promo.CampaignSource = models.NullString{sql.NullString{String: req.CampaignSource, Valid: true}}
	}

	// Handle dates
	if req.StartsAt != "" {
		t, err := time.Parse("2006-01-02", req.StartsAt)
		if err == nil {
			promo.StartsAt = t
		}
	} else {
		promo.StartsAt = time.Now()
	}

	if req.ExpiresAt != "" {
		t, err := time.Parse("2006-01-02", req.ExpiresAt)
		if err == nil {
			promo.ExpiresAt = models.NullTime{sql.NullTime{Time: t, Valid: true}}
		}
	}

	// Handle plan restrictions (convert string UUIDs to UUID array)
	if len(req.AppliesToPlans) > 0 {
		for _, s := range req.AppliesToPlans {
			if id, err := uuid.Parse(s); err == nil {
				promo.AppliesToPlans = append(promo.AppliesToPlans, id)
			}
		}
	}

	// Handle billing intervals
	if len(req.AppliesToBillingIntervals) > 0 {
		promo.AppliesToBillingIntervals = req.AppliesToBillingIntervals
	}

	// Handle email domains
	if len(req.SpecificEmailDomains) > 0 {
		promo.SpecificEmailDomains = req.SpecificEmailDomains
	}

	// Handle specific user IDs
	if len(req.SpecificUserIDs) > 0 {
		for _, s := range req.SpecificUserIDs {
			if id, err := uuid.Parse(s); err == nil {
				promo.SpecificUserIDs = append(promo.SpecificUserIDs, id)
			}
		}
	}

	// Handle stackable codes
	if len(req.StackableWithCodes) > 0 {
		for _, s := range req.StackableWithCodes {
			if id, err := uuid.Parse(s); err == nil {
				promo.StackableWithCodes = append(promo.StackableWithCodes, id)
			}
		}
	}

	created, err := h.adminRepo.CreatePromoCode(r.Context(), promo)
	if err != nil {
		http.Error(w, "Failed to create promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

// UpdatePromoCode updates an existing promo code
func (h *Handler) UpdatePromoCode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid promo code ID", http.StatusBadRequest)
		return
	}

	// Get existing promo code
	existing, err := h.adminRepo.GetPromoCodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to fetch promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if existing == nil {
		http.Error(w, "Promo code not found", http.StatusNotFound)
		return
	}

	var req CreatePromoCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update fields
	if req.Code != "" {
		existing.Code = strings.ToUpper(req.Code)
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = models.NullString{sql.NullString{String: req.Description, Valid: true}}
	}
	if req.DiscountType != "" {
		existing.DiscountType = models.PromoDiscountType(req.DiscountType)
	}
	if req.DiscountValue > 0 {
		existing.DiscountValue = req.DiscountValue
	}
	existing.MaxDiscountCents = req.MaxDiscountCents
	if req.AppliesTo != "" {
		existing.AppliesTo = models.PromoAppliesTo(req.AppliesTo)
	}
	existing.MinimumPurchaseCents = req.MinimumPurchaseCents
	existing.NewUsersOnly = req.NewUsersOnly
	existing.ExistingUsersOnly = req.ExistingUsersOnly
	existing.MaxTotalUses = req.MaxTotalUses
	existing.MaxUsesPerUser = req.MaxUsesPerUser
	existing.DurationMonths = req.DurationMonths
	existing.IsStackable = req.IsStackable

	if req.CampaignName != "" {
		existing.CampaignName = models.NullString{sql.NullString{String: req.CampaignName, Valid: true}}
	}
	if req.CampaignSource != "" {
		existing.CampaignSource = models.NullString{sql.NullString{String: req.CampaignSource, Valid: true}}
	}

	if req.StartsAt != "" {
		if t, err := time.Parse("2006-01-02", req.StartsAt); err == nil {
			existing.StartsAt = t
		}
	}
	if req.ExpiresAt != "" {
		if t, err := time.Parse("2006-01-02", req.ExpiresAt); err == nil {
			existing.ExpiresAt = models.NullTime{sql.NullTime{Time: t, Valid: true}}
		}
	}

	// Handle arrays
	if len(req.AppliesToPlans) > 0 {
		existing.AppliesToPlans = nil
		for _, s := range req.AppliesToPlans {
			if id, err := uuid.Parse(s); err == nil {
				existing.AppliesToPlans = append(existing.AppliesToPlans, id)
			}
		}
	}
	if len(req.AppliesToBillingIntervals) > 0 {
		existing.AppliesToBillingIntervals = req.AppliesToBillingIntervals
	}
	if len(req.SpecificEmailDomains) > 0 {
		existing.SpecificEmailDomains = req.SpecificEmailDomains
	}

	if err := h.adminRepo.UpdatePromoCode(r.Context(), existing); err != nil {
		http.Error(w, "Failed to update promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated code
	updated, _ := h.adminRepo.GetPromoCodeByID(r.Context(), id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// DeactivatePromoCode deactivates a promo code
func (h *Handler) DeactivatePromoCode(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid promo code ID", http.StatusBadRequest)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 {
		json.NewDecoder(r.Body).Decode(&req)
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.adminRepo.DeactivatePromoCode(r.Context(), id, userID, req.Reason); err != nil {
		http.Error(w, "Failed to deactivate promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deactivated"})
}

// GetPromoCodeUsages returns usage history for a promo code
func (h *Handler) GetPromoCodeUsages(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "Invalid promo code ID", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 25
	}

	usages, total, err := h.adminRepo.GetPromoCodeUsages(r.Context(), id, page, limit)
	if err != nil {
		http.Error(w, "Failed to fetch promo code usages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"usages": usages,
		"total":  total,
		"page":   page,
		"limit":  limit,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
