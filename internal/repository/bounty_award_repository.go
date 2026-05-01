package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// BountyCandidate is one row in the "what could be rewarded this month?"
// list. The shape is denormalized so the admin page can render the picker
// without N+1 lookups.
type BountyCandidate struct {
	Type             string          `json:"type"`               // "bug" | "feature"
	TicketID         models.NullUUID `json:"ticket_id,omitempty"`
	RoadmapItemID    models.NullUUID `json:"roadmap_item_id,omitempty"`
	RecipientUserID  uuid.UUID       `json:"recipient_user_id"`
	RecipientEmail   string          `json:"recipient_email"`
	RecipientName    string          `json:"recipient_name"`
	Subject          string          `json:"subject"`            // ticket subject or roadmap title
	ResolvedOrShipAt time.Time       `json:"resolved_or_ship_at"`// drives "past 30 days"
	DupCount         int             `json:"dup_count,omitempty"`// how many users hit this bug (popularity hint)
	AlreadyAwarded   bool            `json:"already_awarded"`    // true if a row in bounty_awards exists for current month
	Decision         string          `json:"decision,omitempty"` // "selected" | "thanks_anyway" | ""

	// SourceTicketID is the ticket where the bounty notification message
	// should be posted. For bugs: same as TicketID. For features: the
	// follower's original ticket (from roadmap_item_followers).
	SourceTicketID models.NullUUID `json:"source_ticket_id,omitempty"`
}

// BountyAward is one persisted award row.
type BountyAward struct {
	ID              uuid.UUID       `json:"id"`
	AwardMonth      time.Time       `json:"award_month"`
	AwardType       string          `json:"award_type"`
	Decision        string          `json:"decision"`
	TicketID        models.NullUUID `json:"ticket_id,omitempty"`
	RoadmapItemID   models.NullUUID `json:"roadmap_item_id,omitempty"`
	RecipientUserID uuid.UUID       `json:"recipient_user_id"`
	PromoCodeID     models.NullUUID `json:"promo_code_id,omitempty"`
	AwardedBy       uuid.UUID       `json:"awarded_by"`
	AwardedAt       time.Time       `json:"awarded_at"`
	Notes           string          `json:"notes,omitempty"`

	// Joined convenience fields (not in bounty_awards).
	RecipientEmail string `json:"recipient_email,omitempty"`
	RecipientName  string `json:"recipient_name,omitempty"`
	PromoCode      string `json:"promo_code,omitempty"`
	Subject        string `json:"subject,omitempty"`
	AwardedByEmail string `json:"awarded_by_email,omitempty"`
}

// BountyAwardRepository is the DB layer for the bounty program.
type BountyAwardRepository interface {
	// EligibleBugs returns reporter+ticket pairs whose ticket was resolved
	// in [windowStart, now] and is type=bug_report and not a duplicate.
	EligibleBugs(ctx context.Context, windowStart time.Time, awardMonth time.Time) ([]BountyCandidate, error)

	// EligibleFeatures returns roadmap_item + follower pairs for items that
	// hit prod_released_at in [windowStart, now] sourced from feature requests.
	EligibleFeatures(ctx context.Context, windowStart time.Time, awardMonth time.Time) ([]BountyCandidate, error)

	// CreateAward inserts a row. award.PromoCodeID must be set for selected
	// awards and nil for thanks_anyway awards (DB CHECK enforces this too).
	CreateAward(ctx context.Context, award *BountyAward) error

	// ListAwards returns all awards in a given month (for the "this month"
	// section of the admin page) joined with recipient + promo code + subject.
	ListAwards(ctx context.Context, awardMonth time.Time) ([]BountyAward, error)

	// HistoryAwards returns recent past awards (most recent N rows across
	// all months) for the audit-trail section.
	HistoryAwards(ctx context.Context, limit int) ([]BountyAward, error)

	// CountSelected returns how many "selected" awards already exist for
	// (awardMonth, awardType) so the service can enforce the per-type caps.
	CountSelected(ctx context.Context, awardMonth time.Time, awardType string) (int, error)
}

type bountyAwardRepo struct {
	db *sql.DB
}

func NewBountyAwardRepo(db *sql.DB) BountyAwardRepository {
	return &bountyAwardRepo{db: db}
}

