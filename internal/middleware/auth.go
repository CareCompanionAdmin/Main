package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// Context keys for storing auth data
type contextKey string

const (
	UserIDKey      contextKey = "userID"
	EmailKey       contextKey = "email"
	FamilyIDKey    contextKey = "familyID"
	RoleKey        contextKey = "role"
	SystemRoleKey  contextKey = "systemRole"
	FirstNameKey   contextKey = "firstName"
	AuthClaimsKey  contextKey = "authClaims"
)

// AuthMiddleware validates JWT tokens and sets user context.
//
// Failure UX: API requests (path under /api/) get a JSON body so the client
// can detect token expiry and show a graceful "session expired" banner
// instead of a raw 401. Web (browser) GET requests get a 303 redirect to
// /login?return=<path> so users land on a usable page rather than seeing
// "Unauthorized" plain text. POST/PUT/etc. on web routes still return 401
// because a redirect to GET /login would silently drop the form submission.
func AuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Debug: Log all cookies for API routes (only for admin API paths)
			if strings.HasPrefix(r.URL.Path, "/api/admin") {
				cookies := r.Cookies()
				log.Printf("Auth debug [%s]: %d cookies received, X-Forwarded-Proto=%s",
					r.URL.Path, len(cookies), r.Header.Get("X-Forwarded-Proto"))
				for _, c := range cookies {
					log.Printf("  Cookie: %s (len=%d)", c.Name, len(c.Value))
				}
			}

			// Get token from header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Try cookie for web requests
				cookie, err := r.Cookie("access_token")
				if err != nil {
					if strings.HasPrefix(r.URL.Path, "/api/admin") {
						log.Printf("Auth debug [%s]: No access_token cookie found, err=%v", r.URL.Path, err)
					}
					unauthorized(w, r, "no_token")
					return
				}
				authHeader = "Bearer " + cookie.Value
			}

			// Validate Bearer format
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				unauthorized(w, r, "bad_header")
				return
			}

			// Validate token
			claims, err := authService.ValidateToken(parts[1])
			if err != nil {
				if strings.HasPrefix(r.URL.Path, "/api/admin") {
					log.Printf("Auth debug [%s]: Token validation failed: %v", r.URL.Path, err)
				}
				unauthorized(w, r, "expired_or_invalid")
				return
			}

			// Set context values
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)
			ctx = context.WithValue(ctx, FamilyIDKey, claims.FamilyID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			ctx = context.WithValue(ctx, SystemRoleKey, claims.SystemRole)
			ctx = context.WithValue(ctx, FirstNameKey, claims.FirstName)
			ctx = context.WithValue(ctx, AuthClaimsKey, claims)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuthMiddleware extracts auth info if present but doesn't require it
func OptionalAuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Try cookie for web requests
				cookie, err := r.Cookie("access_token")
				if err == nil {
					authHeader = "Bearer " + cookie.Value
				}
			}

			if authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					claims, err := authService.ValidateToken(parts[1])
					if err == nil {
						ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
						ctx = context.WithValue(ctx, EmailKey, claims.Email)
						ctx = context.WithValue(ctx, FamilyIDKey, claims.FamilyID)
						ctx = context.WithValue(ctx, RoleKey, claims.Role)
						ctx = context.WithValue(ctx, SystemRoleKey, claims.SystemRole)
						ctx = context.WithValue(ctx, FirstNameKey, claims.FirstName)
						ctx = context.WithValue(ctx, AuthClaimsKey, claims)
						r = r.WithContext(ctx)
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole middleware ensures user has required role
func RequireRole(requiredRole models.FamilyRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(RoleKey).(models.FamilyRole)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !hasRequiredRole(role, requiredRole) {
				http.Error(w, "Forbidden - insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// unauthorized is the single failure exit for AuthMiddleware. It picks the
// right response shape based on the request:
//   - /api/* → 401 + JSON `{"error":"unauthorized","reason":"..."}` so the
//     client can show a graceful banner instead of a raw 401.
//   - GET /<web-page> → 303 redirect to /login?return=<original-path> so a
//     stale browser tab lands on the login form rather than plain text.
//   - non-GET on web routes (form submits with expired session) → 401, since
//     redirecting a POST to GET /login would silently drop the submission.
func unauthorized(w http.ResponseWriter, r *http.Request, reason string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		// Hand-rolled JSON to avoid pulling encoding/json into this hot path.
		fmt.Fprintf(w, `{"error":"unauthorized","reason":%q}`, reason)
		return
	}
	if r.Method == http.MethodGet {
		ret := r.URL.Path
		if r.URL.RawQuery != "" {
			ret += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, "/login?return="+url.QueryEscape(ret), http.StatusSeeOther)
		return
	}
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// RequireFamilyContext ensures a family context is set
func RequireFamilyContext() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			familyID, ok := r.Context().Value(FamilyIDKey).(uuid.UUID)
			if !ok || familyID == uuid.Nil {
				http.Error(w, "No family context set", http.StatusBadRequest)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Helper functions for extracting context values
func GetUserID(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(UserIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func GetEmail(ctx context.Context) string {
	if email, ok := ctx.Value(EmailKey).(string); ok {
		return email
	}
	return ""
}

func GetFamilyID(ctx context.Context) uuid.UUID {
	if id, ok := ctx.Value(FamilyIDKey).(uuid.UUID); ok {
		return id
	}
	return uuid.Nil
}

func GetRole(ctx context.Context) models.FamilyRole {
	if role, ok := ctx.Value(RoleKey).(models.FamilyRole); ok {
		return role
	}
	return ""
}

func GetSystemRole(ctx context.Context) models.SystemRole {
	if role, ok := ctx.Value(SystemRoleKey).(models.SystemRole); ok {
		return role
	}
	return ""
}

func HasSystemRole(ctx context.Context) bool {
	return GetSystemRole(ctx) != ""
}

func GetFirstName(ctx context.Context) string {
	if name, ok := ctx.Value(FirstNameKey).(string); ok {
		return name
	}
	return ""
}

func GetAuthClaims(ctx context.Context) *service.AuthClaims {
	if claims, ok := ctx.Value(AuthClaimsKey).(*service.AuthClaims); ok {
		return claims
	}
	return nil
}

// hasRequiredRole checks if the actual role meets the required role level
func hasRequiredRole(actual, required models.FamilyRole) bool {
	roleHierarchy := map[models.FamilyRole]int{
		models.FamilyRoleParent:          3,
		models.FamilyRoleMedicalProvider: 2,
		models.FamilyRoleCaregiver:       1,
	}

	return roleHierarchy[actual] >= roleHierarchy[required]
}
