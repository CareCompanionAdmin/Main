package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// ============================================================================
// JSON endpoints — back the AJAX edit modal on the subscriptions admin page.
// ============================================================================

// ListFamilySubscriptions returns paginated family subscriptions for the
// admin table. Query params: status, plan, search, page, limit.
func (h *Handler) ListFamilySubscriptions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	subs, total, err := h.adminRepo.ListFamilySubscriptions(
		r.Context(), q.Get("status"), q.Get("plan"), q.Get("search"), page, limit,
	)
	if err != nil {
		http.Error(w, "list failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"subscriptions": subs,
		"total":         total,
		"page":          page,
		"limit":         limit,
	})
}

// GetFamilySubscription returns a single subscription by family_id (the URL
// uses family_id rather than subscription_id since that's what an admin
// looking at the table is going to know).
func (h *Handler) GetFamilySubscription(w http.ResponseWriter, r *http.Request) {
	familyID, err := uuid.Parse(chi.URLParam(r, "family_id"))
	if err != nil {
		http.Error(w, "bad family_id", http.StatusBadRequest)
		return
	}
	sub, err := h.adminRepo.GetFamilySubscriptionByFamilyID(r.Context(), familyID)
	if err != nil {
		http.Error(w, "not found: "+err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// updateFamilySubscriptionRequest is the body for PUT /family-subscriptions/{family_id}.
// All fields optional; only non-zero fields are applied.
type updateFamilySubscriptionRequest struct {
	PlanID             string `json:"plan_id,omitempty"`
	Status             string `json:"status,omitempty"`
	CurrentPeriodEnd   string `json:"current_period_end,omitempty"` // RFC3339
	TrialEnd           string `json:"trial_end,omitempty"`          // RFC3339; "null" clears
	CompReason         string `json:"comp_reason,omitempty"`        // "null" clears
	CompUntil          string `json:"comp_until,omitempty"`         // RFC3339; "null" clears
	CancelAtPeriodEnd  *bool  `json:"cancel_at_period_end,omitempty"`
}

// UpdateFamilySubscription is the generic edit endpoint. The request applies
// only the fields that were sent — empty strings are treated as "no change"
// (use the literal string "null" to clear a nullable field).
func (h *Handler) UpdateFamilySubscription(w http.ResponseWriter, r *http.Request) {
	familyID, err := uuid.Parse(chi.URLParam(r, "family_id"))
	if err != nil {
		http.Error(w, "bad family_id", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "no auth", http.StatusUnauthorized)
		return
	}
	actorID := claims.UserID

	var req updateFamilySubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}

	sub, err := h.adminRepo.GetFamilySubscriptionByFamilyID(r.Context(), familyID)
	if err != nil {
		http.Error(w, "not found: "+err.Error(), http.StatusNotFound)
		return
	}

	// Apply patch
	if req.PlanID != "" {
		pid, err := uuid.Parse(req.PlanID)
		if err != nil {
			http.Error(w, "bad plan_id", http.StatusBadRequest)
			return
		}
		sub.PlanID = pid
	}
	if req.Status != "" {
		sub.Status = models.SubscriptionStatus(req.Status)
	}
	if req.CurrentPeriodEnd != "" {
		t, err := time.Parse(time.RFC3339, req.CurrentPeriodEnd)
		if err != nil {
			http.Error(w, "bad current_period_end (need RFC3339): "+err.Error(), http.StatusBadRequest)
			return
		}
		sub.CurrentPeriodEnd = t
	}
	if req.TrialEnd == "null" {
		sub.TrialEnd = models.NullTime{NullTime: sql.NullTime{Valid: false}}
	} else if req.TrialEnd != "" {
		t, err := time.Parse(time.RFC3339, req.TrialEnd)
		if err != nil {
			http.Error(w, "bad trial_end: "+err.Error(), http.StatusBadRequest)
			return
		}
		sub.TrialEnd = models.NullTime{NullTime: sql.NullTime{Time: t, Valid: true}}
	}
	if req.CompReason == "null" {
		sub.CompReason = models.NullString{NullString: sql.NullString{Valid: false}}
		sub.CompedBy = models.NullUUID{Valid: false}
		sub.CompUntil = models.NullTime{NullTime: sql.NullTime{Valid: false}}
	} else if req.CompReason != "" {
		sub.CompReason = models.NullString{NullString: sql.NullString{String: req.CompReason, Valid: true}}
		sub.CompedBy = models.NullUUID{UUID: actorID, Valid: true}
		// If comp_until wasn't separately provided, default to current_period_end.
		if req.CompUntil == "" {
			sub.CompUntil = models.NullTime{NullTime: sql.NullTime{Time: sub.CurrentPeriodEnd, Valid: true}}
		}
	}
	if req.CompUntil == "null" {
		sub.CompUntil = models.NullTime{NullTime: sql.NullTime{Valid: false}}
	} else if req.CompUntil != "" {
		t, err := time.Parse(time.RFC3339, req.CompUntil)
		if err != nil {
			http.Error(w, "bad comp_until: "+err.Error(), http.StatusBadRequest)
			return
		}
		sub.CompUntil = models.NullTime{NullTime: sql.NullTime{Time: t, Valid: true}}
	}
	if req.CancelAtPeriodEnd != nil {
		sub.CancelAtPeriodEnd = *req.CancelAtPeriodEnd
	}

	if err := h.adminRepo.UpdateFamilySubscription(r.Context(), sub); err != nil {
		http.Error(w, "update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "subscription.update", "family_subscription", sub.ID,
		map[string]interface{}{
			"family":     sub.FamilyName,
			"plan":       sub.PlanName,
			"status":     string(sub.Status),
			"period_end": sub.CurrentPeriodEnd.Format(time.RFC3339),
		})

	updated, _ := h.adminRepo.GetFamilySubscriptionByFamilyID(r.Context(), familyID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// compFamilySubscriptionRequest is the body for POST /family-subscriptions/{family_id}/comp.
type compFamilySubscriptionRequest struct {
	PlanID    string `json:"plan_id"`
	Reason    string `json:"reason"`
	UntilDate string `json:"until_date"` // YYYY-MM-DD
}

// CompFamilySubscription is the "Comp this family" button. Sets status=comped
// for the chosen plan through the chosen date with the given reason. Audited.
func (h *Handler) CompFamilySubscription(w http.ResponseWriter, r *http.Request) {
	familyID, err := uuid.Parse(chi.URLParam(r, "family_id"))
	if err != nil {
		http.Error(w, "bad family_id", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "no auth", http.StatusUnauthorized)
		return
	}
	actorID := claims.UserID

	var req compFamilySubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		http.Error(w, "reason required", http.StatusBadRequest)
		return
	}
	planID, err := uuid.Parse(req.PlanID)
	if err != nil {
		http.Error(w, "bad plan_id", http.StatusBadRequest)
		return
	}
	until, err := time.Parse("2006-01-02", req.UntilDate)
	if err != nil {
		http.Error(w, "bad until_date (YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	// Set to end-of-day in UTC for clarity.
	until = time.Date(until.Year(), until.Month(), until.Day(), 23, 59, 59, 0, time.UTC)

	sub, err := h.adminRepo.CompFamilySubscription(r.Context(), familyID, planID, actorID, req.Reason, until)
	if err != nil {
		http.Error(w, "comp failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "subscription.comp", "family_subscription", sub.ID,
		map[string]interface{}{
			"family": sub.FamilyName,
			"plan":   sub.PlanName,
			"reason": req.Reason,
			"until":  until.Format("2006-01-02"),
		})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sub)
}

// CancelFamilySubscription is the "Cancel" button. immediate=true forces
// enforcement now; immediate=false sets cancel_at_period_end.
func (h *Handler) CancelFamilySubscription(w http.ResponseWriter, r *http.Request) {
	familyID, err := uuid.Parse(chi.URLParam(r, "family_id"))
	if err != nil {
		http.Error(w, "bad family_id", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "no auth", http.StatusUnauthorized)
		return
	}
	actorID := claims.UserID

	var req struct {
		Immediate bool `json:"immediate"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	sub, err := h.adminRepo.GetFamilySubscriptionByFamilyID(r.Context(), familyID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err := h.adminRepo.CancelFamilySubscription(r.Context(), familyID, actorID, req.Immediate); err != nil {
		http.Error(w, "cancel failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	mode := "at_period_end"
	if req.Immediate {
		mode = "immediate"
	}
	h.logAction(r, "subscription.cancel", "family_subscription", sub.ID,
		map[string]interface{}{
			"family": sub.FamilyName,
			"mode":   mode,
		})

	updated, _ := h.adminRepo.GetFamilySubscriptionByFamilyID(r.Context(), familyID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// ============================================================================
// UI page
// ============================================================================

// SubscriptionsPage renders the admin subscriptions table + edit/comp modals.
func (h *Handler) SubscriptionsPage(w http.ResponseWriter, r *http.Request) {
	plans, err := h.adminRepo.ListSubscriptionPlans(r.Context(), true)
	if err != nil {
		http.Error(w, "plans failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := parseTemplates("layout.html", "subscriptions.html")
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	claims := middleware.GetAuthClaims(r.Context())
	_ = tmpl.ExecuteTemplate(w, "layout.html", map[string]interface{}{
		"Title":       "Subscriptions",
		"CurrentUser": claims,
		"Flash":       "",
		"ActivePlans": plans,
	})
}
