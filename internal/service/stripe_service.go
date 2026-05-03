package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/price"
	"github.com/stripe/stripe-go/v76/product"
	"github.com/stripe/stripe-go/v76/webhook"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// StripeService wraps the stripe-go SDK for plan provisioning, Checkout
// session creation, and webhook signature verification. The SDK uses a
// process-global API key (stripe.Key) — we set it once in the constructor.
type StripeService struct {
	cfg         config.StripeConfig
	billingRepo repository.BillingRepository
	subSvc      *SubscriptionService
	successURL  string
	cancelURL   string
}

// NewStripeService initializes the SDK key and returns the service. Callers
// should check StripeConfig.Enabled() before invoking this — when the key
// is empty the SDK will still construct but every API call returns
// "no api key provided". subSvc may be nil at construction; set later via
// SetSubscriptionService once that service is wired up.
func NewStripeService(cfg config.StripeConfig, billingRepo repository.BillingRepository, appURL string) *StripeService {
	stripe.Key = cfg.SecretKey
	return &StripeService{
		cfg:         cfg,
		billingRepo: billingRepo,
		successURL:  appURL + "/billing/success?session_id={CHECKOUT_SESSION_ID}",
		cancelURL:   appURL + "/billing/cancel",
	}
}

// SetSubscriptionService attaches the subscription service. Stripe webhook
// dispatch needs it to write back to family_subscriptions.
func (s *StripeService) SetSubscriptionService(sub *SubscriptionService) {
	s.subSvc = sub
}

// EnsureAllPlansSynced walks every active plan and provisions a Stripe
// Product + Price for any that don't yet have stripe_product_id /
// stripe_price_id set. Idempotent — safe to call on every server boot.
// On a fresh Stripe account with 2 plans this hits the API 4 times then
// stays silent on subsequent boots.
func (s *StripeService) EnsureAllPlansSynced(ctx context.Context) error {
	if !s.cfg.Enabled() {
		return nil
	}
	plans, err := s.billingRepo.GetActivePlans(ctx)
	if err != nil {
		return fmt.Errorf("get active plans: %w", err)
	}
	for _, p := range plans {
		if p.StripeProductID.Valid && p.StripePriceID.Valid {
			continue
		}
		if err := s.syncPlan(ctx, p); err != nil {
			return fmt.Errorf("sync plan %s (%s): %w", p.Name, p.ID, err)
		}
	}
	return nil
}

// syncPlan creates a Stripe Product (or reuses the existing one) and a
// recurring Price matching the plan's cents + interval, then writes both
// IDs back to subscription_plans. Stripe forbids deleting prices that
// have been used, so we never overwrite an existing one — if a plan
// already has a price ID we skip it entirely (handled in the caller).
func (s *StripeService) syncPlan(ctx context.Context, p models.SubscriptionPlan) error {
	productID := ""
	if p.StripeProductID.Valid {
		productID = p.StripeProductID.String
	} else {
		prodParams := &stripe.ProductParams{
			Name: stripe.String(p.Name),
			Metadata: map[string]string{
				"plan_id": p.ID.String(),
			},
		}
		if p.Description.Valid && p.Description.String != "" {
			prodParams.Description = stripe.String(p.Description.String)
		}
		prod, err := product.New(prodParams)
		if err != nil {
			return fmt.Errorf("create product: %w", err)
		}
		productID = prod.ID
		log.Printf("[STRIPE] created product %s for plan %q", productID, p.Name)
	}

	interval := stripeInterval(p.BillingInterval)
	if interval == "" {
		return fmt.Errorf("unsupported billing_interval %q", p.BillingInterval)
	}
	priceParams := &stripe.PriceParams{
		Product:    stripe.String(productID),
		Currency:   stripe.String(string(stripe.CurrencyUSD)),
		UnitAmount: stripe.Int64(int64(p.PriceCents)),
		Recurring: &stripe.PriceRecurringParams{
			Interval: stripe.String(interval),
		},
		Metadata: map[string]string{
			"plan_id": p.ID.String(),
		},
	}
	pr, err := price.New(priceParams)
	if err != nil {
		return fmt.Errorf("create price: %w", err)
	}
	log.Printf("[STRIPE] created price %s ($%.2f/%s) for plan %q",
		pr.ID, float64(p.PriceCents)/100, interval, p.Name)

	return s.billingRepo.UpdatePlanStripeIDs(ctx, p.ID, productID, pr.ID)
}

