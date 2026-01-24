package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// DevModeSettings stores development mode configuration
type DevModeSettings struct {
	ID         string         `json:"id"`
	IsEnabled  bool           `json:"isEnabled"`
	AllowedIP  sql.NullString `json:"-"`
	SGRuleID   sql.NullString `json:"-"`
	EnabledBy  *uuid.UUID     `json:"enabledBy,omitempty"`
	EnabledAt  sql.NullTime   `json:"enabledAt,omitempty"`
	DisabledAt sql.NullTime   `json:"disabledAt,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	UpdatedAt  time.Time      `json:"updatedAt"`
}

// DevModeStatus is the response for the dev mode status API
type DevModeStatus struct {
	IsEnabled   bool   `json:"isEnabled"`
	AllowedIP   string `json:"allowedIp,omitempty"`
	EnabledBy   string `json:"enabledBy,omitempty"`
	EnabledAt   string `json:"enabledAt,omitempty"`
	EnabledByID string `json:"enabledById,omitempty"`
}

// SSHSession represents an active SSH session
type SSHSession struct {
	Username  string `json:"username"`
	TTY       string `json:"tty"`
	SourceIP  string `json:"sourceIp"`
	LoginTime string `json:"loginTime"`
	PID       string `json:"pid"`
	ServerIP  string `json:"serverIp"`
}
