package service

import (
	"context"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// CloudWatchService handles fetching metrics from AWS CloudWatch
type CloudWatchService struct {
	client            *cloudwatch.Client
	asgClient         *autoscaling.Client
	elbClient         *elasticloadbalancingv2.Client
	asgName           string
	rdsInstanceID     string
	elasticacheID     string
	albARN            string
	targetGroupARN    string
	region            string
}

// CloudWatchMetrics contains all metrics fetched from CloudWatch
type CloudWatchMetrics struct {
	// EC2/Compute metrics
	CPUUtilization       float64
	MemoryUtilization    float64 // Requires CloudWatch agent
	NetworkIn            float64 // Bytes
	NetworkOut           float64 // Bytes
	DiskReadOps          float64
	DiskWriteOps         float64

	// RDS metrics
	DBCPUUtilization     float64
	DBFreeStorageSpace   float64 // GB
	DBAllocatedStorage   float64 // GB
	DBStorageUtilization float64 // Percentage
	DBConnections        int
	DBReadIOPS           float64
	DBWriteIOPS          float64
	DBReadLatency        float64 // Seconds
	DBWriteLatency       float64 // Seconds
	DBFreeableMemory     float64 // Bytes

	// ElastiCache metrics
	CacheHitRate         float64
	CacheMissRate        float64
	CacheCPUUtilization  float64
	CacheMemoryUsage     float64 // Bytes
	CacheConnections     int
	CacheEvictions       int64

	// ALB metrics
	ALBRequestCount      float64
	ALBTargetResponseTime float64 // Seconds
	ALB5xxCount          float64
	ALB4xxCount          float64
	ALBHealthyHostCount  int
	ALBUnhealthyHostCount int

	// ASG metrics
	ASG                  *ASGStatus

	// Metadata
	FetchedAt            time.Time
	Errors               []string
}

// ASGStatus contains Auto Scaling Group status information
type ASGStatus struct {
	Name             string            `json:"name"`
	MinSize          int               `json:"min_size"`
	MaxSize          int               `json:"max_size"`
	DesiredCapacity  int               `json:"desired_capacity"`
	CurrentCapacity  int               `json:"current_capacity"`
	InServiceCount   int               `json:"in_service_count"`
	PendingCount     int               `json:"pending_count"`
	TerminatingCount int               `json:"terminating_count"`
	Instances        []ASGInstance     `json:"instances"`
	ScalingPolicies  []ScalingPolicy   `json:"scaling_policies"`
	RecentActivities []ScalingActivity `json:"recent_activities"`
	TargetHealth     []TargetHealth    `json:"target_health"`
	CapacityStatus   string            `json:"capacity_status"` // "at_min", "scaling", "optimal", "at_max"
	ScalingHeadroom  float64           `json:"scaling_headroom"` // percentage of capacity available before hitting max
}

// ASGInstance represents an instance in the ASG
type ASGInstance struct {
	InstanceID      string    `json:"instance_id"`
	HealthStatus    string    `json:"health_status"`
	LifecycleState  string    `json:"lifecycle_state"`
	AvailabilityZone string   `json:"availability_zone"`
	LaunchTime      time.Time `json:"launch_time,omitempty"`
}

// ScalingPolicy represents an ASG scaling policy
type ScalingPolicy struct {
	PolicyName   string  `json:"policy_name"`
	PolicyType   string  `json:"policy_type"`
	MetricType   string  `json:"metric_type"`
	TargetValue  float64 `json:"target_value"`
	CurrentValue float64 `json:"current_value,omitempty"`
	Status       string  `json:"status"` // "active", "cooling_down"
}

// ScalingActivity represents a scaling event
type ScalingActivity struct {
	ActivityID   string    `json:"activity_id"`
	Description  string    `json:"description"`
	Cause        string    `json:"cause"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time,omitempty"`
	StatusCode   string    `json:"status_code"`
	Progress     int       `json:"progress"`
}

// TargetHealth represents health status from load balancer perspective
type TargetHealth struct {
	InstanceID   string `json:"instance_id"`
	Port         int    `json:"port"`
	HealthState  string `json:"health_state"`
	Reason       string `json:"reason,omitempty"`
	Description  string `json:"description,omitempty"`
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
		asgClient:     autoscaling.NewFromConfig(cfg),
		elbClient:     elasticloadbalancingv2.NewFromConfig(cfg),
		asgName:       asgName,
		rdsInstanceID: rdsInstanceID,
		elasticacheID: "carecompanion-redis", // Can be configured
		region:        region,
	}, nil
}

