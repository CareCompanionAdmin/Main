package service

// insight_autoscan.go — exhaustive automatic correlation scanner.
//
// Internal-AI Phase 2, part 1. Where correlation_service.RunCorrelation
// runs on demand for user-specified factor pairs, this scanner runs on
// a schedule across ALL pairs of available metrics for a child,
// applies multiple-comparison correction so we don't chase false
// discoveries, and produces FamilyPattern records for surviving
// statistically significant relationships.
//
// This is the closest thing to "internal AI" we get without an LLM:
// open-ended pattern discovery driven by data, not predefined hypotheses.
// We don't tell it sleep_quality should affect next-day mood — if the
// data supports that relationship, the scanner finds it on its own.
//
// See docs/superpowers/specs/2026-05-11-ai-phi-stripping-and-internal-expansion.md

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// AutoCorrelationScanner runs exhaustive correlation analysis on a child's
// log data and produces FamilyPattern records for discoveries that
// survive Benjamini-Hochberg FDR correction.
type AutoCorrelationScanner struct {
	correlationRepo repository.CorrelationRepository
	childRepo       repository.ChildRepository
	alertService    *AlertService

	// lookbackDays controls how far back we pull data. Default 90.
	lookbackDays int

	// lagsHours is the set of lag values tested for each ordered pair.
	// Defaults to {0, 12, 24, 48}. Set in NewAutoCorrelationScanner.
	lagsHours []int

	// fdrAlpha is the false-discovery-rate target. Default 0.05.
	fdrAlpha float64

	// minAbsR is the minimum absolute Pearson r to consider a relationship
	// clinically interesting even if statistically significant. Default 0.5.
	minAbsR float64

	// dedupeWindow is how recently a near-identical pattern must have been
	// recorded to skip creating a fresh one. Default 7 days.
	dedupeWindow time.Duration
}

// NewAutoCorrelationScanner constructs a scanner with sensible defaults.
func NewAutoCorrelationScanner(
	correlationRepo repository.CorrelationRepository,
	childRepo repository.ChildRepository,
	alertService *AlertService,
) *AutoCorrelationScanner {
	return &AutoCorrelationScanner{
		correlationRepo: correlationRepo,
		childRepo:       childRepo,
		alertService:    alertService,
		lookbackDays:    90,
		lagsHours:       []int{0, 12, 24, 48},
		fdrAlpha:        0.05,
		minAbsR:         SignificantCorrelation, // 0.5 from correlation_service.go
		dedupeWindow:    7 * 24 * time.Hour,
	}
}

// candidate represents one (input, output, lag) result before FDR correction.
type candidate struct {
	input      string
	output     string
	lag        int
	r          float64
	sampleSize int
	pValue     float64
}

