package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// SubscriptionService owns the trial/expiry/comp lifecycle of family
// subscriptions. Stripe checkout + webhook handling live in stripe_service.go
// (Phase 3); this file handles only what's free-tier and admin-driven.
type SubscriptionService struct {
	db *sql.DB

	// The active plan IDs are loaded once at startup. They're never going to
	// change at runtime, and we want fast access in the hot path of "user
	// just signed up; create their trial."
	singleChildPlanID uuid.UUID
	familyPlanID      uuid.UUID

	trialDays int
}

// NewSubscriptionService loads the active plan IDs. Returns an error if the
// expected plans aren't present — without them, signup-trial-creation can't
// pick the right plan_id, so we'd rather fail loudly at startup than silently
// at the first signup.
func NewSubscriptionService(db *sql.DB) (*SubscriptionService, error) {
	s := &SubscriptionService{db: db, trialDays: 14}
	row := s.db.QueryRow(`
        SELECT
            (SELECT id FROM subscription_plans WHERE name = 'Single Child' AND is_active = true LIMIT 1),
            (SELECT id FROM subscription_plans WHERE name = 'Family'       AND is_active = true LIMIT 1)`)
	if err := row.Scan(&s.singleChildPlanID, &s.familyPlanID); err != nil {
		return nil, fmt.Errorf("subscription_service: failed to load plan IDs: %w (did the migration 00011 plans get created?)", err)
	}
	if s.singleChildPlanID == uuid.Nil || s.familyPlanID == uuid.Nil {
		return nil, errors.New("subscription_service: Single Child or Family plan missing/inactive in subscription_plans")
	}
	return s, nil
}

// StartTrialIfNew creates a 14-day Single-Child trial for the family IF and
// only if no family_subscriptions row exists yet. Idempotent on family_id —
// safe to call from a signup hook even if the user retries.
//
// Comped families (founders grandfather, partner family, qa test) already
// have a row and are NOT touched. Same for any family that already finished
// a trial and is now active or past_due.
func (s *SubscriptionService) StartTrialIfNew(ctx context.Context, familyID uuid.UUID) error {
	now := time.Now().UTC()
	trialEnd := now.AddDate(0, 0, s.trialDays)
	_, err := s.db.ExecContext(ctx, `
        INSERT INTO family_subscriptions (
            family_id, plan_id, status,
            current_period_start, current_period_end, trial_end,
            cancel_at_period_end
        ) VALUES ($1, $2, 'trialing', $3, $4, $4, false)
        ON CONFLICT (family_id) DO NOTHING`,
		familyID, s.singleChildPlanID, now, trialEnd,
	)
	if err != nil {
		return fmt.Errorf("StartTrialIfNew: %w", err)
	}
	return nil
}

