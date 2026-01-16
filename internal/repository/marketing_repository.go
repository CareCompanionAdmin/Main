package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
)

// MarketingRepository handles marketing materials data operations
type MarketingRepository interface {
	// Brand Config
	GetBrandConfig(ctx context.Context) (*models.BrandConfig, error)
	UpdateBrandConfig(ctx context.Context, config *models.BrandConfig, updatedBy uuid.UUID) error

	// Marketing Assets
	ListMarketingAssets(ctx context.Context, assetType string) ([]models.MarketingAsset, error)
	GetMarketingAsset(ctx context.Context, id uuid.UUID) (*models.MarketingAsset, error)
	GetMarketingAssetByName(ctx context.Context, name string) (*models.MarketingAsset, error)
	CreateMarketingAsset(ctx context.Context, asset *models.MarketingAsset) error
	UpdateMarketingAsset(ctx context.Context, asset *models.MarketingAsset) error
	DeleteMarketingAsset(ctx context.Context, id uuid.UUID) error

	// Social Templates
	ListSocialTemplates(ctx context.Context, platform string) ([]models.SocialTemplate, error)
	GetSocialTemplate(ctx context.Context, id uuid.UUID) (*models.SocialTemplate, error)

	// Statistics for dynamic content
	GetMarketingStats(ctx context.Context) (*models.MarketingStats, error)
}

// MarketingRepo implements MarketingRepository
type MarketingRepo struct {
	db *sql.DB
}

// NewMarketingRepo creates a new marketing repository
func NewMarketingRepo(db *sql.DB) *MarketingRepo {
	return &MarketingRepo{db: db}
}

// GetBrandConfig retrieves the brand configuration (there's only one row)
func (r *MarketingRepo) GetBrandConfig(ctx context.Context) (*models.BrandConfig, error) {
	query := `
		SELECT id, app_name, tagline, mission_statement,
			primary_color, primary_light, primary_dark,
			secondary_color, secondary_dark,
			accent_color, accent_dark,
			heading_font, body_font,
			brand_voice, writing_guidelines,
			website_url, support_email, contact_phone,
			facebook_url, twitter_url, instagram_url, linkedin_url,
			copyright_text, disclaimer_text,
			updated_at, updated_by
		FROM brand_config
		LIMIT 1
	`

	var config models.BrandConfig
	var missionStatement, brandVoice, writingGuidelines sql.NullString
	var contactPhone, facebookURL, twitterURL, instagramURL, linkedInURL sql.NullString
	var copyrightText, disclaimerText sql.NullString
	var updatedBy sql.NullString

	err := r.db.QueryRowContext(ctx, query).Scan(
		&config.ID, &config.AppName, &config.Tagline, &missionStatement,
		&config.PrimaryColor, &config.PrimaryLight, &config.PrimaryDark,
		&config.SecondaryColor, &config.SecondaryDark,
		&config.AccentColor, &config.AccentDark,
		&config.HeadingFont, &config.BodyFont,
		&brandVoice, &writingGuidelines,
		&config.WebsiteURL, &config.SupportEmail, &contactPhone,
		&facebookURL, &twitterURL, &instagramURL, &linkedInURL,
		&copyrightText, &disclaimerText,
		&config.UpdatedAt, &updatedBy,
	)
	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	config.MissionStatement = missionStatement.String
	config.BrandVoice = brandVoice.String
	config.WritingGuidelines = writingGuidelines.String
	config.ContactPhone = contactPhone.String
	config.FacebookURL = facebookURL.String
	config.TwitterURL = twitterURL.String
	config.InstagramURL = instagramURL.String
	config.LinkedInURL = linkedInURL.String
	config.CopyrightText = copyrightText.String
	config.DisclaimerText = disclaimerText.String

	if updatedBy.Valid {
		id, _ := uuid.Parse(updatedBy.String)
		config.UpdatedBy = &id
	}

	return &config, nil
}

