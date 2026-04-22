package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// AIInsightService integrates with Claude API to generate intelligent insights
type AIInsightService struct {
	config      *config.ClaudeConfig
	httpClient  *http.Client
	logRepo     repository.LogRepository
	childRepo   repository.ChildRepository
	medRepo     repository.MedicationRepository
	insightRepo repository.InsightRepository
	alertService *AlertService
	limiter     *rateLimiter
}

type rateLimiter struct {
	mu         sync.Mutex
	calls      int
	maxCalls   int
	resetTime  time.Time
}

func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	if now.After(rl.resetTime) {
		rl.calls = 0
		rl.resetTime = now.Add(time.Minute)
	}
	if rl.calls >= rl.maxCalls {
		return false
	}
	rl.calls++
	return true
}

// NewAIInsightService creates a new AI insight service
func NewAIInsightService(
	cfg *config.ClaudeConfig,
	logRepo repository.LogRepository,
	childRepo repository.ChildRepository,
	medRepo repository.MedicationRepository,
	insightRepo repository.InsightRepository,
	alertService *AlertService,
) *AIInsightService {
	return &AIInsightService{
		config:       cfg,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		logRepo:      logRepo,
		childRepo:    childRepo,
		medRepo:      medRepo,
		insightRepo:  insightRepo,
		alertService: alertService,
		limiter:      &rateLimiter{maxCalls: 10, resetTime: time.Now().Add(time.Minute)},
	}
}

// aiInsightResult is the expected JSON structure from Claude's response
type aiInsightResult struct {
	Tier                int               `json:"tier"`
	Category            string            `json:"category"`
	Title               string            `json:"title"`
	SimpleDescription   string            `json:"simple_description"`
	DetailedDescription string            `json:"detailed_description"`
	Confidence          float64           `json:"confidence"`
	InputFactors        []string          `json:"input_factors,omitempty"`
	OutputFactors       []string          `json:"output_factors,omitempty"`
	DataPointCount      *int              `json:"data_point_count,omitempty"`
	CohortCriteria      map[string]interface{} `json:"cohort_criteria,omitempty"`
	AlertWorthy         bool              `json:"alert_worthy"`
	AlertSeverity       string            `json:"alert_severity,omitempty"`
}

// AnalyzeChild runs a full AI analysis for a child
func (s *AIInsightService) AnalyzeChild(ctx context.Context, child models.Child) error {
	if !s.config.Enabled || s.config.APIKey == "" {
		return nil
	}

	log.Printf("AI Insights: analyzing %s (child %s)", child.FirstName, child.ID)
	start := time.Now()

	// Get child data
	conditions, err := s.childRepo.GetConditions(ctx, child.ID)
	if err != nil {
		log.Printf("AI Insights: failed to get conditions for %s: %v", child.FirstName, err)
		conditions = nil
	}

	medications, err := s.medRepo.GetByChildID(ctx, child.ID, true)
	if err != nil {
		log.Printf("AI Insights: failed to get medications for %s: %v", child.FirstName, err)
		medications = nil
	}

	// Get recent logs
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	lookbackStart := today.AddDate(0, 0, -s.config.LookbackDays)

	logs, err := s.logRepo.GetLogsForDateRange(ctx, child.ID, lookbackStart, today)
	if err != nil {
		log.Printf("AI Insights: failed to get logs for %s: %v", child.FirstName, err)
		return err
	}

	// Build profile context
	profileCtx := s.buildProfileContext(child, conditions, medications)

	// Check for recent insights to avoid duplicates
	recentInsights, _ := s.insightRepo.GetByChildIDSince(ctx, child.ID, time.Now().Add(-48*time.Hour))

	totalInsights := 0
	totalAlerts := 0

	// Call 1: Tier 1+2 (medical knowledge + cohort patterns) — profile only
	if s.limiter.allow() {
		results, err := s.callClaudeForTier12(ctx, profileCtx, child.FirstName)
		if err != nil {
			log.Printf("AI Insights: Tier 1+2 API error for %s: %v", child.FirstName, err)
		} else {
			n, a := s.storeInsights(ctx, child.ID, child.FamilyID, results, recentInsights)
			totalInsights += n
			totalAlerts += a
		}
	}

	// Call 2: Tier 3 (family-specific patterns) — profile + log data
	if s.limiter.allow() && logs != nil {
		logCtx := s.buildLogContext(child, logs)
		results, err := s.callClaudeForTier3(ctx, profileCtx, logCtx, child.FirstName)
		if err != nil {
			log.Printf("AI Insights: Tier 3 API error for %s: %v", child.FirstName, err)
		} else {
			n, a := s.storeInsights(ctx, child.ID, child.FamilyID, results, recentInsights)
			totalInsights += n
			totalAlerts += a
		}
	}

	duration := time.Since(start)
	log.Printf("AI Insights: completed %s — %d insights, %d alerts generated in %v",
		child.FirstName, totalInsights, totalAlerts, duration)

	return nil
}