// ScanChild runs an exhaustive correlation analysis for one child.
// Returns (patternsCreated, alertsCreated, err).
//
// Skipping conditions:
//   - Pair where input==output (a metric correlated with itself)
//   - Pair where either side has fewer than MinimumDataPointsRequired points
//   - Result with |r| < minAbsR after FDR survival
//   - Pattern that already exists for this (input, output, lag) within dedupeWindow
//
// The scanner only fires when a child has at least 14 days of any metric data;
// children just starting out won't see noisy false-positive patterns.
func (s *AutoCorrelationScanner) ScanChild(ctx context.Context, childID uuid.UUID) (int, int, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -s.lookbackDays)

	data, err := s.correlationRepo.GetCorrelationData(ctx, childID, startDate, endDate)
	if err != nil {
		return 0, 0, fmt.Errorf("get correlation data: %w", err)
	}
	if len(data) < 2 {
		// Need at least two streams to correlate.
		return 0, 0, nil
	}

	// Enumerate factor names. Sort for deterministic iteration order
	// (matters for tests and for stable de-duplication keys).
	factors := make([]string, 0, len(data))
	for name := range data {
		if len(data[name]) >= MinimumDataPointsRequired {
			factors = append(factors, name)
		}
	}
	sort.Strings(factors)
	if len(factors) < 2 {
		return 0, 0, nil
	}

	// Build candidate list across all ordered pairs and lag windows.
	var candidates []candidate
	for _, in := range factors {
		for _, out := range factors {
			if in == out {
				continue
			}
			for _, lag := range s.lagsHours {
				r, n := calculateCorrelation(data[in], data[out], lag)
				if n < MinimumDataPointsRequired {
					continue
				}
				p := PearsonPValue(r, n)
				candidates = append(candidates, candidate{
					input:      in,
					output:     out,
					lag:        lag,
					r:          r,
					sampleSize: n,
					pValue:     p,
				})
			}
		}
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	// Apply Benjamini-Hochberg FDR correction across ALL hypotheses tested.
	pValues := make([]float64, len(candidates))
	for i, c := range candidates {
		pValues[i] = c.pValue
	}
	survives := BenjaminiHochberg(pValues, s.fdrAlpha)

	// Retrieve recent patterns once for de-duplication.
	recentPatterns, err := s.correlationRepo.GetPatterns(ctx, childID, true)
	if err != nil {
		log.Printf("auto-correlation: failed to fetch existing patterns for dedupe: %v", err)
		recentPatterns = nil
	}

	// Resolve child for family_id (needed by alert).
	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil || child == nil {
		log.Printf("auto-correlation: failed to load child %s: %v", childID, err)
		return 0, 0, nil
	}

	patternsCreated := 0
	alertsCreated := 0
	now := time.Now()

	for i, c := range candidates {
		if !survives[i] || math.Abs(c.r) < s.minAbsR {
			continue
		}
		if s.alreadyRecorded(recentPatterns, c, now) {
			continue
		}

		pattern := &models.FamilyPattern{
			ChildID:             childID,
			PatternType:         "auto_correlation",
			InputFactor:         c.input,
			OutputFactor:        c.output,
			CorrelationStrength: c.r,
			ConfidenceScore:     calculateConfidence(c.sampleSize, c.r),
			SampleSize:          c.sampleSize,
			LagHours:            c.lag,
		}
		pattern.FirstDetectedAt.Time = now
		pattern.FirstDetectedAt.Valid = true
		pattern.LastConfirmedAt.Time = now
		pattern.LastConfirmedAt.Valid = true

		if err := s.correlationRepo.CreatePattern(ctx, pattern); err != nil {
			log.Printf("auto-correlation: failed to create pattern: %v", err)
			continue
		}
		patternsCreated++

		// Only alert on strong findings — weaker ones still exist as
		// patterns the parent can browse, but don't push-notify.
		if math.Abs(c.r) >= HighConfidenceThreshold {
			if err := s.alertService.CreatePatternDiscoveredAlert(ctx, childID, child.FamilyID, pattern); err != nil {
				log.Printf("auto-correlation: failed to create alert: %v", err)
			} else {
				alertsCreated++
			}
		}
	}

	return patternsCreated, alertsCreated, nil
}

// alreadyRecorded returns true if a near-identical pattern was already
// stored within the dedupe window. "Near-identical" means same input,
// same output, same lag, same correlation sign, and detected recently.
func (s *AutoCorrelationScanner) alreadyRecorded(existing []models.FamilyPattern, c candidate, now time.Time) bool {
	cutoff := now.Add(-s.dedupeWindow)
	for _, p := range existing {
		if p.InputFactor != c.input || p.OutputFactor != c.output {
			continue
		}
		if p.LagHours != c.lag {
			continue
		}
		// Same direction (both positive or both negative correlations).
		if (p.CorrelationStrength >= 0) != (c.r >= 0) {
			continue
		}
		if !p.LastConfirmedAt.Valid || p.LastConfirmedAt.Time.Before(cutoff) {
			continue
		}
		return true
	}
	return false
}
