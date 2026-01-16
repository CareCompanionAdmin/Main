package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fogleman/gg"
	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// MarketingService handles marketing material generation
type MarketingService struct {
	repo      repository.MarketingRepository
	assetsDir string
}

// NewMarketingService creates a new marketing service
func NewMarketingService(repo repository.MarketingRepository, assetsDir string) *MarketingService {
	return &MarketingService{
		repo:      repo,
		assetsDir: assetsDir,
	}
}

// GetBrandConfig retrieves the current brand configuration
func (s *MarketingService) GetBrandConfig(ctx context.Context) (*models.BrandConfig, error) {
	return s.repo.GetBrandConfig(ctx)
}

// UpdateBrandConfig updates the brand configuration
func (s *MarketingService) UpdateBrandConfig(ctx context.Context, config *models.BrandConfig, updatedBy uuid.UUID) error {
	return s.repo.UpdateBrandConfig(ctx, config, updatedBy)
}

// GetMarketingMaterialsData retrieves all data for the marketing materials page
func (s *MarketingService) GetMarketingMaterialsData(ctx context.Context) (*models.MarketingMaterialsData, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get brand config: %w", err)
	}

	logos, err := s.repo.ListMarketingAssets(ctx, models.AssetTypeLogo)
	if err != nil {
		return nil, fmt.Errorf("failed to list logos: %w", err)
	}

	brochures, err := s.repo.ListMarketingAssets(ctx, models.AssetTypeBrochure)
	if err != nil {
		return nil, fmt.Errorf("failed to list brochures: %w", err)
	}

	socialGraphics, err := s.repo.ListMarketingAssets(ctx, models.AssetTypeSocialGraphic)
	if err != nil {
		return nil, fmt.Errorf("failed to list social graphics: %w", err)
	}

	templates, err := s.repo.ListSocialTemplates(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list social templates: %w", err)
	}

	stats, err := s.repo.GetMarketingStats(ctx)
	if err != nil {
		// Non-fatal, use defaults
		stats = &models.MarketingStats{}
	}

	return &models.MarketingMaterialsData{
		BrandConfig:     config,
		Logos:           logos,
		Brochures:       brochures,
		SocialGraphics:  socialGraphics,
		SocialTemplates: templates,
		Statistics:      stats,
		Features:        models.GetDefaultFeatures(),
		ValueProps:      models.GetDefaultValueProps(),
	}, nil
}

// hexToRGB converts a hex color string to RGB values
func hexToRGB(hex string) (uint8, uint8, uint8) {
	hex = strings.TrimPrefix(hex, "#")
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}

// hexToColor converts a hex string to color.RGBA
func hexToColor(hex string) color.RGBA {
	r, g, b := hexToRGB(hex)
	return color.RGBA{r, g, b, 255}
}

