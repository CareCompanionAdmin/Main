package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

type AuthHandler struct {
	authService *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Register handles user registration
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req service.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("JSON decode error: %v", err)
		middleware.JSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" || req.FirstName == "" {
		middleware.JSONError(w, "Email, password, and first name are required", http.StatusBadRequest)
		return
	}

	user, tokens, err := h.authService.Register(r.Context(), &req)
	if err != nil {
		log.Printf("Registration error: %v", err)
		switch err {
		case service.ErrEmailExists:
			middleware.JSONError(w, "Email already registered", http.StatusConflict)
		default:
			middleware.JSONError(w, "Registration failed", http.StatusInternalServerError)
		}
		return
	}

	// Set cookies for web clients
	h.setUserAuthCookies(w, r, tokens)

	// Return response
	response := map[string]interface{}{
		"user":          user,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req service.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.JSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		middleware.JSONError(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	user, tokens, err := h.authService.LoginWithContext(r.Context(), &req, service.LoginContext{
		Kind:      models.SessionKindUser,
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		switch err {
		case service.ErrInvalidCredentials:
			middleware.JSONError(w, "Invalid email or password", http.StatusUnauthorized)
		case service.ErrUserInactive:
			middleware.JSONError(w, "Account is inactive", http.StatusForbidden)
		default:
			middleware.JSONError(w, "Login failed", http.StatusInternalServerError)
		}
		return
	}

	// Set cookies for web clients
	h.setUserAuthCookies(w, r, tokens)

	response := map[string]interface{}{
		"user":          user,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"expires_at":    tokens.ExpiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Logout handles user logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if err := h.authService.Logout(r.Context(), userID); err != nil {
		middleware.JSONError(w, "Logout failed", http.StatusInternalServerError)
		return
	}

	// Clear cookies
	h.clearUserAuthCookies(w)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out successfully"})
}

// LogoutAll revokes both the user-kind and admin-kind sessions for the
// current user and clears every auth cookie. Useful for "log out of all my
// devices" UX.
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if err := h.authService.LogoutAll(r.Context(), userID); err != nil {
		middleware.JSONError(w, "Logout failed", http.StatusInternalServerError)
		return
	}
	h.clearAllAuthCookies(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out from all sessions"})
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Try to get from cookie
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
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

	h.setUserAuthCookies(w, r, tokens)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

// SwitchFamily switches the user's family context
func (h *AuthHandler) SwitchFamily(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FamilyID string `json:"family_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.JSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	userID := middleware.GetUserID(r.Context())
	familyID, err := parseUUID(req.FamilyID)
	if err != nil {
		middleware.JSONError(w, "Invalid family ID", http.StatusBadRequest)
		return
	}

	tokens, err := h.authService.SwitchFamily(r.Context(), userID, familyID)
	if err != nil {
		middleware.JSONError(w, err.Error(), http.StatusForbidden)
		return
	}

	h.setUserAuthCookies(w, r, tokens)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

// Me returns the current user's info
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		middleware.JSONError(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	response := map[string]interface{}{
		"user_id":    claims.UserID,
		"email":      claims.Email,
		"first_name": claims.FirstName,
		"family_id":  claims.FamilyID,
		"role":       claims.Role,
	}

	// Surface subscription entitlement so the mobile app can show the right
	// "Subscribe / Upgrade" CTA without a second round-trip to /api/family/billing.
	ent := middleware.GetEntitlement(r.Context())
	entJSON := map[string]interface{}{
		"mode":             ent.Mode,
		"status":           ent.Status,
		"is_admin":         ent.IsAdminOverride,
		"has_subscription": ent.HasSubscription,
	}
	if ent.TrialEnd != nil {
		entJSON["trial_end"] = ent.TrialEnd.Format("2006-01-02T15:04:05Z07:00")
	}
	if ent.PeriodEnd != nil {
		entJSON["period_end"] = ent.PeriodEnd.Format("2006-01-02T15:04:05Z07:00")
	}
	if ent.ReadOnlyUntil != nil {
		entJSON["read_only_until"] = ent.ReadOnlyUntil.Format("2006-01-02T15:04:05Z07:00")
	}
	response["entitlement"] = entJSON

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *AuthHandler) setUserAuthCookies(w http.ResponseWriter, r *http.Request, tokens *service.TokenPair) {
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "user_access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		Expires:  tokens.ExpiresAt,
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    tokens.RefreshToken,
		Path:     "/api/auth/refresh",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearUserAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "user_access_token", Value: "", Path: "/", Expires: time.Unix(0, 0), HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "access_token", Value: "", Path: "/", Expires: time.Unix(0, 0), HttpOnly: true})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/api/auth/refresh", Expires: time.Unix(0, 0), HttpOnly: true})
}

func (h *AuthHandler) clearAllAuthCookies(w http.ResponseWriter) {
	h.clearUserAuthCookies(w)
	http.SetCookie(w, &http.Cookie{Name: "admin_access_token", Value: "", Path: "/", Expires: time.Unix(0, 0), HttpOnly: true})
}
