package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Billing Enums
// ============================================================================

type BillingInterval string

const (
	BillingIntervalMonthly  BillingInterval = "monthly"
	BillingIntervalYearly   BillingInterval = "yearly"
	BillingIntervalLifetime BillingInterval = "lifetime"
)

type SubscriptionStatus string

const (
	SubscriptionStatusActive    SubscriptionStatus = "active"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	SubscriptionStatusExpired   SubscriptionStatus = "expired"
	SubscriptionStatusPastDue   SubscriptionStatus = "past_due"
	SubscriptionStatusTrialing  SubscriptionStatus = "trialing"
	SubscriptionStatusPaused    SubscriptionStatus = "paused"
)

type PaymentStatus string

const (
	PaymentStatusPending           PaymentStatus = "pending"
	PaymentStatusSucceeded         PaymentStatus = "succeeded"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusRefunded          PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
)

type PaymentType string

const (
	PaymentTypeSubscription PaymentType = "subscription"
	PaymentTypeOneTime      PaymentType = "one_time"
)

type PromoDiscountType string

const (
	PromoDiscountPercentage    PromoDiscountType = "percentage"
	PromoDiscountFixedAmount   PromoDiscountType = "fixed_amount"
	PromoDiscountFreeTrialDays PromoDiscountType = "free_trial_days"
	PromoDiscountFreeMonths    PromoDiscountType = "free_months"
)

type PromoAppliesTo string

const (
	PromoAppliesToSubscription PromoAppliesTo = "subscription"
	PromoAppliesToOneTime      PromoAppliesTo = "one_time"
	PromoAppliesToBoth         PromoAppliesTo = "both"
)

// ============================================================================
// Subscription Plans
// ============================================================================

type SubscriptionPlan struct {
	ID               uuid.UUID       `json:"id"`
	Name             string          `json:"name"`
	Description      NullString      `json:"description,omitempty"`
	PriceCents       int             `json:"price_cents"`
	BillingInterval  BillingInterval `json:"billing_interval"`
	Features         JSONB           `json:"features"`
	MaxChildren      int             `json:"max_children"`
	MaxFamilyMembers int             `json:"max_family_members"`
	IsActive         bool            `json:"is_active"`
	StripePriceID    NullString      `json:"stripe_price_id,omitempty"`
	StripeProductID  NullString      `json:"stripe_product_id,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// ============================================================================
// User Subscriptions
// ============================================================================

type UserSubscription struct {
	ID                   uuid.UUID          `json:"id"`
	UserID               uuid.UUID          `json:"user_id"`
	PlanID               uuid.UUID          `json:"plan_id"`
	Status               SubscriptionStatus `json:"status"`
	CurrentPeriodStart   time.Time          `json:"current_period_start"`
	CurrentPeriodEnd     time.Time          `json:"current_period_end"`
	TrialEnd             NullTime           `json:"trial_end,omitempty"`
	CancelledAt          NullTime           `json:"cancelled_at,omitempty"`
	CancelAtPeriodEnd    bool               `json:"cancel_at_period_end"`
	StripeSubscriptionID NullString         `json:"stripe_subscription_id,omitempty"`
	StripeCustomerID     NullString         `json:"stripe_customer_id,omitempty"`
	PromoCodeID          NullUUID           `json:"promo_code_id,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
	// Populated fields from JOINs
	PlanName  string `json:"plan_name,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

// ============================================================================
// Payments
// ============================================================================

type Payment struct {
	ID                    uuid.UUID     `json:"id"`
	SubscriptionID        NullUUID      `json:"subscription_id,omitempty"`
	UserID                uuid.UUID     `json:"user_id"`
	PaymentType           PaymentType   `json:"payment_type"`
	AmountCents           int           `json:"amount_cents"`
	Currency              string        `json:"currency"`
	Status                PaymentStatus `json:"status"`
	PaymentMethod         NullString    `json:"payment_method,omitempty"`
	StripePaymentIntentID NullString    `json:"stripe_payment_intent_id,omitempty"`
	StripeInvoiceID       NullString    `json:"stripe_invoice_id,omitempty"`
	Description           NullString    `json:"description,omitempty"`
	PromoCodeID           NullUUID      `json:"promo_code_id,omitempty"`
	DiscountAmountCents   int           `json:"discount_amount_cents"`
	RefundAmountCents     int           `json:"refund_amount_cents"`
	RefundedAt            NullTime      `json:"refunded_at,omitempty"`
	FailureReason         NullString    `json:"failure_reason,omitempty"`
	Metadata              JSONB         `json:"metadata"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	// Populated fields from JOINs
	UserEmail     string `json:"user_email,omitempty"`
	UserName      string `json:"user_name,omitempty"`
	PromoCode     string `json:"promo_code,omitempty"`
	PlanName      string `json:"plan_name,omitempty"`
}

// ============================================================================
// Promo Codes
// ============================================================================