// BumpTrialOnSecondChild upgrades a family from Single Child → Family plan
// AND grants a fresh 14-day trial when they add their 2nd child. The 2nd
// child is the only "plan upgrade trigger" — child #3+ doesn't do anything
// because Family is already unlimited.
//
// Only fires when:
//   - The family is currently on Single Child plan
//   - The family's status is 'trialing' OR 'active' (not comped/cancelled/etc.)
//
// For trialing families: the new trial_end is NOW()+14d (extending if the
// new date is later, otherwise leaving alone — we never shorten a trial).
// For active families: same trial_end logic, but status flips back to
// trialing so they get the 14-day "preview" of the bigger plan before being
// charged the new rate. Stripe webhook reconciles billing from there.
func (s *SubscriptionService) BumpTrialOnSecondChild(ctx context.Context, familyID uuid.UUID) error {
	// Single transaction so the read + write are consistent (avoid a TOCTOU
	// where a concurrent comp lands between SELECT and UPDATE).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var planID uuid.UUID
	var status string
	var existingTrialEnd sql.NullTime
	err = tx.QueryRowContext(ctx, `
        SELECT plan_id, status, trial_end
        FROM family_subscriptions
        WHERE family_id = $1
        FOR UPDATE`, familyID).Scan(&planID, &status, &existingTrialEnd)
	if errors.Is(err, sql.ErrNoRows) {
		// No subscription at all (shouldn't happen post-Phase-2 but be safe).
		// Treat as a new signup with two children; start the Family-plan trial.
		return s.startTrialOnFamilyPlan(ctx, tx, familyID)
	}
	if err != nil {
		return err
	}

	// Don't touch comped, cancelled, paused, expired, terminated families.
	if status != "trialing" && status != "active" {
		return tx.Commit()
	}
	// Don't touch already-on-Family-plan rows.
	if planID == s.familyPlanID {
		return tx.Commit()
	}

	now := time.Now().UTC()
	newTrialEnd := now.AddDate(0, 0, s.trialDays)
	if existingTrialEnd.Valid && existingTrialEnd.Time.After(newTrialEnd) {
		// Don't shorten an existing longer trial.
		newTrialEnd = existingTrialEnd.Time
	}

	_, err = tx.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            plan_id              = $2,
            status               = 'trialing',
            trial_end            = $3,
            current_period_end   = $3,
            updated_at           = NOW()
        WHERE family_id = $1`, familyID, s.familyPlanID, newTrialEnd)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SubscriptionService) startTrialOnFamilyPlan(ctx context.Context, tx *sql.Tx, familyID uuid.UUID) error {
	now := time.Now().UTC()
	trialEnd := now.AddDate(0, 0, s.trialDays)
	_, err := tx.ExecContext(ctx, `
        INSERT INTO family_subscriptions (
            family_id, plan_id, status,
            current_period_start, current_period_end, trial_end,
            cancel_at_period_end
        ) VALUES ($1, $2, 'trialing', $3, $4, $4, false)
        ON CONFLICT (family_id) DO NOTHING`,
		familyID, s.familyPlanID, now, trialEnd,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RunExpiryCheck transitions subscriptions whose clocks have passed:
//   - 'trialing' with trial_end < NOW()                  → 'past_due' (begin 14-day read-only window)
//   - 'past_due' with past_due_since < NOW() - 14 days   → 'terminated' (export-and-delete handled in Phase 5)
//
// 'comped' families are NEVER touched — comp_until is informational only at
// this stage; Bryan will get a separate report when comps lapse so he can
// decide on each one. Stripe-driven 'past_due' (from invoice.payment_failed)
// will set past_due_since via the webhook handler in Phase 3.
//
// We use the dedicated `past_due_since` column rather than `updated_at`
// because the table's BEFORE-UPDATE trigger overwrites updated_at on every
// write — any unrelated edit would push the termination clock back to zero.
func (s *SubscriptionService) RunExpiryCheck(ctx context.Context) (transitioned int, err error) {
	// trial_end has passed: flip to past_due AND stamp past_due_since.
	r, err := s.db.ExecContext(ctx, `
        UPDATE family_subscriptions
        SET status = 'past_due', past_due_since = NOW()
        WHERE status = 'trialing' AND trial_end IS NOT NULL AND trial_end < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("trial_end sweep: %w", err)
	}
	a, _ := r.RowsAffected()

	// past_due for >= 14 days → terminated. Phase 5 wires the cold-storage
	// export hook to this transition.
	r, err = s.db.ExecContext(ctx, `
        UPDATE family_subscriptions
        SET status = 'terminated'
        WHERE status = 'past_due'
          AND past_due_since IS NOT NULL
          AND past_due_since < NOW() - INTERVAL '14 days'`)
	if err != nil {
		return int(a), fmt.Errorf("past_due termination sweep: %w", err)
	}
	b, _ := r.RowsAffected()

	return int(a + b), nil
}

