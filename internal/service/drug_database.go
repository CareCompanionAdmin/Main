package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"carecompanion/internal/models"
)

// DrugDatabaseService handles medication validation and drug information lookups
type DrugDatabaseService struct {
	httpClient  *http.Client
	openFDAURL  string
	dailyMedURL string
}

func NewDrugDatabaseService() *DrugDatabaseService {
	return &DrugDatabaseService{
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		openFDAURL:  "https://api.fda.gov/drug",
		dailyMedURL: "https://dailymed.nlm.nih.gov/dailymed/services/v2",
	}
}

// DrugInfo contains validated medication information
type DrugInfo struct {
	Name              string            `json:"name"`
	GenericName       string            `json:"generic_name"`
	BrandNames        []string          `json:"brand_names"`
	DrugClass         string            `json:"drug_class"`
	Indications       []string          `json:"indications"`
	IndicationsFull   []string          `json:"indications_full"`
	Warnings          []string          `json:"warnings"`
	WarningsFull      []string          `json:"warnings_full"`
	SideEffects       []SideEffect      `json:"side_effects"`
	Interactions      []DrugInteraction `json:"interactions"`
	PediatricWarnings []string          `json:"pediatric_warnings"`
	PediatricFull     []string          `json:"pediatric_full"`
	BlackBoxWarning   *string           `json:"black_box_warning,omitempty"`
	BlackBoxFull      *string           `json:"black_box_full,omitempty"`
	// Physical identification
	PhysicalDescription *PhysicalDescription `json:"physical_description,omitempty"`
	// Source information
	SourceURL    string `json:"source_url,omitempty"`
	DrugsComURL  string `json:"drugs_com_url,omitempty"`
	SetID        string `json:"set_id,omitempty"`
	NDCCodes     []string `json:"ndc_codes,omitempty"`
	// Pill identifier URL for visual verification (drugs.com has best photos)
	PillIdentifierURL string `json:"pill_identifier_url,omitempty"`
	// Dosage used for lookup
	Dosage string `json:"dosage,omitempty"`
}

// PhysicalDescription contains physical characteristics of the medication
type PhysicalDescription struct {
	Description string   `json:"description,omitempty"`
	Color       []string `json:"color,omitempty"`
	Shape       string   `json:"shape,omitempty"`
	Imprint     string   `json:"imprint,omitempty"`
	Size        string   `json:"size,omitempty"`
	Coating     string   `json:"coating,omitempty"`
	Scoring     string   `json:"scoring,omitempty"`
}

// SideEffect represents a known drug side effect
type SideEffect struct {
	Effect    string `json:"effect"`
	Frequency string `json:"frequency"` // common, uncommon, rare
	Severity  string `json:"severity"`  // mild, moderate, severe
}

// DrugInteraction represents a potential drug interaction
type DrugInteraction struct {
	Drug        string `json:"drug"`
	Severity    string `json:"severity"` // minor, moderate, major
	Description string `json:"description"`
}

// DrugValidationResult contains validation results for a medication
type DrugValidationResult struct {
	IsValid          bool     `json:"is_valid"`
	StandardName     string   `json:"standard_name"`
	BrandNames       []string `json:"brand_names"`
	DrugClass        string   `json:"drug_class"`
	HasBlackBox      bool     `json:"has_black_box"`
	CriticalWarnings []string `json:"critical_warnings"`
	PediatricNotes   []string `json:"pediatric_notes"`
}

