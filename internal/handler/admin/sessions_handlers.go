package admin

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// allowKill returns true for super_admin / support / partner — the three
// roles approved to revoke sessions. Mirrors the inline check used by
// RevokeSession in handlers.go.
func allowKill(claims *service.AuthClaims) bool {
	if claims == nil {
		return false
	}
	return claims.HasAnySystemRole(
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRolePartner,
	)
}

// LiveSessionsPage renders /admin/sessions.
func (h *Handler) LiveSessionsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	if h.liveSessionsService == nil {
		http.Error(w, "Live sessions service not configured", http.StatusInternalServerError)
		return
	}

	snap := h.liveSessionsService.Snapshot(r.Context())

	tmpl, err := parseTemplates("layout.html", "sessions.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	data := AdminPageData{
		Title: "Live Sessions",
		CurrentUser: AdminUser{
			ID:         claims.UserID,
			Email:      claims.Email,
			FirstName:  claims.FirstName,
			SystemRole: string(claims.SystemRole),
		},
		Data: snap,
	}
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "Render error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ListLiveSessions returns the snapshot as JSON for any client poller.
func (h *Handler) ListLiveSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	if h.liveSessionsService == nil {
		middleware.JSONError(w, "Live sessions service not configured", http.StatusInternalServerError)
		return
	}
	snap := h.liveSessionsService.Snapshot(r.Context())
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snap)
}

// BulkRevokeSessions accepts {"ids":[...]} and revokes each. Failures don't
// abort the loop. Returns {"revoked":N}.
func (h *Handler) BulkRevokeSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	if h.liveSessionsService == nil {
		middleware.JSONError(w, "Live sessions service not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		middleware.JSONError(w, "Invalid body", http.StatusBadRequest)
		return
	}
	parsed := make([]uuid.UUID, 0, len(body.IDs))
	for _, idStr := range body.IDs {
		if id, err := uuid.Parse(idStr); err == nil {
			parsed = append(parsed, id)
		}
	}
	revoked := h.liveSessionsService.RevokeBulk(r.Context(), parsed, h.authService)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"revoked": revoked})
}

// KillSSHSessionJSON is a JSON wrapper around DevModeService.KillSession for
// the Live Sessions page (the existing form-encoded handler in
// dev_mode_handlers.go redirects to /admin/development which is wrong here).
func (h *Handler) KillSSHSessionJSON(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if !allowKill(claims) {
		middleware.JSONError(w, "Forbidden", http.StatusForbidden)
		return
	}
	if devModeService == nil {
		middleware.JSONError(w, "DevMode service not configured", http.StatusInternalServerError)
		return
	}
	var body struct {
		TTY string `json:"tty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TTY == "" {
		middleware.JSONError(w, "tty required", http.StatusBadRequest)
		return
	}
	if err := devModeService.KillSession(r.Context(), body.TTY); err != nil {
		middleware.JSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
