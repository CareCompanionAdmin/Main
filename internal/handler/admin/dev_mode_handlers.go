package admin

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// devModeService is set by SetDevModeService
var devModeService *service.DevModeService

// SetDevModeService sets the dev mode service for handlers
func (h *Handler) SetDevModeService(svc *service.DevModeService) {
	devModeService = svc
}

// DevelopmentPage renders the development mode control page
func (h *Handler) DevelopmentPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	var status *models.DevModeStatus
	var sessions []models.SSHSession
	var errorMsg, successMsg string
	var hasPEMKey bool
	var instanceIP string

	if devModeService != nil {
		var err error
		status, err = devModeService.GetStatus(r.Context())
		if err != nil {
			errorMsg = "Failed to get dev mode status: " + err.Error()
		}

		sessions, _ = devModeService.ListSSHSessions(r.Context())
		hasPEMKey = devModeService.HasPEMKey()
		instanceIP = devModeService.GetCurrentInstanceIP()
	} else {
		errorMsg = "Development mode service not configured"
	}

	// Check for success message in query params
	if msg := r.URL.Query().Get("success"); msg != "" {
		successMsg = msg
	}

	data := struct {
		Title       string
		CurrentUser AdminUser
		Status      *models.DevModeStatus
		Sessions    []models.SSHSession
		HasPEMKey   bool
		InstanceIP  string
		ErrorMsg    string
		SuccessMsg  string
		Flash       string
	}{
		Title:       "Development Mode",
		CurrentUser: currentUser,
		Status:      status,
		Sessions:    sessions,
		HasPEMKey:   hasPEMKey,
		InstanceIP:  instanceIP,
		ErrorMsg:    errorMsg,
		SuccessMsg:  successMsg,
		Flash:       "",
	}

	tmpl, err := parseTemplates("layout.html", "development.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "Template execution error: "+err.Error(), http.StatusInternalServerError)
	}
}

// DevModeToggle enables or disables development mode
func (h *Handler) DevModeToggle(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	action := r.FormValue("action")
	allowedIP := r.FormValue("allowed_ip")

	var err error
	var successMsg string

	if action == "enable" {
		if allowedIP == "" {
			http.Error(w, "IP address is required", http.StatusBadRequest)
			return
		}
		err = devModeService.EnableDevMode(r.Context(), allowedIP, claims.UserID)
		if err == nil {
			successMsg = "Development mode enabled. SSH access granted for " + allowedIP
		}
	} else if action == "disable" {
		err = devModeService.DisableDevMode(r.Context(), claims.UserID)
		if err == nil {
			successMsg = "Development mode disabled. All SSH sessions terminated."
		}
	} else {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to toggle dev mode: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to development page with success message
	http.Redirect(w, r, "/admin/development?success="+successMsg, http.StatusSeeOther)
}

// DevModeKillSession kills a specific SSH session
func (h *Handler) DevModeKillSession(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	tty := r.FormValue("tty")
	if tty == "" {
		http.Error(w, "TTY is required", http.StatusBadRequest)
		return
	}

	err := devModeService.KillSession(r.Context(), tty)
	if err != nil {
		http.Error(w, "Failed to kill session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/development?success=Session+terminated", http.StatusSeeOther)
}

// DevModeGetPEMKey returns the PEM key content for clipboard copy
func (h *Handler) DevModeGetPEMKey(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	content, err := devModeService.GetPEMKeyContent()
	if err != nil {
		http.Error(w, "Failed to read PEM key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(content))
}

// DevModeDownloadPEM serves the PEM file for download
func (h *Handler) DevModeDownloadPEM(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	content, err := devModeService.GetPEMKeyContent()
	if err != nil {
		http.Error(w, "Failed to read PEM key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", "attachment; filename=carecompanion-key.pem")
	w.Write([]byte(content))
}

// DevModeDownloadPPK converts PEM to PPK and serves for download
func (h *Handler) DevModeDownloadPPK(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	content, err := devModeService.GetPEMKeyContent()
	if err != nil {
		http.Error(w, "Failed to read PEM key: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create temp file with PEM content
	f, err := os.CreateTemp("", "pem-*.pem")
	if err != nil {
		http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	// Convert PEM to PPK using puttygen
	cmd := exec.Command("puttygen", f.Name(), "-o", "/dev/stdout", "-O", "private")
	ppkContent, err := cmd.Output()
	if err != nil {
		http.Error(w, "Failed to convert key to PPK format: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=carecompanion-key.ppk")
	w.Write(ppkContent)
}

// DevModeSessions returns JSON of current SSH sessions (for HTMX refresh)
func (h *Handler) DevModeSessions(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	if claims == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if devModeService == nil {
		http.Error(w, "Development mode service not configured", http.StatusInternalServerError)
		return
	}

	sessions, err := devModeService.ListSSHSessions(r.Context())
	if err != nil {
		http.Error(w, "Failed to list sessions: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}