func (r *bountyAwardRepo) EligibleBugs(ctx context.Context, windowStart time.Time, awardMonth time.Time) ([]BountyCandidate, error) {
	q := `
        SELECT
            'bug' AS type,
            t.id AS ticket_id,
            NULL::uuid AS roadmap_item_id,
            t.user_id AS recipient_user_id,
            COALESCE(u.email, '') AS recipient_email,
            COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), '') AS recipient_name,
            t.subject,
            COALESCE(t.resolved_at, t.updated_at) AS resolved_or_ship_at,
            (SELECT COUNT(*) FROM support_tickets d WHERE d.duplicate_of_ticket_id = t.id) AS dup_count,
            EXISTS (
                SELECT 1 FROM bounty_awards a
                WHERE a.ticket_id = t.id AND a.award_month = $2
            ) AS already_awarded,
            COALESCE((
                SELECT a.decision::text FROM bounty_awards a
                WHERE a.ticket_id = t.id AND a.award_month = $2
                LIMIT 1
            ), '') AS decision,
            t.id AS source_ticket_id
        FROM support_tickets t
        LEFT JOIN users u ON u.id = t.user_id
        WHERE t.type = 'bug_report'
          AND t.status IN ('resolved','closed')
          AND t.duplicate_of_ticket_id IS NULL
          AND t.duplicate_of_roadmap_id IS NULL
          AND COALESCE(t.resolved_at, t.updated_at) >= $1
          AND t.user_id IS NOT NULL
        ORDER BY dup_count DESC, resolved_or_ship_at DESC
    `
	rows, err := r.db.QueryContext(ctx, q, windowStart, awardMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (r *bountyAwardRepo) EligibleFeatures(ctx context.Context, windowStart time.Time, awardMonth time.Time) ([]BountyCandidate, error) {
	// One candidate row per (roadmap_item × follower). Roadmap items qualify
	// when they were promoted from a feature_request ticket AND have hit
	// prod (mark-live-prod called).
	q := `
        SELECT
            'feature' AS type,
            NULL::uuid AS ticket_id,
            r.id AS roadmap_item_id,
            f.user_id AS recipient_user_id,
            COALESCE(u.email,'') AS recipient_email,
            COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), '') AS recipient_name,
            r.title AS subject,
            r.prod_released_at AS resolved_or_ship_at,
            (SELECT COUNT(*) FROM roadmap_item_followers ff WHERE ff.roadmap_id = r.id) AS dup_count,
            EXISTS (
                SELECT 1 FROM bounty_awards a
                WHERE a.roadmap_item_id = r.id
                  AND a.recipient_user_id = f.user_id
                  AND a.award_month = $2
            ) AS already_awarded,
            COALESCE((
                SELECT a.decision::text FROM bounty_awards a
                WHERE a.roadmap_item_id = r.id
                  AND a.recipient_user_id = f.user_id
                  AND a.award_month = $2
                LIMIT 1
            ), '') AS decision,
            f.source_ticket_id
        FROM roadmap_items r
        JOIN roadmap_item_followers f ON f.roadmap_id = r.id
        LEFT JOIN users u ON u.id = f.user_id
        WHERE r.source = 'feature_request'
          AND r.prod_released_at IS NOT NULL
          AND r.prod_released_at >= $1
        ORDER BY r.prod_released_at DESC
    `
	rows, err := r.db.QueryContext(ctx, q, windowStart, awardMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func scanCandidates(rows *sql.Rows) ([]BountyCandidate, error) {
	var out []BountyCandidate
	for rows.Next() {
		c := BountyCandidate{}
		if err := rows.Scan(
			&c.Type, &c.TicketID, &c.RoadmapItemID, &c.RecipientUserID,
			&c.RecipientEmail, &c.RecipientName, &c.Subject, &c.ResolvedOrShipAt,
			&c.DupCount, &c.AlreadyAwarded, &c.Decision, &c.SourceTicketID,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *bountyAwardRepo) CreateAward(ctx context.Context, a *BountyAward) error {
	q := `
        INSERT INTO bounty_awards
            (award_month, award_type, decision, ticket_id, roadmap_item_id,
             recipient_user_id, promo_code_id, awarded_by, notes)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9,''))
        RETURNING id, awarded_at
    `
	return r.db.QueryRowContext(ctx, q,
		a.AwardMonth, a.AwardType, a.Decision, a.TicketID, a.RoadmapItemID,
		a.RecipientUserID, a.PromoCodeID, a.AwardedBy, a.Notes,
	).Scan(&a.ID, &a.AwardedAt)
}

const awardSelectColumns = `
    a.id, a.award_month, a.award_type::text, a.decision::text,
    a.ticket_id, a.roadmap_item_id, a.recipient_user_id, a.promo_code_id,
    a.awarded_by, a.awarded_at, COALESCE(a.notes,''),
    COALESCE(u.email,''),
    COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), ''),
    COALESCE(p.code, ''),
    COALESCE(t.subject, r.title, ''),
    COALESCE(au.email,'')
`

const awardFromJoins = `
    FROM bounty_awards a
    LEFT JOIN users u           ON u.id = a.recipient_user_id
    LEFT JOIN promo_codes p     ON p.id = a.promo_code_id
    LEFT JOIN support_tickets t ON t.id = a.ticket_id
    LEFT JOIN roadmap_items r   ON r.id = a.roadmap_item_id
    LEFT JOIN users au          ON au.id = a.awarded_by
`

func (r *bountyAwardRepo) ListAwards(ctx context.Context, awardMonth time.Time) ([]BountyAward, error) {
	q := "SELECT " + awardSelectColumns + awardFromJoins + " WHERE a.award_month = $1 ORDER BY a.awarded_at DESC"
	rows, err := r.db.QueryContext(ctx, q, awardMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAwards(rows)
}

func (r *bountyAwardRepo) HistoryAwards(ctx context.Context, limit int) ([]BountyAward, error) {
	q := "SELECT " + awardSelectColumns + awardFromJoins + " ORDER BY a.awarded_at DESC LIMIT $1"
	rows, err := r.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAwards(rows)
}

func scanAwards(rows *sql.Rows) ([]BountyAward, error) {
	var out []BountyAward
	for rows.Next() {
		a := BountyAward{}
		if err := rows.Scan(
			&a.ID, &a.AwardMonth, &a.AwardType, &a.Decision,
			&a.TicketID, &a.RoadmapItemID, &a.RecipientUserID, &a.PromoCodeID,
			&a.AwardedBy, &a.AwardedAt, &a.Notes,
			&a.RecipientEmail, &a.RecipientName,
			&a.PromoCode, &a.Subject, &a.AwardedByEmail,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *bountyAwardRepo) CountSelected(ctx context.Context, awardMonth time.Time, awardType string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `
        SELECT COUNT(*) FROM bounty_awards
        WHERE award_month = $1 AND award_type = $2 AND decision = 'selected'
    `, awardMonth, awardType).Scan(&n)
	return n, err
}
