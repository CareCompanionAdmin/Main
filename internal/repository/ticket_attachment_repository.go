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

// ticketAttachmentRepo implements TicketAttachmentRepository.
//
// db        — local app DB used solely for resolving the uploader's email
//              and name from the local users table at write time. Never
//              touched for ticket_attachments rows.
// supportDB — pool that owns ticket_attachments. May equal db (single-env
//              mode) or be a separate pool pointed at prod (shared mode set
//              by SUPPORT_DB_DSN). All attachment SQL routes here.
type ticketAttachmentRepo struct {
	db        *sql.DB
	supportDB *sql.DB
}

// NewTicketAttachmentRepo constructs the repo. supportDB falls back to db
// when nil so single-env deployments behave unchanged.
func NewTicketAttachmentRepo(db, supportDB *sql.DB) TicketAttachmentRepository {
	if supportDB == nil {
		supportDB = db
	}
	return &ticketAttachmentRepo{db: db, supportDB: supportDB}
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

// lookupUploaderDenorm resolves email/first/last for a user against the
// LOCAL DB (r.db) so the snapshot reflects the env where the upload
// happened. Returns empty strings when uploader is nil/missing.
func (r *ticketAttachmentRepo) lookupUploaderDenorm(ctx context.Context, uploader models.NullUUID) (email, firstName, lastName string) {
	if !uploader.Valid || uploader.UUID == uuid.Nil {
		return "", "", ""
	}
	_ = r.db.QueryRowContext(ctx,
		"SELECT COALESCE(email,''), COALESCE(first_name,''), COALESCE(last_name,'') FROM users WHERE id = $1",
		uploader.UUID,
	).Scan(&email, &firstName, &lastName)
	return
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
	email, firstName, lastName := r.lookupUploaderDenorm(ctx, a.UploaderID)
	_, err := r.supportDB.ExecContext(ctx, `
        INSERT INTO ticket_attachments
            (id, ticket_id, uploader_id, kind, content_type, original_name,
             storage_path, storage_driver, size_bytes, created_at,
             uploader_email, uploader_first_name, uploader_last_name)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
    `, a.ID, a.TicketID, uploader, a.Kind, a.ContentType, a.OriginalName,
		a.StoragePath, a.StorageDriver, a.SizeBytes, a.CreatedAt,
		email, firstName, lastName)
	return err
}

func (r *ticketAttachmentRepo) GetByID(ctx context.Context, id uuid.UUID) (*TicketAttachment, error) {
	row := r.supportDB.QueryRowContext(ctx, "SELECT "+attSelectCols+" FROM ticket_attachments WHERE id = $1", id)
	a, err := scanAttachment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func (r *ticketAttachmentRepo) ListByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error) {
	rows, err := r.supportDB.QueryContext(ctx,
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
	err := r.supportDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM ticket_attachments WHERE ticket_id = $1", ticketID).Scan(&n)
	return n, err
}

func (r *ticketAttachmentRepo) DeleteByID(ctx context.Context, id uuid.UUID) error {
	_, err := r.supportDB.ExecContext(ctx, "DELETE FROM ticket_attachments WHERE id = $1", id)
	return err
}

func (r *ticketAttachmentRepo) DeleteAllByTicket(ctx context.Context, ticketID uuid.UUID) ([]TicketAttachment, error) {
	// Read first so the caller has the list of storage paths to nuke.
	atts, err := r.ListByTicket(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if _, err := r.supportDB.ExecContext(ctx, "DELETE FROM ticket_attachments WHERE ticket_id = $1", ticketID); err != nil {
		return nil, err
	}
	return atts, nil
}
