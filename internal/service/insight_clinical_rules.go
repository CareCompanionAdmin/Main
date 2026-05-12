package service

// insight_clinical_rules.go — clinical-rule layer for medication-related insights.
//
// This is the third leg of the internal-AI Phase 2 work. Where the
// auto-correlation scanner finds cross-stream relationships and the
// per-metric scanner finds within-stream patterns, this layer adds
// medical-domain knowledge — what specific medication classes are
// known to commonly cause, and what to watch for when a child starts a
// new medication.
//
// Three sources are designed (per the spec):
//
//	(1) FDA-auto:  drug_database.go pulls side-effect data live from
//	               api.fda.gov on every lookup. Implemented here.
//
//	(2) Admin-curated: a table of custom rules editable in the admin
//	                   portal. Schema + admin UI deferred to follow-up
//	                   (see TODO at bottom of this file).
//
//	(3) Change-point + med-start: when a change-point pattern fires and
//	                              an active medication was started near
//	                              that date, surface a "possible side
//	                              effect to discuss with your provider"
//	                              insight. Implemented here.
//
// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ClinicalRuleScanner produces medication-related insights from FDA
// label data and from change-point/medication-start co-occurrence.
type ClinicalRuleScanner struct {
	medRepo         repository.MedicationRepository
	correlationRepo repository.CorrelationRepository
	childRepo       repository.ChildRepository
	insightRepo     repository.InsightRepository
	alertService    *AlertService
	drugDB          *DrugDatabaseService

	// Tunables.
	medStartLookbackDays  int     // only consider meds started in last N days for FDA insights; default 60
	changePointAnchorDays int     // co-occurrence window (medstart ± N days = change point); default 14
	dedupeWindowDays      int     // skip if same med+kind was insightd within N days; default 14
	maxFDAInsightsPerMed  int     // cap per medication to avoid spam; default 2
	highConfidenceFloor   float64 // confidence floor for alert vs insight-only; default 0.7
}

func NewClinicalRuleScanner(
	medRepo repository.MedicationRepository,
	correlationRepo repository.CorrelationRepository,
	childRepo repository.ChildRepository,
	insightRepo repository.InsightRepository,
	alertService *AlertService,
	drugDB *DrugDatabaseService,
) *ClinicalRuleScanner {
	return &ClinicalRuleScanner{
		medRepo:               medRepo,
		correlationRepo:       correlationRepo,
		childRepo:             childRepo,
		insightRepo:           insightRepo,
		alertService:          alertService,
		drugDB:                drugDB,
		medStartLookbackDays:  60,
		changePointAnchorDays: 14,
		dedupeWindowDays:      14,
		maxFDAInsightsPerMed:  2,
		highConfidenceFloor:   0.7,
	}
}

// ScanChild produces clinical-rule insights for one child.
// Returns (insightsCreated, alertsCreated, err).
func (s *ClinicalRuleScanner) ScanChild(ctx context.Context, childID uuid.UUID) (int, int, error) {
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil || child == nil {
		return 0, 0, fmt.Errorf("load child: %w", err)
	}

	medications, err := s.medRepo.GetByChildID(ctx, childID, true)
	if err != nil {
		return 0, 0, fmt.Errorf("load medications: %w", err)
	}
	if len(medications) == 0 {
		return 0, 0, nil
	}

	// Change-point patterns within the anchor window — used by source 3.
	recentPatterns, err := s.correlationRepo.GetPatterns(ctx, childID, true)
	if err != nil {
		log.Printf("clinical-rules: fetch recent patterns failed: %v", err)
		recentPatterns = nil
	}

	insightsCreated := 0
	alertsCreated := 0

	for _, med := range medications {
		if !med.IsActive {
			continue
		}

		// Source 1: FDA-auto pull of side effects and pediatric warnings.
		nIns, nAlt := s.surfaceFDAFindings(ctx, child, &med)
		insightsCreated += nIns
		alertsCreated += nAlt

		// Source 3: medication start ↔ recent change-point co-occurrence.
		if med.StartDate.Valid {
			nIns, nAlt = s.surfaceMedStartCoincidence(ctx, child, &med, recentPatterns)
			insightsCreated += nIns
			alertsCreated += nAlt
		}
	}

	return insightsCreated, alertsCreated, nil
}

