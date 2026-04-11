package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// PushPriority represents the notification priority
type PushPriority string

const (
	PushPriorityHigh   PushPriority = "high"
	PushPriorityNormal PushPriority = "normal"
)

// PushMessage represents a push notification to send
type PushMessage struct {
	Title    string            `json:"title"`
	Body     string            `json:"body"`
	Data     map[string]string `json:"data,omitempty"`
	Priority PushPriority      `json:"priority"`
	Badge    int               `json:"badge,omitempty"`
}

// PushService handles sending push notifications via Firebase Cloud Messaging
type PushService struct {
	deviceTokenRepo repository.DeviceTokenRepository
	fcmServerKey    string
	enabled         bool
	httpClient      *http.Client
}

// NewPushService creates a new push notification service
func NewPushService(deviceTokenRepo repository.DeviceTokenRepository, fcmServerKey string) *PushService {
	enabled := fcmServerKey != ""
	if !enabled {
		log.Println("Push notifications disabled: FCM_SERVER_KEY not configured")
	} else {
		log.Println("Push notifications enabled")
	}

	return &PushService{
		deviceTokenRepo: deviceTokenRepo,
		fcmServerKey:    fcmServerKey,
		enabled:         enabled,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsEnabled returns whether push notifications are configured
func (s *PushService) IsEnabled() bool {
	return s.enabled
}

// RegisterDevice registers or reactivates a device token
func (s *PushService) RegisterDevice(ctx context.Context, token *models.DeviceToken) error {
	return s.deviceTokenRepo.Upsert(ctx, token)
}

// UnregisterDevice deactivates a device token for a user
func (s *PushService) UnregisterDevice(ctx context.Context, userID uuid.UUID, token string) error {
	return s.deviceTokenRepo.Deactivate(ctx, userID, token)
}

// Send sends a push notification to all active devices for a user
func (s *PushService) Send(ctx context.Context, userID uuid.UUID, msg PushMessage) error {
	if !s.enabled {
		log.Printf("Push notification skipped (not enabled): user=%s title=%q", userID, msg.Title)
		return nil
	}

	tokens, err := s.deviceTokenRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get device tokens: %w", err)
	}

	if len(tokens) == 0 {
		return nil
	}

	var lastErr error
	for _, dt := range tokens {
		if err := s.sendToDevice(ctx, dt.Token, msg); err != nil {
			// If the token is invalid, deactivate it
			if isTokenInvalid(err) {
				log.Printf("Deactivating invalid device token for user %s: %v", userID, err)
				if deactErr := s.deviceTokenRepo.DeactivateByToken(ctx, dt.Token); deactErr != nil {
					log.Printf("Failed to deactivate token: %v", deactErr)
				}
			} else {
				log.Printf("Failed to send push to device %s: %v", dt.ID, err)
				lastErr = err
			}
		}
	}

	return lastErr
}

// SendToUsers sends a push notification to multiple users
func (s *PushService) SendToUsers(ctx context.Context, userIDs []uuid.UUID, msg PushMessage) {
	for _, userID := range userIDs {
		if err := s.Send(ctx, userID, msg); err != nil {
			log.Printf("Failed to send push to user %s: %v", userID, err)
		}
	}
}

// fcmMessage is the FCM HTTP v1 message format (legacy endpoint)
type fcmMessage struct {
	To           string            `json:"to"`
	Priority     string            `json:"priority"`
	Notification *fcmNotification  `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
}

type fcmNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Sound string `json:"sound,omitempty"`
	Badge string `json:"badge,omitempty"`
}

type fcmResponse struct {
	Success int `json:"success"`
	Failure int `json:"failure"`
	Results []struct {
		MessageID string `json:"message_id,omitempty"`
		Error     string `json:"error,omitempty"`
	} `json:"results"`
}

func (s *PushService) sendToDevice(ctx context.Context, token string, msg PushMessage) error {
	priority := "normal"
	if msg.Priority == PushPriorityHigh {
		priority = "high"
	}

	fcmMsg := fcmMessage{
		To:       token,
		Priority: priority,
		Notification: &fcmNotification{
			Title: msg.Title,
			Body:  msg.Body,
			Sound: "default",
		},
		Data: msg.Data,
	}

	body, err := json.Marshal(fcmMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal FCM message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://fcm.googleapis.com/fcm/send", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create FCM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+s.fcmServerKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("FCM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FCM returned status %d", resp.StatusCode)
	}

	var fcmResp fcmResponse
	if err := json.NewDecoder(resp.Body).Decode(&fcmResp); err != nil {
		return fmt.Errorf("failed to decode FCM response: %w", err)
	}

	if fcmResp.Failure > 0 && len(fcmResp.Results) > 0 {
		errMsg := fcmResp.Results[0].Error
		return fmt.Errorf("FCM error: %s", errMsg)
	}

	return nil
}

// isTokenInvalid checks if the error indicates the device token is no longer valid
func isTokenInvalid(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NotRegistered") ||
		strings.Contains(msg, "InvalidRegistration") ||
		strings.Contains(msg, "MismatchSenderId")
}