func stripeInterval(bi models.BillingInterval) string {
	switch bi {
	case models.BillingIntervalMonthly:
		return string(stripe.PriceRecurringIntervalMonth)
	case models.BillingIntervalYearly:
		return string(stripe.PriceRecurringIntervalYear)
	default:
		return ""
	}
}

// CheckoutParams carries the inputs the handler needs to provide for a
// new Stripe Checkout session. We pass family/plan IDs through metadata
// so the webhook can match the resulting subscription back to our DB.
type CheckoutParams struct {
	FamilyID         uuid.UUID
	PlanID           uuid.UUID
	StripePriceID    string
	StripeCustomerID string // empty for first-time customers
	CustomerEmail    string // used when StripeCustomerID is empty
	TrialDays        int    // 0 = no trial
}

// CreateCheckoutSession returns the URL the client should redirect to for
// payment collection. The caller is responsible for emitting a 303 redirect
// to session.URL.
func (s *StripeService) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (*stripe.CheckoutSession, error) {
	if !s.cfg.Enabled() {
		return nil, fmt.Errorf("stripe not configured")
	}
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(p.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(s.successURL),
		CancelURL:  stripe.String(s.cancelURL),
		Metadata: map[string]string{
			"family_id": p.FamilyID.String(),
			"plan_id":   p.PlanID.String(),
		},
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{
				"family_id": p.FamilyID.String(),
				"plan_id":   p.PlanID.String(),
			},
		},
	}
	if p.StripeCustomerID != "" {
		params.Customer = stripe.String(p.StripeCustomerID)
	} else if p.CustomerEmail != "" {
		params.CustomerEmail = stripe.String(p.CustomerEmail)
	}
	if p.TrialDays > 0 {
		params.SubscriptionData.TrialPeriodDays = stripe.Int64(int64(p.TrialDays))
	}
	sess, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("create checkout session: %w", err)
	}
	return sess, nil
}

// VerifyWebhookSignature verifies the Stripe-Signature header against the
// payload using the configured webhook secret. Always reject if the secret
// is unset — silently accepting unsigned events would let any HTTP client
// trigger billing state changes.
func (s *StripeService) VerifyWebhookSignature(payload []byte, sigHeader string) (stripe.Event, error) {
	if s.cfg.WebhookSecret == "" {
		return stripe.Event{}, fmt.Errorf("webhook secret not configured")
	}
	return webhook.ConstructEvent(payload, sigHeader, s.cfg.WebhookSecret)
}

// HandleEvent dispatches a verified Stripe event to the right
// SubscriptionService mutator. Unknown event types are logged and
// ignored (Stripe sends many event types we don't care about — cards
// added to wallets, payment methods updated, etc).
func (s *StripeService) HandleEvent(ctx context.Context, ev stripe.Event) error {
	if s.subSvc == nil {
		return fmt.Errorf("subscription service not wired")
	}
	switch ev.Type {
	case "checkout.session.completed":
		return s.handleCheckoutCompleted(ctx, ev)
	case "customer.subscription.updated", "customer.subscription.deleted":
		return s.handleSubscriptionUpdated(ctx, ev)
	case "invoice.paid", "invoice.payment_succeeded":
		return s.handleInvoicePaid(ctx, ev)
	case "invoice.payment_failed":
		return s.handleInvoicePaymentFailed(ctx, ev)
	default:
		log.Printf("[STRIPE] ignoring event type %s (id=%s)", ev.Type, ev.ID)
		return nil
	}
}