// InteractionWarning represents a drug-drug interaction warning
type InteractionWarning struct {
	Drug1       string `json:"drug1"`
	Drug2       string `json:"drug2"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// LookupDrug fetches drug information from external databases
func (s *DrugDatabaseService) LookupDrug(ctx context.Context, drugName string) (*DrugInfo, error) {
	return s.LookupDrugWithDosage(ctx, drugName, "")
}

// LookupDrugWithDosage fetches drug information with specific dosage
func (s *DrugDatabaseService) LookupDrugWithDosage(ctx context.Context, drugName, dosage string) (*DrugInfo, error) {
	info := &DrugInfo{Name: drugName, Dosage: dosage}

	// Fetch from OpenFDA
	if err := s.fetchFromOpenFDA(ctx, drugName, info); err != nil {
		// Log but don't fail - drug may not be in FDA database
		fmt.Printf("OpenFDA lookup for %s: %v\n", drugName, err)
	}

	// Add drugs.com URL for drug information page
	cleanName := strings.ToLower(strings.ReplaceAll(drugName, " ", "-"))
	info.DrugsComURL = fmt.Sprintf("https://www.drugs.com/%s.html", url.PathEscape(cleanName))

	// Add pill identifier URL (drugs.com has the best clean pill photos)
	// This links directly to their pill identifier with the drug name pre-filled
	info.PillIdentifierURL = fmt.Sprintf("https://www.drugs.com/pill_identification.html?drugname=%s", url.QueryEscape(drugName))

	return info, nil
}
// fetchFromOpenFDA queries the FDA drug database
func (s *DrugDatabaseService) fetchFromOpenFDA(ctx context.Context, drugName string, info *DrugInfo) error {
	// Search drug labels
	labelURL := fmt.Sprintf("%s/label.json?search=openfda.brand_name:%s+OR+openfda.generic_name:%s&limit=1",
		s.openFDAURL,
		url.QueryEscape(drugName),
		url.QueryEscape(drugName),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", labelURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("FDA API returned status %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			SetID   string `json:"set_id"`
			OpenFDA struct {
				BrandName       []string `json:"brand_name"`
				GenericName     []string `json:"generic_name"`
				ProductType     []string `json:"product_type"`
				ProductNDC      []string `json:"product_ndc"`
				PackageNDC      []string `json:"package_ndc"`
				SPLSetID        []string `json:"spl_set_id"`
			} `json:"openfda"`
			Warnings                []string `json:"warnings"`
			AdverseReactions        []string `json:"adverse_reactions"`
			DrugInteractions        []string `json:"drug_interactions"`
			PediatricUse            []string `json:"pediatric_use"`
			BoxedWarning            []string `json:"boxed_warning"`
			IndicationsAndUsage     []string `json:"indications_and_usage"`
			Description             []string `json:"description"`
			SPLProductDataElements  []string `json:"spl_product_data_elements"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	if len(result.Results) > 0 {
		r := result.Results[0]

		if len(r.OpenFDA.GenericName) > 0 {
			info.GenericName = r.OpenFDA.GenericName[0]
		}
		info.BrandNames = r.OpenFDA.BrandName

		// Set ID and source URL
		info.SetID = r.SetID
		if r.SetID != "" {
			info.SourceURL = fmt.Sprintf("https://dailymed.nlm.nih.gov/dailymed/drugInfo.cfm?setid=%s", r.SetID)
		} else if len(r.OpenFDA.SPLSetID) > 0 {
			info.SetID = r.OpenFDA.SPLSetID[0]
			info.SourceURL = fmt.Sprintf("https://dailymed.nlm.nih.gov/dailymed/drugInfo.cfm?setid=%s", r.OpenFDA.SPLSetID[0])
		}

		// NDC codes for pill images
		info.NDCCodes = r.OpenFDA.ProductNDC

		// Parse warnings - both truncated and full versions
		for _, w := range r.Warnings {
			info.Warnings = append(info.Warnings, truncateText(w, 300))
			info.WarningsFull = append(info.WarningsFull, w)
		}

		// Parse side effects from adverse reactions
		if len(r.AdverseReactions) > 0 {
			info.SideEffects = parseSideEffects(r.AdverseReactions[0])
		}

		// Parse interactions
		if len(r.DrugInteractions) > 0 {
			info.Interactions = parseInteractions(r.DrugInteractions[0])
		}

		// Pediatric warnings - both truncated and full
		for _, pw := range r.PediatricUse {
			info.PediatricWarnings = append(info.PediatricWarnings, truncateText(pw, 300))
			info.PediatricFull = append(info.PediatricFull, pw)
		}

		// Black box warning - both truncated and full
		if len(r.BoxedWarning) > 0 {
			warning := truncateText(r.BoxedWarning[0], 500)
			info.BlackBoxWarning = &warning
			full := r.BoxedWarning[0]
			info.BlackBoxFull = &full
		}

		// Indications - both truncated and full
		for _, ind := range r.IndicationsAndUsage {
			info.Indications = append(info.Indications, truncateText(ind, 300))
			info.IndicationsFull = append(info.IndicationsFull, ind)
		}

		// Physical description
		if len(r.Description) > 0 || len(r.SPLProductDataElements) > 0 {
			info.PhysicalDescription = &PhysicalDescription{}

			if len(r.Description) > 0 {
				info.PhysicalDescription.Description = r.Description[0]
			}

			// Parse SPL product data for color, shape, imprint
			if len(r.SPLProductDataElements) > 0 {
				parsePhysicalFromSPL(r.SPLProductDataElements[0], info.PhysicalDescription)
			}
		}
	}

	// Try to fetch additional physical info from NDC endpoint
	if len(info.NDCCodes) > 0 {
		s.fetchNDCDetails(ctx, info.NDCCodes[0], info)
	}

	return nil
}

