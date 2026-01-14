package service

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatchService handles fetching metrics from AWS CloudWatch
type CloudWatchService struct {
	client         *cloudwatch.Client
	asgName        string
	rdsInstanceID  string
	region         string
}

// CloudWatchMetrics contains the metrics fetched from CloudWatch
type CloudWatchMetrics struct {
	CPUUtilization       float64
	DBStorageUtilization float64
	DBFreeStorageSpace   float64
	DBAllocatedStorage   float64
}

// NewCloudWatchService creates a new CloudWatch service
func NewCloudWatchService(asgName, rdsInstanceID, region string) (*CloudWatchService, error) {
	if region == "" {
		region = "us-east-1"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, err
	}

	return &CloudWatchService{
		client:        cloudwatch.NewFromConfig(cfg),
		asgName:       asgName,
		rdsInstanceID: rdsInstanceID,
		region:        region,
	}, nil
}

// GetMetrics fetches current metrics from CloudWatch
func (s *CloudWatchService) GetMetrics(ctx context.Context) (*CloudWatchMetrics, error) {
	metrics := &CloudWatchMetrics{}

	// Get EC2 CPU utilization (average across ASG instances)
	cpuUtil, err := s.getEC2CPUUtilization(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get EC2 CPU utilization: %v", err)
	} else {
		metrics.CPUUtilization = cpuUtil
	}

	// Get RDS storage metrics
	freeStorage, allocatedStorage, err := s.getRDSStorageMetrics(ctx)
	if err != nil {
		log.Printf("Warning: Failed to get RDS storage metrics: %v", err)
	} else {
		metrics.DBFreeStorageSpace = freeStorage
		metrics.DBAllocatedStorage = allocatedStorage
		if allocatedStorage > 0 {
			usedStorage := allocatedStorage - freeStorage
			metrics.DBStorageUtilization = (usedStorage / allocatedStorage) * 100
		}
	}

	return metrics, nil
}

func (s *CloudWatchService) getEC2CPUUtilization(ctx context.Context) (float64, error) {
	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("AutoScalingGroupName"),
				Value: aws.String(s.asgName),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300), // 5 minutes
		Statistics: []types.Statistic{types.StatisticAverage},
	}

	result, err := s.client.GetMetricStatistics(ctx, input)
	if err != nil {
		return 0, err
	}

	if len(result.Datapoints) == 0 {
		return 0, nil
	}

	// Return the most recent datapoint
	var latestTime time.Time
	var latestValue float64
	for _, dp := range result.Datapoints {
		if dp.Timestamp != nil && dp.Timestamp.After(latestTime) {
			latestTime = *dp.Timestamp
			if dp.Average != nil {
				latestValue = *dp.Average
			}
		}
	}

	return latestValue, nil
}

func (s *CloudWatchService) getRDSStorageMetrics(ctx context.Context) (freeSpace, allocatedSpace float64, err error) {
	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	// Get FreeStorageSpace
	freeInput := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String("AWS/RDS"),
		MetricName: aws.String("FreeStorageSpace"),
		Dimensions: []types.Dimension{
			{
				Name:  aws.String("DBInstanceIdentifier"),
				Value: aws.String(s.rdsInstanceID),
			},
		},
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300),
		Statistics: []types.Statistic{types.StatisticAverage},
	}

	freeResult, err := s.client.GetMetricStatistics(ctx, freeInput)
	if err != nil {
		return 0, 0, err
	}

	if len(freeResult.Datapoints) > 0 {
		// Find most recent
		var latestTime time.Time
		for _, dp := range freeResult.Datapoints {
			if dp.Timestamp != nil && dp.Timestamp.After(latestTime) {
				latestTime = *dp.Timestamp
				if dp.Average != nil {
					freeSpace = *dp.Average / (1024 * 1024 * 1024) // Convert bytes to GB
				}
			}
		}
	}

	// Get allocated storage from RDS - we'll use a fixed value or could query RDS API
	// For now, use the provisioned storage size (20 GB default for t3.small)
	allocatedSpace = 20.0 // GB - this should match your RDS instance's allocated storage

	return freeSpace, allocatedSpace, nil
}
