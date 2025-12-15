package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

var (
	ErrCorrelationNotFound     = errors.New("correlation request not found")
	ErrPatternNotFound         = errors.New("pattern not found")
	ErrInsufficientData        = errors.New("insufficient data for correlation analysis")
	MinimumDataPointsRequired  = 14 // At least 2 weeks of data
	SignificantCorrelation     = 0.5
	HighConfidenceThreshold    = 0.7
)

type CorrelationService struct {
	correlationRepo repository.CorrelationRepository
	alertService    *AlertService
	childRepo       repository.ChildRepository
}

func NewCorrelationService(
	correlationRepo repository.CorrelationRepository,
	alertService *AlertService,
	childRepo repository.ChildRepository,
) *CorrelationService {
	return &CorrelationService{
		correlationRepo: correlationRepo,
		alertService:    alertService,
		childRepo:       childRepo,
	}
}

// Baseline management
func (s *CorrelationService) CreateBaseline(ctx context.Context, baseline *models.ChildBaseline) error {
	return s.correlationRepo.CreateBaseline(ctx, baseline)
}

func (s *CorrelationService) GetBaselines(ctx context.Context, childID uuid.UUID) ([]models.ChildBaseline, error) {
	return s.correlationRepo.GetBaselines(ctx, childID)
}

func (s *CorrelationService) GetBaseline(ctx context.Context, childID uuid.UUID, metricName string) (*models.ChildBaseline, error) {
	return s.correlationRepo.GetBaseline(ctx, childID, metricName)
}

func (s *CorrelationService) UpdateBaseline(ctx context.Context, baseline *models.ChildBaseline) error {
	return s.correlationRepo.UpdateBaseline(ctx, baseline)
}

// Calculate baselines from historical data
func (s *CorrelationService) CalculateBaselines(ctx context.Context, childID uuid.UUID) ([]models.ChildBaseline, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, -3, 0) // Last 3 months

	data, err := s.correlationRepo.GetCorrelationData(ctx, childID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	var baselines []models.ChildBaseline
	for metricName, dataPoints := range data {
		if len(dataPoints) < MinimumDataPointsRequired {
			continue
		}

		values := make([]float64, len(dataPoints))
		for i, dp := range dataPoints {
			values[i] = dp.Value
		}

		mean := calculateMean(values)
		stdDev := calculateStdDev(values, mean)

		baseline := models.ChildBaseline{
			ChildID:       childID,
			MetricName:    metricName,
			BaselineValue: mean,
			StdDeviation:  stdDev,
			SampleSize:    len(values),
		}

		// Check if baseline exists and update, otherwise create
		existing, err := s.correlationRepo.GetBaseline(ctx, childID, metricName)
		if err != nil {
			return nil, err
		}

		if existing != nil {
			baseline.ID = existing.ID
			if err := s.correlationRepo.UpdateBaseline(ctx, &baseline); err != nil {
				return nil, err
			}
		} else {
			if err := s.correlationRepo.CreateBaseline(ctx, &baseline); err != nil {
				return nil, err
			}
		}

		baselines = append(baselines, baseline)
	}

	return baselines, nil
}

// Correlation requests
func (s *CorrelationService) CreateCorrelationRequest(ctx context.Context, childID, requestedBy uuid.UUID, req *models.CreateCorrelationRequest) (*models.CorrelationRequest, error) {
	correlationReq := &models.CorrelationRequest{
		ChildID:       childID,
		RequestedBy:   requestedBy,
		InputFactors:  models.StringArray(req.InputFactors),
		OutputFactors: models.StringArray(req.OutputFactors),
	}

	if req.DateRangeStart != nil {
		correlationReq.DateRangeStart.Time = *req.DateRangeStart
		correlationReq.DateRangeStart.Valid = true
	}
	if req.DateRangeEnd != nil {
		correlationReq.DateRangeEnd.Time = *req.DateRangeEnd
		correlationReq.DateRangeEnd.Valid = true
	}

	if err := s.correlationRepo.CreateCorrelationRequest(ctx, correlationReq); err != nil {
		return nil, err
	}

	return correlationReq, nil
}

func (s *CorrelationService) GetCorrelationRequest(ctx context.Context, id uuid.UUID) (*models.CorrelationRequest, error) {
	req, err := s.correlationRepo.GetCorrelationRequest(ctx, id)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, ErrCorrelationNotFound
	}
	return req, nil
}