// surfaceFDAFindings creates insights from live FDA label data for a
// medication, scoped to children whose medication was started recently
// (so we don't spam parents about long-stable medications every day).
func (s *ClinicalRuleScanner) surfaceFDAFindings(
	ctx context.Context, child *models.Child, med *models.Medication,
) (int, int) {
	// Only emit FDA insights for recently started medications. For meds
	// the child has been on for months, the parent has seen the warnings.
	if med.StartDate.Valid {
		daysOn := int(time.Since(med.StartDate.Time).Hours() / 24)
		if daysOn > s.medStartLookbackDays {
			return 0, 0
		}
	}

	info, err := s.drugDB.LookupDrugWithDosage(ctx, med.Name, med.Dosage)
	if err != nil || info == nil {
		return 0, 0
	}

	insightsCreated := 0
	alertsCreated := 0

	// (a) Black-box warning — always surfaced if present, alertable.
	if info.BlackBoxWarning != nil {
		key := s.clinicalDedupeKey("fda-blackbox", med.Name, "")
		if s.shouldEmit(ctx, child.ID, key) {
			conf := 1.0
			ins := &models.Insight{
				ChildID:           &child.ID,
				FamilyID:          &child.FamilyID,
				Tier:              models.TierGlobalMedical,
				Category:          "medication",
				Title:             fmt.Sprintf("%s: FDA Black Box Warning", med.Name),
				SimpleDescription: fmt.Sprintf("%s has an FDA black box warning — the most serious warning the FDA issues. Make sure your prescriber has discussed this with you.", med.Name),
				ConfidenceScore:   &conf,
				IsActive:          true,
			}
			ins.DetailedDescription.String = *info.BlackBoxWarning
			ins.DetailedDescription.Valid = true
			ins.DedupeKey.String = key
			ins.DedupeKey.Valid = true
			if err := s.insightRepo.Create(ctx, ins); err == nil {
				insightsCreated++
				alert := &models.Alert{
					ChildID:         child.ID,
					FamilyID:        child.FamilyID,
					AlertType:       "medication",
					Severity:        models.AlertSeverityWarning,
					Status:          models.AlertStatusActive,
					Title:           ins.Title,
					Description:     ins.SimpleDescription,
					ConfidenceScore: &conf,
					SourceType:      models.CorrelationTypeFamilySpecific,
				}
				if err := s.alertService.Create(ctx, alert); err == nil {
					alertsCreated++
				}
			}
		}
	}

	// (b) Common side effects — surface up to maxFDAInsightsPerMed CUMULATIVE
	// within the dedupe window. Without this, the cap would be per-run and a
	// medication with many common side effects would gradually leak many
	// insights over consecutive scans.
	window := time.Duration(s.dedupeWindowDays) * 24 * time.Hour
	prefix := s.clinicalDedupeKeyPrefix("fda-side-effect", med.Name)
	existingSideEffects, _ := s.insightRepo.CountRecentByDedupeKeyPrefix(ctx, child.ID, prefix, window)
	remaining := s.maxFDAInsightsPerMed - existingSideEffects
	emitted := 0
	for _, se := range info.SideEffects {
		if emitted >= remaining {
			break
		}
		if se.Frequency != "common" {
			continue
		}
		key := s.clinicalDedupeKey("fda-side-effect", med.Name, se.Effect)
		if !s.shouldEmit(ctx, child.ID, key) {
			continue
		}
		conf := 0.85
		ins := &models.Insight{
			ChildID:           &child.ID,
			FamilyID:          &child.FamilyID,
			Tier:              models.TierGlobalMedical,
			Category:          "medication",
			Title:             fmt.Sprintf("%s: watch for %s", med.Name, se.Effect),
			SimpleDescription: fmt.Sprintf("%s is a commonly reported side effect of %s. Keep logging behaviors and sleep — if you notice a pattern, bring it to your prescriber.", strings.Title(se.Effect), med.Name),
			ConfidenceScore:   &conf,
			IsActive:          true,
		}
		ins.DetailedDescription.String = fmt.Sprintf("Severity: %s. From FDA labeling for %s.", se.Severity, med.Name)
		ins.DetailedDescription.Valid = true
		ins.DedupeKey.String = key
		ins.DedupeKey.Valid = true
		if err := s.insightRepo.Create(ctx, ins); err == nil {
			insightsCreated++
			emitted++
		}
	}

	// (c) Pediatric warnings — surface the first one (if any), capped.
	for _, pw := range info.PediatricWarnings {
		key := s.clinicalDedupeKey("fda-pediatric", med.Name, "")
		if !s.shouldEmit(ctx, child.ID, key) {
			break
		}
		conf := 0.9
		ins := &models.Insight{
			ChildID:           &child.ID,
			FamilyID:          &child.FamilyID,
			Tier:              models.TierGlobalMedical,
			Category:          "medication",
			Title:             fmt.Sprintf("%s: pediatric note", med.Name),
			SimpleDescription: truncateAt(pw, 200),
			ConfidenceScore:   &conf,
			IsActive:          true,
		}
		ins.DetailedDescription.String = pw
		ins.DetailedDescription.Valid = true
		ins.DedupeKey.String = key
		ins.DedupeKey.Valid = true
		if err := s.insightRepo.Create(ctx, ins); err == nil {
			insightsCreated++
		}
		break // one pediatric note per medication is enough
	}

	return insightsCreated, alertsCreated
}

