package admin

import (
	"encoding/json"
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/auth"
	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
	"carecompanion/internal/service"
)

// Template data structures
type AdminPageData struct {
	Title          string
	CurrentUser    AdminUser
	Data           interface{}
	Flash          string
	UserTimeFormat string
}

type AdminUser struct {
	ID         uuid.UUID
	Email      string
	FirstName  string
	SystemRole string
}

// templateFuncs provides custom functions for admin templates
var templateFuncs = template.FuncMap{
	// canSee reports whether a role can see a sidebar section. Reads the
	// permission matrix in internal/auth/perm.go.
	"canSee": func(role string, section string) bool {
		return auth.Matrix(models.SystemRole(role), section) != auth.LevelNone
	},
	// matrixLevel returns the access level a role has on a section
	// ("none"/"read"/"write"/"full"). Used to render "(Read Only)" badges.
	"matrixLevel": func(role string, section string) string {
		return string(auth.Matrix(models.SystemRole(role), section))
	},
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
	"join": func(items []string, sep string) string {
		if items == nil {
			return ""
		}
		result := ""
		for i, item := range items {
			if i > 0 {
				result += sep
			}
			result += item
		}
		return result
	},
	"formatTimeStr": func(s string, args ...string) string {
		if s == "" {
			return ""
		}
		format := "12h"
		if len(args) > 0 && args[0] != "" {
			format = args[0]
		}
		for _, layout := range []string{"15:04:05", "15:04"} {
			if t, err := time.Parse(layout, s); err == nil {
				if format == "24h" {
					return t.Format("15:04")
				}
				return t.Format("3:04 PM")
			}
		}
		return s
	},
	"candidateJSON": func(c repository.BountyCandidate) template.JS {
		// Marshal a bounty candidate to JS for inline onclick attributes.
		// Returns template.JS so html/template doesn't escape the braces.
		out := map[string]interface{}{
			"type":             c.Type,
			"recipient_user_id": c.RecipientUserID.String(),
			"recipient_email":  c.RecipientEmail,
			"subject":          c.Subject,
		}
		if c.TicketID.Valid {
			out["ticket_id"] = c.TicketID.UUID.String()
		}
		if c.RoadmapItemID.Valid {
			out["roadmap_item_id"] = c.RoadmapItemID.UUID.String()
		}
		if c.SourceTicketID.Valid {
			out["source_ticket_id"] = c.SourceTicketID.UUID.String()
		}
		b, _ := json.Marshal(out)
		return template.JS(b)
	},
	"formatDateTime": func(t interface{}) string {
		if t == nil {
			return ""
		}
		// Handle time.Time
		if tm, ok := t.(time.Time); ok {
			if tm.IsZero() {
				return ""
			}
			return tm.Format("2006-01-02T15:04")
		}
		// Handle *time.Time
		if tm, ok := t.(*time.Time); ok {
			if tm == nil || tm.IsZero() {
				return ""
			}
			return tm.Format("2006-01-02T15:04")
		}
		return ""
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

// parsePublicTemplate loads a single template from templates/ (not the admin
// subdir) for use by no-auth public pages like beta onboarding.
func parsePublicTemplate(name string) (*template.Template, error) {
	return template.New(name).Funcs(templateFuncs).ParseFiles(
		filepath.Join("templates", name),
	)
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

	loginReq := &service.LoginRequest{Email: email, Password: password}
	user, tokens, err := h.authService.LoginWithContext(r.Context(), loginReq, service.LoginContext{
		Kind:      models.SessionKindAdmin,
		IP:        r.RemoteAddr,
		UserAgent: r.UserAgent(),
	})
	if err != nil {
		tmpl, _ := parseTemplates("login.html")
		tmpl.Execute(w, AdminPageData{Title: "Admin Login", Flash: "Invalid credentials"})
		return
	}

	if !user.HasSystemRole() {
		// Roll back the admin session row that LoginWithContext just created;
		// otherwise a non-admin user would have a dangling kind='admin' row.
		_ = h.authService.LogoutAdmin(r.Context(), user.ID)
		tmpl, _ := parseTemplates("login.html")
		tmpl.Execute(w, AdminPageData{Title: "Admin Login", Flash: "Access denied - admin role required"})
		return
	}

	// "/" path so the cookie covers both /admin UI and /api/admin API routes.
	isSecure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_access_token",
		Value:    tokens.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  tokens.ExpiresAt, // honors the global 8h expiry, not 15m
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

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// AdminLogout revokes the admin session row and clears the admin cookie.
// POSTed from the layout's logout button. Redirects to /admin/login.
func (h *Handler) AdminLogout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	_ = h.authService.LogoutAdmin(r.Context(), userID)
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_access_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	// Belt-and-suspenders: clear the legacy cookie if a stale browser still has it.
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_refresh_token",
		Value:    "",
		Path:     "/api/admin/auth/refresh",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	})
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
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

	status := r.URL.Query().Get("status")
	ticketType := r.URL.Query().Get("type")
	tickets, total, _ := h.adminRepo.GetTickets(r.Context(), status, ticketType, 1, 50)

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

	// If the roadmap service is wired and this ticket is a feature_request,
	// look up whether it's already been promoted so the UI can show
	// either the "Add to Roadmap" button or a link to the existing item.
	var roadmapItem interface{}
	if h.roadmapService != nil && ticket != nil && ticket.Type == "feature_request" {
		if existing, _ := h.roadmapService.GetByTicketID(r.Context(), id); existing != nil {
			roadmapItem = existing
		}
	}

	// Resolve dup canonical info so the banner can render a friendly link.
	var dupCanonicalTicket interface{}
	var dupCanonicalRoadmap interface{}
	if ticket != nil {
		if ticket.DuplicateOfTicketID.Valid {
			dupCanonicalTicket, _ = h.adminRepo.GetTicketByID(r.Context(), ticket.DuplicateOfTicketID.UUID)
		}
		if ticket.DuplicateOfRoadmapID.Valid && h.roadmapService != nil {
			dupCanonicalRoadmap, _ = h.roadmapService.Get(r.Context(), ticket.DuplicateOfRoadmapID.UUID)
		}
	}

	tmpl, err := parseTemplates("layout.html", "ticket_detail.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Ticket #" + id.String()[:8],
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"ticket":            ticket,
			"messages":          messages,
			"roadmap_item":      roadmapItem,
			"is_super":          currentUser.SystemRole == "super_admin",
			"dup_canonical_t":   dupCanonicalTicket,
			"dup_canonical_r":   dupCanonicalRoadmap,
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

// MaterialsPage renders the marketing materials center page
func (h *Handler) MaterialsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "marketing_materials.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Marketing Materials",
		CurrentUser: currentUser,
	})
}

