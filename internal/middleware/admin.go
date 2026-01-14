package middleware

import (
	"net/http"

	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// RequireSystemRole middleware ensures user has one of the specified system roles
func RequireSystemRole(roles ...models.SystemRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetAuthClaims(r.Context())
			if claims == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !claims.HasSystemRole() {
				http.Error(w, "Forbidden - admin access required", http.StatusForbidden)
				return
			}

			// Check if user has any of the required roles
			hasRole := false
			for _, role := range roles {
				if claims.SystemRole == role {
					hasRole = true
					break
				}
			}

			if !hasRole {
				http.Error(w, "Forbidden - insufficient admin permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAnyAdminRole middleware ensures user has any system admin role
func RequireAnyAdminRole() func(http.Handler) http.Handler {
	return RequireSystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRoleMarketing,
	)
}

// RequireSuperAdmin middleware ensures user is a super admin
func RequireSuperAdmin() func(http.Handler) http.Handler {
	return RequireSystemRole(models.SystemRoleSuperAdmin)
}

// RequireSupport middleware ensures user is a support admin (or super admin)
func RequireSupport() func(http.Handler) http.Handler {
	return RequireSystemRole(models.SystemRoleSuperAdmin, models.SystemRoleSupport)
}

// RequireMarketing middleware ensures user is a marketing admin (or super admin)
func RequireMarketing() func(http.Handler) http.Handler {
	return RequireSystemRole(models.SystemRoleSuperAdmin, models.SystemRoleMarketing)
}

// RequireSupportOrMarketing middleware ensures user is support, marketing, or super admin
func RequireSupportOrMarketing() func(http.Handler) http.Handler {
	return RequireSystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRoleMarketing,
	)
}

// AdminAuthMiddleware is a variant of AuthMiddleware that additionally checks for admin role
// This can be used to create a completely separate auth flow for admin portal if needed
func AdminAuthMiddleware(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// First apply normal auth
			AuthMiddleware(authService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Then check for admin role
				claims := GetAuthClaims(r.Context())
				if claims == nil || !claims.HasSystemRole() {
					http.Error(w, "Forbidden - admin access required", http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
			})).ServeHTTP(w, r)
		})
	}
}

// MaintenanceModeMiddleware checks if system is in maintenance mode
// Super admins can bypass maintenance mode
type MaintenanceChecker interface {
	IsMaintenanceMode() bool
	GetMaintenanceMessage() string
}

func MaintenanceModeMiddleware(checker MaintenanceChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if checker.IsMaintenanceMode() {
				// Allow super admins to bypass
				claims := GetAuthClaims(r.Context())
				if claims != nil && claims.IsSuperAdmin() {
					next.ServeHTTP(w, r)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error": "System is under maintenance", "message": "` + checker.GetMaintenanceMessage() + `"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