// fetchNDCDetails fetches detailed product info including physical characteristics
func (s *DrugDatabaseService) fetchNDCDetails(ctx context.Context, ndc string, info *DrugInfo) {
	ndcURL := fmt.Sprintf("%s/ndc.json?search=product_ndc:%s&limit=1",
		s.openFDAURL,
		url.QueryEscape(ndc),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", ndcURL, nil)
	if err != nil {
		return
	}

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Results []struct {
			ProductType     string   `json:"product_type"`
			Route           []string `json:"route"`
			DosageForm      string   `json:"dosage_form"`
			ActiveIngredients []struct {
				Name     string `json:"name"`
				Strength string `json:"strength"`
			} `json:"active_ingredients"`
			Packaging []struct {
				Description string `json:"description"`
			} `json:"packaging"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	if len(result.Results) > 0 {
		r := result.Results[0]
		if info.PhysicalDescription == nil {
			info.PhysicalDescription = &PhysicalDescription{}
		}

		// Add dosage form info
		if r.DosageForm != "" && info.PhysicalDescription.Description == "" {
			info.PhysicalDescription.Description = r.DosageForm
		}
	}
}

// parsePhysicalFromSPL extracts color, shape, and imprint from SPL data
func parsePhysicalFromSPL(splData string, phys *PhysicalDescription) {
	lowerData := strings.ToLower(splData)

	// Extract colors - check both SPL data and description
	colors := []string{"white", "yellow", "orange", "pink", "red", "purple", "blue", "green", "brown", "tan", "black", "gray", "grey", "beige", "peach"}
	lowerDesc := strings.ToLower(phys.Description)
	for _, color := range colors {
		if strings.Contains(lowerData, color) || strings.Contains(lowerDesc, color) {
			// Avoid duplicates
			found := false
			for _, c := range phys.Color {
				if c == color {
					found = true
					break
				}
			}
			if !found {
				phys.Color = append(phys.Color, color)
			}
		}
	}

	// Extract shapes - check both SPL data and description
	shapes := []string{"round", "oval", "oblong", "capsule", "tablet", "rectangle", "square", "diamond", "pentagon", "hexagon", "octagon", "triangle"}
	for _, shape := range shapes {
		if strings.Contains(lowerData, shape) || strings.Contains(lowerDesc, shape) {
			phys.Shape = shape
			break
		}
	}

	// Look for imprint patterns (alphanumeric codes)
	// Pattern: look for "imprint" or "debossed" or "engraved" followed by text
	imprintPatterns := []string{"imprint", "debossed", "engraved", "embossed", "scored"}
	for _, pattern := range imprintPatterns {
		if idx := strings.Index(lowerData, pattern); idx != -1 {
			// Extract next ~30 chars as potential imprint
			end := idx + 50
			if end > len(splData) {
				end = len(splData)
			}
			chunk := splData[idx:end]
			phys.Imprint = truncateText(chunk, 50)
			break
		}
	}

	// Check for scoring
	if strings.Contains(lowerData, "scored") {
		phys.Scoring = "scored"
	}

	// Check for coating
	coatings := []string{"film-coated", "sugar-coated", "enteric-coated", "coated"}
	for _, coating := range coatings {
		if strings.Contains(lowerData, coating) {
			phys.Coating = coating
			break
		}
	}
}

// ValidateMedication checks if a medication name is valid and returns standardized info
func (s *DrugDatabaseService) ValidateMedication(ctx context.Context, name string) (*DrugValidationResult, error) {
	info, err := s.LookupDrug(ctx, name)
	if err != nil {
		return nil, err
	}

	result := &DrugValidationResult{
		IsValid:      info.GenericName != "" || len(info.BrandNames) > 0,
		StandardName: info.GenericName,
		BrandNames:   info.BrandNames,
		DrugClass:    info.DrugClass,
		HasBlackBox:  info.BlackBoxWarning != nil,
	}

	if info.BlackBoxWarning != nil {
		result.CriticalWarnings = append(result.CriticalWarnings, *info.BlackBoxWarning)
	}

	// Add relevant pediatric warnings
	result.PediatricNotes = info.PediatricWarnings

	return result, nil
}

// CheckInteractions checks for interactions between medications
func (s *DrugDatabaseService) CheckInteractions(ctx context.Context, medications []string) ([]InteractionWarning, error) {
	var warnings []InteractionWarning

	for i := 0; i < len(medications); i++ {
		info, err := s.LookupDrug(ctx, medications[i])
		if err != nil {
			continue
		}

		for j := i + 1; j < len(medications); j++ {
			otherDrug := medications[j]
			for _, interaction := range info.Interactions {
				if matchesDrugName(interaction.Drug, otherDrug) {
					warnings = append(warnings, InteractionWarning{
						Drug1:       medications[i],
						Drug2:       otherDrug,
						Severity:    interaction.Severity,
						Description: interaction.Description,
					})
				}
			}
		}
	}

	return warnings, nil
}

// GetTier1Insights generates Tier 1 medical insights for a medication
func (s *DrugDatabaseService) GetTier1Insights(ctx context.Context, medicationName string) ([]models.Insight, error) {
	info, err := s.LookupDrug(ctx, medicationName)
	if err != nil {
		return nil, err
	}

	var insights []models.Insight

	// Generate insight for black box warning
	if info.BlackBoxWarning != nil {
		conf := 1.0
		insight := models.Insight{
			Tier:              models.TierGlobalMedical,
			Category:          "medication",
			Title:             fmt.Sprintf("%s: FDA Black Box Warning", info.Name),
			SimpleDescription: fmt.Sprintf("%s has an FDA black box warning (most serious warning type)", info.Name),
			ConfidenceScore:   &conf,
			IsActive:          true,
		}
		insight.DetailedDescription.String = *info.BlackBoxWarning
		insight.DetailedDescription.Valid = true
		insights = append(insights, insight)
	}

	// Generate insight for each common side effect
	for _, se := range info.SideEffects {
		if se.Frequency == "common" {
			conf := 1.0
			insight := models.Insight{
				Tier:              models.TierGlobalMedical,
				Category:          "medication",
				Title:             fmt.Sprintf("%s: Known Side Effect", info.Name),
				SimpleDescription: fmt.Sprintf("%s is a known %s side effect of %s.", se.Effect, se.Frequency, info.Name),
				ConfidenceScore:   &conf,
				IsActive:          true,
			}
			insight.DetailedDescription.String = fmt.Sprintf("Clinical trials and post-market surveillance have identified %s as a %s side effect of %s. Severity is typically %s.",
				se.Effect, se.Frequency, info.Name, se.Severity)
			insight.DetailedDescription.Valid = true
			insights = append(insights, insight)
		}
	}

	// Generate insight for pediatric warnings
	for _, warning := range info.PediatricWarnings {
		conf := 1.0
		insight := models.Insight{
			Tier:              models.TierGlobalMedical,
			Category:          "medication",
			Title:             fmt.Sprintf("%s: Pediatric Consideration", info.Name),
			SimpleDescription: truncateText(warning, 200),
			ConfidenceScore:   &conf,
			IsActive:          true,
		}
		insight.DetailedDescription.String = warning
		insight.DetailedDescription.Valid = true
		insights = append(insights, insight)
	}

	return insights, nil
}

// Helper functions

func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen-3] + "..."
}

func parseSideEffects(text string) []SideEffect {
	var effects []SideEffect

	// Simple parsing - look for common patterns
	// In production, use proper NLP or structured data
	commonEffects := []string{
		"headache", "nausea", "dizziness", "fatigue", "drowsiness",
		"insomnia", "dry mouth", "constipation", "diarrhea", "weight gain",
		"decreased appetite", "anxiety", "irritability", "mood changes",
	}

	lowerText := strings.ToLower(text)
	for _, effect := range commonEffects {
		if strings.Contains(lowerText, effect) {
			effects = append(effects, SideEffect{
				Effect:    effect,
				Frequency: "common",
				Severity:  "mild",
			})
		}
	}

	return effects
}

func parseInteractions(text string) []DrugInteraction {
	var interactions []DrugInteraction

	// Simple parsing - look for drug names mentioned
	// In production, use a drug interaction database
	commonDrugs := []string{
		"warfarin", "aspirin", "ibuprofen", "acetaminophen",
		"metformin", "lisinopril", "amlodipine", "metoprolol",
		"sertraline", "fluoxetine", "alprazolam", "lorazepam",
	}

	lowerText := strings.ToLower(text)
	for _, drug := range commonDrugs {
		if strings.Contains(lowerText, drug) {
			interactions = append(interactions, DrugInteraction{
				Drug:        drug,
				Severity:    "moderate",
				Description: fmt.Sprintf("Potential interaction with %s mentioned in drug label", drug),
			})
		}
	}

	return interactions
}

func matchesDrugName(name1, name2 string) bool {
	return strings.EqualFold(name1, name2) ||
		strings.Contains(strings.ToLower(name1), strings.ToLower(name2)) ||
		strings.Contains(strings.ToLower(name2), strings.ToLower(name1))
}
