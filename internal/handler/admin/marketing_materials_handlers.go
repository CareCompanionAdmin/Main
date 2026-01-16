package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"carecompanion/internal/middleware"
	"carecompanion/internal/models"
)

// ============================================================================
// MARKETING MATERIALS API HANDLERS
// ============================================================================

// GetBrandConfig returns the current brand configuration
func (h *Handler) GetBrandConfig(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	config, err := h.marketingService.GetBrandConfig(r.Context())
	if err != nil {
		http.Error(w, "Failed to get brand config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, config)
}

// UpdateBrandConfigRequest is the request body for updating brand config
type UpdateBrandConfigRequest struct {
	AppName           string `json:"appName"`
	Tagline           string `json:"tagline"`
	MissionStatement  string `json:"missionStatement"`
	PrimaryColor      string `json:"primaryColor"`
	PrimaryLight      string `json:"primaryLight"`
	PrimaryDark       string `json:"primaryDark"`
	SecondaryColor    string `json:"secondaryColor"`
	SecondaryDark     string `json:"secondaryDark"`
	AccentColor       string `json:"accentColor"`
	AccentDark        string `json:"accentDark"`
	HeadingFont       string `json:"headingFont"`
	BodyFont          string `json:"bodyFont"`
	BrandVoice        string `json:"brandVoice"`
	WritingGuidelines string `json:"writingGuidelines"`
	WebsiteURL        string `json:"websiteUrl"`
	SupportEmail      string `json:"supportEmail"`
	ContactPhone      string `json:"contactPhone"`
	FacebookURL       string `json:"facebookUrl"`
	TwitterURL        string `json:"twitterUrl"`
	InstagramURL      string `json:"instagramUrl"`
	LinkedInURL       string `json:"linkedinUrl"`
	CopyrightText     string `json:"copyrightText"`
	DisclaimerText    string `json:"disclaimerText"`
}

// UpdateBrandConfig updates the brand configuration (super_admin only)
func (h *Handler) UpdateBrandConfig(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req UpdateBrandConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current config to preserve ID
	current, err := h.marketingService.GetBrandConfig(r.Context())
	if err != nil {
		http.Error(w, "Failed to get current config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update fields
	config := &models.BrandConfig{
		ID:                current.ID,
		AppName:           req.AppName,
		Tagline:           req.Tagline,
		MissionStatement:  req.MissionStatement,
		PrimaryColor:      req.PrimaryColor,
		PrimaryLight:      req.PrimaryLight,
		PrimaryDark:       req.PrimaryDark,
		SecondaryColor:    req.SecondaryColor,
		SecondaryDark:     req.SecondaryDark,
		AccentColor:       req.AccentColor,
		AccentDark:        req.AccentDark,
		HeadingFont:       req.HeadingFont,
		BodyFont:          req.BodyFont,
		BrandVoice:        req.BrandVoice,
		WritingGuidelines: req.WritingGuidelines,
		WebsiteURL:        req.WebsiteURL,
		SupportEmail:      req.SupportEmail,
		ContactPhone:      req.ContactPhone,
		FacebookURL:       req.FacebookURL,
		TwitterURL:        req.TwitterURL,
		InstagramURL:      req.InstagramURL,
		LinkedInURL:       req.LinkedInURL,
		CopyrightText:     req.CopyrightText,
		DisclaimerText:    req.DisclaimerText,
	}

	userID := middleware.GetUserID(r.Context())
	if err := h.marketingService.UpdateBrandConfig(r.Context(), config, userID); err != nil {
		http.Error(w, "Failed to update brand config: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "update_brand_config", "brand_config", current.ID, nil)
	respondJSON(w, config)
}

// ListMarketingAssets returns marketing assets, optionally filtered by type
func (h *Handler) ListMarketingAssets(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	data, err := h.marketingService.GetMarketingMaterialsData(r.Context())
	if err != nil {
		http.Error(w, "Failed to get marketing materials: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, data)
}

// DownloadAsset downloads a specific marketing asset
func (h *Handler) DownloadAsset(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid asset ID", http.StatusBadRequest)
		return
	}

	file, filename, err := h.marketingService.GetAssetFile(r.Context(), id)
	if err != nil {
		http.Error(w, "Failed to get asset: "+err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Type", "application/octet-stream")

	if _, err := io.Copy(w, file); err != nil {
		http.Error(w, "Failed to send file", http.StatusInternalServerError)
		return
	}
}

// ListSocialTemplates returns available social media templates
func (h *Handler) ListSocialTemplates(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	data, err := h.marketingService.GetMarketingMaterialsData(r.Context())
	if err != nil {
		http.Error(w, "Failed to get templates: "+err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, data.SocialTemplates)
}

// GenerateSocialGraphicRequest is the request body for generating a social graphic
type GenerateSocialGraphicRequest struct {
	TemplateID uuid.UUID `json:"templateId"`
	Headline   string    `json:"headline"`
	Body       string    `json:"body"`
}

// GenerateSocialGraphic generates a custom social media graphic
func (h *Handler) GenerateSocialGraphic(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	var req GenerateSocialGraphicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.TemplateID == uuid.Nil {
		http.Error(w, "Template ID is required", http.StatusBadRequest)
		return
	}

	if req.Headline == "" {
		http.Error(w, "Headline is required", http.StatusBadRequest)
		return
	}

	// Get the template
	data, err := h.marketingService.GetMarketingMaterialsData(r.Context())
	if err != nil {
		http.Error(w, "Failed to get templates: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var template *models.SocialTemplate
	for _, t := range data.SocialTemplates {
		if t.ID == req.TemplateID {
			template = &t
			break
		}
	}

	if template == nil {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	// Generate the graphic
	imageData, err := h.marketingService.GenerateSocialGraphic(r.Context(), *template, req.Headline, req.Body)
	if err != nil {
		http.Error(w, "Failed to generate graphic: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", "attachment; filename=\"social_graphic.png\"")
	w.Write(imageData)
}

// RegenerateAsset regenerates a specific marketing asset (super_admin only)
func (h *Handler) RegenerateAsset(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	assetType := chi.URLParam(r, "type")
	if assetType == "" {
		http.Error(w, "Asset type is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	var err error

	switch assetType {
	case "single-page-brochure":
		content, genErr := h.marketingService.GenerateSinglePageBrochure(ctx)
		if genErr != nil {
			err = genErr
		} else {
			_, err = h.marketingService.SaveAsset(ctx, "Single Page Brochure", models.AssetTypeBrochure, models.FormatPDF, content, 612, 792)
		}
	case "tri-fold-brochure":
		content, genErr := h.marketingService.GenerateTriFoldBrochure(ctx)
		if genErr != nil {
			err = genErr
		} else {
			_, err = h.marketingService.SaveAsset(ctx, "Tri-Fold Brochure", models.AssetTypeBrochure, models.FormatPDF, content, 792, 612)
		}
	case "style-guide":
		content, genErr := h.marketingService.GenerateStyleGuidePDF(ctx)
		if genErr != nil {
			err = genErr
		} else {
			_, err = h.marketingService.SaveAsset(ctx, "Brand Style Guide", models.AssetTypeStyleGuide, models.FormatPDF, content, 612, 792)
		}
	default:
		http.Error(w, "Unknown asset type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to regenerate asset: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "regenerate_asset", "marketing_asset", uuid.Nil, map[string]interface{}{"type": assetType})
	respondJSON(w, map[string]string{"status": "success", "message": "Asset regenerated successfully"})
}

// RegenerateAllAssets regenerates all marketing assets (super_admin only)
func (h *Handler) RegenerateAllAssets(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	if err := h.marketingService.RegenerateAllAssets(r.Context()); err != nil {
		http.Error(w, "Failed to regenerate assets: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.logAction(r, "regenerate_all_assets", "marketing_assets", uuid.Nil, nil)
	respondJSON(w, map[string]string{"status": "success", "message": "All assets regenerated successfully"})
}

// GenerateBrochure generates a brochure PDF and returns it
func (h *Handler) GenerateBrochure(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	brochureType := r.URL.Query().Get("type")
	if brochureType == "" {
		brochureType = "single"
	}

	var content []byte
	var err error
	var filename string

	switch brochureType {
	case "single":
		content, err = h.marketingService.GenerateSinglePageBrochure(r.Context())
		filename = "carecompanion_brochure_single.pdf"
	case "trifold":
		content, err = h.marketingService.GenerateTriFoldBrochure(r.Context())
		filename = "carecompanion_brochure_trifold.pdf"
	default:
		http.Error(w, "Invalid brochure type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to generate brochure: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Write(content)
}

// GenerateStyleGuide generates the style guide PDF and returns it
func (h *Handler) GenerateStyleGuide(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	content, err := h.marketingService.GenerateStyleGuidePDF(r.Context())
	if err != nil {
		http.Error(w, "Failed to generate style guide: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=\"carecompanion_style_guide.pdf\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Write(content)
}

// GenerateLogo generates a logo in the specified format and variant
func (h *Handler) GenerateLogo(w http.ResponseWriter, r *http.Request) {
	if h.marketingService == nil {
		http.Error(w, "Marketing service not initialized", http.StatusServiceUnavailable)
		return
	}

	variant := r.URL.Query().Get("variant")
	if variant == "" {
		variant = "primary"
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "png"
	}

	sizeStr := r.URL.Query().Get("size")
	size := 256
	if sizeStr != "" {
		if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 && s <= 1024 {
			size = s
		}
	}

	var content []byte
	var contentType string
	var filename string
	var err error

	switch format {
	case "png":
		content, err = h.marketingService.GenerateLogoPNG(r.Context(), variant, size)
		contentType = "image/png"
		filename = "carecompanion_logo_" + variant + "_" + strconv.Itoa(size) + ".png"
	case "svg":
		content, err = h.marketingService.GenerateLogoSVG(r.Context(), variant, size)
		contentType = "image/svg+xml"
		filename = "carecompanion_logo_" + variant + ".svg"
	default:
		http.Error(w, "Invalid format. Use 'png' or 'svg'", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Failed to generate logo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Write(content)
}
