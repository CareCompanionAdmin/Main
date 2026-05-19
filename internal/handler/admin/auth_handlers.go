package admin

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"carecompanion/internal/middleware"
)

// AdminRefreshToken handles admin token refresh.
//
// Public endpoint (mounted in cmd/server/main.go BEFORE the protected
// /api/admin Mount, so AuthMiddleware does NOT run on this path — by design,
// because refresh must work AFTER the access token has lapsed).
//
// Reads admin_refresh_token cookie (preferred) or {"refresh_token":"..."}
// JSON body. Writes a fresh admin_access_token + rotated admin_refresh_token
// cookie. Mirrors the user-side handler at internal/handler/api/auth_handler.go.
func (h *Handler) AdminRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	// Body first. JSON errors are non-fatal — fall back to cookie.
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.RefreshToken == "" {
		cookie, err := r.Cookie("admin_refresh_token")
		if err != nil || cookie.Value == "" {
			middleware.JSONError(w, "Refresh token required", http.StatusBadRequest)
			return
		}
		req.RefreshToken = cookie.Value
	}

	tokens, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		middleware.JSONError(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  tokens.ExpiresAt,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/admin/auth/refresh",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tokens); err != nil {
		log.Printf("[admin] AdminRefreshToken encode error: %v", err)
	}
}

// AdminAuthCheck is a lightweight liveness probe used by admin_session_guard.js.
// Mounted INSIDE the protected /api/admin Routes(), so AuthMiddleware runs first:
// a missing/expired/revoked session yields 401 from the middleware, otherwise
// the handler returns 200 with {"valid":true}.
func (h *Handler) AdminAuthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]bool{"valid": true}); err != nil {
		log.Printf("[admin] AdminAuthCheck encode error: %v", err)
	}
}
