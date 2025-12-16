package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

type chatRepo struct {
	db *sql.DB
}

func NewChatRepo(db *sql.DB) ChatRepository {
	return &chatRepo{db: db}
}

// CreateThread creates a new chat thread
func (r *chatRepo) CreateThread(ctx context.Context, thread *models.ChatThread) error {
	query := `
		INSERT INTO chat_threads (id, family_id, child_id, title, thread_type, related_alert_id, is_active, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	thread.ID = uuid.New()
	thread.IsActive = true
	thread.CreatedAt = time.Now()
	thread.UpdatedAt = time.Now()

	_, err := r.db.ExecContext(ctx, query,
		thread.ID, thread.FamilyID, thread.ChildID, thread.Title, thread.ThreadType,
		thread.RelatedAlertID, thread.IsActive, thread.CreatedBy, thread.CreatedAt, thread.UpdatedAt,
	)
	return err
}

// GetThread retrieves a thread by ID with participants
func (r *chatRepo) GetThread(ctx context.Context, id uuid.UUID) (*models.ChatThread, error) {
	query := `
		SELECT id, family_id, child_id, title, thread_type, related_alert_id, is_active, created_by, created_at, updated_at
		FROM chat_threads
		WHERE id = $1
	`
	thread := &models.ChatThread{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&thread.ID, &thread.FamilyID, &thread.ChildID, &thread.Title, &thread.ThreadType,
		&thread.RelatedAlertID, &thread.IsActive, &thread.CreatedBy, &thread.CreatedAt, &thread.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Load participants
	thread.Participants, _ = r.GetParticipants(ctx, id)

	return thread, nil
}

// GetThreadsByFamily retrieves all threads for a family
func (r *chatRepo) GetThreadsByFamily(ctx context.Context, familyID uuid.UUID) ([]models.ChatThread, error) {
	query := `
		SELECT t.id, t.family_id, t.child_id, t.title, t.thread_type, t.related_alert_id, t.is_active, t.created_by, t.created_at, t.updated_at
		FROM chat_threads t
		WHERE t.family_id = $1 AND t.is_active = true
		ORDER BY t.updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []models.ChatThread
	for rows.Next() {
		var thread models.ChatThread
		err := rows.Scan(
			&thread.ID, &thread.FamilyID, &thread.ChildID, &thread.Title, &thread.ThreadType,
			&thread.RelatedAlertID, &thread.IsActive, &thread.CreatedBy, &thread.CreatedAt, &thread.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

// GetThreadsByChild retrieves all threads for a specific child
func (r *chatRepo) GetThreadsByChild(ctx context.Context, childID uuid.UUID) ([]models.ChatThread, error) {
	query := `
		SELECT id, family_id, child_id, title, thread_type, related_alert_id, is_active, created_by, created_at, updated_at
		FROM chat_threads
		WHERE child_id = $1 AND is_active = true
		ORDER BY updated_at DESC
	`
	rows, err := r.db.QueryContext(ctx, query, childID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []models.ChatThread
	for rows.Next() {
		var thread models.ChatThread
		err := rows.Scan(
			&thread.ID, &thread.FamilyID, &thread.ChildID, &thread.Title, &thread.ThreadType,
			&thread.RelatedAlertID, &thread.IsActive, &thread.CreatedBy, &thread.CreatedAt, &thread.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

// UpdateThread updates a thread
func (r *chatRepo) UpdateThread(ctx context.Context, thread *models.ChatThread) error {
	query := `
		UPDATE chat_threads
		SET title = $2, thread_type = $3, is_active = $4, updated_at = $5
		WHERE id = $1
	`
	thread.UpdatedAt = time.Now()
	_, err := r.db.ExecContext(ctx, query,
		thread.ID, thread.Title, thread.ThreadType, thread.IsActive, thread.UpdatedAt,
	)
	return err
}

// DeleteThread soft-deletes a thread
func (r *chatRepo) DeleteThread(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE chat_threads SET is_active = false, updated_at = $2 WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id, time.Now())
	return err
}

// AddParticipant adds a participant to a thread
func (r *chatRepo) AddParticipant(ctx context.Context, threadID, userID uuid.UUID, role models.FamilyRole) error {
	query := `
		INSERT INTO chat_participants (id, thread_id, user_id, role, joined_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (thread_id, user_id) DO UPDATE SET is_active = true
	`
	_, err := r.db.ExecContext(ctx, query, uuid.New(), threadID, userID, role, time.Now(), true)
	return err
}

// RemoveParticipant removes a participant from a thread
func (r *chatRepo) RemoveParticipant(ctx context.Context, threadID, userID uuid.UUID) error {
	query := `UPDATE chat_participants SET is_active = false WHERE thread_id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, threadID, userID)
	return err
}

// GetParticipants retrieves all participants in a thread
func (r *chatRepo) GetParticipants(ctx context.Context, threadID uuid.UUID) ([]models.ChatParticipant, error) {
	query := `
		SELECT p.id, p.thread_id, p.user_id, p.role, p.joined_at, p.last_read_at, p.is_active,
		       u.id, u.email, u.first_name, u.last_name
		FROM chat_participants p
		JOIN users u ON u.id = p.user_id
		WHERE p.thread_id = $1 AND p.is_active = true
	`
	rows, err := r.db.QueryContext(ctx, query, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []models.ChatParticipant
	for rows.Next() {
		var p models.ChatParticipant
		var user models.User
		err := rows.Scan(
			&p.ID, &p.ThreadID, &p.UserID, &p.Role, &p.JoinedAt, &p.LastReadAt, &p.IsActive,
			&user.ID, &user.Email, &user.FirstName, &user.LastName,
		)
		if err != nil {
			return nil, err
		}
		p.User = &user
		participants = append(participants, p)
	}
	return participants, rows.Err()
}

// IsParticipant checks if a user is a participant in a thread
func (r *chatRepo) IsParticipant(ctx context.Context, threadID, userID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM chat_participants WHERE thread_id = $1 AND user_id = $2 AND is_active = true)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, threadID, userID).Scan(&exists)
	return exists, err
}

// UpdateLastRead updates the last read time for a participant
func (r *chatRepo) UpdateLastRead(ctx context.Context, threadID, userID uuid.UUID) error {
	query := `UPDATE chat_participants SET last_read_at = $3 WHERE thread_id = $1 AND user_id = $2`
	_, err := r.db.ExecContext(ctx, query, threadID, userID, time.Now())
	return err
}

// CreateMessage creates a new message
func (r *chatRepo) CreateMessage(ctx context.Context, message *models.ChatMessage) error {
	query := `
		INSERT INTO chat_messages (id, thread_id, sender_id, message_text, attachments, is_edited, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	message.ID = uuid.New()
	message.CreatedAt = time.Now()
	message.IsEdited = false

	_, err := r.db.ExecContext(ctx, query,
		message.ID, message.ThreadID, message.SenderID, message.MessageText,
		message.Attachments, message.IsEdited, message.CreatedAt,
	)
	if err != nil {
		return err
	}

	// Update thread's updated_at
	r.db.ExecContext(ctx, `UPDATE chat_threads SET updated_at = $2 WHERE id = $1`, message.ThreadID, message.CreatedAt)

	return nil
}

// GetMessages retrieves messages for a thread with pagination
func (r *chatRepo) GetMessages(ctx context.Context, threadID uuid.UUID, limit, offset int) ([]models.ChatMessage, error) {
	query := `
		SELECT m.id, m.thread_id, m.sender_id, m.message_text, m.attachments, m.is_edited, m.edited_at, m.created_at,
		       u.id, u.email, u.first_name, u.last_name
		FROM chat_messages m
		JOIN users u ON u.id = m.sender_id
		WHERE m.thread_id = $1
		ORDER BY m.created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, threadID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.ChatMessage
	for rows.Next() {
		var m models.ChatMessage
		var user models.User
		err := rows.Scan(
			&m.ID, &m.ThreadID, &m.SenderID, &m.MessageText, &m.Attachments, &m.IsEdited, &m.EditedAt, &m.CreatedAt,
			&user.ID, &user.Email, &user.FirstName, &user.LastName,
		)
		if err != nil {
			return nil, err
		}
		m.Sender = &user
		messages = append(messages, m)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}

// GetMessage retrieves a single message
func (r *chatRepo) GetMessage(ctx context.Context, id uuid.UUID) (*models.ChatMessage, error) {
	query := `
		SELECT id, thread_id, sender_id, message_text, attachments, is_edited, edited_at, created_at
		FROM chat_messages
		WHERE id = $1
	`
	message := &models.ChatMessage{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&message.ID, &message.ThreadID, &message.SenderID, &message.MessageText,
		&message.Attachments, &message.IsEdited, &message.EditedAt, &message.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return message, err
}

// UpdateMessage updates a message (edit)
func (r *chatRepo) UpdateMessage(ctx context.Context, message *models.ChatMessage) error {
	query := `
		UPDATE chat_messages
		SET message_text = $2, is_edited = true, edited_at = $3
		WHERE id = $1
	`
	_, err := r.db.ExecContext(ctx, query, message.ID, message.MessageText, time.Now())
	return err
}

// DeleteMessage deletes a message
func (r *chatRepo) DeleteMessage(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM chat_messages WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// GetUnreadCount returns the number of unread messages in a thread for a user
func (r *chatRepo) GetUnreadCount(ctx context.Context, threadID, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM chat_messages m
		JOIN chat_participants p ON p.thread_id = m.thread_id AND p.user_id = $2
		WHERE m.thread_id = $1
		  AND m.sender_id != $2
		  AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
	`
	var count int
	err := r.db.QueryRowContext(ctx, query, threadID, userID).Scan(&count)
	return count, err
}

// GetTotalUnreadCount returns the total unread count across all threads for a user in a family
func (r *chatRepo) GetTotalUnreadCount(ctx context.Context, familyID, userID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM chat_messages m
		JOIN chat_threads t ON t.id = m.thread_id
		JOIN chat_participants p ON p.thread_id = m.thread_id AND p.user_id = $2
		WHERE t.family_id = $1
		  AND t.is_active = true
		  AND m.sender_id != $2
		  AND (p.last_read_at IS NULL OR m.created_at > p.last_read_at)
	`
	var count int
	err := r.db.QueryRowContext(ctx, query, familyID, userID).Scan(&count)
	return count, err
}
