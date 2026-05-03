package web

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// CheckoutPost handles POST /billing/checkout. The form posts a plan_id
// from the upgrade button on the settings page. We resolve the plan to
// its Stripe price, look up the family's existing Stripe customer (if
// any from a prior subscription), and create a Stripe Checkout session.
// Then we either:
//   - HTMX request: return HX-Redirect so the browser navigates
//   - Plain form: 303 See Other to session.URL
func (h *WebHandlers) CheckoutPost(w http.ResponseWriter, r *http.Request) {
	if h.services.Stripe == nil {
		renderError(w, "Billing is not configured on this server", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		renderError(w, "Invalid form", http.StatusBadRequest)
		return
	}
	planIDStr := r.FormValue("plan_id")
	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		renderError(w, "Invalid plan", http.StatusBadRequest)
		return
	}
	userID := middleware.GetUserID(r.Context())
	familyID := middleware.GetFamilyID(r.Context())
	email := middleware.GetEmail(r.Context())
	if familyID == uuid.Nil {
		renderError(w, "No family context", http.StatusBadRequest)
		return
	}

	// Resolve plan → Stripe price ID. Reading active plans is cheap and
	// returns both the price ID and a sanity check that the plan exists.
	plans, err := h.services.Billing.GetAvailablePlans(r.Context())
	if err != nil {
		renderError(w, "Failed to load plans", http.StatusInternalServerError)
		return
	}
	var priceID string
	for _, p := range plans {
		if p.ID == planID {
			if !p.StripePriceID.Valid || p.StripePriceID.String == "" {
				renderError(w, "Plan not yet provisioned in Stripe", http.StatusServiceUnavailable)
				return
			}
			priceID = p.StripePriceID.String
			break
		}
	}
	if priceID == "" {
		renderError(w, "Plan not found", http.StatusBadRequest)
		return
	}

	// Reuse an existing Stripe customer if the family already has one.
	// Otherwise pass the email so Stripe creates the customer at checkout.
	existingSub, _ := h.services.Billing.GetFamilySubscription(r.Context(), familyID)
	customerID := ""
	if existingSub != nil && existingSub.StripeCustomerID.Valid {
		customerID = existingSub.StripeCustomerID.String
	}

	sess, err := h.services.Stripe.CreateCheckoutSession(r.Context(), service.CheckoutParams{
		FamilyID:         familyID,
		PlanID:           planID,
		StripePriceID:    priceID,
		StripeCustomerID: customerID,
		CustomerEmail:    email,
	})
	if err != nil {
		log.Printf("[STRIPE] CreateCheckoutSession failed user=%s family=%s plan=%s: %v",
			userID, familyID, planID, err)
		renderError(w, "Failed to start checkout. Please try again.", http.StatusBadGateway)
		return
	}
	if HTMXRequest(r) {
		HTMXRedirect(w, sess.URL)
		return
	}
	http.Redirect(w, r, sess.URL, http.StatusSeeOther)
}

// BillingSuccess renders a confirmation page after Stripe redirects back
// from a completed checkout. Webhooks already updated family_subscriptions
// independently — this page is purely UX.
func (h *WebHandlers) BillingSuccess(w http.ResponseWriter, r *http.Request) {
	familyID := middleware.GetFamilyID(r.Context())
	var info interface{}
	if familyID != uuid.Nil {
		info, _ = h.services.Billing.GetFamilyBillingInfo(r.Context(), familyID)
	}
	data := map[string]interface{}{
		"Title":   "Subscription Active",
		"Billing": info,
	}
	renderTemplate(w, "billing_success", data)
}

// BillingCancel renders the cancel page when a user backs out of Checkout.
// No state changes — the family_subscriptions row is unchanged.
func (h *WebHandlers) BillingCancel(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "billing_cancel", map[string]interface{}{
		"Title": "Checkout Cancelled",
	})
}

// StripeWebhook is the public, unauthenticated POST endpoint Stripe calls
// for subscription lifecycle events. CRITICAL: no JSON-decoding middleware
// can run before this handler — the signature is computed over the exact
// raw body bytes, so we MUST read r.Body directly without modification.
//
// The handler always responds 200 to verified events (even if our DB
// write fails) UNLESS the failure is retriable; in that case we return
// 500 so Stripe re-delivers. For unverified or malformed events we return
// 400 — Stripe won't retry, and that's correct (those didn't come from
// Stripe).
func (h *WebHandlers) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	if h.services.Stripe == nil {
		http.Error(w, "stripe not configured", http.StatusServiceUnavailable)
		return
	}
	const maxBody = 1 << 20 // 1 MB cap; Stripe events are ~5 KB
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("Stripe-Signature")
	ev, err := h.services.Stripe.VerifyWebhookSignature(body, sig)
	if err != nil {
		log.Printf("[STRIPE] webhook signature verification failed: %v", err)
		http.Error(w, "signature verification failed", http.StatusBadRequest)
		return
	}
	// Use a separate timeout from the request context — webhooks must
	// complete even if the client disconnects (Stripe doesn't keep the
	// connection open while waiting on retries).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.services.Stripe.HandleEvent(ctx, ev); err != nil {
		log.Printf("[STRIPE] event %s (%s) failed: %v", ev.Type, ev.ID, err)
		http.Error(w, fmt.Sprintf("event handling failed: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