// StatusPage renders the infrastructure status page
func (h *Handler) StatusPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "status.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Infrastructure Status",
		CurrentUser: currentUser,
	})
}

// ErrorsPage renders the error logs page
func (h *Handler) ErrorsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "errors.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Error Logs",
		CurrentUser: currentUser,
	})
}

// FinancialsPage renders the financials dashboard page
func (h *Handler) FinancialsPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	tmpl, err := parseTemplates("layout.html", "financials.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Financials",
		CurrentUser: currentUser,
	})
}

// PromoCodesPage renders the promo codes list page
func (h *Handler) PromoCodesPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	// Get subscription plans for display/filtering
	plans, _ := h.adminRepo.ListSubscriptionPlans(r.Context(), true)

	tmpl, err := parseTemplates("layout.html", "promo_codes.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Promo Codes",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"plans": plans,
		},
	})
}

// PromoCodeNewPage renders the new promo code form page
func (h *Handler) PromoCodeNewPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	// Get subscription plans for the form
	plans, _ := h.adminRepo.ListSubscriptionPlans(r.Context(), true)

	tmpl, err := parseTemplates("layout.html", "promo_code_form.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Create Promo Code",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"plans":  plans,
			"isNew":  true,
			"action": "/api/admin/super/promo-codes",
		},
	})
}

// PromoCodeEditPage renders the edit promo code form page
func (h *Handler) PromoCodeEditPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID:         claims.UserID,
		Email:      claims.Email,
		FirstName:  claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid promo code ID", http.StatusBadRequest)
		return
	}

	promo, err := h.adminRepo.GetPromoCodeByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to fetch promo code: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if promo == nil {
		http.Error(w, "Promo code not found", http.StatusNotFound)
		return
	}

	// Get subscription plans for the form
	plans, _ := h.adminRepo.ListSubscriptionPlans(r.Context(), true)

	tmpl, err := parseTemplates("layout.html", "promo_code_form.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Edit Promo Code",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"promo":  promo,
			"plans":  plans,
			"isNew":  false,
			"action": "/api/admin/super/promo-codes/" + id.String(),
		},
	})
}
