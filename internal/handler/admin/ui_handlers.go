package admin

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// Template data structures
type AdminPageData struct {
	Title       string
	CurrentUser AdminUser
	Data        interface{}
	Flash       string
}

type AdminUser struct {
	ID         uuid.UUID
	Email      string
	FirstName  string
	SystemRole string
}

// templateFuncs provides custom functions for admin templates
var templateFuncs = template.FuncMap{
	"divf": func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"mulf": func(a, b float64) float64 {
		return a * b
	},
	"float64": func(i int) float64 {
		return float64(i)
	},
}

// parseTemplates loads admin templates with custom functions
func parseTemplates(names ...string) (*template.Template, error) {
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join("templates", "admin", name)
	}
	return template.New(names[0]).Funcs(templateFuncs).ParseFiles(paths...)
}

// AdminLoginPage renders the admin login page
func (h *Handler) AdminLoginPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := parseTemplates("login.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, AdminPageData{Title: "Admin Login"})
}

// AdminLoginSubmit handles admin login form submission
func (h *Handler) AdminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	// Use auth service to login
	loginReq := &service.LoginRequest{
		Email:    email,
		Password: password,
	}

	user, tokens, err := h.authService.Login(r.Context(), loginReq)
	if err != nil {
		tmpl, _ := parseTemplates("login.html")
		tmpl.Execute(w, AdminPageData{
			Title: "Admin Login",
			Flash: "Invalid credentials",
		})
		return
	}

	// Check if user has admin role
	if !user.HasSystemRole() {
		tmpl, _ := parseTemplates("login.html")
		tmpl.Execute(w, AdminPageData{
			Title: "Admin Login",
			Flash: "Access denied - admin role required",
		})
		return
	}

	// Set cookie - use "/" path to cover both /admin UI and /api/admin API routes
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(15 * time.Minute),
	})

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// AdminDashboard renders the main admin dashboard based on role
func (h *Handler) AdminDashboard(w http.ResponseWriter, r *http.Request) {
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

	// Get metrics for dashboard
	metrics, _ := h.adminRepo.GetCachedMetrics(r.Context())

	tmpl, err := parseTemplates("layout.html", "dashboard.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Admin Dashboard",
		CurrentUser: currentUser,
		Data:        metrics,
	})
}

// AdminUsersPage renders the admin users management page
func (h *Handler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	admins, _ := h.adminRepo.ListAdminUsers(r.Context())

	tmpl, err := parseTemplates("layout.html", "admins.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Admin Users",
		CurrentUser: currentUser,
		Data:        admins,
	})
}

// SettingsPage renders the system settings page
func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	settings, _ := h.adminRepo.GetAllSettings(r.Context())

	tmpl, err := parseTemplates("layout.html", "settings.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "System Settings",
		CurrentUser: currentUser,
		Data:        settings,
	})
}

// AuditLogPage renders the audit log page
func (h *Handler) AuditLogPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	entries, total, _ := h.adminRepo.GetAuditLog(r.Context(), uuid.Nil, "", 1, 50)

	tmpl, err := parseTemplates("layout.html", "audit.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Audit Log",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"entries": entries,
			"total":   total,
		},
	})
}

// TicketsPage renders the support tickets page
func (h *Handler) TicketsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tickets, total, _ := h.adminRepo.GetTickets(r.Context(), "", 1, 50)

	tmpl, err := parseTemplates("layout.html", "tickets.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Support Tickets",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"tickets": tickets,
			"total":   total,
		},
	})
}

// TicketDetailPage renders a single ticket detail page
func (h *Handler) TicketDetailPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	id, _ := uuid.Parse(chi.URLParam(r, "id"))
	ticket, _ := h.adminRepo.GetTicketByID(r.Context(), id)
	messages, _ := h.adminRepo.GetTicketMessages(r.Context(), id)

	tmpl, err := parseTemplates("layout.html", "ticket_detail.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Ticket #" + id.String()[:8],
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"ticket":   ticket,
			"messages": messages,
		},
	})
}

// UsersPage renders the user management page
func (h *Handler) UsersPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	query := r.URL.Query().Get("q")
	users, total, _ := h.adminRepo.SearchUsers(r.Context(), query, 1, 50)

	tmpl, err := parseTemplates("layout.html", "users.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "User Management",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"users": users,
			"total": total,
			"query": query,
		},
	})
}

// FamiliesPage renders the families list page
func (h *Handler) FamiliesPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	families, total, _ := h.adminRepo.ListFamilies(r.Context(), 1, 50)

	tmpl, err := parseTemplates("layout.html", "families.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Families",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"families": families,
			"total":    total,
		},
	})
}

// MarketingDashboardPage renders the marketing metrics dashboard
func (h *Handler) MarketingDashboardPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	metrics, _ := h.adminRepo.GetCachedMetrics(r.Context())

	tmpl, err := parseTemplates("layout.html", "marketing.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Marketing Dashboard",
		CurrentUser: currentUser,
		Data:        metrics,
	})
}
