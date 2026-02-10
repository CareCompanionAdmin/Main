package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// BillingRepository handles family billing operations
type BillingRepository interface {
	GetFamilySubscription(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error)
	GetActivePlans(ctx context.Context) ([]models.SubscriptionPlan, error)
	GetFamilyBillingInfo(ctx context.Context, familyID uuid.UUID) (*models.FamilyBillingInfo, error)
}

type billingRepo struct {
	db *sql.DB
}

// NewBillingRepo creates a new billing repository
func NewBillingRepo(db *sql.DB) BillingRepository {
	return &billingRepo{db: db}
}

// GetFamilySubscription retrieves the subscription for a family
func (r *billingRepo) GetFamilySubscription(ctx context.Context, familyID uuid.UUID) (*models.FamilySubscription, error) {
	query := `
		SELECT
			fs.id, fs.family_id, fs.plan_id, fs.status,
			fs.current_period_start, fs.current_period_end,
			fs.trial_end, fs.cancelled_at, fs.cancel_at_period_end,
			fs.stripe_subscription_id, fs.stripe_customer_id, fs.promo_code_id,
			fs.created_at, fs.updated_at,
			sp.name as plan_name,
			f.name as family_name
		FROM family_subscriptions fs
		JOIN subscription_plans sp ON fs.plan_id = sp.id
		JOIN families f ON fs.family_id = f.id
		WHERE fs.family_id = $1
	`

	var sub models.FamilySubscription
	err := r.db.QueryRowContext(ctx, query, familyID).Scan(
		&sub.ID, &sub.FamilyID, &sub.PlanID, &sub.Status,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.TrialEnd, &sub.CancelledAt, &sub.CancelAtPeriodEnd,
		&sub.StripeSubscriptionID, &sub.StripeCustomerID, &sub.PromoCodeID,
		&sub.CreatedAt, &sub.UpdatedAt,
		&sub.PlanName, &sub.FamilyName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get family subscription: %w", err)
	}

	return &sub, nil
}

// GetActivePlans retrieves all active subscription plans
func (r *billingRepo) GetActivePlans(ctx context.Context) ([]models.SubscriptionPlan, error) {
	query := `
		SELECT
			id, name, description, price_cents, billing_interval,
			features, max_children, max_family_members, is_active,
			stripe_price_id, stripe_product_id, created_at, updated_at
		FROM subscription_plans
		WHERE is_active = TRUE
		ORDER BY price_cents ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active plans: %w", err)
	}
	defer rows.Close()

	var plans []models.SubscriptionPlan
	for rows.Next() {
		var p models.SubscriptionPlan
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.BillingInterval,
			&p.Features, &p.MaxChildren, &p.MaxFamilyMembers, &p.IsActive,
			&p.StripePriceID, &p.StripeProductID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan plan: %w", err)
		}
		plans = append(plans, p)
	}

	return plans, rows.Err()
}

// GetFamilyBillingInfo retrieves combined billing info for the settings page
func (r *billingRepo) GetFamilyBillingInfo(ctx context.Context, familyID uuid.UUID) (*models.FamilyBillingInfo, error) {
	query := `
		SELECT
			fs.id as subscription_id,
			fs.status,
			fs.current_period_start,
			fs.current_period_end,
			sp.id as plan_id,
			sp.name as plan_name,
			sp.price_cents,
			sp.max_children,
			(SELECT COUNT(*) FROM children c WHERE c.family_id = fs.family_id) as child_count
		FROM family_subscriptions fs
		JOIN subscription_plans sp ON fs.plan_id = sp.id
		WHERE fs.family_id = $1
	`

	var info models.FamilyBillingInfo
	var maxChildren int

	err := r.db.QueryRowContext(ctx, query, familyID).Scan(
		&info.SubscriptionID,
		&info.Status,
		&info.CurrentPeriodStart,
		&info.CurrentPeriodEnd,
		&info.PlanID,
		&info.PlanName,
		&info.PriceCents,
		&maxChildren,
		&info.ChildCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get family billing info: %w", err)
	}

	info.MaxChildren = maxChildren

	// Calculate if more children can be added
	// -1 means unlimited
	if maxChildren == -1 {
		info.CanAddMoreChildren = true
	} else {
		info.CanAddMoreChildren = info.ChildCount < maxChildren
	}

	// Format display strings
	if info.PriceCents%100 == 0 {
		info.PriceDisplay = fmt.Sprintf("$%d", info.PriceCents/100)
	} else {
		info.PriceDisplay = fmt.Sprintf("$%d.%02d", info.PriceCents/100, info.PriceCents%100)
	}

	if maxChildren == -1 {
		info.ChildLimitDisplay = "Unlimited"
	} else {
		info.ChildLimitDisplay = fmt.Sprintf("%d", maxChildren)
	}

	info.ExpirationDisplay = info.CurrentPeriodEnd.Format("01/02/2006")

	return &info, nil
}