// UpdateBrandConfig updates the brand configuration
func (r *MarketingRepo) UpdateBrandConfig(ctx context.Context, config *models.BrandConfig, updatedBy uuid.UUID) error {
	query := `
		UPDATE brand_config SET
			app_name = $1,
			tagline = $2,
			mission_statement = $3,
			primary_color = $4,
			primary_light = $5,
			primary_dark = $6,
			secondary_color = $7,
			secondary_dark = $8,
			accent_color = $9,
			accent_dark = $10,
			heading_font = $11,
			body_font = $12,
			brand_voice = $13,
			writing_guidelines = $14,
			website_url = $15,
			support_email = $16,
			contact_phone = $17,
			facebook_url = $18,
			twitter_url = $19,
			instagram_url = $20,
			linkedin_url = $21,
			copyright_text = $22,
			disclaimer_text = $23,
			updated_at = NOW(),
			updated_by = $24
		WHERE id = $25
	`

	_, err := r.db.ExecContext(ctx, query,
		config.AppName, config.Tagline, nullIfEmpty(config.MissionStatement),
		config.PrimaryColor, config.PrimaryLight, config.PrimaryDark,
		config.SecondaryColor, config.SecondaryDark,
		config.AccentColor, config.AccentDark,
		config.HeadingFont, config.BodyFont,
		nullIfEmpty(config.BrandVoice), nullIfEmpty(config.WritingGuidelines),
		config.WebsiteURL, config.SupportEmail, nullIfEmpty(config.ContactPhone),
		nullIfEmpty(config.FacebookURL), nullIfEmpty(config.TwitterURL),
		nullIfEmpty(config.InstagramURL), nullIfEmpty(config.LinkedInURL),
		nullIfEmpty(config.CopyrightText), nullIfEmpty(config.DisclaimerText),
		updatedBy, config.ID,
	)

	return err
}