type PromoCode struct {
	ID          uuid.UUID         `json:"id"`
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	Description NullString        `json:"description,omitempty"`

	// Discount Configuration
	DiscountType     PromoDiscountType `json:"discount_type"`
	DiscountValue    float64           `json:"discount_value"`
	MaxDiscountCents *int              `json:"max_discount_cents,omitempty"`
	AppliesTo        PromoAppliesTo    `json:"applies_to"`

	// Plan Restrictions
	AppliesToPlans            UUIDArray   `json:"applies_to_plans,omitempty"`
	AppliesToBillingIntervals StringArray `json:"applies_to_billing_intervals,omitempty"`
	MinimumPurchaseCents      int         `json:"minimum_purchase_cents"`

	// User Eligibility
	NewUsersOnly         bool        `json:"new_users_only"`
	ExistingUsersOnly    bool        `json:"existing_users_only"`
	SpecificUserIDs      UUIDArray   `json:"specific_user_ids,omitempty"`
	SpecificEmailDomains StringArray `json:"specific_email_domains,omitempty"`

	// Usage Limits
	MaxTotalUses     *int `json:"max_total_uses,omitempty"`
	MaxUsesPerUser   int  `json:"max_uses_per_user"`
	CurrentTotalUses int  `json:"current_total_uses"`

	// Time Constraints
	StartsAt  time.Time `json:"starts_at"`
	ExpiresAt NullTime  `json:"expires_at,omitempty"`

	// Duration (for recurring discounts)
	DurationMonths *int `json:"duration_months,omitempty"`

	// Stacking Rules
	IsStackable        bool      `json:"is_stackable"`
	StackableWithCodes UUIDArray `json:"stackable_with_codes,omitempty"`

	// Campaign Tracking
	CampaignName   NullString `json:"campaign_name,omitempty"`
	CampaignSource NullString `json:"campaign_source,omitempty"`
	AffiliateID    NullUUID   `json:"affiliate_id,omitempty"`

	// Financial Tracking
	TotalDiscountGivenCents    int64 `json:"total_discount_given_cents"`
	TotalRevenueAttributedCents int64 `json:"total_revenue_attributed_cents"`

	// Status
	IsActive           bool       `json:"is_active"`
	DeactivatedAt      NullTime   `json:"deactivated_at,omitempty"`
	DeactivatedBy      NullUUID   `json:"deactivated_by,omitempty"`
	DeactivationReason NullString `json:"deactivation_reason,omitempty"`

	// Metadata
	CreatedBy NullUUID  `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Populated fields from JOINs
	CreatedByEmail string `json:"created_by_email,omitempty"`
}

// PromoCodeUsage tracks when a promo code was used
type PromoCodeUsage struct {
	ID                   uuid.UUID `json:"id"`
	PromoCodeID          uuid.UUID `json:"promo_code_id"`
	UserID               uuid.UUID `json:"user_id"`
	SubscriptionID       NullUUID  `json:"subscription_id,omitempty"`
	PaymentID            NullUUID  `json:"payment_id,omitempty"`
	DiscountAppliedCents int       `json:"discount_applied_cents"`
	UsedAt               time.Time `json:"used_at"`
	// Populated fields from JOINs
	PromoCode string `json:"promo_code,omitempty"`
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

// ============================================================================
// Revenue Tracking
// ============================================================================

type DailyRevenueSnapshot struct {
	ID                     int       `json:"id"`
	SnapshotDate           time.Time `json:"snapshot_date"`
	TotalRevenueCents      int64     `json:"total_revenue_cents"`
	NewSubscriptions       int       `json:"new_subscriptions"`
	CancelledSubscriptions int       `json:"cancelled_subscriptions"`
	Upgrades               int       `json:"upgrades"`
	Downgrades             int       `json:"downgrades"`
	RefundsCents           int64     `json:"refunds_cents"`
	PromoDiscountsCents    int64     `json:"promo_discounts_cents"`
	CalculatedAt           time.Time `json:"calculated_at"`
}

type ExpectedRevenueDay struct {
	Date         time.Time `json:"date"`
	AmountCents  int64     `json:"amount_cents"`
	RenewalCount int       `json:"renewal_count"`
}

// ============================================================================
// Financial Overview
// ============================================================================

type FinancialOverview struct {
	// Last 24 hours
	LicensesBought24h int   `json:"licenses_bought_24h"`
	Revenue24hCents   int64 `json:"revenue_24h_cents"`

	// Month to date
	RevenueMTDCents         int64 `json:"revenue_mtd_cents"`
	NewSubscriptionsMTD     int   `json:"new_subscriptions_mtd"`
	ChurnedSubscriptionsMTD int   `json:"churned_subscriptions_mtd"`

	// Year to date
	RevenueYTDCents          int64 `json:"revenue_ytd_cents"`
	TotalActiveSubscriptions int   `json:"total_active_subscriptions"`

	// Breakdown by plan
	SubscriptionsByPlan []PlanSubscriptionCount `json:"subscriptions_by_plan"`

	// Promo impact
	TotalDiscountsYTDCents int64 `json:"total_discounts_ytd_cents"`
}

type PlanSubscriptionCount struct {
	PlanID   uuid.UUID `json:"plan_id"`
	PlanName string    `json:"plan_name"`
	Count    int       `json:"count"`
	MRRCents int64     `json:"mrr_cents"` // Monthly Recurring Revenue
}