// buildProfileContext creates a compact text summary of the child's profile
func (s *AIInsightService) buildProfileContext(child models.Child, conditions []models.ChildCondition, medications []models.Medication) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Child: %s\n", child.FirstName))
	if !child.DateOfBirth.IsZero() {
		age := time.Since(child.DateOfBirth)
		years := int(age.Hours() / 8760)
		months := int(age.Hours()/730) % 12
		b.WriteString(fmt.Sprintf("Age: %d years %d months\n", years, months))
	}
	if child.Gender.Valid && child.Gender.String != "" {
		b.WriteString(fmt.Sprintf("Gender: %s\n", child.Gender.String))
	}

	if len(conditions) > 0 {
		b.WriteString("\nDiagnosed Conditions:\n")
		for _, c := range conditions {
			if !c.IsActive {
				continue
			}
			line := fmt.Sprintf("- %s", c.ConditionName)
			if c.ICDCode.Valid {
				line += fmt.Sprintf(" (ICD: %s)", c.ICDCode.String)
			}
			if c.Severity.Valid {
				line += fmt.Sprintf(", severity: %s", c.Severity.String)
			}
			b.WriteString(line + "\n")
		}
	}

	if len(medications) > 0 {
		b.WriteString("\nActive Medications:\n")
		for _, m := range medications {
			if !m.IsActive {
				continue
			}
			b.WriteString(fmt.Sprintf("- %s %s %s, %s\n", m.Name, m.Dosage, m.DosageUnit, m.Frequency))
		}
	}

	return b.String()
}

