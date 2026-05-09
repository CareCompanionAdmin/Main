package models

import (
	"time"

	"github.com/google/uuid"
)

type SessionKind string

const (
	SessionKindUser  SessionKind = "user"
	SessionKindAdmin SessionKind = "admin"
)

type Session struct {
	ID         uuid.UUID   `json:"id"`
	UserID     uuid.UUID   `json:"user_id"`
	Kind       SessionKind `json:"kind"`
	SystemRole NullString  `json:"system_role,omitempty"`
	FamilyID   NullUUID    `json:"family_id,omitempty"`
	IPAtStart  NullString  `json:"ip_at_start,omitempty"`
	UserAgent  NullString  `json:"user_agent,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	LastSeenAt time.Time   `json:"last_seen_at"`
	RevokedAt  *time.Time  `json:"revoked_at,omitempty"`
	ExpiresAt  time.Time   `json:"expires_at"`
}

// IsActive returns true when the session is neither revoked nor expired.
func (s *Session) IsActive(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return now.Before(s.ExpiresAt)
}
