package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// --- Helpers ---

type proQAPageData struct {
	AdminPageData
	ActiveTab string
}

func (h *Handler) proQAData(r *http.Request, title, activeTab string) proQAPageData {
	claims := middleware.GetAuthClaims(r.Context())
	pd := proQAPageData{ActiveTab: activeTab}
	pd.Title = title
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

func (h *Handler) renderProQA(w http.ResponseWriter, tmplName string, data interface{}) {
	tmpl, err := parseTemplates("layout.html", "pro_qa_layout.html", tmplName)
	if err != nil {
		http.Error(w, "template parse: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, "template exec: "+err.Error(), http.StatusInternalServerError)
	}
}

// --- Intro ---

func (h *Handler) ProQAIntroPage(w http.ResponseWriter, r *http.Request) {
	data := h.proQAData(r, "Pro QA — Intro", "intro")
	h.renderProQA(w, "pro_qa_intro.html", data)
}

// --- Info ---

type proQAInfoView struct {
	proQAPageData
	Info     *models.ProQAInfo
	BodyHTML interface{}
}

func (h *Handler) ProQAInfoPage(w http.ResponseWriter, r *http.Request) {
	info, err := h.proQAService.GetInfo(r.Context())
	if err != nil {
		http.Error(w, "load info: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := proQAInfoView{
		proQAPageData: h.proQAData(r, "Pro QA — Info", "info"),
		Info:          info,
		BodyHTML:      h.proQAService.RenderMarkdown(info.BodyMD),
	}
	h.renderProQA(w, "pro_qa_info.html", v)
}

func (h *Handler) ProQAInfoSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	body := r.FormValue("body_md")
	claims := middleware.GetAuthClaims(r.Context())
	email := ""
	if claims != nil {
		email = claims.Email
	}
	if err := h.proQAService.UpdateInfo(r.Context(), body, email); err != nil {
		http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/info", http.StatusSeeOther)
}

// --- Requested checks ---

type proQAChecksView struct {
	proQAPageData
	Checks         []models.ProQARequestedCheck
	RenderedBodies map[uuid.UUID]interface{}
	Statuses       []string
}

func (h *Handler) ProQAChecksPage(w http.ResponseWriter, r *http.Request) {
	checks, err := h.proQAService.ListChecks(r.Context())
	if err != nil {
		http.Error(w, "load checks: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rendered := make(map[uuid.UUID]interface{}, len(checks))
	for _, c := range checks {
		rendered[c.ID] = h.proQAService.RenderMarkdown(c.BodyMD)
	}
	v := proQAChecksView{
		proQAPageData:  h.proQAData(r, "Pro QA — Requested Checks", "checks"),
		Checks:         checks,
		RenderedBodies: rendered,
		Statuses:       models.ProQACheckStatuses,
	}
	h.renderProQA(w, "pro_qa_checks.html", v)
}

func (h *Handler) ProQAChecksCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	email := ""
	if claims != nil {
		email = claims.Email
	}
	if _, err := h.proQAService.CreateCheck(r.Context(),
		strings.TrimSpace(r.FormValue("title")),
		r.FormValue("body_md"),
		email); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

func (h *Handler) ProQAChecksUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := &models.ProQARequestedCheck{
		ID:        id,
		Title:     strings.TrimSpace(r.FormValue("title")),
		BodyMD:    r.FormValue("body_md"),
		Status:    r.FormValue("status"),
		SortOrder: sortOrder,
	}
	if err := h.proQAService.UpdateCheck(r.Context(), c); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

func (h *Handler) ProQAChecksDelete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.proQAService.DeleteCheck(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/checks", http.StatusSeeOther)
}

// --- Issues ---

type proQAIssuesView struct {
	proQAPageData
	Issues       []models.ProQAIssue
	FilterStatus string
	Statuses     []string
	Severities   []string
	Envs         []string
	Platforms    []string
}

func (h *Handler) ProQAIssuesPage(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("status")
	issues, err := h.proQAService.ListIssues(r.Context(), filter)
	if err != nil {
		http.Error(w, "load: "+err.Error(), http.StatusInternalServerError)
		return
	}
	v := proQAIssuesView{
		proQAPageData: h.proQAData(r, "Pro QA — Issues", "issues"),
		Issues:        issues,
		FilterStatus:  filter,
		Statuses:      models.ProQAIssueStatuses,
		Severities:    models.ProQAIssueSeverity,
		Envs:          models.ProQAEnvironments,
		Platforms:     models.ProQAPlatforms,
	}
	h.renderProQA(w, "pro_qa_issues.html", v)
}

func (h *Handler) ProQAIssueCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	email := ""
	if claims != nil {
		email = claims.Email
	}
	issue := &models.ProQAIssue{
		Title:          strings.TrimSpace(r.FormValue("title")),
		DescriptionMD:  r.FormValue("description_md"),
		Environment:    r.FormValue("environment"),
		Platform:       r.FormValue("platform"),
		Severity:       r.FormValue("severity"),
		Status:         "open",
		CreatedByEmail: email,
	}
	if parent := r.FormValue("parent_issue_id"); parent != "" {
		if pid, perr := uuid.Parse(parent); perr == nil {
			issue.ParentIssueID = &pid
		}
	}
	if err := h.proQAService.CreateIssue(r.Context(), issue); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+issue.ID.String(), http.StatusSeeOther)
}

type proQAIssueDetailView struct {
	proQAPageData
	Issue            *models.ProQAIssue
	DescriptionHTML  interface{}
	Comments         []models.ProQAIssueComment
	RenderedComments map[uuid.UUID]interface{}
	Attachments      []models.ProQAAttachment
	Statuses         []string
	Severities       []string
	Envs             []string
	Platforms        []string
}

func (h *Handler) ProQAIssueDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	issue, err := h.proQAService.GetIssue(r.Context(), id)
	if err != nil {
		http.Error(w, "load: "+err.Error(), http.StatusInternalServerError)
		return
	}
	comments, err := h.proQAService.ListComments(r.Context(), id)
	if err != nil {
		http.Error(w, "comments: "+err.Error(), http.StatusInternalServerError)
		return
	}
	attachments, err := h.proQAService.ListAttachments(r.Context(), id)
	if err != nil {
		http.Error(w, "attachments: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rc := make(map[uuid.UUID]interface{}, len(comments))
	for _, c := range comments {
		rc[c.ID] = h.proQAService.RenderMarkdown(c.BodyMD)
	}
	v := proQAIssueDetailView{
		proQAPageData:    h.proQAData(r, "Pro QA — Issue #"+strconv.Itoa(issue.IssueNumber), "issues"),
		Issue:            issue,
		DescriptionHTML:  h.proQAService.RenderMarkdown(issue.DescriptionMD),
		Comments:         comments,
		RenderedComments: rc,
		Attachments:      attachments,
		Statuses:         models.ProQAIssueStatuses,
		Severities:       models.ProQAIssueSeverity,
		Envs:             models.ProQAEnvironments,
		Platforms:        models.ProQAPlatforms,
	}
	h.renderProQA(w, "pro_qa_issue_detail.html", v)
}

func (h *Handler) ProQAIssueUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	cur, err := h.proQAService.GetIssue(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	cur.Title = strings.TrimSpace(r.FormValue("title"))
	cur.DescriptionMD = r.FormValue("description_md")
	cur.Environment = r.FormValue("environment")
	cur.Platform = r.FormValue("platform")
	cur.Severity = r.FormValue("severity")
	if parent := r.FormValue("parent_issue_id"); parent != "" {
		if pid, perr := uuid.Parse(parent); perr == nil {
			cur.ParentIssueID = &pid
		}
	} else {
		cur.ParentIssueID = nil
	}
	if err := h.proQAService.UpdateIssue(r.Context(), cur); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String(), http.StatusSeeOther)
}

func (h *Handler) ProQAIssueChangeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	newStatus := r.FormValue("status")
	claims := middleware.GetAuthClaims(r.Context())
	email, name := "", ""
	if claims != nil {
		email = claims.Email
		name = claims.FirstName
	}
	if err := h.proQAService.ChangeStatus(r.Context(), id, newStatus, email, name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String(), http.StatusSeeOther)
}

func (h *Handler) ProQAIssueComment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	claims := middleware.GetAuthClaims(r.Context())
	email, name := "", ""
	if claims != nil {
		email = claims.Email
		name = claims.FirstName
	}
	if _, err := h.proQAService.AddComment(r.Context(), id, r.FormValue("body_md"), email, name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/pro-qa/issues/"+id.String()+"#comments", http.StatusSeeOther)
}

// ProQAUploadAttachment accepts multipart/form-data with field "file".
// Returns JSON: {id, filename, url}.
func (h *Handler) ProQAUploadAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := r.ParseMultipartForm(20 << 20); err != nil { // 20 MB cap
		http.Error(w, "bad upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	claims := middleware.GetAuthClaims(r.Context())
	email := ""
	if claims != nil {
		email = claims.Email
	}
	contentType := hdr.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	att, err := h.proQAService.UploadAttachment(r.Context(), id, nil,
		hdr.Filename, contentType, email, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id":       att.ID.String(),
		"filename": att.Filename,
		"url":      "/admin/pro-qa/attachments/" + att.ID.String(),
	})
}

func (h *Handler) ProQAFetchAttachment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	att, rc, err := h.proQAService.FetchAttachment(r.Context(), id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", att.ContentType)
	w.Header().Set("Content-Disposition", `inline; filename="`+att.Filename+`"`)
	_, _ = io.Copy(w, rc)
}