// ApplyCheckoutCompleted is called from the Stripe webhook when a Checkout
// session completes. The session metadata carries family_id + plan_id (we
// stamped both on creation in stripe_service.go), and the resulting
// subscription gives us the customer ID, subscription ID, and the period
// end. Status flips to 'active' (or stays 'trialing' if Stripe issued a
// trial). Idempotent: if the same subscription_id arrives twice, the
// second UPDATE is a no-op.
func (s *SubscriptionService) ApplyCheckoutCompleted(
	ctx context.Context,
	familyID, planID uuid.UUID,
	stripeCustomerID, stripeSubscriptionID string,
	status string,
	currentPeriodEnd time.Time,
) error {
	_, err := s.db.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            plan_id                = $2,
            status                 = $3,
            stripe_customer_id     = $4,
            stripe_subscription_id = $5,
            current_period_start   = NOW(),
            current_period_end     = $6,
            past_due_since         = NULL,
            cancelled_at           = NULL,
            cancel_at_period_end   = false,
            updated_at             = NOW()
        WHERE family_id = $1`,
		familyID, planID, status, stripeCustomerID, stripeSubscriptionID, currentPeriodEnd,
	)
	if err != nil {
		return fmt.Errorf("ApplyCheckoutCompleted: %w", err)
	}
	return nil
}

// ApplySubscriptionUpdated is called for customer.subscription.updated and
// customer.subscription.deleted events. We trust Stripe's view of status,
// period_end, and cancel_at_period_end for the matching subscription.
func (s *SubscriptionService) ApplySubscriptionUpdated(
	ctx context.Context,
	stripeSubscriptionID string,
	status string,
	currentPeriodEnd time.Time,
	cancelAtPeriodEnd bool,
	cancelledAt *time.Time,
) error {
	var cancelledAtArg interface{}
	if cancelledAt != nil {
		cancelledAtArg = *cancelledAt
	} else {
		cancelledAtArg = nil
	}
	_, err := s.db.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            status               = $2,
            current_period_end   = $3,
            cancel_at_period_end = $4,
            cancelled_at         = COALESCE($5::timestamptz, cancelled_at),
            past_due_since       = CASE WHEN $2 = 'past_due' AND past_due_since IS NULL THEN NOW() ELSE past_due_since END,
            updated_at           = NOW()
        WHERE stripe_subscription_id = $1`,
		stripeSubscriptionID, status, currentPeriodEnd, cancelAtPeriodEnd, cancelledAtArg,
	)
	if err != nil {
		return fmt.Errorf("ApplySubscriptionUpdated: %w", err)
	}
	return nil
}

// ApplyInvoicePaid extends current_period_end after a successful renewal,
// clears past_due_since, and inserts a payments row for revenue tracking.
// The payments row is best-effort: if insertion fails (e.g. no parent of
// the family found) we still update the subscription state — better to
// have correct entitlement than to fail the whole webhook for a logging
// row.
func (s *SubscriptionService) ApplyInvoicePaid(
	ctx context.Context,
	stripeSubscriptionID string,
	currentPeriodEnd time.Time,
	amountCents int64,
	currency string,
	stripeInvoiceID string,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ApplyInvoicePaid begin: %w", err)
	}
	defer tx.Rollback()

	var subscriptionID, familyID uuid.UUID
	err = tx.QueryRowContext(ctx, `
        UPDATE family_subscriptions SET
            status             = 'active',
            current_period_end = $2,
            past_due_since     = NULL,
            updated_at         = NOW()
        WHERE stripe_subscription_id = $1
        RETURNING id, family_id`,
		stripeSubscriptionID, currentPeriodEnd,
	).Scan(&subscriptionID, &familyID)
	if errors.Is(err, sql.ErrNoRows) {
		// Subscription not tracked locally — could be a test event for an
		// unrelated account. Don't error.
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("ApplyInvoicePaid update: %w", err)
	}

	// Pick any active parent on the family for the payments.user_id FK.
	// If the family has no parents (shouldn't happen but defensive), skip
	// the payments row — subscription state is what matters for entitlement.
	var ownerID uuid.UUID
	err = tx.QueryRowContext(ctx, `
        SELECT user_id FROM family_memberships
        WHERE family_id = $1 AND role = 'parent' AND is_active = true
        ORDER BY created_at ASC LIMIT 1`, familyID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		log.Printf("[STRIPE] invoice paid for family %s but no parent found — skipping payments row", familyID)
		return tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("ApplyInvoicePaid find owner: %w", err)
	}

	if currency == "" {
		currency = "USD"
	}
	_, err = tx.ExecContext(ctx, `
        INSERT INTO payments (
            subscription_id, user_id, payment_type, amount_cents, currency,
            status, stripe_invoice_id, description
        ) VALUES ($1, $2, 'subscription', $3, $4, 'succeeded', $5, $6)
        ON CONFLICT DO NOTHING`,
		subscriptionID, ownerID, amountCents, currency,
		stripeInvoiceID, fmt.Sprintf("Subscription renewal (%s)", stripeSubscriptionID),
	)
	if err != nil {
		return fmt.Errorf("ApplyInvoicePaid insert payment: %w", err)
	}
	return tx.Commit()
}