// SetALBConfig sets ALB and target group ARNs for load balancer metrics
func (s *CloudWatchService) SetALBConfig(albARN, targetGroupARN string) {
	s.albARN = albARN
	s.targetGroupARN = targetGroupARN
}

// SetElastiCacheID sets the ElastiCache cluster ID
func (s *CloudWatchService) SetElastiCacheID(id string) {
	s.elasticacheID = id
}

// GetMetrics fetches current metrics from CloudWatch
func (s *CloudWatchService) GetMetrics(ctx context.Context) (*CloudWatchMetrics, error) {
	metrics := &CloudWatchMetrics{
		FetchedAt: time.Now(),
		Errors:    []string{},
	}

	// Fetch all metrics concurrently would be ideal, but for simplicity, fetch sequentially
	s.fetchEC2Metrics(ctx, metrics)
	s.fetchRDSMetrics(ctx, metrics)
	s.fetchElastiCacheMetrics(ctx, metrics)
	s.fetchALBMetrics(ctx, metrics)
	s.fetchASGStatus(ctx, metrics)

	return metrics, nil
}

func (s *CloudWatchService) fetchEC2Metrics(ctx context.Context, metrics *CloudWatchMetrics) {
	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	// CPU Utilization
	cpuVal, err := s.getMetricValue(ctx, "AWS/EC2", "CPUUtilization",
		[]types.Dimension{{Name: aws.String("AutoScalingGroupName"), Value: aws.String(s.asgName)}},
		startTime, endTime, types.StatisticAverage)
	if err != nil {
		metrics.Errors = append(metrics.Errors, "EC2 CPU: "+err.Error())
	} else {
		metrics.CPUUtilization = cpuVal
	}

	// Network In
	netIn, err := s.getMetricValue(ctx, "AWS/EC2", "NetworkIn",
		[]types.Dimension{{Name: aws.String("AutoScalingGroupName"), Value: aws.String(s.asgName)}},
		startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("EC2 NetworkIn: %v", err)
	} else {
		metrics.NetworkIn = netIn
	}

	// Network Out
	netOut, err := s.getMetricValue(ctx, "AWS/EC2", "NetworkOut",
		[]types.Dimension{{Name: aws.String("AutoScalingGroupName"), Value: aws.String(s.asgName)}},
		startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("EC2 NetworkOut: %v", err)
	} else {
		metrics.NetworkOut = netOut
	}
}

func (s *CloudWatchService) fetchRDSMetrics(ctx context.Context, metrics *CloudWatchMetrics) {
	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)
	dims := []types.Dimension{{Name: aws.String("DBInstanceIdentifier"), Value: aws.String(s.rdsInstanceID)}}

	// CPU Utilization
	cpuVal, err := s.getMetricValue(ctx, "AWS/RDS", "CPUUtilization", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		metrics.Errors = append(metrics.Errors, "RDS CPU: "+err.Error())
	} else {
		metrics.DBCPUUtilization = cpuVal
	}

	// Free Storage Space (bytes -> GB)
	freeStorage, err := s.getMetricValue(ctx, "AWS/RDS", "FreeStorageSpace", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		metrics.Errors = append(metrics.Errors, "RDS Storage: "+err.Error())
	} else {
		metrics.DBFreeStorageSpace = freeStorage / (1024 * 1024 * 1024)
	}

	// Database Connections
	connections, err := s.getMetricValue(ctx, "AWS/RDS", "DatabaseConnections", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS Connections: %v", err)
	} else {
		metrics.DBConnections = int(connections)
	}

	// Read IOPS
	readIOPS, err := s.getMetricValue(ctx, "AWS/RDS", "ReadIOPS", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS ReadIOPS: %v", err)
	} else {
		metrics.DBReadIOPS = readIOPS
	}

	// Write IOPS
	writeIOPS, err := s.getMetricValue(ctx, "AWS/RDS", "WriteIOPS", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS WriteIOPS: %v", err)
	} else {
		metrics.DBWriteIOPS = writeIOPS
	}

	// Read Latency
	readLatency, err := s.getMetricValue(ctx, "AWS/RDS", "ReadLatency", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS ReadLatency: %v", err)
	} else {
		metrics.DBReadLatency = readLatency
	}

	// Write Latency
	writeLatency, err := s.getMetricValue(ctx, "AWS/RDS", "WriteLatency", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS WriteLatency: %v", err)
	} else {
		metrics.DBWriteLatency = writeLatency
	}

	// Freeable Memory (bytes)
	freeMemory, err := s.getMetricValue(ctx, "AWS/RDS", "FreeableMemory", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("RDS FreeableMemory: %v", err)
	} else {
		metrics.DBFreeableMemory = freeMemory
	}

	// Calculate storage utilization (assuming 20GB allocated - should be configurable)
	metrics.DBAllocatedStorage = 20.0
	if metrics.DBAllocatedStorage > 0 && metrics.DBFreeStorageSpace > 0 {
		usedStorage := metrics.DBAllocatedStorage - metrics.DBFreeStorageSpace
		metrics.DBStorageUtilization = (usedStorage / metrics.DBAllocatedStorage) * 100
	}
}