func (s *StripeService) handleCheckoutCompleted(ctx context.Context, ev stripe.Event) error {
	var sess stripe.CheckoutSession
	if err := json.Unmarshal(ev.Data.Raw, &sess); err != nil {
		return fmt.Errorf("decode checkout session: %w", err)
	}
	familyIDStr := sess.Metadata["family_id"]
	planIDStr := sess.Metadata["plan_id"]
	familyID, err := uuid.Parse(familyIDStr)
	if err != nil {
		return fmt.Errorf("checkout missing/invalid family_id metadata: %q", familyIDStr)
	}
	planID, err := uuid.Parse(planIDStr)
	if err != nil {
		return fmt.Errorf("checkout missing/invalid plan_id metadata: %q", planIDStr)
	}
	if sess.Subscription == nil || sess.Subscription.ID == "" {
		return fmt.Errorf("checkout session has no subscription")
	}
	customerID := ""
	if sess.Customer != nil {
		customerID = sess.Customer.ID
	}
	// CheckoutSession doesn't always include subscription period info — we
	// pull current_period_end off the embedded Subscription if present, else
	// default to NOW()+30d (the customer.subscription.updated event lands
	// almost immediately after and overwrites it).
	periodEnd := time.Now().Add(30 * 24 * time.Hour)
	status := "active"
	if sess.Subscription != nil {
		if sess.Subscription.CurrentPeriodEnd != 0 {
			periodEnd = time.Unix(sess.Subscription.CurrentPeriodEnd, 0)
		}
		if sess.Subscription.Status != "" {
			status = string(sess.Subscription.Status)
		}
	}
	log.Printf("[STRIPE] checkout.session.completed family=%s plan=%s sub=%s status=%s",
		familyID, planID, sess.Subscription.ID, status)
	return s.subSvc.ApplyCheckoutCompleted(ctx, familyID, planID, customerID, sess.Subscription.ID, status, periodEnd)
}

func (s *StripeService) handleSubscriptionUpdated(ctx context.Context, ev stripe.Event) error {
	var sub stripe.Subscription
	if err := json.Unmarshal(ev.Data.Raw, &sub); err != nil {
		return fmt.Errorf("decode subscription: %w", err)
	}
	periodEnd := time.Unix(sub.CurrentPeriodEnd, 0)
	var cancelledAt *time.Time
	if sub.CanceledAt != 0 {
		t := time.Unix(sub.CanceledAt, 0)
		cancelledAt = &t
	}
	status := string(sub.Status)
	if ev.Type == "customer.subscription.deleted" {
		status = "cancelled"
	}
	log.Printf("[STRIPE] %s sub=%s status=%s cancel_at_period_end=%v",
		ev.Type, sub.ID, status, sub.CancelAtPeriodEnd)
	return s.subSvc.ApplySubscriptionUpdated(ctx, sub.ID, status, periodEnd, sub.CancelAtPeriodEnd, cancelledAt)
}

func (s *StripeService) handleInvoicePaid(ctx context.Context, ev stripe.Event) error {
	var inv stripe.Invoice
	if err := json.Unmarshal(ev.Data.Raw, &inv); err != nil {
		return fmt.Errorf("decode invoice: %w", err)
	}
	if inv.Subscription == nil || inv.Subscription.ID == "" {
		// One-off invoices (not subscription-related) don't move our state.
		return nil
	}
	// Use the line item period end if present, else fall back to NOW()+30d.
	periodEnd := time.Now().Add(30 * 24 * time.Hour)
	if inv.Lines != nil && len(inv.Lines.Data) > 0 {
		if inv.Lines.Data[0].Period != nil && inv.Lines.Data[0].Period.End != 0 {
			periodEnd = time.Unix(inv.Lines.Data[0].Period.End, 0)
		}
	}
	log.Printf("[STRIPE] invoice paid sub=%s amount=%d period_end=%s",
		inv.Subscription.ID, inv.AmountPaid, periodEnd.Format(time.RFC3339))
	return s.subSvc.ApplyInvoicePaid(ctx, inv.Subscription.ID, periodEnd, inv.AmountPaid, string(inv.Currency), inv.ID)
}

func (s *StripeService) handleInvoicePaymentFailed(ctx context.Context, ev stripe.Event) error {
	var inv stripe.Invoice
	if err := json.Unmarshal(ev.Data.Raw, &inv); err != nil {
		return fmt.Errorf("decode invoice: %w", err)
	}
	if inv.Subscription == nil || inv.Subscription.ID == "" {
		return nil
	}
	log.Printf("[STRIPE] invoice payment_failed sub=%s", inv.Subscription.ID)
	return s.subSvc.ApplyInvoicePaymentFailed(ctx, inv.Subscription.ID)
}