// GenerateSinglePageBrochure creates a single-page PDF brochure
func (s *MarketingService) GenerateSinglePageBrochure(ctx context.Context) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	stats, _ := s.repo.GetMarketingStats(ctx)
	if stats == nil {
		stats = &models.MarketingStats{}
	}

	// Create PDF (Letter size: 8.5" x 11")
	pdf := fpdf.New("P", "in", "Letter", "")
	pdf.SetMargins(0.5, 0.5, 0.5)
	pdf.AddPage()

	// Get brand colors
	pr, pg, pb := hexToRGB(config.PrimaryColor)
	sr, sg, sb := hexToRGB(config.SecondaryColor)
	ar, ag, ab := hexToRGB(config.AccentColor)

	// Header gradient bar
	pdf.SetFillColor(int(pr), int(pg), int(pb))
	pdf.Rect(0, 0, 8.5, 1.5, "F")

	// App name and tagline in header
	pdf.SetFont("Helvetica", "B", 32)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(0.5, 0.4)
	pdf.Cell(5, 0.5, config.AppName)

	pdf.SetFont("Helvetica", "", 14)
	pdf.SetXY(0.5, 0.9)
	pdf.Cell(5, 0.3, config.Tagline)

	// Main content area
	pdf.SetTextColor(31, 41, 55) // Dark gray

	// The Challenge section
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetXY(0.5, 1.8)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.Cell(4, 0.4, "The Challenge")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.5, 2.3)

	autismStats := models.GetAutismStatistics()
	challengeText := fmt.Sprintf(
		"%s children are diagnosed with autism. %s of mothers experience depression, "+
			"and %s of parents report ongoing anxiety. Annual out-of-pocket costs exceed %s per family.",
		autismStats["prevalence"],
		autismStats["mother_depression"],
		autismStats["parent_anxiety"],
		autismStats["annual_cost"],
	)
	pdf.MultiCell(7.5, 0.25, challengeText, "", "", false)

	// Our Solution section
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetXY(0.5, 3.3)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.Cell(4, 0.4, "Our Solution")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.5, 3.8)
	pdf.MultiCell(7.5, 0.25, config.MissionStatement, "", "", false)

	// Features section (3 columns)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetXY(0.5, 5.0)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.Cell(4, 0.4, "Key Features")

	features := models.GetDefaultFeatures()
	colWidth := 2.3
	startY := 5.5
	for i, feature := range features {
		if i >= 6 {
			break
		}
		col := i % 3
		row := i / 3
		x := 0.5 + float64(col)*colWidth
		y := startY + float64(row)*1.2

		// Feature box
		pdf.SetFillColor(int(sr), int(sg), int(sb))
		pdf.Rect(x, y, 0.3, 0.3, "F")

		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(31, 41, 55)
		pdf.SetXY(x+0.4, y)
		pdf.Cell(1.8, 0.25, feature.Title)

		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(107, 114, 128)
		pdf.SetXY(x+0.4, y+0.25)
		// Truncate description if too long
		desc := feature.Description
		if len(desc) > 80 {
			desc = desc[:77] + "..."
		}
		pdf.MultiCell(1.8, 0.15, desc, "", "", false)
	}

	// Call to Action section
	pdf.SetFillColor(int(ar), int(ag), int(ab))
	pdf.Rect(0.5, 8.5, 7.5, 1.2, "F")

	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(0.7, 8.7)
	pdf.Cell(3, 0.4, "Start Your Journey Today")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(0.7, 9.2)
	pdf.Cell(4, 0.3, fmt.Sprintf("Visit %s to learn more", config.WebsiteURL))

	// Footer
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(107, 114, 128)
	pdf.SetXY(0.5, 10.3)
	pdf.Cell(7.5, 0.3, fmt.Sprintf("%s | %s", config.CopyrightText, config.SupportEmail))

	// Output to bytes
	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GenerateTriFoldBrochure creates a tri-fold PDF brochure
