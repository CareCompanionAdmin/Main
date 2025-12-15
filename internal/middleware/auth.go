package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// Context keys for storing auth data
type contextKey string

const (
	UserIDKey     contextKey = "userID"
	EmailKey      contextKey = "email"
	FamilyIDKey   contextKey = "familyID"
	RoleKey       contextKey = "role"
	FirstNameKey  contextKey = "firstName"
	AuthClaimsKey contextKey = "authClaims"
)

// AuthMiddleware validates JWT tokens and sets user context
func AuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get token from header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Try cookie for web requests
				cookie, err := r.Cookie("access_token")
				if err != nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				authHeader = "Bearer " + cookie.Value
			}

			// Validate Bearer format
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}

			// Validate token
			claims, err := authService.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Set context values
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)
			ctx = context.WithValue(ctx, FamilyIDKey, claims.FamilyID)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
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
