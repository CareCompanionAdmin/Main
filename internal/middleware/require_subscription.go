package middleware

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// EntitlementMode classifies what the user can do based on their family's
// subscription state. Used by route-level enforcement to gate writes.
type EntitlementMode string

const (
	EntitlementFull     EntitlementMode = "full"
	EntitlementReadOnly EntitlementMode = "read_only"
	EntitlementBlocked  EntitlementMode = "blocked"
)

// Entitlement is the per-request subscription view set by LoadEntitlement
// and consulted by EnforceWriteEntitlement.
type Entitlement struct {
	Mode             EntitlementMode
	Status           string // raw subscription_status enum from DB
	TrialEnd         *time.Time
	PeriodEnd        *time.Time
	PastDueSince     *time.Time
	ReadOnlyUntil    *time.Time // past_due_since + 14d
	IsAdminOverride  bool       // true when system_role bypassed the check
	HasSubscription  bool       // false if family has no row (treat as full per current alpha behavior)
}

const EntitlementKey contextKey = "entitlement"

// readOnlyWindow is the grace period after trial-end / payment-failure
// during which the family can read + export + delete + contact support
// but cannot create new content. Locked to 14 days per the billing spec.
const readOnlyWindow = 14 * 24 * time.Hour

// LoadEntitlement reads the family's subscription row (if any) and stores
// an Entitlement in the request context. Always continues to the next
// handler — the writeable / blocked decision happens in
// EnforceWriteEntitlement, applied per route group. Admins (system_role
// in super_admin/support/marketing) get a hard-coded "full" entitlement
// without a DB hit.
func LoadEntitlement(db *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ent := computeEntitlement(r.Context(), db)
			ctx := context.WithValue(r.Context(), EntitlementKey, ent)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func computeEntitlement(ctx context.Context, db *sql.DB) Entitlement {
	// Admin bypass — they get to use the app regardless of family billing
	// state. There's no row in family_subscriptions for admin-only users
	// anyway (they typically have no family).
	role := GetSystemRole(ctx)
	switch models.SystemRole(role) {
	case models.SystemRoleSuperAdmin, models.SystemRoleSupport, models.SystemRoleMarketing:
		return Entitlement{Mode: EntitlementFull, IsAdminOverride: true, HasSubscription: false}
	}

	familyID := GetFamilyID(ctx)
	if familyID == uuid.Nil {
		// No family context — let the request through; the handler will
		// redirect to /family/new or return its own error. We can't gate
		// on a subscription that doesn't exist yet.
		return Entitlement{Mode: EntitlementFull, HasSubscription: false}
	}

	var (
		status        string
		trialEnd      sql.NullTime
		periodEnd     sql.NullTime
		pastDueSince  sql.NullTime
	)
	err := db.QueryRowContext(ctx, `
        SELECT status, trial_end, current_period_end, past_due_since
        FROM family_subscriptions
        WHERE family_id = $1`, familyID,
	).Scan(&status, &trialEnd, &periodEnd, &pastDueSince)
	if errors.Is(err, sql.ErrNoRows) {
		// Family has no subscription row. During alpha we permit access
		// (Phase 2 fires StartTrialIfNew on every signup, but legacy rows
		// might still exist). Switch this to EntitlementBlocked once
		// every family is guaranteed to have a row.
		return Entitlement{Mode: EntitlementFull, HasSubscription: false}
	}
	if err != nil {
		// DB error — fail OPEN. A transient blip on the subscription read
		// shouldn't lock users out of their data.
		return Entitlement{Mode: EntitlementFull, HasSubscription: false}
	}

	ent := Entitlement{
		Status:          status,
		HasSubscription: true,
	}
	if trialEnd.Valid {
		t := trialEnd.Time
		ent.TrialEnd = &t
	}
	if periodEnd.Valid {
		t := periodEnd.Time
		ent.PeriodEnd = &t
	}
	if pastDueSince.Valid {
		t := pastDueSince.Time
		ent.PastDueSince = &t
		deadline := t.Add(readOnlyWindow)
		ent.ReadOnlyUntil = &deadline
	}

	now := time.Now()
	switch status {
	case "active", "comped":
		// period_end may be in the past on `active` if the periodic Stripe
		// renewal hasn't landed yet — be lenient by 24h to absorb that lag.
		if ent.PeriodEnd == nil || ent.PeriodEnd.Add(24*time.Hour).After(now) {
			ent.Mode = EntitlementFull
		} else {
			ent.Mode = EntitlementBlocked
		}
	case "trialing":
		if ent.TrialEnd != nil && ent.TrialEnd.After(now) {
			ent.Mode = EntitlementFull
		} else {
			// Trial has lapsed but the hourly sweeper hasn't flipped to
			// past_due yet. Treat as read-only with a synthetic deadline
			// so the user isn't fully blocked between sweeps.
			ent.Mode = EntitlementReadOnly
			if ent.TrialEnd != nil {
				deadline := ent.TrialEnd.Add(readOnlyWindow)
				ent.ReadOnlyUntil = &deadline
			}
		}
	case "past_due":
		if ent.ReadOnlyUntil != nil && ent.ReadOnlyUntil.After(now) {
			ent.Mode = EntitlementReadOnly
		} else {
			ent.Mode = EntitlementBlocked
		}
	case "paused":
		ent.Mode = EntitlementReadOnly
	case "cancelled", "expired", "terminated":
		ent.Mode = EntitlementBlocked
	default:
		ent.Mode = EntitlementFull
	}
	return ent
}

// GetEntitlement extracts the entitlement set by LoadEntitlement. Falls
// back to "full" when the middleware wasn't applied — never call this
// from a route that hasn't first run LoadEntitlement, or you'll always
// get the lenient default.
func GetEntitlement(ctx context.Context) Entitlement {
	if e, ok := ctx.Value(EntitlementKey).(Entitlement); ok {
		return e
	}
	return Entitlement{Mode: EntitlementFull, HasSubscription: false}
}

// EnforceWriteEntitlement blocks state-mutating requests when the family
// is in read-only or blocked mode. GET / HEAD / OPTIONS always pass —
// reads remain available even when the family is locked out.
//
// Apply this AFTER LoadEntitlement, on route groups whose mutations
// should be gated. Routes that should always work (data deletion, support
// tickets, billing pages, account self-service) do not get this
// middleware.
//
// On block, returns:
//   - 402 Payment Required + JSON body for /api/* requests
//   - 303 redirect to /settings (where the upgrade CTA lives) for web
//     POSTs (form submits) since 402 doesn't render a page
func EnforceWriteEntitlement() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}
			ent := GetEntitlement(r.Context())
			if ent.Mode == EntitlementFull {
				next.ServeHTTP(w, r)
				return
			}
			// Read-only families retain the right to delete their data and
			// export it. Blocked families lose that too — at that point the
			// data has been moved to cold storage (Phase 5).
			if ent.Mode == EntitlementReadOnly && r.Method == http.MethodDelete {
				next.ServeHTTP(w, r)
				return
			}
			writeEntitlementBlock(w, r, ent)
		})
	}
}

func writeEntitlementBlock(w http.ResponseWriter, r *http.Request, ent Entitlement) {
	isAPI := strings.HasPrefix(r.URL.Path, "/api/")
	if isAPI {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		body := map[string]interface{}{
			"error":  "subscription_required",
			"mode":   ent.Mode,
			"status": ent.Status,
		}
		if ent.ReadOnlyUntil != nil {
			body["read_only_until"] = ent.ReadOnlyUntil.Format(time.RFC3339)
		}
		if ent.TrialEnd != nil {
			body["trial_end"] = ent.TrialEnd.Format(time.RFC3339)
		}
		if ent.PeriodEnd != nil {
			body["period_end"] = ent.PeriodEnd.Format(time.RFC3339)
		}
		_ = json.NewEncoder(w).Encode(body)
		return
	}
	// Web (form-submit) path — 303 to settings, where the Subscribe CTA
	// is wired. We don't have a paywall page yet (Phase 4 follow-up); the
	// settings page already shows the relevant state + upgrade button.
	http.Redirect(w, r, "/settings?paywall=1", http.StatusSeeOther)
}