func (s *MarketingService) GenerateTriFoldBrochure(ctx context.Context) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Create PDF (Letter size landscape for tri-fold: 11" x 8.5")
	pdf := fpdf.New("L", "in", "Letter", "")
	pdf.SetMargins(0.25, 0.25, 0.25)

	// Get brand colors
	pr, pg, pb := hexToRGB(config.PrimaryColor)
	sr, sg, sb := hexToRGB(config.SecondaryColor)
	ar, ag, ab := hexToRGB(config.AccentColor)

	panelWidth := 3.5 // Each panel is ~3.5" wide

	// === OUTSIDE (Page 1) ===
	// Panel 1 (Back) | Panel 2 (Front Cover) | Panel 3 (Inside Flap)
	pdf.AddPage()

	// Panel 1: Back panel (Contact info, QR code placeholder)
	pdf.SetFillColor(245, 245, 245)
	pdf.Rect(0, 0, panelWidth, 8.5, "F")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.25, 2)
	pdf.Cell(3, 0.4, "Contact Us")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.25, 2.6)
	pdf.Cell(3, 0.3, config.WebsiteURL)
	pdf.SetXY(0.25, 3.0)
	pdf.Cell(3, 0.3, config.SupportEmail)
	if config.ContactPhone != "" {
		pdf.SetXY(0.25, 3.4)
		pdf.Cell(3, 0.3, config.ContactPhone)
	}

	// Social links
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(0.25, 5)
	pdf.Cell(3, 0.3, "Follow Us")
	pdf.SetFont("Helvetica", "", 9)
	socialY := 5.5
	if config.FacebookURL != "" {
		pdf.SetXY(0.25, socialY)
		pdf.Cell(3, 0.25, "Facebook")
		socialY += 0.3
	}
	if config.TwitterURL != "" {
		pdf.SetXY(0.25, socialY)
		pdf.Cell(3, 0.25, "Twitter/X")
		socialY += 0.3
	}
	if config.InstagramURL != "" {
		pdf.SetXY(0.25, socialY)
		pdf.Cell(3, 0.25, "Instagram")
		socialY += 0.3
	}
	if config.LinkedInURL != "" {
		pdf.SetXY(0.25, socialY)
		pdf.Cell(3, 0.25, "LinkedIn")
	}

	// Copyright at bottom
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(107, 114, 128)
	pdf.SetXY(0.25, 7.8)
	pdf.Cell(3, 0.3, config.CopyrightText)

	// Panel 2: Front cover
	pdf.SetFillColor(int(pr), int(pg), int(pb))
	pdf.Rect(panelWidth, 0, panelWidth, 8.5, "F")

	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(panelWidth+0.25, 3)
	pdf.Cell(3, 0.6, config.AppName)

	pdf.SetFont("Helvetica", "", 14)
	pdf.SetXY(panelWidth+0.25, 4)
	pdf.MultiCell(3, 0.35, config.Tagline, "", "", false)

	// Panel 3: Inside flap (Statistics)
	pdf.SetFillColor(255, 255, 255)
	pdf.Rect(panelWidth*2, 0, panelWidth, 8.5, "F")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(panelWidth*2+0.25, 1)
	pdf.Cell(3, 0.4, "Did You Know?")

	autismStats := models.GetAutismStatistics()
	statsY := 1.8

	// Stat boxes
	statItems := []struct {
		value string
		desc  string
	}{
		{autismStats["prevalence"], autismStats["prevalence_desc"]},
		{autismStats["mother_depression"], autismStats["mother_dep_desc"]},
		{autismStats["parent_anxiety"], autismStats["parent_anx_desc"]},
		{autismStats["annual_cost"], autismStats["annual_cost_desc"]},
	}

	for _, stat := range statItems {
		pdf.SetFillColor(int(sr), int(sg), int(sb))
		pdf.Rect(panelWidth*2+0.25, statsY, 3, 1, "F")

		pdf.SetFont("Helvetica", "B", 20)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetXY(panelWidth*2+0.35, statsY+0.2)
		pdf.Cell(2.8, 0.4, stat.value)

		pdf.SetFont("Helvetica", "", 9)
		pdf.SetXY(panelWidth*2+0.35, statsY+0.6)
		pdf.Cell(2.8, 0.3, stat.desc)

		statsY += 1.3
	}

	// === INSIDE (Page 2) ===
	// Panel 4 (The Challenge) | Panel 5 (Our Solution) | Panel 6 (How It Works)
	pdf.AddPage()

	// Panel 4: The Challenge
	pdf.SetFillColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.25, 0.5)
	pdf.Cell(3, 0.4, "The Challenge")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.25, 1.1)

	challenges := []string{
		"Overwhelming daily chaos with medications, behaviors, and appointments",
		"Information scattered across multiple apps and notebooks",
		"Difficulty identifying patterns in complex health data",
		"Isolation and burnout from constant caregiving",
		"Communication gaps between care team members",
	}

	challengeY := 1.1
	for _, challenge := range challenges {
		pdf.SetFillColor(int(ar), int(ag), int(ab))
		pdf.Rect(0.25, challengeY, 0.15, 0.15, "F")
		pdf.SetXY(0.5, challengeY-0.05)
		pdf.MultiCell(2.8, 0.25, challenge, "", "", false)
		challengeY += 0.8
	}

	// Panel 5: Our Solution
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(panelWidth+0.25, 0.5)
	pdf.Cell(3, 0.4, "Our Solution")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(panelWidth+0.25, 1.1)
	pdf.MultiCell(3, 0.25, config.MissionStatement, "", "", false)

	features := models.GetDefaultFeatures()
	featureY := 3.0
	for i, feature := range features {
		if i >= 4 {
			break
		}
		pdf.SetFillColor(int(sr), int(sg), int(sb))
		pdf.Rect(panelWidth+0.25, featureY, 0.2, 0.2, "F")

		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(31, 41, 55)
		pdf.SetXY(panelWidth+0.55, featureY)
		pdf.Cell(2.5, 0.25, feature.Title)

		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(107, 114, 128)
		pdf.SetXY(panelWidth+0.55, featureY+0.25)
		desc := feature.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		pdf.MultiCell(2.5, 0.2, desc, "", "", false)
		featureY += 1.0
	}

	// Panel 6: How It Works + CTA
	pdf.SetFillColor(int(pr), int(pg), int(pb))
	pdf.Rect(panelWidth*2, 0, panelWidth+0.5, 8.5, "F")

	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(panelWidth*2+0.25, 0.5)
	pdf.Cell(3, 0.4, "How It Works")

	steps := []struct {
		num  string
		text string
	}{
		{"1", "Sign up and create your family"},
		{"2", "Add your children and care team"},
		{"3", "Log daily activities quickly"},
		{"4", "Receive AI-powered insights"},
	}

	stepY := 1.2
	for _, step := range steps {
		pdf.SetFillColor(255, 255, 255)
		pdf.Circle(panelWidth*2+0.5, stepY+0.15, 0.2, "F")

		pdf.SetFont("Helvetica", "B", 12)
		pdf.SetTextColor(int(pr), int(pg), int(pb))
		pdf.SetXY(panelWidth*2+0.35, stepY)
		pdf.Cell(0.3, 0.3, step.num)

		pdf.SetFont("Helvetica", "", 11)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetXY(panelWidth*2+0.8, stepY)
		pdf.Cell(2.5, 0.3, step.text)
		stepY += 0.7
	}

	// CTA at bottom
	pdf.SetFillColor(int(ar), int(ag), int(ab))
	pdf.Rect(panelWidth*2+0.25, 6.5, 3, 1.2, "F")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(panelWidth*2+0.4, 6.7)
	pdf.Cell(2.7, 0.4, "Get Started Today!")

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(panelWidth*2+0.4, 7.2)
	pdf.Cell(2.7, 0.3, config.WebsiteURL)

	// Output to bytes
	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GenerateStyleGuidePDF creates a style guide PDF
