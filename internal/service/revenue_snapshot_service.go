package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// RevenueSnapshotService aggregates yesterday's payments into the
// `daily_revenue_snapshots` table and projects each active subscription's
// next 90 days of renewals into `expected_revenue_calendar`. The admin
// /admin/financials dashboard reads from both tables.
type RevenueSnapshotService struct {
	db *sql.DB

	// projectionDays controls how far forward `expected_revenue_calendar`
	// is filled. 90 days lines up with the dashboard's typical view.
	projectionDays int
}

func NewRevenueSnapshotService(db *sql.DB) *RevenueSnapshotService {
	return &RevenueSnapshotService{db: db, projectionDays: 90}
}

// SnapshotYesterday writes (or updates) one row in daily_revenue_snapshots
// for yesterday in UTC. Idempotent on snapshot_date — re-running for the
// same date overwrites the row, never duplicates.
func (s *RevenueSnapshotService) SnapshotYesterday(ctx context.Context) error {
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	return s.SnapshotDate(ctx, yesterday)
}

// SnapshotDate computes the daily snapshot for a specific UTC date. Pulls
// from payments + family_subscriptions + promo_codes_usages. Counts upgrade
// + downgrade events from looking at plan_id transitions inside
// family_subscriptions.updated_at — but since we don't have a history
// table, those two columns stay 0 for now (Phase 7 follow-up if Bryan
// wants exact transition counts).
func (s *RevenueSnapshotService) SnapshotDate(ctx context.Context, day time.Time) error {
	dayStr := day.Format("2006-01-02")

	var (
		revenueCents     int64
		refundsCents     int64
		promoDiscCents   int64
		newSubs          int
		cancelledSubs    int
	)

	// total revenue + refunds from payments rows that landed yesterday
	err := s.db.QueryRowContext(ctx, `
        SELECT
            COALESCE(SUM(CASE WHEN status='succeeded' THEN amount_cents ELSE 0 END), 0)::bigint,
            COALESCE(SUM(refund_amount_cents), 0)::bigint,
            COALESCE(SUM(discount_amount_cents), 0)::bigint
        FROM payments
        WHERE created_at::date = $1`, dayStr,
	).Scan(&revenueCents, &refundsCents, &promoDiscCents)
	if err != nil {
		return fmt.Errorf("payments aggregate: %w", err)
	}

	// new subscriptions = family_subscriptions created on this day with status active
	err = s.db.QueryRowContext(ctx, `
        SELECT COUNT(*) FROM family_subscriptions
        WHERE created_at::date = $1 AND status IN ('active','trialing')`, dayStr,
	).Scan(&newSubs)
	if err != nil {
		return fmt.Errorf("new subs count: %w", err)
	}

	// cancelled = cancelled_at landed on this day
	err = s.db.QueryRowContext(ctx, `
        SELECT COUNT(*) FROM family_subscriptions
        WHERE cancelled_at::date = $1`, dayStr,
	).Scan(&cancelledSubs)
	if err != nil {
		return fmt.Errorf("cancelled subs count: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
        INSERT INTO daily_revenue_snapshots (
            snapshot_date, total_revenue_cents, refunds_cents,
            promo_discounts_cents, new_subscriptions, cancelled_subscriptions,
            calculated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, NOW())
        ON CONFLICT (snapshot_date) DO UPDATE SET
            total_revenue_cents     = EXCLUDED.total_revenue_cents,
            refunds_cents           = EXCLUDED.refunds_cents,
            promo_discounts_cents   = EXCLUDED.promo_discounts_cents,
            new_subscriptions       = EXCLUDED.new_subscriptions,
            cancelled_subscriptions = EXCLUDED.cancelled_subscriptions,
            calculated_at           = NOW()`,
		dayStr, revenueCents, refundsCents, promoDiscCents, newSubs, cancelledSubs,
	)
	if err != nil {
		return fmt.Errorf("upsert snapshot: %w", err)
	}
	log.Printf("[REVENUE] snapshot %s: revenue=%d¢ refunds=%d¢ new=%d cancelled=%d",
		dayStr, revenueCents, refundsCents, newSubs, cancelledSubs)
	return nil
}

// RebuildExpectedRevenue clears + repopulates expected_revenue_calendar
// for the next `projectionDays` days. For each active or trialing
// subscription, projects renewals at monthly/yearly cadence based on the
// plan's billing_interval. Trialing subs use trial_end as their first
// expected charge date. Cancelled / past_due / terminated subs aren't
// projected (no expected revenue).
func (s *RevenueSnapshotService) RebuildExpectedRevenue(ctx context.Context) error {
	now := time.Now().UTC()
	until := now.AddDate(0, 0, s.projectionDays)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM expected_revenue_calendar WHERE expected_date BETWEEN $1 AND $2`,
		now.Format("2006-01-02"), until.Format("2006-01-02"),
	); err != nil {
		return fmt.Errorf("clear projection: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
        SELECT
            fs.id, fs.status, fs.current_period_end, fs.trial_end,
            sp.name, sp.price_cents, sp.billing_interval
        FROM family_subscriptions fs
        JOIN subscription_plans sp ON fs.plan_id = sp.id
        WHERE fs.status IN ('active', 'trialing')
          AND fs.cancel_at_period_end = false`)
	if err != nil {
		return fmt.Errorf("read active subs: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var (
			id              string
			status          string
			periodEnd       sql.NullTime
			trialEnd        sql.NullTime
			planName        string
			priceCents      int
			billingInterval string
		)
		if err := rows.Scan(&id, &status, &periodEnd, &trialEnd, &planName, &priceCents, &billingInterval); err != nil {
			return fmt.Errorf("scan sub: %w", err)
		}
		// Trialing: first charge lands at trial_end (Stripe converts trial → active there).
		// Active: next charge lands at current_period_end.
		var firstCharge time.Time
		if status == "trialing" && trialEnd.Valid {
			firstCharge = trialEnd.Time
		} else if periodEnd.Valid {
			firstCharge = periodEnd.Time
		} else {
			continue
		}
		step := monthStep(billingInterval)
		if step == 0 {
			continue
		}
		for d := firstCharge; d.Before(until); d = d.AddDate(0, step, 0) {
			if d.Before(now) {
				continue
			}
			_, err := tx.ExecContext(ctx, `
                INSERT INTO expected_revenue_calendar (
                    expected_date, subscription_id, expected_amount_cents,
                    plan_name, is_renewal
                ) VALUES ($1, $2, $3, $4, true)
                ON CONFLICT (expected_date, subscription_id) DO NOTHING`,
				d.Format("2006-01-02"), id, priceCents, planName,
			)
			if err != nil {
				return fmt.Errorf("insert projection: %w", err)
			}
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("[REVENUE] projected %d renewals over next %d days", count, s.projectionDays)
	return nil
}

func monthStep(billingInterval string) int {
	switch billingInterval {
	case "monthly":
		return 1
	case "yearly":
		return 12
	default:
		return 0
	}
}

// RevenueSnapshotScheduler runs the snapshot + projection rebuild daily
// at 01:00 UTC. The choice of UTC midnight + 1h matches the financial
// dashboard's "yesterday" view and gives Stripe webhook retries up to
// an hour to land before we aggregate.
type RevenueSnapshotScheduler struct {
	svc *RevenueSnapshotService
}

func NewRevenueSnapshotScheduler(svc *RevenueSnapshotService) *RevenueSnapshotScheduler {
	return &RevenueSnapshotScheduler{svc: svc}
}

func (s *RevenueSnapshotScheduler) Start(ctx context.Context) {
	log.Println("Revenue snapshot scheduler started (target: 01:00 UTC daily)")
	// Run once at boot — gives the admin dashboard recent data on a fresh
	// deploy without waiting for the next 01:00.
	go func() {
		if err := s.svc.SnapshotYesterday(ctx); err != nil {
			log.Printf("Revenue snapshot: initial run failed: %v", err)
		}
		if err := s.svc.RebuildExpectedRevenue(ctx); err != nil {
			log.Printf("Revenue projection: initial run failed: %v", err)
		}
	}()
	for {
		next := nextUTCRunAt(time.Now().UTC(), 1, 0)
		select {
		case <-ctx.Done():
			log.Println("Revenue snapshot scheduler stopped")
			return
		case <-time.After(time.Until(next)):
			if err := s.svc.SnapshotYesterday(ctx); err != nil {
				log.Printf("Revenue snapshot: tick failed: %v", err)
			}
			if err := s.svc.RebuildExpectedRevenue(ctx); err != nil {
				log.Printf("Revenue projection: tick failed: %v", err)
			}
		}
	}
}

func nextUTCRunAt(now time.Time, hour, minute int) time.Time {
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !target.After(now) {
		target = target.AddDate(0, 0, 1)
	}
	return target
}
