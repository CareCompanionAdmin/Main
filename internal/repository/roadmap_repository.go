package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// RoadmapItem is the persisted shape of a roadmap row plus a few joined
// convenience fields so the admin UI can render a list without a second
// round-trip per row.
type RoadmapItem struct {
	ID              uuid.UUID       `json:"id"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	Status          string          `json:"status"`
	Priority        string          `json:"priority"`
	Source          string          `json:"source"`
	SourceTicketID  models.NullUUID `json:"source_ticket_id,omitempty"`
	RequesterUserID models.NullUUID `json:"requester_user_id,omitempty"`
	NotifyOnDev     bool            `json:"notify_on_dev"`
	NotifyOnProd    bool            `json:"notify_on_prod"`
	DevReleasedAt   models.NullTime `json:"dev_released_at,omitempty"`
	ProdReleasedAt  models.NullTime `json:"prod_released_at,omitempty"`
	CreatedBy       models.NullUUID `json:"created_by,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`

	// Joined / convenience fields (not in DB).
	RequesterEmail string `json:"requester_email,omitempty"`
	RequesterName  string `json:"requester_name,omitempty"`
	TicketSubject  string `json:"ticket_subject,omitempty"`
	FollowerCount  int    `json:"follower_count,omitempty"`
}

// RoadmapRepository is a thin DB layer over roadmap_items. The cross-table
// orchestration (closing tickets, sending emails) lives in the service.
type RoadmapRepository interface {
	List(ctx context.Context, status, priority, source string) ([]RoadmapItem, error)
	GetByID(ctx context.Context, id uuid.UUID) (*RoadmapItem, error)
	GetByTicketID(ctx context.Context, ticketID uuid.UUID) (*RoadmapItem, error)
	Create(ctx context.Context, item *RoadmapItem) error
	Update(ctx context.Context, item *RoadmapItem) error
	Delete(ctx context.Context, id uuid.UUID) error
	MarkDevReleased(ctx context.Context, id uuid.UUID) error
	MarkProdReleased(ctx context.Context, id uuid.UUID) error

	// Followers — multiple users can be subscribed to release notifications
	// for a single roadmap item (one canonical promoter + N duplicates).
	AddFollower(ctx context.Context, roadmapID, userID uuid.UUID, sourceTicketID *uuid.UUID, notifyDev, notifyProd bool) error
	ListFollowers(ctx context.Context, roadmapID uuid.UUID) ([]RoadmapFollower, error)
	FollowerCount(ctx context.Context, roadmapID uuid.UUID) (int, error)
}

// RoadmapFollower is a user subscribed to release notifications for a roadmap
// item, plus the ticket through which they expressed interest (so the release
// message can be posted on their own thread).
type RoadmapFollower struct {
	RoadmapID      uuid.UUID       `json:"roadmap_id"`
	UserID         uuid.UUID       `json:"user_id"`
	SourceTicketID models.NullUUID `json:"source_ticket_id,omitempty"`
	NotifyOnDev    bool            `json:"notify_on_dev"`
	NotifyOnProd   bool            `json:"notify_on_prod"`
	CreatedAt      time.Time       `json:"created_at"`
	// Joined
	UserEmail     string `json:"user_email,omitempty"`
	UserName      string `json:"user_name,omitempty"`
	TicketSubject string `json:"ticket_subject,omitempty"`
}

type roadmapRepo struct {
	db *sql.DB
}

// NewRoadmapRepo creates a RoadmapRepository.
func NewRoadmapRepo(db *sql.DB) RoadmapRepository {
	return &roadmapRepo{db: db}
}

const roadmapSelectColumns = `
    r.id, r.title, r.description, r.status, r.priority, r.source,
    r.source_ticket_id, r.requester_user_id,
    r.notify_on_dev, r.notify_on_prod,
    r.dev_released_at, r.prod_released_at,
    r.created_by, r.created_at, r.updated_at,
    COALESCE(u.email, '')                                  AS requester_email,
    COALESCE(u.first_name || ' ' || u.last_name, '')       AS requester_name,
    COALESCE(t.subject, '')                                AS ticket_subject,
    (SELECT COUNT(*) FROM roadmap_item_followers f WHERE f.roadmap_id = r.id) AS follower_count
`

const roadmapFromJoins = `
    FROM roadmap_items r
    LEFT JOIN users u           ON r.requester_user_id = u.id
    LEFT JOIN support_tickets t ON r.source_ticket_id   = t.id
`