func (s *CloudWatchService) fetchElastiCacheMetrics(ctx context.Context, metrics *CloudWatchMetrics) {
	if s.elasticacheID == "" {
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)
	dims := []types.Dimension{
		{Name: aws.String("CacheClusterId"), Value: aws.String(s.elasticacheID)},
		{Name: aws.String("CacheNodeId"), Value: aws.String("0001")},
	}

	// CPU Utilization
	cpuVal, err := s.getMetricValue(ctx, "AWS/ElastiCache", "CPUUtilization", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("ElastiCache CPU: %v", err)
	} else {
		metrics.CacheCPUUtilization = cpuVal
	}

	// Current Connections
	connections, err := s.getMetricValue(ctx, "AWS/ElastiCache", "CurrConnections", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("ElastiCache Connections: %v", err)
	} else {
		metrics.CacheConnections = int(connections)
	}

	// Cache Hits
	hits, err := s.getMetricValue(ctx, "AWS/ElastiCache", "CacheHits", dims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ElastiCache Hits: %v", err)
	}

	// Cache Misses
	misses, err := s.getMetricValue(ctx, "AWS/ElastiCache", "CacheMisses", dims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ElastiCache Misses: %v", err)
	}

	// Calculate hit rate
	if hits+misses > 0 {
		metrics.CacheHitRate = (hits / (hits + misses)) * 100
		metrics.CacheMissRate = (misses / (hits + misses)) * 100
	}

	// Bytes Used For Cache
	bytesUsed, err := s.getMetricValue(ctx, "AWS/ElastiCache", "BytesUsedForCache", dims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("ElastiCache BytesUsed: %v", err)
	} else {
		metrics.CacheMemoryUsage = bytesUsed
	}

	// Evictions
	evictions, err := s.getMetricValue(ctx, "AWS/ElastiCache", "Evictions", dims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ElastiCache Evictions: %v", err)
	} else {
		metrics.CacheEvictions = int64(evictions)
	}
}

