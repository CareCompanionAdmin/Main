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
