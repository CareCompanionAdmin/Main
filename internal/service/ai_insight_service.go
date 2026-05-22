package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/google/uuid"

	"carecompanion/internal/config"
	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ErrClaudeUnavailable is returned by callClaude when the upstream Bedrock
// service is overloaded (throttling, service unavailable), the per-call
// deadline was exceeded, or any other transient transport-level failure
// occurred. Callers should treat this as "no insights this run" rather
// than a hard failure — the next scheduled batch will try again.
var ErrClaudeUnavailable = errors.New("claude api unavailable")

// claudeCallTimeout caps how long any single Claude API call can take.
// AnalyzeChild is often invoked from a background batch with a parent
// context that has no deadline; without this cap a single hung request
// would block the worker indefinitely.
const claudeCallTimeout = 45 * time.Second

// bedrockAnthropicVersion is the on-Bedrock Anthropic protocol version
// string. Unlike the Anthropic-direct API (which uses the model field +
// anthropic-version header), Bedrock requires this in the request body
// and accepts the model via the InvokeModel input.
const bedrockAnthropicVersion = "bedrock-2023-05-31"

// AIInsightService integrates with Claude (via AWS Bedrock) to generate
// intelligent insights. The Bedrock integration is governed by the AWS BAA
// signed for this account, satisfying HIPAA processing requirements for the
// de-identified payloads emitted by the PHI stripper (Phase 1). For prompts
// that include opt-in user free-text (Phase 3), the BAA is load-bearing.
type AIInsightService struct {
	config        *config.ClaudeConfig
	bedrockClient *bedrockruntime.Client
	logRepo       repository.LogRepository
	childRepo     repository.ChildRepository
	medRepo       repository.MedicationRepository
	insightRepo   repository.InsightRepository
	alertService  *AlertService
	limiter       *rateLimiter

	// narrativeConsent gates whether free-text fields are included in
	// outbound prompts. Optional (nil-safe — nil means "always strip").
	// Phase 3 of the internal-AI initiative; defaults to dormant.
	narrativeConsent *AINarrativeConsentService
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

// NewAIInsightService creates a new AI insight service. The Bedrock client
// loads AWS credentials from the default chain — on prod EC2 that's the
// instance role (which carries the BedrockClaudeInvoke inline policy);
// on dev (admin EC2) that's the same role. If config loading fails we
// log and continue with a nil client — callClaude will return
// ErrClaudeUnavailable rather than panic. AI Insights is opt-in via
// CLAUDE_ENABLED so a startup failure is non-fatal.
func NewAIInsightService(
	cfg *config.ClaudeConfig,
	logRepo repository.LogRepository,
	childRepo repository.ChildRepository,
	medRepo repository.MedicationRepository,
	insightRepo repository.InsightRepository,
	alertService *AlertService,
) *AIInsightService {
	var bedrockClient *bedrockruntime.Client
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Printf("[AI] LoadDefaultConfig failed, Bedrock client unavailable: %v", err)
	} else {
		bedrockClient = bedrockruntime.NewFromConfig(awsCfg)
	}
	return &AIInsightService{
		config:        cfg,
		bedrockClient: bedrockClient,
		logRepo:       logRepo,
		childRepo:     childRepo,
		medRepo:       medRepo,
		insightRepo:   insightRepo,
		alertService:  alertService,
		limiter:       &rateLimiter{maxCalls: 10, resetTime: time.Now().Add(time.Minute)},
	}
}

