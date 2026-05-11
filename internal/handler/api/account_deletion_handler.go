package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"carecompanion/internal/middleware"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

// AccountDeletionHandler exposes the user-facing deletion flow: request,
// confirm, restore-by-token, plus a status endpoint for the Settings UI.
type AccountDeletionHandler struct {
	svc      *service.AccountDeletionService
	repo     repository.AccountDeletionRepository
}

func NewAccountDeletionHandler(svc *service.AccountDeletionService, repo repository.AccountDeletionRepository) *AccountDeletionHandler {
	return &AccountDeletionHandler{svc: svc, repo: repo}
}

// GetStatus returns the user's current deletion-request state plus their
// family-role breakdown so the frontend can render the right disclaimer
// copy before they trigger the flow.
func (h *AccountDeletionHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	roles, err := h.repo.GetFamilyRolesForUser(r.Context(), userID)
	if err != nil {
		respondInternalError(w, "Failed to load account status")
		return
	}
	req, err := h.repo.GetActiveByUser(r.Context(), userID)
	if err != nil {
		respondInternalError(w, "Failed to load account status")
		return
	}

	respondOK(w, map[string]interface{}{
		"family_roles":      roles,
		"active_request":    req,
	})
}

// RequestDeletion starts the flow — emails the OTP, returns the request id.
func (h *AccountDeletionHandler) RequestDeletion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	req, err := h.svc.RequestDeletion(r.Context(), userID, r.RemoteAddr, r.UserAgent())
	if errors.Is(err, service.ErrDeletionAlreadyPending) {
		// Surface the existing request so the UI can show "code sent"
		respondOK(w, map[string]interface{}{
			"request":         req,
			"already_pending": true,
		})
		return
	}
	if err != nil {
		respondInternalError(w, "Could not start account deletion: "+err.Error())
		return
	}
	respondOK(w, map[string]interface{}{
		"request":         req,
		"already_pending": false,
	})
}

// ConfirmDeletion accepts the OTP. On success the user is now soft-deleted
// and signed out; the response includes the scheduled hard-delete date so
// the client can confirm and redirect to the goodbye page.
func (h *AccountDeletionHandler) ConfirmDeletion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondBadRequest(w, "Invalid request body")
		return
	}
	if body.Code == "" {
		respondBadRequest(w, "Code is required")
		return
	}

	req, err := h.svc.ConfirmDeletion(r.Context(), userID, body.Code)
	switch {
	case errors.Is(err, service.ErrDeletionNotFound):
		respondError(w, "No deletion request in progress. Start over from your account settings.", http.StatusGone)
		return
	case errors.Is(err, service.ErrDeletionCodeInvalid):
		respondError(w, "That code is invalid or expired. Try again or request a new code.", http.StatusUnauthorized)
		return
	case errors.Is(err, service.ErrDeletionCodeMaxAttempts):
		respondError(w, "Too many failed attempts. Request a new code from your account settings.", http.StatusTooManyRequests)
		return
	case err != nil:
		respondInternalError(w, "Could not confirm deletion: "+err.Error())
		return
	}

	// Clear auth cookies so the WebView / browser stops being logged in.
	clearAuthCookies(w, r)

	respondOK(w, map[string]interface{}{
		"request":                    req,
		"scheduled_hard_delete_at":   req.ScheduledHardDeleteAt,
		"message":                    "Your account deletion is in progress. Check your email for the recovery link.",
	})
}

// RestoreByToken is the public-facing landing endpoint that backs the
// /account/restore?token=... URL we email out. Anonymous endpoint — the
// token IS the credential. Returns success/failure; the web handler that
// wraps this renders the human-readable confirmation page.
func (h *AccountDeletionHandler) RestoreByToken(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		respondBadRequest(w, "Missing token")
		return
	}
	req, err := h.svc.RestoreByToken(r.Context(), token)
	if errors.Is(err, service.ErrDeletionRestoreInvalid) {
		respondError(w, "This restore link is invalid or expired. If you still have access to your email, you can contact support@mycarecompanion.net for help.", http.StatusGone)
		return
	}
	if err != nil {
		respondInternalError(w, "Restore failed: "+err.Error())
		return
	}
	respondOK(w, map[string]interface{}{
		"request": req,
		"message": "Your account has been restored. You can sign in normally now.",
	})
}

// clearAuthCookies wipes the user-facing auth cookies on the same domain
// and path the AuthMiddleware reads them from. Per cookieLegacy = "access_token"
// + cookieUser = "user_access_token" in middleware/auth.go. Best-effort —
// it's OK if a cookie doesn't exist.
func clearAuthCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{"access_token", "refresh_token", "user_access_token"} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		})
	}
}
