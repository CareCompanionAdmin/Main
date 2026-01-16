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
	CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority string) (*SupportTicket, error)

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
}

// userSupportRepo implements UserSupportRepository
type userSupportRepo struct {
	db *sql.DB
}

// NewUserSupportRepo creates a new user support repository
func NewUserSupportRepo(db *sql.DB) UserSupportRepository {
	return &userSupportRepo{db: db}
}

// CreateTicket creates a new support ticket for a user
func (r *userSupportRepo) CreateTicket(ctx context.Context, userID uuid.UUID, subject, description, priority string) (*SupportTicket, error) {
	id := uuid.New()
	now := time.Now()
	if priority == "" {
		priority = "normal"
	}

	query := `
		INSERT INTO support_tickets (id, user_id, subject, description, priority, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'open', $6, $6)
		RETURNING id
	`
	err := r.db.QueryRowContext(ctx, query, id, userID, subject, description, priority, now).Scan(&id)
	if err != nil {
		return nil, err
	}

	return r.GetTicketByID(ctx, id, userID)
}

// GetTickets returns all tickets for a specific user
func (r *userSupportRepo) GetTickets(ctx context.Context, userID uuid.UUID) ([]SupportTicket, error) {
	query := `
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.user_id = $1
		ORDER BY t.updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []SupportTicket
	for rows.Next() {
		var t SupportTicket
		if err := rows.Scan(&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority,
			&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
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
		SELECT t.id, t.user_id, t.subject, t.description, t.status, t.priority,
		       t.assigned_to, t.created_at, t.updated_at, t.resolved_at, t.resolved_by,
		       COALESCE(u.email, '') as user_email,
		       COALESCE(a.first_name || ' ' || a.last_name, '') as assignee_name
		FROM support_tickets t
		LEFT JOIN users u ON t.user_id = u.id
		LEFT JOIN users a ON t.assigned_to = a.id
		WHERE t.id = $1 AND t.user_id = $2
	`
	t := &SupportTicket{}
	err := r.db.QueryRowContext(ctx, query, ticketID, userID).Scan(
		&t.ID, &t.UserID, &t.Subject, &t.Description, &t.Status, &t.Priority,
		&t.AssignedTo, &t.CreatedAt, &t.UpdatedAt, &t.ResolvedAt, &t.ResolvedBy,
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
	err := r.db.QueryRowContext(ctx, "SELECT user_id FROM support_tickets WHERE id = $1", ticketID).Scan(&ownerID)
	if err != nil {
		return nil, err
	}
	if !ownerID.Valid || ownerID.UUID != userID {
		return nil, nil // Not the owner
	}

	// Get non-internal messages only
	query := `
		SELECT m.id, m.ticket_id, m.sender_id, m.message, m.is_internal, m.created_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '') as sender_name,
		       COALESCE(u.email, '') as sender_email
		FROM ticket_messages m
		LEFT JOIN users u ON m.sender_id = u.id
		WHERE m.ticket_id = $1 AND m.is_internal = false
		ORDER BY m.created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, ticketID)
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
	err := r.db.QueryRowContext(ctx, "SELECT user_id FROM support_tickets WHERE id = $1", ticketID).Scan(&ownerID)
	if err != nil {
		return err
	}
	if !ownerID.Valid || ownerID.UUID != userID {
		return sql.ErrNoRows // Not the owner
	}

	id := uuid.New()
	query := `INSERT INTO ticket_messages (id, ticket_id, sender_id, message, is_internal, created_at) VALUES ($1, $2, $3, $4, false, NOW())`
	_, err = r.db.ExecContext(ctx, query, id, ticketID, userID, message)
	if err != nil {
		return err
	}

	// Update ticket updated_at timestamp
	_, err = r.db.ExecContext(ctx, "UPDATE support_tickets SET updated_at = NOW() WHERE id = $1", ticketID)
	return err
}

// MarkTicketRead updates the last_user_read_at timestamp for a ticket
func (r *userSupportRepo) MarkTicketRead(ctx context.Context, ticketID, userID uuid.UUID) error {
	query := `UPDATE support_tickets SET last_user_read_at = NOW() WHERE id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, ticketID, userID)
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
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&hasUnread)
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
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&count)
	return count, err
}
