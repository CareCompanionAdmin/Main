package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// UserSupportRepository handles user-facing support ticket operations
type UserSupportRepository interface {
	// CreateTicket creates a new ticket for the current user
	CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority, ticketType string) (*SupportTicket, error)

	// GetTickets gets all tickets for a user
	GetTickets(ctx context.Context, userID uuid.UUID) ([]SupportTicket, error)

	// GetTicketByID gets a specific ticket (validates ownership)
	GetTicketByID(ctx context.Context, ticketID, userID uuid.UUID) (*SupportTicket, error)

	// GetTicketMessages gets non-internal messages for a ticket
	GetTicketMessages(ctx context.Context, ticketID, userID uuid.UUID) ([]TicketMessage, error)

	// AddMessage adds a message from the user
	AddMessage(ctx context.Context, ticketID, userID uuid.UUID, message string) error

	// MarkTicketRead updates last_user_read_at timestamp
	MarkTicketRead(ctx context.Context, ticketID, userID uuid.UUID) error

	// HasUnreadSupportMessages checks if user has any tickets with unread support replies
	HasUnreadSupportMessages(ctx context.Context, userID uuid.UUID) (bool, error)

	// GetUnreadTicketCount returns count of tickets with unread messages
	GetUnreadTicketCount(ctx context.Context, userID uuid.UUID) (int, error)

	// ReopenTicket flips a resolved/closed ticket back to 'open', clears
	// resolved_at, and stamps reopened_at. Validates user ownership.
	// Returns ErrTicketNotFound if the ticket doesn't exist, is not owned
	// by the user, or is already in an open/in_progress/waiting state.
	ReopenTicket(ctx context.Context, ticketID, userID uuid.UUID) error
}

// userSupportRepo implements UserSupportRepository.
//
// db        — local app DB (always the env's own users table). Used for
//              identity lookups (lookupUserDenorm) so the actor's email/name
//              are captured into denorm columns at write time.
// supportDB — pool that owns the support_tickets / ticket_messages tables.
//              May be the same as db (single-env mode) or a separate pool
//              pointing at prod's RDS (shared-support-DB mode set by
//              SUPPORT_DB_DSN). All ticket SQL routes through supportDB.
type userSupportRepo struct {
	db        *sql.DB
	supportDB *sql.DB
}

// NewUserSupportRepo creates a new user support repository. When supportDB is
// nil it falls back to db, preserving single-env behavior.
func NewUserSupportRepo(db, supportDB *sql.DB) UserSupportRepository {
	if supportDB == nil {
		supportDB = db
	}
	return &userSupportRepo{db: db, supportDB: supportDB}
}

// lookupUserDenorm fetches the actor's email + name from the LOCAL users
// table for snapshotting into a support row. Always hits r.db so the denorm
// reflects the env where the action originated.
func (r *userSupportRepo) lookupUserDenorm(ctx context.Context, userID uuid.UUID) (email, firstName, lastName string) {
	if userID == uuid.Nil {
		return "", "", ""
	}
	_ = r.db.QueryRowContext(ctx,
		"SELECT COALESCE(email,''), COALESCE(first_name,''), COALESCE(last_name,'') FROM users WHERE id = $1",
		userID,
	).Scan(&email, &firstName, &lastName)
	return
}

// CreateTicket creates a new support ticket for a user
func (r *userSupportRepo) CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority, ticketType string) (*SupportTicket, error) {
	id := uuid.New()
	now := time.Now()
	if priority == "" {
		priority = "normal"
	}
	if ticketType == "" {
		ticketType = "general"
	}

	email, firstName, lastName := r.lookupUserDenorm(ctx, userID)
	query := `
		INSERT INTO support_tickets (id, user_id, subject, description, priority, type, status, created_at, updated_at, user_email, user_first_name, user_last_name)
		VALUES ($1, $2, $3, $4, $5, $6, 'open', $7, $7, $8, $9, $10)
		RETURNING id
	`
	err := r.supportDB.QueryRowContext(ctx, query, id, userID, subject, description, priority, ticketType, now, email, firstName, lastName).Scan(&id)
	if err != nil {
		return nil, err
	}

	return r.GetTicketByID(ctx, id, userID)
}