func (s *CloudWatchService) fetchALBMetrics(ctx context.Context, metrics *CloudWatchMetrics) {
	if s.albARN == "" {
		return
	}

	endTime := time.Now()
	startTime := endTime.Add(-10 * time.Minute)

	// Extract the ALB name from ARN for dimensions
	// ARN format: arn:aws:elasticloadbalancing:region:account:loadbalancer/app/name/id
	albDims := []types.Dimension{{Name: aws.String("LoadBalancer"), Value: aws.String(s.albARN)}}

	// Request Count
	reqCount, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "RequestCount", albDims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ALB RequestCount: %v", err)
	} else {
		metrics.ALBRequestCount = reqCount
	}

	// Target Response Time
	respTime, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "TargetResponseTime", albDims, startTime, endTime, types.StatisticAverage)
	if err != nil {
		log.Printf("ALB ResponseTime: %v", err)
	} else {
		metrics.ALBTargetResponseTime = respTime
	}

	// 5xx Count
	count5xx, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "HTTPCode_ELB_5XX_Count", albDims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ALB 5xx: %v", err)
	} else {
		metrics.ALB5xxCount = count5xx
	}

	// 4xx Count
	count4xx, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "HTTPCode_ELB_4XX_Count", albDims, startTime, endTime, types.StatisticSum)
	if err != nil {
		log.Printf("ALB 4xx: %v", err)
	} else {
		metrics.ALB4xxCount = count4xx
	}

	// Target health requires target group
	if s.targetGroupARN != "" {
		tgDims := []types.Dimension{
			{Name: aws.String("LoadBalancer"), Value: aws.String(s.albARN)},
			{Name: aws.String("TargetGroup"), Value: aws.String(s.targetGroupARN)},
		}

		healthy, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "HealthyHostCount", tgDims, startTime, endTime, types.StatisticAverage)
		if err != nil {
			log.Printf("ALB HealthyHosts: %v", err)
		} else {
			metrics.ALBHealthyHostCount = int(healthy)
		}

		unhealthy, err := s.getMetricValue(ctx, "AWS/ApplicationELB", "UnHealthyHostCount", tgDims, startTime, endTime, types.StatisticAverage)
		if err != nil {
			log.Printf("ALB UnhealthyHosts: %v", err)
		} else {
			metrics.ALBUnhealthyHostCount = int(unhealthy)
		}
	}
}

func (s *CloudWatchService) getMetricValue(ctx context.Context, namespace, metricName string, dimensions []types.Dimension, startTime, endTime time.Time, statistic types.Statistic) (float64, error) {
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(namespace),
		MetricName: aws.String(metricName),
		Dimensions: dimensions,
		StartTime:  aws.Time(startTime),
		EndTime:    aws.Time(endTime),
		Period:     aws.Int32(300), // 5 minutes
		Statistics: []types.Statistic{statistic},
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
			switch statistic {
			case types.StatisticAverage:
				if dp.Average != nil {
					latestValue = *dp.Average
				}
			case types.StatisticSum:
				if dp.Sum != nil {
					latestValue = *dp.Sum
				}
			case types.StatisticMaximum:
				if dp.Maximum != nil {
					latestValue = *dp.Maximum
				}
			case types.StatisticMinimum:
				if dp.Minimum != nil {
					latestValue = *dp.Minimum
				}
			}
		}
	}

	return latestValue, nil
}

