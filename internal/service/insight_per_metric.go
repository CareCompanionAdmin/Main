package service

// insight_per_metric.go — single-metric scanners running over all
// available log streams: anomaly detection, trend detection, and
// change-point detection. Whereas insight_autoscan.go finds cross-stream
// relationships, this scanner finds within-stream patterns.
//
// Each metric gets:
//   1. Anomaly check: is today's value an outlier vs the last 30 days?
//   2. Trend check: does the last 21 days show a statistically significant
//      slope?
//   3. Change-point check: did the mean shift dramatically at some recent
//      day, suggesting an inflection event (medication start, schedule
//      change, regression, etc.)?
//
// Findings become FamilyPattern records with pattern_type set to one of
// "anomaly", "trend", or "change_point" so the UI can render them with
// appropriate framing.
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

// PerMetricScanner runs anomaly, trend, and change-point analyses on
// each available time-series metric for a child.
type PerMetricScanner struct {
	correlationRepo repository.CorrelationRepository
	childRepo       repository.ChildRepository
	alertService    *AlertService

	// Tunables.
	lookbackDays       int     // how far back to pull data; default 30
	trendWindowDays    int     // how many days to fit a trend over; default 21
	anomalyZ           float64 // |z-score| above which today is "unusual"; default 2.5
	trendPThreshold    float64 // p-value below which a slope is significant; default 0.02
	changePointMinSeg  int     // smallest segment in change-point detector; default 5
	changePointThresh  float64 // z-score threshold for change-point; default 3.0
	changePointMaxAge  int     // only surface change points within the last N days; default 14
	dedupeWindowDays   int     // skip if same metric+kind was created within N days; default 7
}

// NewPerMetricScanner constructs a scanner with sensible defaults.
func NewPerMetricScanner(
	correlationRepo repository.CorrelationRepository,
	childRepo repository.ChildRepository,
	alertService *AlertService,
) *PerMetricScanner {
	return &PerMetricScanner{
		correlationRepo:   correlationRepo,
		childRepo:         childRepo,
		alertService:      alertService,
		lookbackDays:      30,
		trendWindowDays:   21,
		anomalyZ:          2.5,
		trendPThreshold:   0.02,
		changePointMinSeg: 5,
		changePointThresh: 3.0,
		changePointMaxAge: 14,
		dedupeWindowDays:  7,
	}
}

// ScanChild runs the three single-metric scans across every metric with
// sufficient data. Returns (patternsCreated, alertsCreated, err).
func (s *PerMetricScanner) ScanChild(ctx context.Context, childID uuid.UUID) (int, int, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -s.lookbackDays)

	data, err := s.correlationRepo.GetCorrelationData(ctx, childID, startDate, endDate)
	if err != nil {
		return 0, 0, fmt.Errorf("get correlation data: %w", err)
	}
	if len(data) == 0 {
		return 0, 0, nil
	}

	// Sorted metric names for deterministic order.
	metrics := make([]string, 0, len(data))
	for m := range data {
		metrics = append(metrics, m)
	}
	sort.Strings(metrics)

	child, err := s.childRepo.GetByID(ctx, childID)
	if err != nil || child == nil {
		log.Printf("per-metric scan: failed to load child %s: %v", childID, err)
		return 0, 0, nil
	}

	recentPatterns, err := s.correlationRepo.GetPatterns(ctx, childID, true)
	if err != nil {
		log.Printf("per-metric scan: failed to fetch existing patterns: %v", err)
		recentPatterns = nil
	}

	patternsCreated := 0
	alertsCreated := 0
	now := time.Now()

	for _, metric := range metrics {
		series := data[metric]
		if len(series) < MinimumDataPointsRequired {
			continue
		}
		// Sort by date ascending (the repo returns in order, but defensive).
		sort.Slice(series, func(i, j int) bool {
			return series[i].Date.Before(series[j].Date)
		})

		values := make([]float64, len(series))
		for i, dp := range series {
			values[i] = dp.Value
		}

		// 1. Anomaly check on the most recent observation.
		nP, nA := s.checkAnomaly(ctx, childID, child.FamilyID, metric, series, values, recentPatterns, now)
		patternsCreated += nP
		alertsCreated += nA

		// 2. Trend check across the trend window (or all data, whichever is shorter).
		nP, nA = s.checkTrend(ctx, childID, child.FamilyID, metric, series, values, recentPatterns, now)
		patternsCreated += nP
		alertsCreated += nA

		// 3. Change-point check.
		nP, nA = s.checkChangePoint(ctx, childID, child.FamilyID, metric, series, values, recentPatterns, now)
		patternsCreated += nP
		alertsCreated += nA
	}

	return patternsCreated, alertsCreated, nil
}