// GetTickets returns all tickets for a specific user
func (r *userSupportRepo) GetTickets(ctx context.Context, userID uuid.UUID) ([]SupportTicket, error) {
	query := `
		SELECT t.id, t.ticket_number, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
		       COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.user_id = $1
		ORDER BY t.updated_at DESC
	`
	rows, err := r.supportDB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []SupportTicket
	for rows.Next() {
		var t SupportTicket
		if err := rows.Scan(&t.ID, &t.Number, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.Type,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
			&t.DuplicateOfTicketID, &t.DuplicateOfRoadmapID,
			&t.UserEmail, &t.AssigneeName); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// GetTicketByID returns a specific ticket, validating user ownership
func (r *userSupportRepo) GetTicketByID(ctx context.Context, ticketID, userID uuid.UUID) (*SupportTicket, error) {
	query := `
		SELECT t.id, t.ticket_number, t.user_id, t.subject, t.description, t.status, t.priority, t.type,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       t.duplicate_of_ticket_id, t.duplicate_of_roadmap_id,
		       COALESCE(NULLIF(t.user_email, ''), u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.id = $1 AND t.user_id = $2
	`
	t := &SupportTicket{}
	err := r.supportDB.QueryRowContext(ctx, query, ticketID, userID).Scan(
		&t.ID, &t.Number, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority, &t.Type,
		&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
		&t.DuplicateOfTicketID, &t.DuplicateOfRoadmapID,
		&t.UserEmail, &t.AssigneeName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetTicketMessages returns non-internal messages for a ticket (validates ownership)
func (r *userSupportRepo) GetTicketMessages(ctx context.Context, ticketID, userID uuid.UUID) ([]TicketMessage, error) {
	// First verify ownership
	var ownerID models.NullUUID
	err := r.supportDB.QueryRowContext(ctx, "SELECT user_id FROM support_tickets WHERE id = $1", ticketID).Scan(&ownerID)
	if err != nil {
		return nil, err
	}
	if !ownerID.Valid || ownerID.UUID != userID {
		return nil, nil // Not the owner
	}

	// Get non-internal messages only. COALESCE prefers the row's denorm
	// columns (set at write-time), then the JOIN against the support DB's
	// users table for legacy rows, then empty.
	query := `
		SELECT m.id, m.ticket_id, m.sender_id, m.message, m.is_internal, m.created_at,
		       COALESCE(NULLIF(TRIM(BOTH ' ' FROM (m.sender_first_name || ' ' || m.sender_last_name)), ''),
		                NULLIF(TRIM(BOTH ' ' FROM (u.first_name || ' ' || u.last_name)), ''),
		                '') as sender_name,
		       COALESCE(NULLIF(m.sender_email, ''), u.email, '') as sender_email
		FROM ticket_messages m
		LEFT JOIN users u ON m.sender_id = u.id
		WHERE m.ticket_id = $1 AND m.is_internal = false
		ORDER BY m.created_at ASC
	`
	rows, err := r.supportDB.QueryContext(ctx, query, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []TicketMessage
	for rows.Next() {
		var m TicketMessage
		if err := rows.Scan(&m.ID, &m.TicketID, &m.SenderID, &m.Message, &m.IsInternal,
			&m.CreatedAt, &m.SenderName, &m.SenderEmail); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// AddMessage adds a message from the user to a ticket
func (r *userSupportRepo) AddMessage(ctx context.Context, ticketID, userID uuid.UUID, message string) error {
	// First verify ownership
	var ownerID models.NullUUID
	err := r.supportDB.QueryRowContext(ctx, "SELECT user_id FROM support_tickets WHERE id = $1", ticketID).Scan(&ownerID)
	if err != nil {
		return err
	}
	if !ownerID.Valid || ownerID.UUID != userID {
		return sql.ErrNoRows // Not the owner
	}

	id := uuid.New()
	email, firstName, lastName := r.lookupUserDenorm(ctx, userID)
	query := `INSERT INTO ticket_messages (id, ticket_id, sender_id, message, is_internal, created_at, sender_email, sender_first_name, sender_last_name) VALUES ($1, $2, $3, $4, false, NOW(), $5, $6, $7)`
	_, err = r.supportDB.ExecContext(ctx, query, id, ticketID, userID, message, email, firstName, lastName)
	if err != nil {
		return err
	}

	// Update ticket updated_at timestamp
	_, err = r.supportDB.ExecContext(ctx, "UPDATE support_tickets SET updated_at = NOW() WHERE id = $1", ticketID)
	return err
}

// MarkTicketRead updates the last_user_read_at timestamp for a ticket
func (r *userSupportRepo) MarkTicketRead(ctx context.Context, ticketID, userID uuid.UUID) error {
	query := `UPDATE support_tickets SET last_user_read_at = NOW() WHERE id = $1 AND user_id = $2`
	_, err := r.supportDB.ExecContext(ctx, query, ticketID, userID)
	return err
}

// HasUnreadSupportMessages checks if user has any tickets with unread support replies
// A message is "unread" if:
// - sender_id != user_id (message from support, not user)
// - is_internal = false (not an internal note)
// - message.created_at > ticket.last_user_read_at (or last_user_read_at is NULL)
func (r *userSupportRepo) HasUnreadSupportMessages(ctx context.Context, userID uuid.UUID) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM support_tickets t
			JOIN ticket_messages m ON m.ticket_id = t.id
			WHERE t.user_id = $1
			  AND m.sender_id != t.user_id
			  AND m.is_internal = false
			  AND (t.last_user_read_at IS NULL OR m.created_at > t.last_user_read_at)
			LIMIT 1
		)
	`
	var hasUnread bool
	err := r.supportDB.QueryRowContext(ctx, query, userID).Scan(&hasUnread)
	return hasUnread, err
}

// GetUnreadTicketCount returns count of tickets with unread support messages
func (r *userSupportRepo) GetUnreadTicketCount(ctx context.Context, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(DISTINCT t.id)
		FROM support_tickets t
		JOIN ticket_messages m ON m.ticket_id = t.id
		WHERE t.user_id = $1
		  AND m.sender_id != t.user_id
		  AND m.is_internal = false
		  AND (t.last_user_read_at IS NULL OR m.created_at > t.last_user_read_at)
	`
	var count int
	err := r.supportDB.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}

// ReopenTicket flips a resolved/closed ticket back to open for the
// owning user. The single UPDATE narrows on (id, user_id, status IN
// ('resolved','closed')) so an unauthorized or wrong-state attempt
// affects zero rows and returns ErrTicketNotFound to the caller.
func (r *userSupportRepo) ReopenTicket(ctx context.Context, ticketID, userID uuid.UUID) error {
	res, err := r.supportDB.ExecContext(ctx, `
		UPDATE support_tickets
		   SET status      = 'open',
		       resolved_at = NULL,
		       reopened_at = NOW(),
		       updated_at  = NOW()
		 WHERE id = $1
		   AND user_id = $2
		   AND status IN ('resolved', 'closed')
	`, ticketID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
