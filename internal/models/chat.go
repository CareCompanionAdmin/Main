package models

import (
	"time"

	"github.com/google/uuid"
)

// ChatThreadType represents different types of chat threads
type ChatThreadType string

const (
	ChatThreadGeneral  ChatThreadType = "general"
	ChatThreadFamily   ChatThreadType = "family"
	ChatThreadProvider ChatThreadType = "provider"
	ChatThreadAlert    ChatThreadType = "alert"
)

// ChatThread represents a conversation thread
type ChatThread struct {
	ID             uuid.UUID       `json:"id"`
	FamilyID       uuid.UUID       `json:"family_id"`
	ChildID        *uuid.UUID      `json:"child_id,omitempty"`
	Title          NullString      `json:"title"`
	ThreadType     ChatThreadType  `json:"thread_type"`
	RelatedAlertID *uuid.UUID      `json:"related_alert_id,omitempty"`
	IsActive       bool            `json:"is_active"`
	CreatedBy      uuid.UUID       `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`

	// Populated on fetch
	Participants []ChatParticipant `json:"participants,omitempty"`
	Messages     []ChatMessage     `json:"messages,omitempty"`
	UnreadCount  int               `json:"unread_count,omitempty"`
	LastMessage  *ChatMessage      `json:"last_message,omitempty"`
	Creator      *User             `json:"creator,omitempty"`
	ChildName    string            `json:"child_name,omitempty"`
}

// ChatParticipant represents a user in a chat thread
type ChatParticipant struct {
	ID         uuid.UUID  `json:"id"`
	ThreadID   uuid.UUID  `json:"thread_id"`
	UserID     uuid.UUID  `json:"user_id"`
	Role       FamilyRole `json:"role"`
	JoinedAt   time.Time  `json:"joined_at"`
	LastReadAt NullTime   `json:"last_read_at"`
	IsActive   bool       `json:"is_active"`

	// Populated on fetch
	User *User `json:"user,omitempty"`
}

// ChatMessage represents a single message in a thread
type ChatMessage struct {
	ID          uuid.UUID   `json:"id"`
	ThreadID    uuid.UUID   `json:"thread_id"`
	SenderID    uuid.UUID   `json:"sender_id"`
	MessageText string      `json:"message_text"`
	Attachments Attachments `json:"attachments"`
	IsEdited    bool        `json:"is_edited"`
	EditedAt    NullTime    `json:"edited_at"`
	CreatedAt   time.Time   `json:"created_at"`

	// Populated on fetch
	Sender *User `json:"sender,omitempty"`
}

// Request types
type CreateThreadRequest struct {
	ChildID        *uuid.UUID     `json:"child_id,omitempty"`
	Title          string         `json:"title"`
	ThreadType     ChatThreadType `json:"thread_type"`
	RelatedAlertID *uuid.UUID     `json:"related_alert_id,omitempty"`
	Participants   []uuid.UUID    `json:"participants"`
	InitialMessage string         `json:"initial_message,omitempty"`
}

type SendMessageRequest struct {
	MessageText string      `json:"message_text"`
	Attachments Attachments `json:"attachments,omitempty"`
}

type EditMessageRequest struct {
	MessageText string `json:"message_text"`
}

// ChatThreadSummary for listing threads
type ChatThreadSummary struct {
	ID               uuid.UUID      `json:"id"`
	Title            NullString     `json:"title"`
	ThreadType       ChatThreadType `json:"thread_type"`
	ChildID          *uuid.UUID     `json:"child_id,omitempty"`
	ChildName        string         `json:"child_name,omitempty"`
	ParticipantCount int            `json:"participant_count"`
	ParticipantNames []string       `json:"participant_names,omitempty"`
	UnreadCount      int            `json:"unread_count"`
	LastMessageAt    NullTime       `json:"last_message_at"`
	LastMessageText  string         `json:"last_message_text,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}