// ListMarketingAssets lists marketing assets, optionally filtered by type
func (r *MarketingRepo) ListMarketingAssets(ctx context.Context, assetType string) ([]models.MarketingAsset, error) {
	var query string
	var args []interface{}

	if assetType != "" {
		query = `
			SELECT id, name, description, asset_type, format,
				width_px, height_px, file_path, file_size_bytes,
				is_auto_generated, generation_template, last_generated_at,
				is_active, created_at, updated_at
			FROM marketing_assets
			WHERE is_active = TRUE AND asset_type = $1
			ORDER BY name
		`
		args = []interface{}{assetType}
	} else {
		query = `
			SELECT id, name, description, asset_type, format,
				width_px, height_px, file_path, file_size_bytes,
				is_auto_generated, generation_template, last_generated_at,
				is_active, created_at, updated_at
			FROM marketing_assets
			WHERE is_active = TRUE
			ORDER BY asset_type, name
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assets []models.MarketingAsset
	for rows.Next() {
		var asset models.MarketingAsset
		var description, filePath, generationTemplate sql.NullString
		var widthPx, heightPx sql.NullInt64
		var fileSizeBytes sql.NullInt64
		var lastGeneratedAt sql.NullTime

		err := rows.Scan(
			&asset.ID, &asset.Name, &description, &asset.AssetType, &asset.Format,
			&widthPx, &heightPx, &filePath, &fileSizeBytes,
			&asset.IsAutoGenerated, &generationTemplate, &lastGeneratedAt,
			&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		asset.Description = description.String
		asset.FilePath = filePath.String
		asset.GenerationTemplate = generationTemplate.String
		asset.WidthPx = int(widthPx.Int64)
		asset.HeightPx = int(heightPx.Int64)
		asset.FileSizeBytes = fileSizeBytes.Int64

		if lastGeneratedAt.Valid {
			asset.LastGeneratedAt = &lastGeneratedAt.Time
		}

		assets = append(assets, asset)
	}

	return assets, rows.Err()
}

// GetMarketingAsset retrieves a single marketing asset by ID
func (r *MarketingRepo) GetMarketingAsset(ctx context.Context, id uuid.UUID) (*models.MarketingAsset, error) {
	query := `
		SELECT id, name, description, asset_type, format,
			width_px, height_px, file_path, file_size_bytes,
			is_auto_generated, generation_template, last_generated_at,
			is_active, created_at, updated_at
		FROM marketing_assets
		WHERE id = $1
	`

	var asset models.MarketingAsset
	var description, filePath, generationTemplate sql.NullString
	var widthPx, heightPx sql.NullInt64
	var fileSizeBytes sql.NullInt64
	var lastGeneratedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&asset.ID, &asset.Name, &description, &asset.AssetType, &asset.Format,
		&widthPx, &heightPx, &filePath, &fileSizeBytes,
		&asset.IsAutoGenerated, &generationTemplate, &lastGeneratedAt,
		&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	asset.Description = description.String
	asset.FilePath = filePath.String
	asset.GenerationTemplate = generationTemplate.String
	asset.WidthPx = int(widthPx.Int64)
	asset.HeightPx = int(heightPx.Int64)
	asset.FileSizeBytes = fileSizeBytes.Int64

	if lastGeneratedAt.Valid {
		asset.LastGeneratedAt = &lastGeneratedAt.Time
	}

	return &asset, nil
}

// GetMarketingAssetByName retrieves a marketing asset by name
func (r *MarketingRepo) GetMarketingAssetByName(ctx context.Context, name string) (*models.MarketingAsset, error) {
	query := `
		SELECT id, name, description, asset_type, format,
			width_px, height_px, file_path, file_size_bytes,
			is_auto_generated, generation_template, last_generated_at,
			is_active, created_at, updated_at
		FROM marketing_assets
		WHERE name = $1 AND is_active = TRUE
	`

	var asset models.MarketingAsset
	var description, filePath, generationTemplate sql.NullString
	var widthPx, heightPx sql.NullInt64
	var fileSizeBytes sql.NullInt64
	var lastGeneratedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&asset.ID, &asset.Name, &description, &asset.AssetType, &asset.Format,
		&widthPx, &heightPx, &filePath, &fileSizeBytes,
		&asset.IsAutoGenerated, &generationTemplate, &lastGeneratedAt,
		&asset.IsActive, &asset.CreatedAt, &asset.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	asset.Description = description.String
	asset.FilePath = filePath.String
	asset.GenerationTemplate = generationTemplate.String
	asset.WidthPx = int(widthPx.Int64)
	asset.HeightPx = int(heightPx.Int64)
	asset.FileSizeBytes = fileSizeBytes.Int64

	if lastGeneratedAt.Valid {
		asset.LastGeneratedAt = &lastGeneratedAt.Time
	}

	return &asset, nil
}

// CreateMarketingAsset creates a new marketing asset
func (r *MarketingRepo) CreateMarketingAsset(ctx context.Context, asset *models.MarketingAsset) error {
	if asset.ID == uuid.Nil {
		asset.ID = uuid.New()
	}

	query := `
		INSERT INTO marketing_assets (
			id, name, description, asset_type, format,
			width_px, height_px, file_path, file_size_bytes,
			is_auto_generated, generation_template, last_generated_at,
			is_active, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW()
		)
	`

	var lastGenerated *time.Time
	if asset.LastGeneratedAt != nil {
		lastGenerated = asset.LastGeneratedAt
	}

	_, err := r.db.ExecContext(ctx, query,
		asset.ID, asset.Name, nullIfEmpty(asset.Description), asset.AssetType, asset.Format,
		nullIfZero(asset.WidthPx), nullIfZero(asset.HeightPx),
		nullIfEmpty(asset.FilePath), nullIfZero64(asset.FileSizeBytes),
		asset.IsAutoGenerated, nullIfEmpty(asset.GenerationTemplate), lastGenerated,
		asset.IsActive,
	)

	return err
}

// UpdateMarketingAsset updates an existing marketing asset
func (r *MarketingRepo) UpdateMarketingAsset(ctx context.Context, asset *models.MarketingAsset) error {
	query := `
		UPDATE marketing_assets SET
			name = $1,
			description = $2,
			asset_type = $3,
			format = $4,
			width_px = $5,
			height_px = $6,
			file_path = $7,
			file_size_bytes = $8,
			is_auto_generated = $9,
			generation_template = $10,
			last_generated_at = $11,
			is_active = $12,
			updated_at = NOW()
		WHERE id = $13
	`

	_, err := r.db.ExecContext(ctx, query,
		asset.Name, nullIfEmpty(asset.Description), asset.AssetType, asset.Format,
		nullIfZero(asset.WidthPx), nullIfZero(asset.HeightPx),
		nullIfEmpty(asset.FilePath), nullIfZero64(asset.FileSizeBytes),
		asset.IsAutoGenerated, nullIfEmpty(asset.GenerationTemplate),
		asset.LastGeneratedAt, asset.IsActive,
		asset.ID,
	)

	return err
}

// DeleteMarketingAsset soft-deletes a marketing asset
func (r *MarketingRepo) DeleteMarketingAsset(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE marketing_assets SET is_active = FALSE, updated_at = NOW() WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

// ListSocialTemplates lists social media templates, optionally filtered by platform
func (r *MarketingRepo) ListSocialTemplates(ctx context.Context, platform string) ([]models.SocialTemplate, error) {
	var query string
	var args []interface{}

	if platform != "" {
		query = `
			SELECT id, name, platform, template_type, width_px, height_px,
				headline_max_chars, body_max_chars, is_active, created_at
			FROM social_templates
			WHERE is_active = TRUE AND platform = $1
			ORDER BY name
		`
		args = []interface{}{platform}
	} else {
		query = `
			SELECT id, name, platform, template_type, width_px, height_px,
				headline_max_chars, body_max_chars, is_active, created_at
			FROM social_templates
			WHERE is_active = TRUE
			ORDER BY platform, name
		`
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []models.SocialTemplate
	for rows.Next() {
		var t models.SocialTemplate
		var headlineMax, bodyMax sql.NullInt64

		err := rows.Scan(
			&t.ID, &t.Name, &t.Platform, &t.TemplateType,
			&t.WidthPx, &t.HeightPx, &headlineMax, &bodyMax,
			&t.IsActive, &t.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		t.HeadlineMaxChars = int(headlineMax.Int64)
		t.BodyMaxChars = int(bodyMax.Int64)

		templates = append(templates, t)
	}

	return templates, rows.Err()
}

// GetSocialTemplate retrieves a single social template by ID
func (r *MarketingRepo) GetSocialTemplate(ctx context.Context, id uuid.UUID) (*models.SocialTemplate, error) {
	query := `
		SELECT id, name, platform, template_type, width_px, height_px,
			headline_max_chars, body_max_chars, is_active, created_at
		FROM social_templates
		WHERE id = $1
	`

	var t models.SocialTemplate
	var headlineMax, bodyMax sql.NullInt64

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&t.ID, &t.Name, &t.Platform, &t.TemplateType,
		&t.WidthPx, &t.HeightPx, &headlineMax, &bodyMax,
		&t.IsActive, &t.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	t.HeadlineMaxChars = int(headlineMax.Int64)
	t.BodyMaxChars = int(bodyMax.Int64)

	return &t, nil
}

// GetMarketingStats retrieves statistics for dynamic marketing content
func (r *MarketingRepo) GetMarketingStats(ctx context.Context) (*models.MarketingStats, error) {
	stats := &models.MarketingStats{}

	// Get total families
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM families").Scan(&stats.TotalFamilies)
	if err != nil {
		return nil, err
	}

	// Get total entries (behavior logs as proxy)
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM behavior_logs").Scan(&stats.TotalEntries)
	if err != nil {
		return nil, err
	}

	// Get average entries per day (last 30 days)
	err = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(daily_count), 0)
		FROM (
			SELECT DATE(logged_at) as log_date, COUNT(*) as daily_count
			FROM behavior_logs
			WHERE logged_at > NOW() - INTERVAL '30 days'
			GROUP BY DATE(logged_at)
		) daily_counts
	`).Scan(&stats.AverageEntriesPerDay)
	if err != nil {
		return nil, err
	}

	// Get insights generated
	err = r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM insights").Scan(&stats.InsightsGenerated)
	if err != nil {
		return nil, err
	}

	return stats, nil
}

// Helper functions
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

func nullIfZero64(i int64) interface{} {
	if i == 0 {
		return nil
	}
	return i
}