// buildLogContext creates a compact text summary of recent log data
func (s *AIInsightService) buildLogContext(child models.Child, logs *models.DailyLogPage) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n=== Recent Log Data for %s (last %d days) ===\n", child.FirstName, s.config.LookbackDays))

	// Behavior logs
	if len(logs.BehaviorLogs) > 0 {
		b.WriteString("\nBehavior Logs:\n")
		for _, l := range logs.BehaviorLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.MoodLevel != nil {
				line += fmt.Sprintf(" mood=%d/10", *l.MoodLevel)
			}
			if l.EnergyLevel != nil {
				line += fmt.Sprintf(" energy=%d/10", *l.EnergyLevel)
			}
			if l.AnxietyLevel != nil {
				line += fmt.Sprintf(" anxiety=%d/10", *l.AnxietyLevel)
			}
			if l.Meltdowns > 0 {
				line += fmt.Sprintf(" meltdowns=%d", l.Meltdowns)
			}
			if l.StimmingEpisodes > 0 {
				line += fmt.Sprintf(" stimming=%d", l.StimmingEpisodes)
			}
			if l.AggressionIncidents > 0 {
				line += fmt.Sprintf(" aggression=%d", l.AggressionIncidents)
			}
			if l.Notes.Valid && l.Notes.String != "" {
				line += fmt.Sprintf(" notes=%q", aiTruncate(l.Notes.String, 100))
			}
			b.WriteString(line + "\n")
		}
	}

	// Sleep logs
	if len(logs.SleepLogs) > 0 {
		b.WriteString("\nSleep Logs:\n")
		for _, l := range logs.SleepLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.TotalSleepMinutes != nil {
				hours := float64(*l.TotalSleepMinutes) / 60.0
				line += fmt.Sprintf(" %.1f hours", hours)
			}
			if l.SleepQuality.Valid && l.SleepQuality.String != "" {
				line += fmt.Sprintf(" quality=%s", l.SleepQuality.String)
			}
			if l.NightWakings > 0 {
				line += fmt.Sprintf(" wakings=%d", l.NightWakings)
			}
			if l.Nightmares {
				line += " nightmares=yes"
			}
			if l.BedWetting {
				line += " bedwetting=yes"
			}
			b.WriteString(line + "\n")
		}
	}

	// Medication logs
	if len(logs.MedicationLogs) > 0 {
		b.WriteString("\nMedication Adherence:\n")
		taken := 0
		missed := 0
		for _, l := range logs.MedicationLogs {
			if string(l.Status) == "taken" {
				taken++
			} else if string(l.Status) == "missed" || string(l.Status) == "skipped" {
				missed++
			}
		}
		total := taken + missed
		if total > 0 {
			b.WriteString(fmt.Sprintf("  %d/%d doses taken (%.0f%% adherence)\n", taken, total, float64(taken)/float64(total)*100))
		}
		// List missed medications
		for _, l := range logs.MedicationLogs {
			if string(l.Status) == "missed" || string(l.Status) == "skipped" {
				b.WriteString(fmt.Sprintf("  MISSED: %s on %s\n", l.MedicationName, l.LogDate.Format("01/02")))
			}
		}
	}

	// Diet logs
	if len(logs.DietLogs) > 0 {
		b.WriteString("\nDiet Logs:\n")
		for _, l := range logs.DietLogs {
			mealType := "meal"
			if l.MealType.Valid {
				mealType = l.MealType.String
			}
			line := fmt.Sprintf("  %s %s:", l.LogDate.Format("01/02"), mealType)
			if len(l.FoodsEaten) > 0 {
				line += fmt.Sprintf(" ate=%s", strings.Join(stringSlice(l.FoodsEaten), ","))
			}
			if len(l.FoodsRefused) > 0 {
				line += fmt.Sprintf(" refused=%s", strings.Join(stringSlice(l.FoodsRefused), ","))
			}
			if l.AppetiteLevel.Valid && l.AppetiteLevel.String != "" {
				line += fmt.Sprintf(" appetite=%s", l.AppetiteLevel.String)
			}
			if l.AllergicReaction {
				line += " ALLERGY_REACTION"
			}
			b.WriteString(line + "\n")
		}
	}

	// Sensory logs
	if len(logs.SensoryLogs) > 0 {
		b.WriteString("\nSensory Logs:\n")
		for _, l := range logs.SensoryLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.OverloadEpisodes > 0 {
				line += fmt.Sprintf(" overloads=%d", l.OverloadEpisodes)
			}
			if l.OverallRegulation != nil {
				line += fmt.Sprintf(" regulation=%d/5", *l.OverallRegulation)
			}
			if len(l.OverloadTriggers) > 0 {
				line += fmt.Sprintf(" triggers=%s", strings.Join(stringSlice(l.OverloadTriggers), ","))
			}
			b.WriteString(line + "\n")
		}
	}

	// Social logs
	if len(logs.SocialLogs) > 0 {
		b.WriteString("\nSocial Logs:\n")
		for _, l := range logs.SocialLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.EyeContactLevel != nil {
				line += fmt.Sprintf(" eye_contact=%d/5", *l.EyeContactLevel)
			}
			if l.SocialEngagementLevel != nil {
				line += fmt.Sprintf(" engagement=%d/5", *l.SocialEngagementLevel)
			}
			if l.PeerInteractions > 0 {
				line += fmt.Sprintf(" peer_interactions=%d", l.PeerInteractions)
			}
			if l.Conflicts > 0 {
				line += fmt.Sprintf(" conflicts=%d", l.Conflicts)
			}
			b.WriteString(line + "\n")
		}
	}

	// Speech logs
	if len(logs.SpeechLogs) > 0 {
		b.WriteString("\nSpeech Logs:\n")
		for _, l := range logs.SpeechLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.VerbalOutputLevel != nil {
				line += fmt.Sprintf(" verbal=%d/5", *l.VerbalOutputLevel)
			}
			if l.ClarityLevel != nil {
				line += fmt.Sprintf(" clarity=%d/5", *l.ClarityLevel)
			}
			if len(l.NewWords) > 0 {
				line += fmt.Sprintf(" new_words=%s", strings.Join(stringSlice(l.NewWords), ","))
			}
			if len(l.LostWords) > 0 {
				line += fmt.Sprintf(" lost_words=%s", strings.Join(stringSlice(l.LostWords), ","))
			}
			b.WriteString(line + "\n")
		}
	}

	// Bowel logs
	if len(logs.BowelLogs) > 0 {
		b.WriteString("\nBowel Logs:\n")
		for _, l := range logs.BowelLogs {
			line := fmt.Sprintf("  %s:", l.LogDate.Format("01/02"))
			if l.BristolScale != nil {
				line += fmt.Sprintf(" bristol=%d", *l.BristolScale)
			}
			if l.HadAccident {
				line += " accident=yes"
			}
			if l.PainLevel != nil && *l.PainLevel > 0 {
				line += fmt.Sprintf(" pain=%d/10", *l.PainLevel)
			}
			b.WriteString(line + "\n")
		}
	}

	// Therapy logs
	if len(logs.TherapyLogs) > 0 {
		b.WriteString("\nTherapy Logs:\n")
		for _, l := range logs.TherapyLogs {
			therapyType := "therapy"
			if l.TherapyType.Valid {
				therapyType = l.TherapyType.String
			}
			line := fmt.Sprintf("  %s: %s", l.LogDate.Format("01/02"), therapyType)
			if l.DurationMinutes != nil {
				line += fmt.Sprintf(" %dmin", *l.DurationMinutes)
			}
			if l.ProgressNotes.Valid && l.ProgressNotes.String != "" {
				line += fmt.Sprintf(" notes=%q", aiTruncate(l.ProgressNotes.String, 80))
			}
			b.WriteString(line + "\n")
		}
	}

	// Seizure logs
	if len(logs.SeizureLogs) > 0 {
		b.WriteString("\nSeizure Logs:\n")
		for _, l := range logs.SeizureLogs {
			seizureType := "unknown"
			if l.SeizureType.Valid {
				seizureType = l.SeizureType.String
			}
			line := fmt.Sprintf("  %s: type=%s", l.LogDate.Format("01/02"), seizureType)
			if l.DurationSeconds != nil {
				line += fmt.Sprintf(" duration=%ds", *l.DurationSeconds)
			}
			if l.RescueMedicationGiven {
				line += " rescue_med=yes"
			}
			b.WriteString(line + "\n")
		}
	}

	// Health events
	if len(logs.HealthEventLogs) > 0 {
		b.WriteString("\nHealth Events:\n")
		for _, l := range logs.HealthEventLogs {
			eventType := "event"
			if l.EventType.Valid {
				eventType = l.EventType.String
			}
			line := fmt.Sprintf("  %s: %s", l.LogDate.Format("01/02"), eventType)
			if l.Description.Valid && l.Description.String != "" {
				line += fmt.Sprintf(" — %s", aiTruncate(l.Description.String, 100))
			}
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

const tier12SystemPrompt = `You are a medical knowledge assistant for MyCareCompanion, a family care tracking app for children with autism spectrum disorder and related conditions. Given a child's profile (age, diagnoses, medications), provide relevant insights.

Generate TWO types of insights:

TIER 1 (Medical Knowledge): Known medication side effects relevant to current dosages, drug interactions if multiple medications, age-appropriate developmental expectations, condition-specific medical knowledge.
- Do NOT diagnose or recommend medication changes
- Always note families should consult their healthcare provider
- Set "tier": 1

TIER 2 (Research-Based Patterns): What published research and clinical practice suggest for children with similar profiles. Frame as "Research suggests..." or "Children with similar profiles often..."
- Draw on published autism/ADHD research and clinical practice patterns
- Be specific to the child's actual profile (age, conditions, medications)
- Set "tier": 2

Return a JSON array (no markdown, no code fences, just the array):
[{
  "tier": 1 or 2,
  "category": "medication|condition|development|safety|behavior|sleep|diet|sensory|social|therapy",
  "title": "Short descriptive title mentioning the child's name (under 80 chars)",
  "simple_description": "Parent-friendly explanation (2-3 sentences). Use the child's name.",
  "detailed_description": "Clinical detail for advanced view",
  "confidence": 0.7 to 1.0,
  "alert_worthy": false,
  "alert_severity": "info"
}]

Return 2-3 insights total (1-2 Tier 1, 1 Tier 2). Only include genuinely relevant information, not generic advice.`

const tier3SystemPrompt = `You are a data analyst for MyCareCompanion, a care tracking app for children with autism. You are given a child's recent log data across multiple dimensions (behavior, sleep, diet, medication, sensory, social, therapy, bowel, seizure, health events, speech).

Analyze the data for:
1. Correlations between different log types (e.g., sleep quality vs next-day mood)
2. Trends over time (improving, declining, stable)
3. Notable events or outliers
4. Positive developments worth celebrating
5. Concerning patterns that warrant attention

Be SPECIFIC. Reference actual data values and dates. Use the child's name.
Example good insight: "Emma's mood averaged 7/10 on days following 9+ hours of sleep, vs 4/10 after less than 7 hours (observed 5 of 7 days)"
Example bad insight: "Sleep affects mood" (too generic)

Return a JSON array (no markdown, no code fences, just the raw JSON array):
[{
  "tier": 3,
  "category": "behavior|sleep|diet|medication|sensory|social|therapy|bowel|health|speech|general",
  "title": "Short title with child's name (under 80 chars)",
  "simple_description": "Parent-friendly with specific data references (2-3 sentences)",
  "detailed_description": "Statistical detail and reasoning",
  "confidence": 0.5 to 1.0,
  "input_factors": ["factor1"],
  "output_factors": ["factor2"],
  "data_point_count": N,
  "alert_worthy": true or false,
  "alert_severity": "info|warning"
}]

Return 3-5 insights, ordered by importance. Include at least one positive observation if the data supports it. If there is insufficient data to find meaningful patterns, return fewer insights but be honest about data limitations.`

func (s *AIInsightService) callClaudeForTier12(ctx context.Context, profileCtx, childName string) ([]aiInsightResult, error) {
	userPrompt := fmt.Sprintf("Analyze this child's profile and provide Tier 1 (medical knowledge) and Tier 2 (research-based) insights.\n\n%s", profileCtx)
	return s.callClaude(ctx, tier12SystemPrompt, userPrompt)
}

func (s *AIInsightService) callClaudeForTier3(ctx context.Context, profileCtx, logCtx, childName string) ([]aiInsightResult, error) {
	userPrompt := fmt.Sprintf("Analyze this child's recent data and identify patterns, trends, and observations.\n\n%s\n%s", profileCtx, logCtx)
	return s.callClaude(ctx, tier3SystemPrompt, userPrompt)
}

// claudeRequest is the API request body
type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *AIInsightService) callClaude(ctx context.Context, systemPrompt, userPrompt string) ([]aiInsightResult, error) {
	reqBody := claudeRequest{
		Model:     s.config.Model,
		MaxTokens: s.config.MaxTokens,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.config.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if claudeResp.Error != nil {
		return nil, fmt.Errorf("API error: %s — %s", claudeResp.Error.Type, claudeResp.Error.Message)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response content")
	}

	log.Printf("AI Insights: API usage — %d input tokens, %d output tokens",
		claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens)

	// Parse the JSON array from the response text
	text := claudeResp.Content[0].Text

	// Strip markdown code fences if present
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var results []aiInsightResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		log.Printf("AI Insights: failed to parse response JSON: %v\nRaw: %s", err, text[:min(len(text), 500)])
		return nil, fmt.Errorf("parse response JSON: %w", err)
	}

	// Validate results
	var valid []aiInsightResult
	for _, r := range results {
		if r.Title == "" || r.SimpleDescription == "" {
			continue
		}
		if r.Confidence < 0 {
			r.Confidence = 0
		}
		if r.Confidence > 1 {
			r.Confidence = 1
		}
		if r.Category == "" {
			r.Category = "general"
		}
		if r.Tier < 1 || r.Tier > 3 {
			continue
		}
		valid = append(valid, r)
	}

	return valid, nil
}

// storeInsights saves parsed insights to the database, deduplicating against recent insights.
// Returns (insights_created, alerts_created)
func (s *AIInsightService) storeInsights(ctx context.Context, childID, familyID uuid.UUID, results []aiInsightResult, recentInsights []models.Insight) (int, int) {
	insightsCreated := 0
	alertsCreated := 0

	for _, r := range results {
		// Dedup: check if a similar title exists in recent insights
		if isDuplicate(r.Title, recentInsights) {
			log.Printf("AI Insights: skipping duplicate — %s", r.Title)
			continue
		}

		insight := &models.Insight{
			ChildID:             &childID,
			FamilyID:            &familyID,
			Tier:                models.InsightTier(r.Tier),
			Category:            r.Category,
			Title:               r.Title,
			SimpleDescription:   r.SimpleDescription,
			DetailedDescription: models.NullString{NullString: sql.NullString{String: r.DetailedDescription, Valid: r.DetailedDescription != ""}},
			ConfidenceScore:     &r.Confidence,
			IsActive:            true,
		}

		// Tier-specific fields
		if r.Tier == 2 && r.CohortCriteria != nil {
			insight.CohortCriteria = models.JSONB(r.CohortCriteria)
		}
		if r.Tier == 3 {
			if len(r.InputFactors) > 0 {
				insight.InputFactors = models.StringArray(r.InputFactors)
			}
			if len(r.OutputFactors) > 0 {
				insight.OutputFactors = models.StringArray(r.OutputFactors)
			}
			if r.DataPointCount != nil {
				insight.DataPointCount = r.DataPointCount
			}
			now := time.Now()
			start := now.AddDate(0, 0, -s.config.LookbackDays)
			insight.DateRangeStart = models.NullTime{NullTime: sql.NullTime{Time: start, Valid: true}}
			insight.DateRangeEnd = models.NullTime{NullTime: sql.NullTime{Time: now, Valid: true}}
		}

		if err := s.insightRepo.Create(ctx, insight); err != nil {
			log.Printf("AI Insights: failed to store insight %q: %v", r.Title, err)
			continue
		}
		insightsCreated++

		// Create alert if warranted
		if r.AlertWorthy {
			severity := models.AlertSeverityInfo
			if r.AlertSeverity == "warning" {
				severity = models.AlertSeverityWarning
			} else if r.AlertSeverity == "critical" {
				severity = models.AlertSeverityCritical
			}

			conf := r.Confidence
			alert := &models.Alert{
				ChildID:         childID,
				FamilyID:        familyID,
				AlertType:       "ai_insight",
				Title:           r.Title,
				Description:     r.SimpleDescription,
				Severity:        severity,
				Status:          models.AlertStatusActive,
				ConfidenceScore: &conf,
				SourceType:      models.CorrelationTypeFamilySpecific,
			}
			if err := s.alertService.Create(ctx, alert); err != nil {
				log.Printf("AI Insights: failed to create alert for %q: %v", r.Title, err)
			} else {
				alertsCreated++
			}
		}
	}

	return insightsCreated, alertsCreated
}

// isDuplicate checks if a similar title exists in recent insights
func isDuplicate(title string, recent []models.Insight) bool {
	titleLower := strings.ToLower(title)
	for _, ins := range recent {
		if strings.ToLower(ins.Title) == titleLower {
			return true
		}
		// Fuzzy match: if 80% of words overlap
		titleWords := strings.Fields(titleLower)
		insWords := strings.Fields(strings.ToLower(ins.Title))
		if len(titleWords) > 0 && len(insWords) > 0 {
			matches := 0
			for _, tw := range titleWords {
				for _, iw := range insWords {
					if tw == iw {
						matches++
						break
					}
				}
			}
			overlap := float64(matches) / float64(max(len(titleWords), len(insWords)))
			if overlap >= 0.8 {
				return true
			}
		}
	}
	return false
}

func aiTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func stringSlice(arr models.StringArray) []string {
	if arr == nil {
		return nil
	}
	return []string(arr)
}