// ApplyInvoicePaymentFailed flips the subscription to past_due. Stamps
// past_due_since only if it wasn't already set — the 14-day termination
// clock should track the FIRST failure, not the latest retry.
func (s *SubscriptionService) ApplyInvoicePaymentFailed(
	ctx context.Context,
	stripeSubscriptionID string,
) error {
	_, err := s.db.ExecContext(ctx, `
        UPDATE family_subscriptions SET
            status         = 'past_due',
            past_due_since = COALESCE(past_due_since, NOW()),
            updated_at     = NOW()
        WHERE stripe_subscription_id = $1`,
		stripeSubscriptionID,
	)
	if err != nil {
		return fmt.Errorf("ApplyInvoicePaymentFailed: %w", err)
	}
	return nil
}

// LookupFamilyByStripeSubscription returns the family_id + plan_id for a
// Stripe subscription that's already been linked. Used as a fallback when
// the webhook event metadata is incomplete. Returns uuid.Nil twice if the
// subscription isn't tracked (e.g. test events for an unrelated account).
func (s *SubscriptionService) LookupFamilyByStripeSubscription(ctx context.Context, stripeSubscriptionID string) (uuid.UUID, uuid.UUID, error) {
	var familyID, planID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
        SELECT family_id, plan_id
        FROM family_subscriptions
        WHERE stripe_subscription_id = $1`, stripeSubscriptionID,
	).Scan(&familyID, &planID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return familyID, planID, nil
}

// PlanIDForStripePrice resolves the local plan UUID from a Stripe price ID.
// Used when the webhook gives us a subscription with line items but no
// plan_id metadata (defensive — we always stamp plan_id ourselves).
func (s *SubscriptionService) PlanIDForStripePrice(ctx context.Context, stripePriceID string) (uuid.UUID, error) {
	var planID uuid.UUID
	err := s.db.QueryRowContext(ctx, `
        SELECT id FROM subscription_plans WHERE stripe_price_id = $1 LIMIT 1`,
		stripePriceID,
	).Scan(&planID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, err
	}
	return planID, nil
}

// SubscriptionScheduler runs RunExpiryCheck periodically. Hourly is plenty —
// the user-visible state granularity is "days remaining" so there's no point
// running more often than that.
type SubscriptionScheduler struct {
	svc *SubscriptionService
}

func NewSubscriptionScheduler(svc *SubscriptionService) *SubscriptionScheduler {
	return &SubscriptionScheduler{svc: svc}
}

func (s *SubscriptionScheduler) Start(ctx context.Context) {
	log.Println("Subscription expiry scheduler started")
	// Run once at boot so dev iteration doesn't need to wait an hour.
	if n, err := s.svc.RunExpiryCheck(ctx); err != nil {
		log.Printf("Subscription expiry: initial run failed: %v", err)
	} else if n > 0 {
		log.Printf("Subscription expiry: initial run transitioned %d row(s)", n)
	}
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("Subscription expiry scheduler stopped")
			return
		case <-ticker.C:
			if n, err := s.svc.RunExpiryCheck(ctx); err != nil {
				log.Printf("Subscription expiry: tick failed: %v", err)
			} else if n > 0 {
				log.Printf("Subscription expiry: transitioned %d row(s)", n)
			}
		}
	}
}
