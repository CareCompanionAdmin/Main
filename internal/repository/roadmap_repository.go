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
    COALESCE(t.subject, '')                                AS ticket_subject
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
		&it.RequesterEmail, &it.RequesterName, &it.TicketSubject,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

func nullableUUID(n models.NullUUID) interface{} {
	if !n.Valid {
		return nil
	}
	return n.UUID
}