// SetNarrativeConsent wires the Phase 3 consent gate. Call this once
// after services are constructed. If never called (or passed nil), the
// AI service treats all callers as not-consented and continues stripping
// free-text — the safe Phase 1 default.
func (s *AIInsightService) SetNarrativeConsent(svc *AINarrativeConsentService) {
	s.narrativeConsent = svc
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

// AnalyzeChild runs a full AI analysis for a child. Returns early (no-op)
// when the AI feature is disabled or the Bedrock client failed to
// initialize at startup.
func (s *AIInsightService) AnalyzeChild(ctx context.Context, child models.Child) error {
	if !s.config.Enabled || s.bedrockClient == nil {
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
			if errors.Is(err, ErrClaudeUnavailable) {
				log.Printf("AI Insights: Tier 1+2 skipped for %s — Claude unavailable: %v", child.FirstName, err)
			} else {
				log.Printf("AI Insights: Tier 1+2 API error for %s: %v", child.FirstName, err)
			}
		} else {
			n, a := s.storeInsights(ctx, child.ID, child.FamilyID, child.FirstName, results, recentInsights)
			totalInsights += n
			totalAlerts += a
		}
	}

	// Call 2: Tier 3 (family-specific patterns) — profile + log data.
	// Narrative free-text is included only when the family's primary
	// parent has explicitly opted in (Phase 3 consent gate) AND the
	// server-side feature flag is on. The default — and the only state
	// in prod through Phases 3-4 — is "always strip."
	includeNarrative := s.narrativeConsent != nil && s.narrativeConsent.AllowsNarrativeForFamily(ctx, child.FamilyID)
	if s.limiter.allow() && logs != nil {
		logCtx := s.buildLogContext(child, logs, includeNarrative)
		results, err := s.callClaudeForTier3(ctx, profileCtx, logCtx, child.FirstName)
		if err != nil {
			if errors.Is(err, ErrClaudeUnavailable) {
				log.Printf("AI Insights: Tier 3 skipped for %s — Claude unavailable: %v", child.FirstName, err)
			} else {
				log.Printf("AI Insights: Tier 3 API error for %s: %v", child.FirstName, err)
			}
		} else {
			n, a := s.storeInsights(ctx, child.ID, child.FamilyID, child.FirstName, results, recentInsights)
			totalInsights += n
			totalAlerts += a
		}
	}

	duration := time.Since(start)
	log.Printf("AI Insights: completed %s — %d insights, %d alerts generated in %v",
		child.FirstName, totalInsights, totalAlerts, duration)

	return nil
}

// buildProfileContext creates a de-identified text summary of the child's
// profile for outbound LLM calls. All HIPAA Safe Harbor identifiers are
// stripped here — first name becomes "[CHILD]" placeholder, DOB becomes a
// 2-year age band, ICD codes become coarse disease categories, and
// medication names become drug classes. See ai_phi_stripper.go.
func (s *AIInsightService) buildProfileContext(child models.Child, conditions []models.ChildCondition, medications []models.Medication) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Subject: %s\n", NamePlaceholder))
	if band := AgeBand(child.DateOfBirth); band != "" {
		b.WriteString(fmt.Sprintf("Age band: %s\n", band))
	}
	if child.Gender.Valid && child.Gender.String != "" {
		b.WriteString(fmt.Sprintf("Gender: %s\n", child.Gender.String))
	}

	if len(conditions) > 0 {
		b.WriteString("\nDiagnosed Conditions (category-level):\n")
		seen := make(map[string]string) // category -> severity
		for _, c := range conditions {
			if !c.IsActive {
				continue
			}
			cat := ""
			if c.ICDCode.Valid {
				cat = GeneralizeICD(c.ICDCode.String)
			}
			if cat == "" {
				cat = "unspecified"
			}
			sev := ""
			if c.Severity.Valid {
				sev = c.Severity.String
			}
			// Keep the most-severe value if a category appears multiple times.
			if existing, ok := seen[cat]; !ok || sev > existing {
				seen[cat] = sev
			}
		}
		// Deterministic ordering for repeatable output.
		cats := make([]string, 0, len(seen))
		for cat := range seen {
			cats = append(cats, cat)
		}
		sort.Strings(cats)
		for _, cat := range cats {
			line := "- " + cat
			if seen[cat] != "" {
				line += ", severity: " + seen[cat]
			}
			b.WriteString(line + "\n")
		}
	}

	if len(medications) > 0 {
		b.WriteString("\nActive Medications (drug class only):\n")
		classCounts := make(map[string]int)
		for _, m := range medications {
			if !m.IsActive {
				continue
			}
			classCounts[DrugClass(m.Name)]++
		}
		classes := make([]string, 0, len(classCounts))
		for c := range classCounts {
			classes = append(classes, c)
		}
		sort.Strings(classes)
		for _, c := range classes {
			if classCounts[c] > 1 {
				b.WriteString(fmt.Sprintf("- %s × %d\n", c, classCounts[c]))
			} else {
				b.WriteString(fmt.Sprintf("- %s\n", c))
			}
		}
	}

	return b.String()
}