// surfaceMedStartCoincidence checks whether a change-point pattern fired
// in any metric within ±changePointAnchorDays of the medication's start
// date. If so, surface a "possible side effect to discuss" insight.
func (s *ClinicalRuleScanner) surfaceMedStartCoincidence(
	ctx context.Context, child *models.Child, med *models.Medication,
	recentPatterns []models.FamilyPattern,
) (int, int) {
	if !med.StartDate.Valid {
		return 0, 0
	}
	startDate := med.StartDate.Time
	now := time.Now()

	// Only consider meds started within the last 90 days — older
	// medication starts that finally show in change-point analysis are
	// statistical noise more often than real causal links.
	if now.Sub(startDate).Hours()/24 > 90 {
		return 0, 0
	}

	for _, p := range recentPatterns {
		if p.PatternType != "change_point" {
			continue
		}
		if !p.FirstDetectedAt.Valid {
			continue
		}
		// LagHours on a change_point row holds "days ago" — translate
		// back to a calendar date.
		changeDate := now.AddDate(0, 0, -p.LagHours)
		gap := math.Abs(changeDate.Sub(startDate).Hours() / 24)
		if gap > float64(s.changePointAnchorDays) {
			continue
		}
		key := s.clinicalDedupeKey("med-start-changepoint", med.Name, p.InputFactor)
		if !s.shouldEmit(ctx, child.ID, key) {
			continue
		}

		conf := 0.7
		ins := &models.Insight{
			ChildID:           &child.ID,
			FamilyID:          &child.FamilyID,
			Tier:              models.TierFamilySpecific,
			Category:          "medication",
			Title:             fmt.Sprintf("%s and %s shifted around the same time", med.Name, friendlyMetricName(p.InputFactor)),
			SimpleDescription: fmt.Sprintf("%s was started %s, and %s shifted around the same time. This may be a side effect — worth mentioning to your prescriber.", med.Name, relativeDayPhrase(startDate, now), friendlyMetricName(p.InputFactor)),
			ConfidenceScore:   &conf,
			IsActive:          true,
		}
		ins.DetailedDescription.String = fmt.Sprintf("Change-point detected with z-score=%.1f at approximately Day-%d. Medication start: %s.", p.CorrelationStrength, p.LagHours, startDate.Format("2006-01-02"))
		ins.DetailedDescription.Valid = true
		ins.DedupeKey.String = key
		ins.DedupeKey.Valid = true

		if err := s.insightRepo.Create(ctx, ins); err == nil {
			alert := &models.Alert{
				ChildID:         child.ID,
				FamilyID:        child.FamilyID,
				AlertType:       "medication",
				Severity:        models.AlertSeverityWarning,
				Status:          models.AlertStatusActive,
				Title:           ins.Title,
				Description:     ins.SimpleDescription,
				ConfidenceScore: &conf,
				SourceType:      models.CorrelationTypeFamilySpecific,
			}
			alerts := 0
			if err := s.alertService.Create(ctx, alert); err == nil {
				alerts = 1
			}
			return 1, alerts
		}
	}
	return 0, 0
}

