package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/auth"
	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// builtinRoleView mirrors what we render for the four locked production
// roles in the list page. The grid is hardcoded from auth.Matrix() at
// render time so the display always reflects what perm.go says today.
type builtinRoleView struct {
	Name        string
	DisplayName string
	Description string
	Permissions map[string]string // section -> "read"|"write"|"full"|"none"
}

func builtinRoleViews() []builtinRoleView {
	all := []models.SystemRole{
		models.SystemRoleSuperAdmin,
		models.SystemRoleSupport,
		models.SystemRoleMarketing,
		models.SystemRolePartner,
	}
	labels := map[models.SystemRole]struct{ display, desc string }{
		models.SystemRoleSuperAdmin: {"Super Admin", "Full access to every admin section. Cannot be edited or removed."},
		models.SystemRoleSupport:    {"Support", "Customer-support staff — tickets, users, families, live sessions."},
		models.SystemRoleMarketing:  {"Marketing", "Marketing materials, beta program, bounty program, metrics."},
		models.SystemRolePartner:    {"Partner", "Broad read access plus full access to ops + roadmap surfaces."},
	}
	out := make([]builtinRoleView, 0, len(all))
	for _, r := range all {
		v := builtinRoleView{
			Name:        string(r),
			DisplayName: labels[r].display,
			Description: labels[r].desc,
			Permissions: make(map[string]string, len(auth.Sections)),
		}
		for _, s := range auth.Sections {
			v.Permissions[s] = string(auth.Matrix(r, s))
		}
		out = append(out, v)
	}
	return out
}

type userRolesListView struct {
	AdminPageData
	BuiltinRoles []builtinRoleView
	CustomRoles  []models.CustomRole
	Sections     []string
	Labels       map[string]string
}

func (h *Handler) userRolesData(r *http.Request) AdminPageData {
	claims := middleware.GetAuthClaims(r.Context())
	pd := AdminPageData{Title: "User Roles"}
	if claims != nil {
		pd.CurrentUser = AdminUser{
			ID:         claims.UserID,
			Email:      claims.Email,
			FirstName:  claims.FirstName,
			SystemRole: string(claims.SystemRole),
		}
	}
	return pd
}

func (h *Handler) UserRolesPage(w http.ResponseWriter, r *http.Request) {
	customs, err := h.roleService.List(r.Context())
	if err != nil {
		http.Error(w, "load custom roles: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := userRolesListView{
		AdminPageData: h.userRolesData(r),
		BuiltinRoles:  builtinRoleViews(),
		CustomRoles:   customs,
		Sections:      auth.Sections,
		Labels:        auth.SectionLabels,
	}
	tmpl, err := parseTemplates("layout.html", "user_roles.html")
	if err != nil {
		http.Error(w, "template parse: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout.html", v); err != nil {
		http.Error(w, "template exec: "+err.Error(), http.StatusInternalServerError)
	}
}

type userRoleFormView struct {
	AdminPageData
	IsNew    bool
	Role     *models.CustomRole
	Sections []string
	Labels   map[string]string
	Flash    string
	InUse    *service.RoleInUseError
}

func (h *Handler) UserRoleFormPage(w http.ResponseWriter, r *http.Request) {
	v := userRoleFormView{
		AdminPageData: h.userRolesData(r),
		Sections:      auth.Sections,
		Labels:        auth.SectionLabels,
	}
	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		// new form
		v.IsNew = true
		v.Role = &models.CustomRole{Permissions: map[string]string{}}
		v.AdminPageData.Title = "New Custom Role"
	} else {
		id, err := uuid.Parse(idParam)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		role, err := h.roleService.Get(r.Context(), id)
		if err != nil {
			http.Error(w, "load: "+err.Error(), http.StatusNotFound)
			return
		}
		v.Role = role
		v.AdminPageData.Title = "Edit Role — " + role.DisplayName
	}
	if v.Role.Permissions == nil {
		v.Role.Permissions = map[string]string{}
	}
	tmpl, err := parseTemplates("layout.html", "user_role_form.html")
	if err != nil {
		http.Error(w, "template parse: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout.html", v); err != nil {
		http.Error(w, "template exec: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) UserRoleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	email := ""
	if claims != nil {
		email = claims.Email
	}
	role := &models.CustomRole{
		Name:           strings.TrimSpace(r.FormValue("name")),
		DisplayName:    strings.TrimSpace(r.FormValue("display_name")),
		Description:    strings.TrimSpace(r.FormValue("description")),
		CreatedByEmail: email,
		Permissions:    parsePermissionsFromForm(r),
	}
	if err := h.roleService.Create(r.Context(), role); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/user-roles/"+role.ID.String(), http.StatusSeeOther)
}

func (h *Handler) UserRoleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	role, err := h.roleService.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	role.DisplayName = strings.TrimSpace(r.FormValue("display_name"))
	role.Description = strings.TrimSpace(r.FormValue("description"))
	role.Permissions = parsePermissionsFromForm(r)
	if err := h.roleService.Update(r.Context(), role); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/user-roles/"+role.ID.String(), http.StatusSeeOther)
}

func (h *Handler) UserRoleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.roleService.Delete(r.Context(), id); err != nil {
		var inUse *service.RoleInUseError
		if errors.As(err, &inUse) {
			// Render the form back with a friendly error and the
			// affected admin emails so super-admin can reassign.
			role, gerr := h.roleService.Get(r.Context(), id)
			if gerr != nil {
				http.Error(w, "delete blocked: "+err.Error(), http.StatusConflict)
				return
			}
			v := userRoleFormView{
				AdminPageData: h.userRolesData(r),
				Role:          role,
				Sections:      auth.Sections,
				Labels:        auth.SectionLabels,
				Flash:         "Cannot delete: this role is still assigned to admin users. Reassign them first.",
				InUse:         inUse,
			}
			v.AdminPageData.Title = "Edit Role — " + role.DisplayName
			tmpl, perr := parseTemplates("layout.html", "user_role_form.html")
			if perr != nil {
				http.Error(w, "template parse: "+perr.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusConflict)
			tmpl.ExecuteTemplate(w, "layout.html", v)
			return
		}
		http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/user-roles/", http.StatusSeeOther)
}

// parsePermissionsFromForm reads each known section's radio button.
// Missing sections / "none" → not in map. The repo only stores read|write rows.
func parsePermissionsFromForm(r *http.Request) map[string]string {
	out := map[string]string{}
	for _, section := range auth.Sections {
		val := r.FormValue("perm_" + section)
		if val == "read" || val == "write" {
			out[section] = val
		}
	}
	return out
}

// isAssignableRole reports whether the given role name can be assigned to
// an admin user — either a built-in or a custom role that currently exists.
func (h *Handler) isAssignableRole(ctx context.Context, name string) bool {
	if models.IsValidSystemRole(name) {
		return true
	}
	if h.roleService == nil {
		return false
	}
	role, err := h.roleService.GetByName(ctx, name)
	return err == nil && role != nil
}
