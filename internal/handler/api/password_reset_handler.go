package api

import (
	"net/http"

	"carecompanion/internal/service"
)

type PasswordResetHandler struct {
	passwordResetService *service.PasswordResetService
}

func NewPasswordResetHandler(passwordResetService *service.PasswordResetService) *PasswordResetHandler {
	return &PasswordResetHandler{
		passwordResetService: passwordResetService,
	}
}

type RequestResetRequest struct {
	Email string `json:"email"`
}

// RequestReset initiates a password reset flow
func (h *PasswordResetHandler) RequestReset(w http.ResponseWriter, r *http.Request) {
	var req RequestResetRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Email == "" {
		respondBadRequest(w, "Email is required")
		return
	}

	// Always return success to avoid email enumeration
	if err := h.passwordResetService.RequestReset(r.Context(), req.Email); err != nil {
		// Log but don't expose the error
		respondOK(w, map[string]interface{}{
			"success": true,
			"message": "If an account with that email exists, a password reset link has been sent.",
		})
		return
	}

	respondOK(w, map[string]interface{}{
		"success": true,
		"message": "If an account with that email exists, a password reset link has been sent.",
	})
}

type ValidateTokenRequest struct {
	Token string `json:"token"`
}

// ValidateToken checks if a reset token is still valid
func (h *PasswordResetHandler) ValidateToken(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respondBadRequest(w, "Token is required")
		return
	}

	valid, err := h.passwordResetService.ValidateToken(r.Context(), token)
	if err != nil {
		respondInternalError(w, "Failed to validate token")
		return
	}

	respondOK(w, map[string]interface{}{
		"valid": valid,
	})
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// ResetPassword completes the password reset
func (h *PasswordResetHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req ResetPasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}

	if req.Token == "" {
		respondBadRequest(w, "Token is required")
		return
	}
	if req.NewPassword == "" {
		respondBadRequest(w, "New password is required")
		return
	}
	if len(req.NewPassword) < 8 {
		respondBadRequest(w, "Password must be at least 8 characters")
		return
	}

	err := h.passwordResetService.ResetPassword(r.Context(), req.Token, req.NewPassword)
	if err != nil {
		switch err {
		case service.ErrResetTokenInvalid:
			respondBadRequest(w, "Invalid reset token")
		case service.ErrResetTokenExpired:
			respondBadRequest(w, "Reset token has expired. Please request a new one.")
		case service.ErrResetTokenUsed:
			respondBadRequest(w, "This reset token has already been used.")
		default:
			respondInternalError(w, "Failed to reset password")
		}
		return
	}

	respondOK(w, map[string]interface{}{
		"success": true,
		"message": "Password has been reset successfully. You can now log in with your new password.",
	})
}
