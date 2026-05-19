package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/google/uuid"
	"google.golang.org/api/option"

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
	fcmClient       *messaging.Client
	enabled         bool
}

// NewPushService creates a new push notification service
func NewPushService(deviceTokenRepo repository.DeviceTokenRepository, fcmServerKey string) *PushService {
	return &PushService{
		deviceTokenRepo: deviceTokenRepo,
		enabled:         false,
	}
}

// InitFirebase initializes the Firebase Admin SDK with a service account key file
func (s *PushService) InitFirebase(serviceAccountKeyFile string) {
	if serviceAccountKeyFile == "" {
		log.Println("Push notifications disabled: FIREBASE_SERVICE_ACCOUNT_KEY not configured")
		return
	}

	ctx := context.Background()
	opt := option.WithCredentialsFile(serviceAccountKeyFile)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		log.Printf("Push notifications disabled: failed to initialize Firebase: %v", err)
		return
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		log.Printf("Push notifications disabled: failed to get FCM client: %v", err)
		return
	}

	s.fcmClient = client
	s.enabled = true
	log.Println("Push notifications enabled (Firebase Admin SDK)")
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

	// Track per-device outcomes so the caller can distinguish "all sends
	// failed" from "one device failed but the user got the message on
	// another." Previously the function returned lastErr — which depended
	// on iteration order and could mask a per-device failure if a later
	// device happened to succeed (or vice-versa).
	var sendErrs []string
	succeeded := 0
	for _, dt := range tokens {
		if err := s.sendToDevice(ctx, dt.Token, msg); err != nil {
			if isTokenInvalid(err) {
				log.Printf("Deactivating invalid device token for user %s: %v", userID, err)
				if deactErr := s.deviceTokenRepo.DeactivateByToken(ctx, dt.Token); deactErr != nil {
					log.Printf("Failed to deactivate token: %v", deactErr)
				}
				// Don't count invalid-token errors as send failures — the
				// token is dead and we've cleaned it up; nothing the caller
				// can do about it.
				continue
			}
			log.Printf("Failed to send push to device %s: %v", dt.ID, err)
			sendErrs = append(sendErrs, fmt.Sprintf("device %s: %v", dt.ID, err))
			continue
		}
		succeeded++
	}

	// If at least one device got the message, treat the send as successful
	// from the caller's perspective — the user was notified.
	if succeeded > 0 {
		if len(sendErrs) > 0 {
			log.Printf("Push partially delivered to user %s: %d succeeded, %d failed", userID, succeeded, len(sendErrs))
		}
		return nil
	}
	// All eligible devices failed — surface a single aggregate error.
	if len(sendErrs) > 0 {
		return fmt.Errorf("all push sends failed for user %s: %s", userID, strings.Join(sendErrs, "; "))
	}
	return nil
}

// SendToUsers sends a push notification to multiple users
func (s *PushService) SendToUsers(ctx context.Context, userIDs []uuid.UUID, msg PushMessage) {
	for _, userID := range userIDs {
		if err := s.Send(ctx, userID, msg); err != nil {
			log.Printf("Failed to send push to user %s: %v", userID, err)
		}
	}
}

func (s *PushService) sendToDevice(ctx context.Context, token string, msg PushMessage) error {
	priority := "normal"
	if msg.Priority == PushPriorityHigh {
		priority = "high"
	}

	fcmMsg := &messaging.Message{
		Token: token,
		Notification: &messaging.Notification{
			Title: msg.Title,
			Body:  msg.Body,
		},
		Data: msg.Data,
		Android: &messaging.AndroidConfig{
			Priority: priority,
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority": func() string {
					if msg.Priority == PushPriorityHigh {
						return "10"
					}
					return "5"
				}(),
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
			},
		},
	}

	_, err := s.fcmClient.Send(ctx, fcmMsg)
	if err != nil {
		return fmt.Errorf("FCM send failed: %w", err)
	}

	return nil
}

// isTokenInvalid checks if the error indicates the device token is no longer valid
func isTokenInvalid(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "registration-token-not-registered") ||
		strings.Contains(msg, "invalid-registration-token") ||
		strings.Contains(msg, "NotRegistered") ||
		strings.Contains(msg, "InvalidRegistration")
}
