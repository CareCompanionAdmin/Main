package middleware

import (
	"net/http"

	"carecompanion/internal/auth"
)

// RequireSection gates a route by the permission matrix in internal/auth.
// The required level is derived from the HTTP method (GET/HEAD/OPTIONS need
// read, DELETE needs full, others need write). Super admin short-circuits
// before any matrix lookup so a typo in the matrix can never lock out the
// only role that can fix it.
//
// Failures: 401 if no auth claims (caller forgot to chain AuthMiddleware
// first); 403 with reason "section_<name>_denied" if the role's matrix
// entry doesn't satisfy the required level.
func RequireSection(section string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetAuthClaims(r.Context())
			if claims == nil {
				JSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if claims.IsSuperAdmin() {
				next.ServeHTTP(w, r)
				return
			}
			if !auth.Allows(claims.SystemRole, section, r.Method) {
				JSONError(w, "Forbidden: section_"+section+"_denied", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