func (s *PerMetricScanner) checkAnomaly(
	ctx context.Context, childID, familyID uuid.UUID, metric string,
	series []models.DataPoint, values []float64,
	recent []models.FamilyPattern, now time.Time,
) (int, int) {
	// Compare the LATEST value against the distribution of the prior values.
	if len(values) < MinimumDataPointsRequired {
		return 0, 0
	}
	latest := values[len(values)-1]
	prior := values[:len(values)-1]
	z := ZScore(prior, latest)
	if math.Abs(z) < s.anomalyZ {
		return 0, 0
	}
	// Latest must be recent (within last 2 days) — stale spikes shouldn't fire.
	latestDate := series[len(series)-1].Date
	if now.Sub(latestDate) > 48*time.Hour {
		return 0, 0
	}
	if s.alreadyRecorded(recent, metric, "anomaly", now) {
		return 0, 0
	}

	pattern := &models.FamilyPattern{
		ChildID:             childID,
		PatternType:         "anomaly",
		InputFactor:         metric,
		OutputFactor:        metric,
		CorrelationStrength: z, // store z-score in correlation_strength field
		ConfidenceScore:     math.Min(math.Abs(z)/4.0, 1.0),
		SampleSize:          len(prior),
		LagHours:            0,
	}
	pattern.FirstDetectedAt.Time = now
	pattern.FirstDetectedAt.Valid = true
	pattern.LastConfirmedAt.Time = now
	pattern.LastConfirmedAt.Valid = true

	if err := s.correlationRepo.CreatePattern(ctx, pattern); err != nil {
		log.Printf("per-metric anomaly: create pattern failed: %v", err)
		return 0, 0
	}
	alerts := 0
	// Alert if the spike is severe enough.
	if math.Abs(z) >= s.anomalyZ+0.5 {
		if err := s.alertService.CreatePatternDiscoveredAlert(ctx, childID, familyID, pattern); err != nil {
			log.Printf("per-metric anomaly: create alert failed: %v", err)
		} else {
			alerts = 1
		}
	}
	return 1, alerts
}

