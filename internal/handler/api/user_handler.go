package api

import (
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// UserHandler handles user profile and password endpoints
type UserHandler struct {
	userService *service.UserService
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{userService: userService}
}

// UpdateProfile handles PATCH /api/users/profile
func (h *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req models.UpdateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if err := h.userService.UpdateProfile(r.Context(), userID, &req); err != nil {
		switch err {
		case service.ErrEmailTaken:
			respondBadRequest(w, "That email address is already in use by another account")
		case service.ErrEmailInvalid:
			respondBadRequest(w, "Please enter a valid email address")
		case service.ErrUserNotFound:
			respondNotFound(w, "User not found")
		default:
			respondInternalError(w, "Failed to update profile")
		}
		return
	}

	respondOK(w, SuccessResponse{Success: true, Message: "Profile updated"})
}

// ChangePassword handles POST /api/users/password
func (h *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req service.ChangePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		respondBadRequest(w, "Current password and new password are required")
		return
	}

	if len(req.NewPassword) < 8 {
		respondBadRequest(w, "New password must be at least 8 characters")
		return
	}

	if err := h.userService.ChangePassword(r.Context(), userID, &req); err != nil {
		if err == service.ErrPasswordMismatch {
			respondBadRequest(w, "Current password is incorrect")
			return
		}
		respondInternalError(w, "Failed to change password")
		return
	}

	respondOK(w, SuccessResponse{Success: true, Message: "Password changed"})
}
