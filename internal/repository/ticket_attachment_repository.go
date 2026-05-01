package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// TicketAttachment is one uploaded file attached to a support ticket.
// storage_path + storage_driver together tell the storage layer how to
// fetch / delete the file.
type TicketAttachment struct {
	ID            uuid.UUID       `json:"id"`
	TicketID      uuid.UUID       `json:"ticket_id"`
	UploaderID    models.NullUUID `json:"uploader_id,omitempty"`
	Kind          string          `json:"kind"`
	ContentType   string          `json:"content_type"`
	OriginalName  string          `json:"original_name"`
	StoragePath   string          `json:"-"` // internal — never serialize
	StorageDriver string          `json:"-"`
	SizeBytes     int64           `json:"size_bytes"`
	CreatedAt     time.Time       `json:"created_at"`
}

// TicketAttachmentRepository handles ticket_attachments rows. The actual file
// bytes are managed separately by the AttachmentStorage interface in service.
type TicketAttachmentRepository interface {
	Create(ctx context.Context, a *TicketAttachment) error
	GetByID(ctx context.Context, id uuid.UUID) (*TicketAttachment, error)
	ListByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error)
	CountByTicket(ctx context.Context, ticketID uuid.UUID) (int, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
	// DeleteAllByTicket returns the rows that were deleted so the caller can
	// also delete the underlying storage objects.
	DeleteAllByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error)
}

type ticketAttachmentRepo struct {
	db *sql.DB
}

// NewTicketAttachmentRepo constructs the repo.
func NewTicketAttachmentRepo(db *sql.DB) TicketAttachmentRepository {
	return &ticketAttachmentRepo{db: db}
}

const attSelectCols = `
    id, ticket_id, uploader_id, kind, content_type, original_name,
    storage_path, storage_driver, size_bytes, created_at
`

func scanAttachment(s rowScannerLike) (*TicketAttachment, error) {
	a := &TicketAttachment{}
	if err := s.Scan(
		&a.ID, &a.TicketID, &a.UploaderID, &a.Kind, &a.ContentType, &a.OriginalName,
		&a.StoragePath, &a.StorageDriver, &a.SizeBytes, &a.CreatedAt,
	); err != nil {
		return nil, err
	}
	return a, nil
}

// rowScannerLike mirrors the rowScanner interface from roadmap_repository.go;
// duplicated here to avoid a cross-file coupling.
type rowScannerLike interface {
	Scan(dest ...interface{}) error
}

func (r *ticketAttachmentRepo) Create(ctx context.Context, a *TicketAttachment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if a.Kind == "" {
		a.Kind = "other"
	}
	var uploader interface{}
	if a.UploaderID.Valid {
		uploader = a.UploaderID.UUID
	}
	_, err := r.db.ExecContext(ctx, `
        INSERT INTO ticket_attachments
            (id, ticket_id, uploader_id, kind, content_type, original_name,
             storage_path, storage_driver, size_bytes, created_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `, a.ID, a.TicketID, uploader, a.Kind, a.ContentType, a.OriginalName,
		a.StoragePath, a.StorageDriver, a.SizeBytes, a.CreatedAt)
	return err
}

func (r *ticketAttachmentRepo) GetByID(ctx context.Context, id uuid.UUID) (*TicketAttachment, error) {
	row := r.db.QueryRowContext(ctx, "SELECT "+attSelectCols+" FROM ticket_attachments WHERE id = $1", id)
	a, err := scanAttachment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (r *ticketAttachmentRepo) ListByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+attSelectCols+" FROM ticket_attachments WHERE ticket_id = $1 ORDER BY created_at ASC",
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TicketAttachment
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (r *ticketAttachmentRepo) CountByTicket(ctx context.Context, ticketID uuid.UUID) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ticket_attachments WHERE ticket_id = $1", ticketID).Scan(&n)
	return n, err
}

func (r *ticketAttachmentRepo) DeleteByID(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM ticket_attachments WHERE id = $1", id)
	return err
}

func (r *ticketAttachmentRepo) DeleteAllByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error) {
	// Read first so the caller has the list of storage paths to nuke.
	atts, err := r.ListByTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if _, err := r.db.ExecContext(ctx, "DELETE FROM ticket_attachments WHERE ticket_id = $1", ticketID); err != nil {
		return nil, err
	}
	return atts, nil
}