func (s *CorrelationService) GetCorrelationRequests(ctx context.Context, childID uuid.UUID, status *models.CorrelationStatus) ([]models.CorrelationRequest, error) {
	return s.correlationRepo.GetCorrelationRequests(ctx, childID, status)
}

// Run correlation analysis
func (s *CorrelationService) RunCorrelation(ctx context.Context, requestID uuid.UUID) error {
	req, err := s.correlationRepo.GetCorrelationRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil {
		return ErrCorrelationNotFound
	}

	// Update status to processing
	req.Status = models.CorrelationStatusProcessing
	now := time.Now()
	req.StartedAt.Time = now
	req.StartedAt.Valid = true
	if err := s.correlationRepo.UpdateCorrelationRequest(ctx, req); err != nil {
		return err
	}

	// Determine date range
	endDate := time.Now()
	startDate := endDate.AddDate(0, -3, 0)
	if req.DateRangeStart.Valid {
		startDate = req.DateRangeStart.Time
	}
	if req.DateRangeEnd.Valid {
		endDate = req.DateRangeEnd.Time
	}

	// Get correlation data
	data, err := s.correlationRepo.GetCorrelationData(ctx, req.ChildID, startDate, endDate)
	if err != nil {
		req.Status = models.CorrelationStatusFailed
		req.ErrorMessage.String = err.Error()
		req.ErrorMessage.Valid = true
		s.correlationRepo.UpdateCorrelationRequest(ctx, req)
		return err
	}

	// Run correlations for each input-output pair
	results := make(map[string]interface{})
	correlations := []map[string]interface{}{}

	for _, inputFactor := range req.InputFactors {
		inputData, hasInput := data[inputFactor]
		if !hasInput || len(inputData) < MinimumDataPointsRequired {
			continue
		}

		for _, outputFactor := range req.OutputFactors {
			outputData, hasOutput := data[outputFactor]
			if !hasOutput || len(outputData) < MinimumDataPointsRequired {
				continue
			}

			// Try different lag periods (0, 12, 24, 48 hours)
			lagHours := []int{0, 12, 24, 48}
			for _, lag := range lagHours {
				correlation, sampleSize := calculateCorrelation(inputData, outputData, lag)
				if sampleSize >= MinimumDataPointsRequired && math.Abs(correlation) >= SignificantCorrelation {
					correlations = append(correlations, map[string]interface{}{
						"input_factor":         inputFactor,
						"output_factor":        outputFactor,
						"correlation_strength": correlation,
						"lag_hours":            lag,
						"sample_size":          sampleSize,
					})
				}
			}
		}
	}

	results["correlations"] = correlations
	results["analyzed_at"] = time.Now()
	results["date_range"] = map[string]interface{}{
		"start": startDate,
		"end":   endDate,
	}

	// Update request with results
	req.Status = models.CorrelationStatusCompleted
	req.Results = models.JSONB(results)
	completedAt := time.Now()
	req.CompletedAt.Time = completedAt
	req.CompletedAt.Valid = true
	if err := s.correlationRepo.UpdateCorrelationRequest(ctx, req); err != nil {
		return err
	}

	// Create patterns from significant correlations
	for _, corr := range correlations {
		if corr["correlation_strength"].(float64) >= HighConfidenceThreshold {
			pattern := &models.FamilyPattern{
				ChildID:             req.ChildID,
				PatternType:         "correlation",
				InputFactor:         corr["input_factor"].(string),
				OutputFactor:        corr["output_factor"].(string),
				CorrelationStrength: corr["correlation_strength"].(float64),
				ConfidenceScore:     calculateConfidence(corr["sample_size"].(int), corr["correlation_strength"].(float64)),
				SampleSize:          corr["sample_size"].(int),
				LagHours:            corr["lag_hours"].(int),
			}
			pattern.FirstDetectedAt.Time = time.Now()
			pattern.FirstDetectedAt.Valid = true
			pattern.LastConfirmedAt.Time = time.Now()
			pattern.LastConfirmedAt.Valid = true

			if err := s.correlationRepo.CreatePattern(ctx, pattern); err != nil {
				continue
			}

			// Get child's family ID for alert
			child, err := s.childRepo.GetByID(ctx, req.ChildID)
			if err == nil && child != nil {
				s.alertService.CreatePatternDiscoveredAlert(ctx, req.ChildID, child.FamilyID, pattern)
			}
		}
	}

	return nil
}

