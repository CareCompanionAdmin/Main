package models

import (
	"time"

	"github.com/google/uuid"
)

// DeviceToken represents a registered mobile device for push notifications
type DeviceToken struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	Token      string    `json:"token"`
	Platform   string    `json:"platform"` // "ios" or "android"
	DeviceName string    `json:"device_name,omitempty"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// RegisterDeviceRequest is the request body for device registration
type RegisterDeviceRequest struct {
	Token      string `json:"token"`
	Platform   string `json:"platform"`
	DeviceName string `json:"device_name,omitempty"`
}

// UnregisterDeviceRequest is the request body for device unregistration
type UnregisterDeviceRequest struct {
	Token string `json:"token"`
}

// AppConfig is the response for the mobile app config endpoint
type AppConfig struct {
	Environment   string `json:"environment"`
	MinAppVersion string `json:"min_app_version"`
	Maintenance   bool   `json:"maintenance_mode"`
}
