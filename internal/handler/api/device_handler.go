package api

import (
	"net/http"

	"carecompanion/internal/config"
	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// DeviceHandler handles device registration and app config endpoints
type DeviceHandler struct {
	pushService *service.PushService
	appConfig   *config.AppConfig
}

// NewDeviceHandler creates a new device handler
func NewDeviceHandler(pushService *service.PushService, appConfig *config.AppConfig) *DeviceHandler {
	return &DeviceHandler{
		pushService: pushService,
		appConfig:   appConfig,
	}
}

// RegisterDevice registers a device token for push notifications
func (h *DeviceHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.RegisterDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Token == "" {
		respondBadRequest(w, "Token is required")
		return
	}

	if req.Platform != "ios" && req.Platform != "android" {
		respondBadRequest(w, "Platform must be 'ios' or 'android'")
		return
	}

	token := &models.DeviceToken{
		UserID:     claims.UserID,
		Token:      req.Token,
		Platform:   req.Platform,
		DeviceName: req.DeviceName,
	}

	if err := h.pushService.RegisterDevice(r.Context(), token); err != nil {
		respondInternalError(w, "Failed to register device")
		return
	}

	respondOK(w, SuccessResponse{Success: true, Message: "Device registered"})
}

// UnregisterDevice deactivates a device token
func (h *DeviceHandler) UnregisterDevice(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		respondError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req models.UnregisterDeviceRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Token == "" {
		respondBadRequest(w, "Token is required")
		return
	}

	if err := h.pushService.UnregisterDevice(r.Context(), claims.UserID, req.Token); err != nil {
		respondInternalError(w, "Failed to unregister device")
		return
	}

	respondOK(w, SuccessResponse{Success: true, Message: "Device unregistered"})
}

// GetAppConfig returns app configuration for mobile clients
func (h *DeviceHandler) GetAppConfig(w http.ResponseWriter, r *http.Request) {
	env := "production"
	if h.appConfig.Env == "development" {
		env = "development"
	}

	cfg := models.AppConfig{
		Environment:   env,
		MinAppVersion: "1.0.0",
		Maintenance:   false, // TODO: read from admin settings
	}

	respondOK(w, cfg)
}