// Pattern management
func (s *CorrelationService) GetPattern(ctx context.Context, id uuid.UUID) (*models.FamilyPattern, error) {
	pattern, err := s.correlationRepo.GetPattern(ctx, id)
	if err != nil {
		return nil, err
	}
	if pattern == nil {
		return nil, ErrPatternNotFound
	}
	return pattern, nil
}

func (s *CorrelationService) GetPatterns(ctx context.Context, childID uuid.UUID, activeOnly bool) ([]models.FamilyPattern, error) {
	return s.correlationRepo.GetPatterns(ctx, childID, activeOnly)
}

func (s *CorrelationService) UpdatePattern(ctx context.Context, pattern *models.FamilyPattern) error {
	return s.correlationRepo.UpdatePattern(ctx, pattern)
}

func (s *CorrelationService) DeletePattern(ctx context.Context, id uuid.UUID) error {
	return s.correlationRepo.DeletePattern(ctx, id)
}

// Clinical validations
func (s *CorrelationService) CreateValidation(ctx context.Context, validation *models.ClinicalValidation) error {
	return s.correlationRepo.CreateValidation(ctx, validation)
}

func (s *CorrelationService) GetValidations(ctx context.Context, childID uuid.UUID) ([]models.ClinicalValidation, error) {
	return s.correlationRepo.GetValidations(ctx, childID)
}

func (s *CorrelationService) GetValidation(ctx context.Context, id uuid.UUID) (*models.ClinicalValidation, error) {
	return s.correlationRepo.GetValidation(ctx, id)
}

// Insights page
func (s *CorrelationService) GetInsightsPage(ctx context.Context, childID uuid.UUID) (*models.InsightsPage, error) {
	return s.correlationRepo.GetInsightsPage(ctx, childID)
}

// Helper functions
func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	sumSquares := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(values)-1))
}

func calculateCorrelation(input, output []models.DataPoint, lagHours int) (float64, int) {
	// Create maps for quick lookup by date
	inputMap := make(map[string]float64)
	for _, dp := range input {
		key := dp.Date.Format("2006-01-02")
		inputMap[key] = dp.Value
	}

	// Pair values with lag
	var inputValues, outputValues []float64
	lagDuration := time.Duration(lagHours) * time.Hour

	for _, outDp := range output {
		inputDate := outDp.Date.Add(-lagDuration).Format("2006-01-02")
		if inputVal, exists := inputMap[inputDate]; exists {
			inputValues = append(inputValues, inputVal)
			outputValues = append(outputValues, outDp.Value)
		}
	}

	if len(inputValues) < MinimumDataPointsRequired {
		return 0, len(inputValues)
	}

	// Calculate Pearson correlation
	inputMean := calculateMean(inputValues)
	outputMean := calculateMean(outputValues)

	var numerator, inputSumSq, outputSumSq float64
	for i := 0; i < len(inputValues); i++ {
		inputDiff := inputValues[i] - inputMean
		outputDiff := outputValues[i] - outputMean
		numerator += inputDiff * outputDiff
		inputSumSq += inputDiff * inputDiff
		outputSumSq += outputDiff * outputDiff
	}

	if inputSumSq == 0 || outputSumSq == 0 {
		return 0, len(inputValues)
	}

	correlation := numerator / math.Sqrt(inputSumSq*outputSumSq)
	return correlation, len(inputValues)
}

func calculateConfidence(sampleSize int, correlationStrength float64) float64 {
	// Simple confidence calculation based on sample size and correlation strength
	sizeWeight := math.Min(float64(sampleSize)/100.0, 1.0)
	return math.Abs(correlationStrength) * sizeWeight
}

// GetTopPatterns returns the top N patterns by correlation strength
func (s *CorrelationService) GetTopPatterns(ctx context.Context, childID uuid.UUID, limit int) ([]models.FamilyPattern, error) {
	patterns, err := s.correlationRepo.GetPatterns(ctx, childID, true)
	if err != nil {
		return nil, err
	}

	// Sort by correlation strength descending
	sort.Slice(patterns, func(i, j int) bool {
		return math.Abs(patterns[i].CorrelationStrength) > math.Abs(patterns[j].CorrelationStrength)
	})

	if limit > 0 && len(patterns) > limit {
		patterns = patterns[:limit]
	}

	return patterns, nil
}
