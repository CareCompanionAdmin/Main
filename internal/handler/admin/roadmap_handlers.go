package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/service"
)

// roadmapForm is what the create/update endpoints accept.
type roadmapForm struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Priority     string `json:"priority"`
	Source       string `json:"source"`
	NotifyOnDev  bool   `json:"notify_on_dev"`
	NotifyOnProd bool   `json:"notify_on_prod"`
}

func (f roadmapForm) toServiceInput() service.CreateRoadmapInput {
	return service.CreateRoadmapInput{
		Title:        f.Title,
		Description:  f.Description,
		Status:       f.Status,
		Priority:     f.Priority,
		Source:       f.Source,
		NotifyOnDev:  f.NotifyOnDev,
		NotifyOnProd: f.NotifyOnProd,
	}
}

// ============================================================================
// JSON API
// ============================================================================

func (h *Handler) ListRoadmapItems(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	items, err := h.roadmapService.List(
		r.Context(),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("priority"),
		r.URL.Query().Get("source"),
	)
	if err != nil {
		http.Error(w, "Failed to list roadmap: "+err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, items)
}

func (h *Handler) GetRoadmapItem(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	item, err := h.roadmapService.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get roadmap item: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if item == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	respondJSON(w, item)
}

func (h *Handler) CreateRoadmapItem(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	var form roadmapForm
	if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	item, err := h.roadmapService.Create(r.Context(), form.toServiceInput(), claims.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.logAction(r, "create_roadmap_item", "roadmap", item.ID, nil)
	respondJSON(w, item)
}

func (h *Handler) UpdateRoadmapItem(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	var form roadmapForm
	if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}
	item, err := h.roadmapService.Update(r.Context(), id, form.toServiceInput())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	h.logAction(r, "update_roadmap_item", "roadmap", id, nil)
	respondJSON(w, item)
}

func (h *Handler) DeleteRoadmapItem(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	if err := h.roadmapService.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "delete_roadmap_item", "roadmap", id, nil)
	respondJSON(w, map[string]string{"status": "deleted"})
}

// AddRoadmapFromTicket promotes a feature_request ticket onto the roadmap.
// The body may include `priority` (defaults to p2 in the service).
func (h *Handler) AddRoadmapFromTicket(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	ticketID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid ticket id", http.StatusBadRequest)
		return
	}
	var body struct {
		Priority string `json:"priority"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // body is optional

	claims := middleware.GetAuthClaims(r.Context())
	item, err := h.roadmapService.AddFromTicket(r.Context(), ticketID, claims.UserID, body.Priority)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRoadmapTicketAlready):
			http.Error(w, err.Error(), http.StatusConflict)
		case errors.Is(err, service.ErrRoadmapTicketWrongType):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	h.logAction(r, "add_roadmap_from_ticket", "roadmap", item.ID, map[string]interface{}{"ticket_id": ticketID})
	respondJSON(w, item)
}

func (h *Handler) MarkRoadmapLiveDev(w http.ResponseWriter, r *http.Request) {
	h.markRoadmapLive(w, r, "dev")
}

func (h *Handler) MarkRoadmapLiveProd(w http.ResponseWriter, r *http.Request) {
	h.markRoadmapLive(w, r, "prod")
}

func (h *Handler) markRoadmapLive(w http.ResponseWriter, r *http.Request, env string) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	var item interface{}
	if env == "dev" {
		item, err = h.roadmapService.MarkLiveDev(r.Context(), id, claims.UserID)
	} else {
		item, err = h.roadmapService.MarkLiveProd(r.Context(), id, claims.UserID)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logAction(r, "mark_roadmap_live_"+env, "roadmap", id, nil)
	respondJSON(w, item)
}

// ============================================================================
// UI PAGES
// ============================================================================

func (h *Handler) RoadmapListPage(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID: claims.UserID, Email: claims.Email, FirstName: claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	items, _ := h.roadmapService.List(
		r.Context(),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("priority"),
		r.URL.Query().Get("source"),
	)
	tmpl, err := parseTemplates("layout.html", "roadmap_list.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Roadmap",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"items":          items,
			"filter_status":  r.URL.Query().Get("status"),
			"filter_prio":    r.URL.Query().Get("priority"),
			"filter_source":  r.URL.Query().Get("source"),
		},
	})
}

func (h *Handler) RoadmapNewPage(w http.ResponseWriter, r *http.Request) {
	h.renderRoadmapForm(w, r, nil)
}

func (h *Handler) RoadmapEditPage(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	item, err := h.roadmapService.Get(r.Context(), id)
	if err != nil || item == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	h.renderRoadmapForm(w, r, item)
}

func (h *Handler) renderRoadmapForm(w http.ResponseWriter, r *http.Request, item interface{}) {
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID: claims.UserID, Email: claims.Email, FirstName: claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}
	tmpl, err := parseTemplates("layout.html", "roadmap_form.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Roadmap Item",
		CurrentUser: currentUser,
		Data: map[string]interface{}{
			"item":    item,
			"is_edit": item != nil,
		},
	})
}

func (h *Handler) RoadmapDetailPage(w http.ResponseWriter, r *http.Request) {
	if h.roadmapService == nil {
		http.Error(w, "Roadmap service unavailable", http.StatusServiceUnavailable)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	currentUser := AdminUser{
		ID: claims.UserID, Email: claims.Email, FirstName: claims.FirstName,
		SystemRole: string(claims.SystemRole),
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}
	item, err := h.roadmapService.Get(r.Context(), id)
	if err != nil || item == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	tmpl, err := parseTemplates("layout.html", "roadmap_detail.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = tmpl.ExecuteTemplate(w, "layout.html", AdminPageData{
		Title:       "Roadmap: " + item.Title,
		CurrentUser: currentUser,
		Data:        map[string]interface{}{"item": item},
	})
}