func (s *PerMetricScanner) checkTrend(
	ctx context.Context, childID, familyID uuid.UUID, metric string,
	series []models.DataPoint, values []float64,
	recent []models.FamilyPattern, now time.Time,
) (int, int) {
	// Use the last trendWindowDays of data.
	window := s.trendWindowDays
	if len(values) < window {
		window = len(values)
	}
	if window < MinimumDataPointsRequired {
		return 0, 0
	}
	tail := values[len(values)-window:]
	// xs are day offsets from the start of the window (uniform 1..N).
	xs := make([]float64, window)
	for i := range xs {
		xs[i] = float64(i)
	}
	slope, _, _, pValue := LinearRegression(xs, tail)
	if pValue > s.trendPThreshold {
		return 0, 0
	}
	// Require a meaningful effect size — slope per day relative to the
	// metric's overall range over the window.
	rangeWindow := sliceMax(tail) - sliceMin(tail)
	if rangeWindow <= 0 {
		return 0, 0
	}
	relSlope := math.Abs(slope) * float64(window) / rangeWindow
	if relSlope < 0.5 {
		// Trend explains less than half the observed range — not worth flagging.
		return 0, 0
	}
	if s.alreadyRecorded(recent, metric, "trend", now) {
		return 0, 0
	}

	pattern := &models.FamilyPattern{
		ChildID:             childID,
		PatternType:         "trend",
		InputFactor:         metric,
		OutputFactor:        metric,
		CorrelationStrength: slope, // slope per day in original units
		ConfidenceScore:     1.0 - pValue,
		SampleSize:          window,
		LagHours:            0,
	}
	pattern.FirstDetectedAt.Time = now
	pattern.FirstDetectedAt.Valid = true
	pattern.LastConfirmedAt.Time = now
	pattern.LastConfirmedAt.Valid = true

	if err := s.correlationRepo.CreatePattern(ctx, pattern); err != nil {
		log.Printf("per-metric trend: create pattern failed: %v", err)
		return 0, 0
	}
	alerts := 0
	// Alert on highly significant trends.
	if pValue < 0.005 && relSlope >= 0.75 {
		if err := s.alertService.CreatePatternDiscoveredAlert(ctx, childID, familyID, pattern); err != nil {
			log.Printf("per-metric trend: create alert failed: %v", err)
		} else {
			alerts = 1
		}
	}
	return 1, alerts
}

func (s *PerMetricScanner) checkChangePoint(
	ctx context.Context, childID, familyID uuid.UUID, metric string,
	series []models.DataPoint, values []float64,
	recent []models.FamilyPattern, now time.Time,
) (int, int) {
	if len(values) < 2*s.changePointMinSeg {
		return 0, 0
	}
	k, score := DetectChangePoint(values, s.changePointMinSeg, s.changePointThresh)
	if k < 0 {
		return 0, 0
	}
	// Translate k back to a calendar date and check recency.
	changeDate := series[k].Date
	daysAgo := int(now.Sub(changeDate).Hours() / 24)
	if daysAgo > s.changePointMaxAge || daysAgo < 0 {
		return 0, 0
	}
	if s.alreadyRecorded(recent, metric, "change_point", now) {
		return 0, 0
	}

	pattern := &models.FamilyPattern{
		ChildID:             childID,
		PatternType:         "change_point",
		InputFactor:         metric,
		OutputFactor:        metric,
		CorrelationStrength: score, // z-score-like magnitude of the shift
		ConfidenceScore:     math.Min(score/6.0, 1.0),
		SampleSize:          len(values),
		LagHours:            daysAgo, // store the "days ago" of the change in LagHours
	}
	pattern.FirstDetectedAt.Time = now
	pattern.FirstDetectedAt.Valid = true
	pattern.LastConfirmedAt.Time = now
	pattern.LastConfirmedAt.Valid = true

	if err := s.correlationRepo.CreatePattern(ctx, pattern); err != nil {
		log.Printf("per-metric change-point: create pattern failed: %v", err)
		return 0, 0
	}
	alerts := 0
	if score >= s.changePointThresh+1 {
		if err := s.alertService.CreatePatternDiscoveredAlert(ctx, childID, familyID, pattern); err != nil {
			log.Printf("per-metric change-point: create alert failed: %v", err)
		} else {
			alerts = 1
		}
	}
	return 1, alerts
}

// alreadyRecorded returns true if a pattern of the same kind for the
// same metric was created within the dedupeWindow.
func (s *PerMetricScanner) alreadyRecorded(existing []models.FamilyPattern, metric, kind string, now time.Time) bool {
	cutoff := now.AddDate(0, 0, -s.dedupeWindowDays)
	for _, p := range existing {
		if p.PatternType != kind {
			continue
		}
		if p.InputFactor != metric {
			continue
		}
		if !p.LastConfirmedAt.Valid || p.LastConfirmedAt.Time.Before(cutoff) {
			continue
		}
		return true
	}
	return false
}

func sliceMax(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x > m {
			m = x
		}
	}
	return m
}

func sliceMin(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}