// buildLogContext creates a de-identified summary of recent log data for
// outbound LLM calls. Free-text fields (behavior notes, therapy progress
// notes, health-event descriptions) are dropped UNLESS includeNarrative
// is true — Phase 3 narrative opt-in gates that. Calendar dates become
// relative day labels ("Day-3"), and missed medications are aggregated
// by drug class rather than by name. See ai_phi_stripper.go.
func (s *AIInsightService) buildLogContext(_ models.Child, logs *models.DailyLogPage, includeNarrative bool) string {
	var b strings.Builder
	now := time.Now()
	b.WriteString(fmt.Sprintf("\n=== Recent log data for %s (last %d days) ===\n", NamePlaceholder, s.config.LookbackDays))

	// Behavior logs — numerical only; free-text notes intentionally dropped
	if len(logs.BehaviorLogs) > 0 {
		b.WriteString("\nBehavior Logs:\n")
		for _, l := range logs.BehaviorLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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
			// Free-text notes: Phase 1 default strips; Phase 3 opt-in includes.
			if includeNarrative && l.Notes.Valid && l.Notes.String != "" {
				line += fmt.Sprintf(" notes=%q", aiTruncate(l.Notes.String, 100))
			}
			b.WriteString(line + "\n")
		}
	}

	// Sleep logs — purely numerical/categorical, no PHI
	if len(logs.SleepLogs) > 0 {
		b.WriteString("\nSleep Logs:\n")
		for _, l := range logs.SleepLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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

	// Medication adherence — aggregate adherence rate + missed-doses by drug class
	if len(logs.MedicationLogs) > 0 {
		b.WriteString("\nMedication Adherence:\n")
		taken := 0
		missed := 0
		missedByClass := make(map[string]int)
		for _, l := range logs.MedicationLogs {
			switch string(l.Status) {
			case "taken":
				taken++
			case "missed", "skipped":
				missed++
				missedByClass[DrugClass(l.MedicationName)]++
			}
		}
		total := taken + missed
		if total > 0 {
			b.WriteString(fmt.Sprintf("  %d/%d doses taken (%.0f%% adherence)\n", taken, total, float64(taken)/float64(total)*100))
		}
		// Deterministic ordering: sort classes alphabetically
		classes := make([]string, 0, len(missedByClass))
		for c := range missedByClass {
			classes = append(classes, c)
		}
		sort.Strings(classes)
		for _, c := range classes {
			b.WriteString(fmt.Sprintf("  MISSED: %d dose(s) of class=%s\n", missedByClass[c], c))
		}
	}

	// Diet logs — foods are categorical (no PHI)
	if len(logs.DietLogs) > 0 {
		b.WriteString("\nDiet Logs:\n")
		for _, l := range logs.DietLogs {
			mealType := "meal"
			if l.MealType.Valid {
				mealType = l.MealType.String
			}
			line := fmt.Sprintf("  %s %s:", RelativeDayLabel(l.LogDate, now), mealType)
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

	// Sensory logs — counts and triggers, no free-text
	if len(logs.SensoryLogs) > 0 {
		b.WriteString("\nSensory Logs:\n")
		for _, l := range logs.SensoryLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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

	// Social logs — purely numerical
	if len(logs.SocialLogs) > 0 {
		b.WriteString("\nSocial Logs:\n")
		for _, l := range logs.SocialLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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

	// Speech logs — verbal output + word lists (categorical)
	if len(logs.SpeechLogs) > 0 {
		b.WriteString("\nSpeech Logs:\n")
		for _, l := range logs.SpeechLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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

	// Bowel logs — Bristol scale + boolean flags, no PHI
	if len(logs.BowelLogs) > 0 {
		b.WriteString("\nBowel Logs:\n")
		for _, l := range logs.BowelLogs {
			line := fmt.Sprintf("  %s:", RelativeDayLabel(l.LogDate, now))
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

	// Therapy logs — type + duration; progress_notes (free-text) intentionally dropped
	if len(logs.TherapyLogs) > 0 {
		b.WriteString("\nTherapy Logs:\n")
		for _, l := range logs.TherapyLogs {
			therapyType := "therapy"
			if l.TherapyType.Valid {
				therapyType = l.TherapyType.String
			}
			line := fmt.Sprintf("  %s: %s", RelativeDayLabel(l.LogDate, now), therapyType)
			if l.DurationMinutes != nil {
				line += fmt.Sprintf(" %dmin", *l.DurationMinutes)
			}
			// Free-text progress notes: Phase 1 default strips; Phase 3 opt-in includes.
			if includeNarrative && l.ProgressNotes.Valid && l.ProgressNotes.String != "" {
				line += fmt.Sprintf(" notes=%q", aiTruncate(l.ProgressNotes.String, 80))
			}
			b.WriteString(line + "\n")
		}
	}

	// Seizure logs — type + duration, no free-text
	if len(logs.SeizureLogs) > 0 {
		b.WriteString("\nSeizure Logs:\n")
		for _, l := range logs.SeizureLogs {
			seizureType := "unknown"
			if l.SeizureType.Valid {
				seizureType = l.SeizureType.String
			}
			line := fmt.Sprintf("  %s: type=%s", RelativeDayLabel(l.LogDate, now), seizureType)
			if l.DurationSeconds != nil {
				line += fmt.Sprintf(" duration=%ds", *l.DurationSeconds)
			}
			if l.RescueMedicationGiven {
				line += " rescue_med=yes"
			}
			b.WriteString(line + "\n")
		}
	}

	// Health events — type only; description (free-text) intentionally dropped
	if len(logs.HealthEventLogs) > 0 {
		b.WriteString("\nHealth Events:\n")
		for _, l := range logs.HealthEventLogs {
			eventType := "event"
			if l.EventType.Valid {
				eventType = l.EventType.String
			}
			line := fmt.Sprintf("  %s: %s", RelativeDayLabel(l.LogDate, now), eventType)
			// Free-text description: Phase 1 default strips; Phase 3 opt-in includes.
			if includeNarrative && l.Description.Valid && l.Description.String != "" {
				line += fmt.Sprintf(" — %s", aiTruncate(l.Description.String, 100))
			}
			b.WriteString(line + "\n")
		}
	}

	return b.String()
}

const tier12SystemPrompt = `You are a medical knowledge assistant for MyCareCompanion, a family care tracking app for children with autism spectrum disorder and related conditions. Given a child's de-identified profile (age band, diagnosis categories, medication classes — NO real names), provide relevant insights.

PRIVACY NOTE: the subject child is referred to as "[CHILD]" — this is a placeholder. The real first name will be substituted into your output client-side. ALWAYS use the literal string "[CHILD]" in your title and descriptions where you would normally refer to the child by name. Do not invent a name. Do not write "the child" — write "[CHILD]". Possessive form: "[CHILD]'s".

Generate TWO types of insights:

TIER 1 (Medical Knowledge): Known medication-class side effects, interactions between medication classes, age-appropriate developmental expectations, condition-category medical knowledge.
- Do NOT diagnose or recommend medication changes
- Always note families should consult their healthcare provider
- Set "tier": 1

TIER 2 (Research-Based Patterns): What published research and clinical practice suggest for children with similar profiles. Frame as "Research suggests..." or "Children with similar profiles often..."
- Draw on published autism/ADHD research and clinical practice patterns
- Be specific to the child's actual profile (age band, condition categories, medication classes)
- Set "tier": 2

Return a JSON array (no markdown, no code fences, just the array):
[{
  "tier": 1 or 2,
  "category": "medication|condition|development|safety|behavior|sleep|diet|sensory|social|therapy",
  "title": "Short descriptive title using [CHILD] placeholder (under 80 chars)",
  "simple_description": "Parent-friendly explanation (2-3 sentences). Use [CHILD] where you would name the child.",
  "detailed_description": "Clinical detail for advanced view",
  "confidence": 0.7 to 1.0,
  "alert_worthy": false,
  "alert_severity": "info"
}]

Return 2-3 insights total (1-2 Tier 1, 1 Tier 2). Only include genuinely relevant information, not generic advice.`

const tier3SystemPrompt = `You are a data analyst for MyCareCompanion, a care tracking app for children with autism. You are given a child's de-identified recent log data across multiple dimensions (behavior, sleep, diet, medication, sensory, social, therapy, bowel, seizure, health events, speech). Dates are relative ("Day-3" = three days ago). Medications appear by drug class only. The subject child is referred to as "[CHILD]".

PRIVACY NOTE: ALWAYS use the literal string "[CHILD]" where you would refer to the child by name. The real first name will be substituted client-side. Do not invent a name.

Analyze the data for:
1. Correlations between different log types (e.g., sleep quality vs next-day mood)
2. Trends over time (improving, declining, stable)
3. Notable events or outliers
4. Positive developments worth celebrating
5. Concerning patterns that warrant attention

Be SPECIFIC. Reference actual data values and relative day labels.
Example good insight: "[CHILD]'s mood averaged 7/10 on days following 9+ hours of sleep, vs 4/10 after less than 7 hours (observed 5 of 7 days)"
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

// claudeRequest is the body shape Bedrock expects for Anthropic Claude
// invocations. Differs from the Anthropic-direct API in two ways: the
// top-level field is `anthropic_version` (not `model` — model goes in the
// InvokeModel input wrapper), and the version string is the on-Bedrock
// constant `bedrock-2023-05-31`.
type claudeRequest struct {
	AnthropicVersion string          `json:"anthropic_version"`
	MaxTokens        int             `json:"max_tokens"`
	System           string          `json:"system"`
	Messages         []claudeMessage `json:"messages"`
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
	if s.bedrockClient == nil {
		return nil, fmt.Errorf("%w: bedrock client not initialized", ErrClaudeUnavailable)
	}

	// Apply a per-call deadline so a hung Bedrock request can't stall the
	// caller forever. AnalyzeChild's parent context is often a background
	// batch ctx with no deadline of its own.
	ctx, cancel := context.WithTimeout(ctx, claudeCallTimeout)
	defer cancel()

	reqBody := claudeRequest{
		AnthropicVersion: bedrockAnthropicVersion,
		MaxTokens:        s.config.MaxTokens,
		System:           systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	modelID := s.config.Model
	contentType := "application/json"
	accept := "application/json"
	out, err := s.bedrockClient.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
		ModelId:     &modelID,
		Body:        jsonBody,
		ContentType: &contentType,
		Accept:      &accept,
	})
	if err != nil {
		// Deadline exceeded or transient AWS throttling/unavailability —
		// caller treats both as "skip this run".
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrClaudeUnavailable, err)
		}
		msg := err.Error()
		if strings.Contains(msg, "ThrottlingException") ||
			strings.Contains(msg, "ServiceUnavailableException") ||
			strings.Contains(msg, "ModelTimeoutException") {
			return nil, fmt.Errorf("%w: %v", ErrClaudeUnavailable, err)
		}
		return nil, fmt.Errorf("bedrock InvokeModel: %w", err)
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(out.Body, &claudeResp); err != nil {
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

	// Parse the JSON array from the response text. Claude occasionally:
	//   (a) wraps the array in markdown code fences ("```json...```")
	//   (b) returns prose starting with "[CHILD]'s ..." — the literal
	//       placeholder we use causes naive byte-search to mistake it
	//       for the array opener.
	//   (c) emits the array followed by trailing prose or backticks.
	//
	// To handle all three, we find the first "[" that's followed (after
	// whitespace) by "{" — the only valid opener for our array-of-objects
	// schema — then walk forward tracking JSON string state so the matching
	// "]" we return is the actual array close and not a "]" inside a string.
	text := extractJSONArray(claudeResp.Content[0].Text)
	if text == "" {
		log.Printf("AI Insights: no JSON array found in response. Raw: %s", truncateLog(claudeResp.Content[0].Text, 500))
		return nil, fmt.Errorf("no JSON array in response")
	}

	var results []aiInsightResult
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		log.Printf("AI Insights: failed to parse response JSON: %v\nRaw: %s", err, truncateLog(text, 500))
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
// Returns (insights_created, alerts_created).
//
// firstName is used to substitute the "[CHILD]" placeholder Claude was instructed to use
// back to the real child name client-side, so the LLM never sees the actual name but
// the parent sees personable, named output.
func (s *AIInsightService) storeInsights(ctx context.Context, childID, familyID uuid.UUID, firstName string, results []aiInsightResult, recentInsights []models.Insight) (int, int) {
	insightsCreated := 0
	alertsCreated := 0

	for _, r := range results {
		// Substitute the name placeholder back to the real first name.
		r.Title = ApplyNamePlaceholder(r.Title, firstName)
		r.SimpleDescription = ApplyNamePlaceholder(r.SimpleDescription, firstName)
		r.DetailedDescription = ApplyNamePlaceholder(r.DetailedDescription, firstName)

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
			DedupeKey:           sql.NullString{String: aiInsightDedupeKey(r.Tier, r.Category, r.Title), Valid: true},
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

// extractJSONArray scans `text` for the first JSON array of objects and
// returns its substring bounds. Returns "" if no well-formed array can
// be located. Handles three problem cases observed in Claude output:
//
//   - Markdown fences wrapping the array (```json ... ```)
//   - Trailing prose or fence after the array
//   - Prose containing the literal "[CHILD]" placeholder that would
//     otherwise confuse a naive first-`[` / last-`]` extractor
//
// The walker tracks JSON string state so brackets inside strings (like
// "[CHILD]" inside a description value) don't affect depth counting.
func extractJSONArray(text string) string {
	// 1. Find the first '[' followed (after whitespace) by '{' — that's
	//    the start of an array of objects.
	parseStart := -1
	for i := 0; i < len(text); i++ {
		if text[i] != '[' {
			continue
		}
		j := i + 1
		for j < len(text) {
			c := text[j]
			if c == ' ' || c == '\n' || c == '\t' || c == '\r' {
				j++
				continue
			}
			break
		}
		if j < len(text) && text[j] == '{' {
			parseStart = i
			break
		}
	}
	if parseStart < 0 {
		return ""
	}

	// 2. Walk forward from parseStart tracking JSON string state to
	//    find the matching closing ']'.
	depth := 0
	inString := false
	escape := false
	for i := parseStart; i < len(text); i++ {
		c := text[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return text[parseStart : i+1]
			}
		}
	}
	// Unbalanced — likely a truncated response. Return "" to signal
	// failure rather than feeding the parser obviously-broken input.
	return ""
}

// truncateLog returns at most maxLen runes of s, with "..." appended
// when truncation occurs. Safe on s == "".
func truncateLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// isDuplicate checks if a similar title exists in recent insights
// aiInsightDedupeKey builds a structured key for an AI-generated insight.
// Format: ai:<tier>:<category>:<3-significant-tokens-of-title>. Stop-words
// and non-alphanumerics are stripped so phrasing variants ("Emma's sleep
// timing" vs "sleep timing for Emma") collapse onto roughly the same key.
//
// Not used for hard dedupe today — the in-memory isDuplicate is still the
// primary dedupe — but persisting the key gives the /admin/capacity page
// and any future analysis a stable identity per (child, tier, category,
// concept). Set alongside DedupeKey on every emitted insight.
func aiInsightDedupeKey(tier int, category, title string) string {
	stopWords := map[string]bool{
		"a": true, "an": true, "and": true, "or": true, "the": true,
		"of": true, "in": true, "on": true, "for": true, "to": true,
		"with": true, "at": true, "by": true, "is": true, "are": true,
		"this": true, "that": true, "as": true, "from": true, "your": true,
		"my": true, "our": true, "their": true, "his": true, "her": true,
		"its": true, "child": true, "childs": true, "kid": true, "kids": true,
	}
	var sig []string
	for _, w := range strings.Fields(strings.ToLower(title)) {
		var norm strings.Builder
		for _, r := range w {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				norm.WriteRune(r)
			}
		}
		token := norm.String()
		if token == "" || stopWords[token] {
			continue
		}
		sig = append(sig, token)
		if len(sig) == 3 {
			break
		}
	}
	categoryNorm := strings.ToLower(strings.TrimSpace(category))
	if categoryNorm == "" {
		categoryNorm = "general"
	}
	return fmt.Sprintf("ai:%d:%s:%s", tier, categoryNorm, strings.Join(sig, "-"))
}

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