// fetchASGStatus fetches Auto Scaling Group status using the ASG and ELB APIs
func (s *CloudWatchService) fetchASGStatus(ctx context.Context, metrics *CloudWatchMetrics) {
	if s.asgName == "" {
		return
	}

	asgStatus := &ASGStatus{
		Name:             s.asgName,
		Instances:        []ASGInstance{},
		ScalingPolicies:  []ScalingPolicy{},
		RecentActivities: []ScalingActivity{},
		TargetHealth:     []TargetHealth{},
	}

	// Get ASG details
	asgResult, err := s.asgClient.DescribeAutoScalingGroups(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: []string{s.asgName},
	})
	if err != nil {
		log.Printf("ASG DescribeAutoScalingGroups: %v", err)
		metrics.Errors = append(metrics.Errors, "ASG details: "+err.Error())
	} else if len(asgResult.AutoScalingGroups) > 0 {
		asg := asgResult.AutoScalingGroups[0]
		asgStatus.MinSize = int(aws.ToInt32(asg.MinSize))
		asgStatus.MaxSize = int(aws.ToInt32(asg.MaxSize))
		asgStatus.DesiredCapacity = int(aws.ToInt32(asg.DesiredCapacity))

		// Count instances by state
		for _, inst := range asg.Instances {
			instance := ASGInstance{
				InstanceID:       aws.ToString(inst.InstanceId),
				HealthStatus:     aws.ToString(inst.HealthStatus),
				LifecycleState:   string(inst.LifecycleState),
				AvailabilityZone: aws.ToString(inst.AvailabilityZone),
			}
			asgStatus.Instances = append(asgStatus.Instances, instance)

			switch inst.LifecycleState {
			case "InService":
				asgStatus.InServiceCount++
			case "Pending", "Pending:Wait", "Pending:Proceed":
				asgStatus.PendingCount++
			case "Terminating", "Terminating:Wait", "Terminating:Proceed":
				asgStatus.TerminatingCount++
			}
		}
		asgStatus.CurrentCapacity = len(asg.Instances)

		// Calculate capacity status
		if asgStatus.CurrentCapacity <= asgStatus.MinSize {
			asgStatus.CapacityStatus = "at_min"
		} else if asgStatus.CurrentCapacity >= asgStatus.MaxSize {
			asgStatus.CapacityStatus = "at_max"
		} else if asgStatus.PendingCount > 0 || asgStatus.TerminatingCount > 0 {
			asgStatus.CapacityStatus = "scaling"
		} else {
			asgStatus.CapacityStatus = "optimal"
		}

		// Calculate scaling headroom (percentage of capacity available)
		if asgStatus.MaxSize > 0 {
			asgStatus.ScalingHeadroom = float64(asgStatus.MaxSize-asgStatus.CurrentCapacity) / float64(asgStatus.MaxSize) * 100
		}
	}

	// Get scaling policies
	policiesResult, err := s.asgClient.DescribePolicies(ctx, &autoscaling.DescribePoliciesInput{
		AutoScalingGroupName: aws.String(s.asgName),
	})
	if err != nil {
		log.Printf("ASG DescribePolicies: %v", err)
	} else {
		for _, policy := range policiesResult.ScalingPolicies {
			sp := ScalingPolicy{
				PolicyName: aws.ToString(policy.PolicyName),
				PolicyType: aws.ToString(policy.PolicyType),
				Status:     "active",
			}
			if policy.TargetTrackingConfiguration != nil {
				if policy.TargetTrackingConfiguration.TargetValue != nil {
					sp.TargetValue = *policy.TargetTrackingConfiguration.TargetValue
				}
				if policy.TargetTrackingConfiguration.PredefinedMetricSpecification != nil {
					sp.MetricType = string(policy.TargetTrackingConfiguration.PredefinedMetricSpecification.PredefinedMetricType)
				}
				// Set current CPU value for comparison
				if sp.MetricType == "ASGAverageCPUUtilization" {
					sp.CurrentValue = metrics.CPUUtilization
				}
			}
			asgStatus.ScalingPolicies = append(asgStatus.ScalingPolicies, sp)
		}
	}

	// Get recent scaling activities
	activitiesResult, err := s.asgClient.DescribeScalingActivities(ctx, &autoscaling.DescribeScalingActivitiesInput{
		AutoScalingGroupName: aws.String(s.asgName),
		MaxRecords:          aws.Int32(10),
	})
	if err != nil {
		log.Printf("ASG DescribeScalingActivities: %v", err)
	} else {
		for _, activity := range activitiesResult.Activities {
			sa := ScalingActivity{
				ActivityID:  aws.ToString(activity.ActivityId),
				Description: aws.ToString(activity.Description),
				Cause:       aws.ToString(activity.Cause),
				StatusCode:  string(activity.StatusCode),
				Progress:    int(aws.ToInt32(activity.Progress)),
			}
			if activity.StartTime != nil {
				sa.StartTime = *activity.StartTime
			}
			if activity.EndTime != nil {
				sa.EndTime = *activity.EndTime
			}
			asgStatus.RecentActivities = append(asgStatus.RecentActivities, sa)
		}
	}

	// Get target health from load balancer
	if s.targetGroupARN != "" {
		healthResult, err := s.elbClient.DescribeTargetHealth(ctx, &elasticloadbalancingv2.DescribeTargetHealthInput{
			TargetGroupArn: aws.String(s.targetGroupARN),
		})
		if err != nil {
			log.Printf("ELB DescribeTargetHealth: %v", err)
		} else {
			for _, th := range healthResult.TargetHealthDescriptions {
				target := TargetHealth{
					InstanceID:  aws.ToString(th.Target.Id),
					Port:        int(aws.ToInt32(th.Target.Port)),
					HealthState: string(th.TargetHealth.State),
				}
				if th.TargetHealth.Reason != "" {
					target.Reason = string(th.TargetHealth.Reason)
				}
				if th.TargetHealth.Description != nil {
					target.Description = *th.TargetHealth.Description
				}
				asgStatus.TargetHealth = append(asgStatus.TargetHealth, target)
			}
		}
	}

	metrics.ASG = asgStatus
}