func (r *roadmapRepo) List(ctx context.Context, status, priority, source string) ([]RoadmapItem, error) {
	var whereParts []string
	var args []interface{}
	if status != "" {
		args = append(args, status)
		whereParts = append(whereParts, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if priority != "" {
		args = append(args, priority)
		whereParts = append(whereParts, fmt.Sprintf("r.priority = $%d", len(args)))
	}
	if source != "" {
		args = append(args, source)
		whereParts = append(whereParts, fmt.Sprintf("r.source = $%d", len(args)))
	}
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	// Prioritize p0 first, then by status, then by created_at desc.
	query := "SELECT " + roadmapSelectColumns + roadmapFromJoins + where +
		" ORDER BY r.priority ASC, r.created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []RoadmapItem
	for rows.Next() {
		item, err := scanRoadmapRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *roadmapRepo) GetByID(ctx context.Context, id uuid.UUID) (*RoadmapItem, error) {
	query := "SELECT " + roadmapSelectColumns + roadmapFromJoins + " WHERE r.id = $1"
	row := r.db.QueryRowContext(ctx, query, id)
	item, err := scanRoadmapRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *roadmapRepo) GetByTicketID(ctx context.Context, ticketID uuid.UUID) (*RoadmapItem, error) {
	query := "SELECT " + roadmapSelectColumns + roadmapFromJoins + " WHERE r.source_ticket_id = $1"
	row := r.db.QueryRowContext(ctx, query, ticketID)
	item, err := scanRoadmapRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (r *roadmapRepo) Create(ctx context.Context, item *RoadmapItem) error {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	now := time.Now()
	item.CreatedAt = now
	item.UpdatedAt = now

	query := `
        INSERT INTO roadmap_items
            (id, title, description, status, priority, source,
             source_ticket_id, requester_user_id,
             notify_on_dev, notify_on_prod,
             created_by, created_at, updated_at)
        VALUES
            ($1, $2, $3, $4, $5, $6,
             $7, $8,
             $9, $10,
             $11, $12, $12)
    `
	_, err := r.db.ExecContext(ctx, query,
		item.ID, item.Title, item.Description, item.Status, item.Priority, item.Source,
		nullableUUID(item.SourceTicketID), nullableUUID(item.RequesterUserID),
		item.NotifyOnDev, item.NotifyOnProd,
		nullableUUID(item.CreatedBy), now,
	)
	return err
}

func (r *roadmapRepo) Update(ctx context.Context, item *RoadmapItem) error {
	query := `
        UPDATE roadmap_items SET
            title          = $2,
            description    = $3,
            status         = $4,
            priority       = $5,
            notify_on_dev  = $6,
            notify_on_prod = $7,
            updated_at     = NOW()
        WHERE id = $1
    `
	_, err := r.db.ExecContext(ctx, query,
		item.ID, item.Title, item.Description, item.Status, item.Priority,
		item.NotifyOnDev, item.NotifyOnProd,
	)
	return err
}

func (r *roadmapRepo) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM roadmap_items WHERE id = $1", id)
	return err
}

func (r *roadmapRepo) MarkDevReleased(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE roadmap_items
        SET dev_released_at = COALESCE(dev_released_at, NOW()),
            status          = CASE WHEN status IN ('in_prod', 'cancelled') THEN status ELSE 'in_dev' END,
            updated_at      = NOW()
        WHERE id = $1
    `, id)
	return err
}

func (r *roadmapRepo) MarkProdReleased(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
        UPDATE roadmap_items
        SET prod_released_at = COALESCE(prod_released_at, NOW()),
            status           = CASE WHEN status = 'cancelled' THEN status ELSE 'in_prod' END,
            updated_at       = NOW()
        WHERE id = $1
    `, id)
	return err
}

// rowScanner abstracts over *sql.Row and *sql.Rows so scanRoadmapRow can serve both.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRoadmapRow(s rowScanner) (*RoadmapItem, error) {
	var it RoadmapItem
	if err := s.Scan(
		&it.ID, &it.Title, &it.Description, &it.Status, &it.Priority, &it.Source,
		&it.SourceTicketID, &it.RequesterUserID,
		&it.NotifyOnDev, &it.NotifyOnProd,
		&it.DevReleasedAt, &it.ProdReleasedAt,
		&it.CreatedBy, &it.CreatedAt, &it.UpdatedAt,
		&it.RequesterEmail, &it.RequesterName, &it.TicketSubject, &it.FollowerCount,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

// AddFollower upserts (roadmap_id, user_id) into roadmap_item_followers.
// Calling it again for the same pair is a no-op.
func (r *roadmapRepo) AddFollower(ctx context.Context, roadmapID, userID uuid.UUID, sourceTicketID *uuid.UUID, notifyDev, notifyProd bool) error {
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO roadmap_item_followers
            (roadmap_id, user_id, source_ticket_id, notify_on_dev, notify_on_prod, created_at)
        VALUES ($1, $2, $3, $4, $5, NOW())
        ON CONFLICT (roadmap_id, user_id) DO NOTHING
    `, roadmapID, userID, sourceTicketID, notifyDev, notifyProd)
	return err
}

// ListFollowers returns all followers of a roadmap item with their email,
// display name, and originating ticket subject for UI rendering.
func (r *roadmapRepo) ListFollowers(ctx context.Context, roadmapID uuid.UUID) ([]RoadmapFollower, error) {
	rows, err := r.db.QueryContext(ctx, `
        SELECT f.roadmap_id, f.user_id, f.source_ticket_id,
               f.notify_on_dev, f.notify_on_prod, f.created_at,
               COALESCE(u.email, '')                                  AS user_email,
               COALESCE(u.first_name || ' ' || u.last_name, '')       AS user_name,
               COALESCE(t.subject, '')                                AS ticket_subject
        FROM roadmap_item_followers f
        LEFT JOIN users           u ON f.user_id          = u.id
        LEFT JOIN support_tickets t ON f.source_ticket_id = t.id
        WHERE f.roadmap_id = $1
        ORDER BY f.created_at ASC
    `, roadmapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RoadmapFollower
	for rows.Next() {
		var f RoadmapFollower
		if err := rows.Scan(
			&f.RoadmapID, &f.UserID, &f.SourceTicketID,
			&f.NotifyOnDev, &f.NotifyOnProd, &f.CreatedAt,
			&f.UserEmail, &f.UserName, &f.TicketSubject,
		); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FollowerCount returns the number of users subscribed to a roadmap item.
// Cheap convenience wrapper used in spots where the full list isn't needed.
func (r *roadmapRepo) FollowerCount(ctx context.Context, roadmapID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM roadmap_item_followers WHERE roadmap_id = $1", roadmapID).Scan(&n)
	return n, err
}

func nullableUUID(n models.NullUUID) interface{} {
	if !n.Valid {
		return nil
	}
	return n.UUID
}