// normForKey lowercases + strips non-alphanumeric so spacing and
// punctuation variants of the same name collapse to one canonical token.
func normForKey(x string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(x) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clinicalDedupeKey builds the structured dedupe key for one clinical
// finding. Format: "clinical:<rule>:<med-normalized>[:<extra-normalized>]".
func (s *ClinicalRuleScanner) clinicalDedupeKey(rule, medName, extra string) string {
	key := "clinical:" + rule + ":" + normForKey(medName)
	if extra != "" {
		key += ":" + normForKey(extra)
	}
	return key
}

// clinicalDedupeKeyPrefix returns the prefix that all keys for a given
// (rule, medication) share — used for cumulative cap counts.
func (s *ClinicalRuleScanner) clinicalDedupeKeyPrefix(rule, medName string) string {
	return "clinical:" + rule + ":" + normForKey(medName) + ":"
}

// shouldEmit returns true when no insight with this dedupe_key has been
// created for this child inside the rolling dedupe window. A DB lookup
// error fails-open (returns true) so a transient error doesn't suppress
// alerts the parent would want to see.
func (s *ClinicalRuleScanner) shouldEmit(ctx context.Context, childID uuid.UUID, key string) bool {
	window := time.Duration(s.dedupeWindowDays) * 24 * time.Hour
	exists, err := s.insightRepo.ExistsRecentByDedupeKey(ctx, childID, key, window)
	if err != nil {
		log.Printf("clinical-rules: dedupe lookup failed for key=%q: %v", key, err)
		return true
	}
	return !exists
}

func truncateAt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func relativeDayPhrase(t, now time.Time) string {
	days := int(now.Sub(t).Hours() / 24)
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 14:
		return fmt.Sprintf("%d days ago", days)
	case days < 60:
		return fmt.Sprintf("about %d weeks ago", days/7)
	default:
		return fmt.Sprintf("about %d months ago", days/30)
	}
}

// friendlyMetricName maps internal metric keys (used in DataPoint maps)
// to parent-readable phrases.
func friendlyMetricName(metric string) string {
	switch metric {
	case "mood":
		return "mood"
	case "energy":
		return "energy"
	case "anxiety":
		return "anxiety"
	case "meltdowns":
		return "meltdown frequency"
	case "stimming":
		return "stimming episodes"
	case "sleep_minutes":
		return "sleep duration"
	case "night_wakings":
		return "night wakings"
	case "medication_adherence":
		return "medication adherence"
	case "bristol_scale":
		return "bowel pattern"
	case "bowel_count":
		return "bowel frequency"
	}
	return strings.ReplaceAll(metric, "_", " ")
}

// TODO (Phase 2 follow-up — Source 2 / Admin-curated rules):
//   - Migration creating clinical_rules table: id, name, description,
//     condition_dsl (text), trigger_template (text), severity,
//     is_active, created_at, updated_at.
//   - Repository methods List/Get/Create/Update/Delete on ClinicalRule.
//   - Admin handler at /admin/clinical-rules with HTMX-style CRUD UI.
//   - DSL evaluator that supports simple expressions like
//     "metric=mood AND trend=declining AND days>=14"
//     and emits insights matching the trigger_template.
//   - Wire into ScanChild as a third source after FDA and change-point.
//
// This is real work (~1 day) and deserves its own commit + design pass.