func (s *MarketingService) GenerateStyleGuidePDF(ctx context.Context) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	pdf := fpdf.New("P", "in", "Letter", "")
	pdf.SetMargins(0.75, 0.75, 0.75)

	pr, pg, pb := hexToRGB(config.PrimaryColor)
	sr, sg, sb := hexToRGB(config.SecondaryColor)
	_, _, _ = hexToRGB(config.AccentColor) // Accent colors available but not used in style guide PDF

	// Page 1: Cover
	pdf.AddPage()
	pdf.SetFillColor(int(pr), int(pg), int(pb))
	pdf.Rect(0, 0, 8.5, 11, "F")

	pdf.SetFont("Helvetica", "B", 48)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(0.75, 4)
	pdf.Cell(7, 1, config.AppName)

	pdf.SetFont("Helvetica", "", 24)
	pdf.SetXY(0.75, 5.2)
	pdf.Cell(7, 0.5, "Brand Style Guide")

	pdf.SetFont("Helvetica", "", 14)
	pdf.SetXY(0.75, 9)
	pdf.Cell(7, 0.4, time.Now().Format("January 2006"))

	// Page 2: Brand Overview
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 0.75)
	pdf.Cell(7, 0.6, "Brand Overview")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 1.8)
	pdf.Cell(7, 0.4, "Mission Statement")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.75, 2.3)
	pdf.MultiCell(7, 0.25, config.MissionStatement, "", "", false)

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 4)
	pdf.Cell(7, 0.4, "Tagline")

	pdf.SetFont("Helvetica", "I", 16)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 4.5)
	pdf.Cell(7, 0.4, fmt.Sprintf("\"%s\"", config.Tagline))

	// Page 3: Color Palette
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 0.75)
	pdf.Cell(7, 0.6, "Color Palette")

	// Primary colors
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 1.8)
	pdf.Cell(7, 0.4, "Primary Colors")

	colors := []struct {
		name string
		hex  string
	}{
		{"Primary", config.PrimaryColor},
		{"Primary Light", config.PrimaryLight},
		{"Primary Dark", config.PrimaryDark},
	}

	x := 0.75
	for _, c := range colors {
		r, g, b := hexToRGB(c.hex)
		pdf.SetFillColor(int(r), int(g), int(b))
		pdf.Rect(x, 2.3, 2, 1.2, "F")

		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(31, 41, 55)
		pdf.SetXY(x, 3.6)
		pdf.Cell(2, 0.25, c.name)

		pdf.SetFont("Helvetica", "", 10)
		pdf.SetXY(x, 3.9)
		pdf.Cell(2, 0.25, c.hex)
		x += 2.2
	}

	// Secondary colors
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 4.5)
	pdf.Cell(7, 0.4, "Secondary & Accent Colors")

	colors2 := []struct {
		name string
		hex  string
	}{
		{"Secondary", config.SecondaryColor},
		{"Secondary Dark", config.SecondaryDark},
		{"Accent", config.AccentColor},
		{"Accent Dark", config.AccentDark},
	}

	x = 0.75
	for _, c := range colors2 {
		r, g, b := hexToRGB(c.hex)
		pdf.SetFillColor(int(r), int(g), int(b))
		pdf.Rect(x, 5, 1.6, 1, "F")

		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetTextColor(31, 41, 55)
		pdf.SetXY(x, 6.1)
		pdf.Cell(1.6, 0.2, c.name)

		pdf.SetFont("Helvetica", "", 9)
		pdf.SetXY(x, 6.35)
		pdf.Cell(1.6, 0.2, c.hex)
		x += 1.75
	}

	// Page 4: Typography
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 0.75)
	pdf.Cell(7, 0.6, "Typography")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 1.8)
	pdf.Cell(7, 0.4, fmt.Sprintf("Heading Font: %s", config.HeadingFont))

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.75, 2.3)
	pdf.Cell(7, 0.3, "Use for headlines, titles, and prominent text")

	pdf.SetFont("Helvetica", "B", 36)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 2.8)
	pdf.Cell(7, 0.7, "Aa Bb Cc 123")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 4)
	pdf.Cell(7, 0.4, fmt.Sprintf("Body Font: %s", config.BodyFont))

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.75, 4.5)
	pdf.Cell(7, 0.3, "Use for body copy, descriptions, and general text")

	pdf.SetFont("Helvetica", "", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 5)
	pdf.Cell(7, 0.3, "Aa Bb Cc Dd Ee Ff Gg Hh Ii Jj Kk Ll Mm")
	pdf.SetXY(0.75, 5.4)
	pdf.Cell(7, 0.3, "Nn Oo Pp Qq Rr Ss Tt Uu Vv Ww Xx Yy Zz")
	pdf.SetXY(0.75, 5.8)
	pdf.Cell(7, 0.3, "0123456789")

	// Page 5: Voice & Tone
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 0.75)
	pdf.Cell(7, 0.6, "Voice & Tone")

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 1.8)
	pdf.Cell(7, 0.4, "Brand Voice")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.75, 2.3)
	pdf.MultiCell(7, 0.25, config.BrandVoice, "", "", false)

	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetTextColor(31, 41, 55)
	pdf.SetXY(0.75, 5)
	pdf.Cell(7, 0.4, "Writing Guidelines")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	pdf.SetXY(0.75, 5.5)
	pdf.MultiCell(7, 0.25, config.WritingGuidelines, "", "", false)

	// Page 6: Contact Information
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 28)
	pdf.SetTextColor(int(pr), int(pg), int(pb))
	pdf.SetXY(0.75, 0.75)
	pdf.Cell(7, 0.6, "Contact Information")

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetTextColor(55, 65, 81)
	y := 1.8

	contacts := []struct {
		label string
		value string
	}{
		{"Website", config.WebsiteURL},
		{"Support Email", config.SupportEmail},
		{"Phone", config.ContactPhone},
		{"Facebook", config.FacebookURL},
		{"Twitter/X", config.TwitterURL},
		{"Instagram", config.InstagramURL},
		{"LinkedIn", config.LinkedInURL},
	}

	for _, c := range contacts {
		if c.value != "" {
			pdf.SetFont("Helvetica", "B", 11)
			pdf.SetXY(0.75, y)
			pdf.Cell(2, 0.3, c.label+":")

			pdf.SetFont("Helvetica", "", 11)
			pdf.SetXY(2.75, y)
			pdf.Cell(5, 0.3, c.value)
			y += 0.4
		}
	}

	// Footer
	pdf.SetFillColor(int(sr), int(sg), int(sb))
	pdf.Rect(0, 9.5, 8.5, 1.5, "F")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(0.75, 10)
	pdf.Cell(7, 0.3, config.CopyrightText)

	// Output
	var buf bytes.Buffer
	err = pdf.Output(&buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GenerateLogoPNG generates a PNG logo
func (s *MarketingService) GenerateLogoPNG(ctx context.Context, variant string, size int) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(size, size)

	var bgColor, textColor color.RGBA
	switch variant {
	case "white":
		bgColor = hexToColor("#FFFFFF")
		textColor = hexToColor(config.PrimaryColor)
	case "dark":
		bgColor = hexToColor(config.PrimaryDark)
		textColor = color.RGBA{255, 255, 255, 255}
	default: // primary
		bgColor = hexToColor(config.PrimaryColor)
		textColor = color.RGBA{255, 255, 255, 255}
	}

	// Background
	dc.SetColor(bgColor)
	dc.Clear()

	// Draw rounded rectangle background
	margin := float64(size) * 0.1
	dc.SetColor(bgColor)
	dc.DrawRoundedRectangle(margin, margin, float64(size)-2*margin, float64(size)-2*margin, float64(size)*0.1)
	dc.Fill()

	// Draw text (first letter of app name)
	dc.SetColor(textColor)
	fontSize := float64(size) * 0.5

	// Try to load a font, fall back to basic drawing if not available
	if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", fontSize); err != nil {
		// Fallback: draw a simple "C" using shapes
		dc.SetLineWidth(float64(size) * 0.08)
		cx := float64(size) / 2
		cy := float64(size) / 2
		r := float64(size) * 0.25
		dc.DrawArc(cx, cy, r, 0.5, 5.5)
		dc.Stroke()
	} else {
		text := string(config.AppName[0])
		w, h := dc.MeasureString(text)
		dc.DrawString(text, (float64(size)-w)/2, (float64(size)+h)/2-h*0.1)
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// GenerateLogoSVG generates an SVG logo
func (s *MarketingService) GenerateLogoSVG(ctx context.Context, variant string, size int) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	var bgColor, textColor string
	switch variant {
	case "white":
		bgColor = "#FFFFFF"
		textColor = config.PrimaryColor
	case "dark":
		bgColor = config.PrimaryDark
		textColor = "#FFFFFF"
	default:
		bgColor = config.PrimaryColor
		textColor = "#FFFFFF"
	}

	margin := size / 10
	cornerRadius := size / 10
	fontSize := size / 2

	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">
  <rect x="%d" y="%d" width="%d" height="%d" rx="%d" fill="%s"/>
  <text x="%d" y="%d" font-family="%s, Arial, sans-serif" font-size="%d" font-weight="bold" fill="%s" text-anchor="middle" dominant-baseline="central">%s</text>
</svg>`,
		size, size, size, size,
		margin, margin, size-2*margin, size-2*margin, cornerRadius, bgColor,
		size/2, size/2, config.HeadingFont, fontSize, textColor, string(config.AppName[0]),
	)

	return []byte(svg), nil
}

// GenerateSocialGraphic generates a social media graphic
func (s *MarketingService) GenerateSocialGraphic(ctx context.Context, template models.SocialTemplate, headline, body string) ([]byte, error) {
	config, err := s.repo.GetBrandConfig(ctx)
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(template.WidthPx, template.HeightPx)

	// Gradient background (simplified - solid primary color)
	bgColor := hexToColor(config.PrimaryColor)
	dc.SetColor(bgColor)
	dc.Clear()

	// Add a subtle gradient overlay
	lightColor := hexToColor(config.PrimaryLight)
	for y := 0; y < template.HeightPx; y++ {
		alpha := float64(y) / float64(template.HeightPx) * 0.3
		r := uint8(float64(lightColor.R)*alpha + float64(bgColor.R)*(1-alpha))
		g := uint8(float64(lightColor.G)*alpha + float64(bgColor.G)*(1-alpha))
		b := uint8(float64(lightColor.B)*alpha + float64(bgColor.B)*(1-alpha))
		dc.SetColor(color.RGBA{r, g, b, 255})
		dc.DrawLine(0, float64(y), float64(template.WidthPx), float64(y))
		dc.Stroke()
	}

	// White content area
	margin := float64(template.WidthPx) * 0.1
	contentHeight := float64(template.HeightPx) * 0.5
	dc.SetColor(color.RGBA{255, 255, 255, 240})
	dc.DrawRoundedRectangle(margin, float64(template.HeightPx)*0.25, float64(template.WidthPx)-2*margin, contentHeight, 20)
	dc.Fill()

	// Headline
	dc.SetColor(hexToColor(config.PrimaryDark))
	headlineFontSize := float64(template.WidthPx) * 0.045
	if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", headlineFontSize); err == nil {
		dc.DrawStringWrapped(headline, float64(template.WidthPx)/2, float64(template.HeightPx)*0.4, 0.5, 0.5, float64(template.WidthPx)-4*margin, 1.2, gg.AlignCenter)
	}

	// Body text
	if body != "" {
		dc.SetColor(color.RGBA{75, 85, 99, 255})
		bodyFontSize := float64(template.WidthPx) * 0.025
		if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", bodyFontSize); err == nil {
			dc.DrawStringWrapped(body, float64(template.WidthPx)/2, float64(template.HeightPx)*0.55, 0.5, 0.5, float64(template.WidthPx)-4*margin, 1.4, gg.AlignCenter)
		}
	}

	// App name at bottom
	dc.SetColor(color.RGBA{255, 255, 255, 255})
	brandFontSize := float64(template.WidthPx) * 0.03
	if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", brandFontSize); err == nil {
		dc.DrawString(config.AppName, margin, float64(template.HeightPx)-margin)
	}

	// Website URL
	urlFontSize := float64(template.WidthPx) * 0.02
	if err := dc.LoadFontFace("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", urlFontSize); err == nil {
		dc.DrawStringAnchored(config.WebsiteURL, float64(template.WidthPx)-margin, float64(template.HeightPx)-margin, 1, 0)
	}

	// Encode to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// SaveAsset saves generated content to file and database
func (s *MarketingService) SaveAsset(ctx context.Context, name, assetType, format string, content []byte, width, height int) (*models.MarketingAsset, error) {
	// Ensure directory exists
	dir := filepath.Join(s.assetsDir, assetType+"s")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename
	filename := fmt.Sprintf("%s.%s", strings.ReplaceAll(strings.ToLower(name), " ", "_"), format)
	filePath := filepath.Join(dir, filename)

	// Write file
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Check if asset already exists
	existing, err := s.repo.GetMarketingAssetByName(ctx, name)
	now := time.Now()

	if err == nil && existing != nil {
		// Update existing
		existing.FilePath = filePath
		existing.FileSizeBytes = int64(len(content))
		existing.WidthPx = width
		existing.HeightPx = height
		existing.LastGeneratedAt = &now
		existing.UpdatedAt = now

		if err := s.repo.UpdateMarketingAsset(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	// Create new
	asset := &models.MarketingAsset{
		ID:              uuid.New(),
		Name:            name,
		AssetType:       assetType,
		Format:          format,
		WidthPx:         width,
		HeightPx:        height,
		FilePath:        filePath,
		FileSizeBytes:   int64(len(content)),
		IsAutoGenerated: true,
		LastGeneratedAt: &now,
		IsActive:        true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.repo.CreateMarketingAsset(ctx, asset); err != nil {
		return nil, err
	}

	return asset, nil
}

// RegenerateAllAssets regenerates all marketing assets
func (s *MarketingService) RegenerateAllAssets(ctx context.Context) error {
	// Generate brochures
	singlePage, err := s.GenerateSinglePageBrochure(ctx)
	if err != nil {
		return fmt.Errorf("single page brochure: %w", err)
	}
	if _, err := s.SaveAsset(ctx, "Single Page Brochure", models.AssetTypeBrochure, models.FormatPDF, singlePage, 612, 792); err != nil {
		return err
	}

	triFold, err := s.GenerateTriFoldBrochure(ctx)
	if err != nil {
		return fmt.Errorf("tri-fold brochure: %w", err)
	}
	if _, err := s.SaveAsset(ctx, "Tri-Fold Brochure", models.AssetTypeBrochure, models.FormatPDF, triFold, 792, 612); err != nil {
		return err
	}

	// Generate style guide
	styleGuide, err := s.GenerateStyleGuidePDF(ctx)
	if err != nil {
		return fmt.Errorf("style guide: %w", err)
	}
	if _, err := s.SaveAsset(ctx, "Brand Style Guide", models.AssetTypeStyleGuide, models.FormatPDF, styleGuide, 612, 792); err != nil {
		return err
	}

	// Generate logos
	logoSizes := []int{64, 128, 256, 512}
	variants := []string{"primary", "white", "dark"}

	for _, variant := range variants {
		for _, size := range logoSizes {
			// PNG
			png, err := s.GenerateLogoPNG(ctx, variant, size)
			if err != nil {
				return fmt.Errorf("logo PNG %s %d: %w", variant, size, err)
			}
			name := fmt.Sprintf("Logo %s %dx%d", strings.Title(variant), size, size)
			if _, err := s.SaveAsset(ctx, name, models.AssetTypeLogo, models.FormatPNG, png, size, size); err != nil {
				return err
			}
		}

		// SVG (scalable, just one size reference)
		svg, err := s.GenerateLogoSVG(ctx, variant, 512)
		if err != nil {
			return fmt.Errorf("logo SVG %s: %w", variant, err)
		}
		name := fmt.Sprintf("Logo %s SVG", strings.Title(variant))
		if _, err := s.SaveAsset(ctx, name, models.AssetTypeLogo, models.FormatSVG, svg, 512, 512); err != nil {
			return err
		}
	}

	// Generate social media graphics
	templates, err := s.repo.ListSocialTemplates(ctx, "")
	if err != nil {
		return fmt.Errorf("list social templates: %w", err)
	}

	config, _ := s.repo.GetBrandConfig(ctx)
	for _, tmpl := range templates {
		graphic, err := s.GenerateSocialGraphic(ctx, tmpl, config.Tagline, "Track. Discover. Coordinate.")
		if err != nil {
			return fmt.Errorf("social graphic %s: %w", tmpl.Name, err)
		}
		if _, err := s.SaveAsset(ctx, tmpl.Name+" Default", models.AssetTypeSocialGraphic, models.FormatPNG, graphic, tmpl.WidthPx, tmpl.HeightPx); err != nil {
			return err
		}
	}

	return nil
}

// GetAssetFile reads an asset file from disk
func (s *MarketingService) GetAssetFile(ctx context.Context, id uuid.UUID) (io.ReadCloser, string, error) {
	asset, err := s.repo.GetMarketingAsset(ctx, id)
	if err != nil {
		return nil, "", err
	}

	file, err := os.Open(asset.FilePath)
	if err != nil {
		return nil, "", err
	}

	return file, asset.Name + "." + asset.Format, nil
}

// LoadMascotImage loads the Matty mascot image if available
func (s *MarketingService) LoadMascotImage() (image.Image, error) {
	mascotPath := filepath.Join(s.assetsDir, "..", "images", "mattyfullbody_clear.png")
	file, err := os.Open(mascotPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	return img, err
}
