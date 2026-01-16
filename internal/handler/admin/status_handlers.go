package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"carecompanion/internal/models"
	"carecompanion/internal/service"
)

// GetInfrastructureStatus returns comprehensive infrastructure metrics with actionable alerts
func (h *Handler) GetInfrastructureStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	status := &models.InfrastructureStatus{
		LastUpdated: now,
		Alerts:      []models.InfrastructureAlert{},
	}

	// Use a timeout context for database calls
	dbCtx, dbCancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer dbCancel()

	// Get application metrics from database
	errorCount, _ := h.adminRepo.GetUnacknowledgedErrorCount(dbCtx)
	cachedMetrics, _ := h.adminRepo.GetCachedMetrics(dbCtx)

	// Initialize with defaults
	status.Application = models.ApplicationMetrics{
		ErrorCount5m: errorCount,
		Status:       models.HealthStatusHealthy,
	}

	status.Compute = models.ComputeMetrics{
		InstanceCount:    1,
		HealthyInstances: 1,
		Status:           models.HealthStatusHealthy,
	}

	status.Database = models.DatabaseMetrics{
		Status: models.HealthStatusHealthy,
	}

	status.Cache = models.CacheMetrics{
		Available: false,
		Status:    models.HealthStatusHealthy,
	}

	// Use cached metrics if available
	if cachedMetrics != nil {
		status.Application.AverageResponseTimeMs = cachedMetrics.AvgResponseTimeMs
		if cachedMetrics.CPUUtilization > 0 {
			status.Compute.CPUUtilization = cachedMetrics.CPUUtilization
		}
		if cachedMetrics.DBStorageUtilization > 0 {
			status.Database.StorageUtilization = cachedMetrics.DBStorageUtilization
		}
	}

	// Get real-time metrics from CloudWatch with timeout
	if h.cloudwatchService != nil {
		// Use a separate timeout for CloudWatch calls (10 seconds max)
		cwCtx, cwCancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cwCancel()

		log.Println("Fetching CloudWatch metrics...")
		cwMetrics, err := h.cloudwatchService.GetMetrics(cwCtx)
		if err != nil {
			log.Printf("CloudWatch GetMetrics error: %v", err)
		} else if cwMetrics != nil {
			log.Printf("CloudWatch metrics fetched: ASG=%v, Errors=%v", cwMetrics.ASG != nil, cwMetrics.Errors)
			populateFromCloudWatch(status, cwMetrics, now)
		}
	} else {
		log.Println("CloudWatch service not initialized")
	}

	// Generate alerts based on metrics
	generateAlerts(status, errorCount, now)

	// Calculate overall health
	status.OverallHealth, status.HealthSummary, status.AlertCount, status.WarningCount = calculateOverallHealth(status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// populateFromCloudWatch fills in status from CloudWatch metrics
func populateFromCloudWatch(status *models.InfrastructureStatus, cw *service.CloudWatchMetrics, now time.Time) {
	// Compute metrics
	status.Compute.CPUUtilization = cw.CPUUtilization
	status.Compute.MemoryUtilization = cw.MemoryUtilization
	if cw.NetworkIn > 0 || cw.NetworkOut > 0 {
		// Convert to MB for display
		status.Compute.StatusMessage = fmt.Sprintf("Network: %.1f MB in, %.1f MB out (10m)",
			cw.NetworkIn/(1024*1024), cw.NetworkOut/(1024*1024))
	}

	// Map ASG status from CloudWatch service to models
	if cw.ASG != nil {
		status.ASG = &models.ASGStatus{
			Name:             cw.ASG.Name,
			MinSize:          cw.ASG.MinSize,
			MaxSize:          cw.ASG.MaxSize,
			DesiredCapacity:  cw.ASG.DesiredCapacity,
			CurrentCapacity:  cw.ASG.CurrentCapacity,
			InServiceCount:   cw.ASG.InServiceCount,
			PendingCount:     cw.ASG.PendingCount,
			TerminatingCount: cw.ASG.TerminatingCount,
			CapacityStatus:   cw.ASG.CapacityStatus,
			ScalingHeadroom:  cw.ASG.ScalingHeadroom,
			Instances:        make([]models.ASGInstance, 0, len(cw.ASG.Instances)),
			ScalingPolicies:  make([]models.ScalingPolicy, 0, len(cw.ASG.ScalingPolicies)),
			RecentActivities: make([]models.ScalingActivity, 0, len(cw.ASG.RecentActivities)),
			TargetHealth:     make([]models.TargetHealth, 0, len(cw.ASG.TargetHealth)),
		}

		// Map instances
		for _, inst := range cw.ASG.Instances {
			status.ASG.Instances = append(status.ASG.Instances, models.ASGInstance{
				InstanceID:       inst.InstanceID,
				HealthStatus:     inst.HealthStatus,
				LifecycleState:   inst.LifecycleState,
				AvailabilityZone: inst.AvailabilityZone,
				LaunchTime:       inst.LaunchTime,
			})
		}

		// Map scaling policies
		for _, policy := range cw.ASG.ScalingPolicies {
			status.ASG.ScalingPolicies = append(status.ASG.ScalingPolicies, models.ScalingPolicy{
				PolicyName:   policy.PolicyName,
				PolicyType:   policy.PolicyType,
				MetricType:   policy.MetricType,
				TargetValue:  policy.TargetValue,
				CurrentValue: policy.CurrentValue,
				Status:       policy.Status,
			})
		}

		// Map recent activities
		for _, activity := range cw.ASG.RecentActivities {
			status.ASG.RecentActivities = append(status.ASG.RecentActivities, models.ScalingActivity{
				ActivityID:  activity.ActivityID,
				Description: activity.Description,
				Cause:       activity.Cause,
				StartTime:   activity.StartTime,
				EndTime:     activity.EndTime,
				StatusCode:  activity.StatusCode,
				Progress:    activity.Progress,
			})
		}

		// Map target health
		for _, th := range cw.ASG.TargetHealth {
			status.ASG.TargetHealth = append(status.ASG.TargetHealth, models.TargetHealth{
				InstanceID:  th.InstanceID,
				Port:        th.Port,
				HealthState: th.HealthState,
				Reason:      th.Reason,
				Description: th.Description,
			})
		}

		// Update compute metrics from ASG data
		status.Compute.InstanceCount = cw.ASG.CurrentCapacity
		status.Compute.HealthyInstances = cw.ASG.InServiceCount
		status.Compute.UnhealthyInstances = cw.ASG.CurrentCapacity - cw.ASG.InServiceCount
	}

	// Database metrics
	status.Database.CPUUtilization = cw.DBCPUUtilization
	status.Database.StorageUsedGB = cw.DBAllocatedStorage - cw.DBFreeStorageSpace
	status.Database.StorageTotalGB = cw.DBAllocatedStorage
	status.Database.StorageUtilization = cw.DBStorageUtilization
	status.Database.ConnectionsActive = cw.DBConnections
	status.Database.ConnectionsMax = 100 // Default for db.t3.small
	if status.Database.ConnectionsMax > 0 {
		status.Database.ConnectionUtilization = float64(cw.DBConnections) / float64(status.Database.ConnectionsMax) * 100
	}
	status.Database.ReadIOPS = cw.DBReadIOPS
	status.Database.WriteIOPS = cw.DBWriteIOPS
	status.Database.ReadLatencyMs = cw.DBReadLatency * 1000  // Convert seconds to ms
	status.Database.WriteLatencyMs = cw.DBWriteLatency * 1000

	// Cache metrics
	if cw.CacheConnections > 0 || cw.CacheHitRate > 0 || cw.CacheMemoryUsage > 0 {
		status.Cache.Available = true
		status.Cache.ConnectionsActive = cw.CacheConnections
		status.Cache.HitRate = cw.CacheHitRate
		status.Cache.MissRate = cw.CacheMissRate
		status.Cache.MemoryUsedMB = cw.CacheMemoryUsage / (1024 * 1024)
		status.Cache.EvictedKeys = cw.CacheEvictions
	}

	// Application metrics from ALB if available
	if cw.ALBRequestCount > 0 {
		status.Application.RequestsPerMinute = cw.ALBRequestCount / 10 // 10 minute window
		status.Application.AverageResponseTimeMs = cw.ALBTargetResponseTime * 1000
		if cw.ALBRequestCount > 0 {
			status.Application.ErrorRate = (cw.ALB5xxCount / cw.ALBRequestCount) * 100
			status.Application.SuccessRate = 100 - status.Application.ErrorRate
		}
	}

	// Update statuses based on thresholds
	status.Compute.Status = determineHealthStatus(cw.CPUUtilization, 70, 85)
	status.Database.Status = determineHealthStatus(cw.DBCPUUtilization, 70, 85)
	if status.Cache.Available {
		status.Cache.Status = determineHealthStatus(100-cw.CacheHitRate, 30, 50) // Alert if hit rate drops
	}
}

// generateAlerts creates detailed alerts with actionable information
func generateAlerts(status *models.InfrastructureStatus, errorCount int, now time.Time) {
	// --- COMPUTE ALERTS ---

	// High CPU
	if status.Compute.CPUUtilization >= 85 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "compute-cpu-critical",
			Severity:     models.HealthStatusCritical,
			Component:    "compute",
			Title:        "Critical CPU Utilization",
			Description:  "EC2 instance CPU usage is critically high, which may cause request timeouts and degraded performance.",
			CurrentValue: fmt.Sprintf("%.1f%%", status.Compute.CPUUtilization),
			Threshold:    "85%",
			Recommendation: "1. Check for runaway processes or memory leaks\n" +
				"2. Consider scaling up instance type (t3.small -> t3.medium)\n" +
				"3. Review recent deployments for performance regressions\n" +
				"4. Increase ASG desired capacity for horizontal scaling",
			DetectedAt: now,
		})
		status.Compute.Status = models.HealthStatusCritical
	} else if status.Compute.CPUUtilization >= 70 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "compute-cpu-warning",
			Severity:     models.HealthStatusDegraded,
			Component:    "compute",
			Title:        "Elevated CPU Utilization",
			Description:  "EC2 instance CPU usage is elevated. Monitor for further increases.",
			CurrentValue: fmt.Sprintf("%.1f%%", status.Compute.CPUUtilization),
			Threshold:    "70%",
			Recommendation: "1. Monitor trend over next 15 minutes\n" +
				"2. If sustained, consider scaling actions\n" +
				"3. Check for any batch jobs or scheduled tasks running",
			DetectedAt: now,
		})
		status.Compute.Status = models.HealthStatusDegraded
	}

	// Memory (if available)
	if status.Compute.MemoryUtilization >= 90 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "compute-memory-critical",
			Severity:     models.HealthStatusCritical,
			Component:    "compute",
			Title:        "Critical Memory Utilization",
			Description:  "Instance memory is nearly exhausted. OOM killer may terminate processes.",
			CurrentValue: fmt.Sprintf("%.1f%%", status.Compute.MemoryUtilization),
			Threshold:    "90%",
			Recommendation: "1. Restart the application container to clear memory\n" +
				"2. Check for memory leaks in recent changes\n" +
				"3. Scale to larger instance type with more RAM\n" +
				"4. Review application heap settings",
			DetectedAt: now,
		})
		status.Compute.Status = models.HealthStatusCritical
	}

	// --- DATABASE ALERTS ---

	// High DB CPU
	if status.Database.CPUUtilization >= 85 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-cpu-critical",
			Severity:     models.HealthStatusCritical,
			Component:    "database",
			Title:        "Critical Database CPU",
			Description:  "RDS database CPU is critically high. Queries may timeout.",
			CurrentValue: fmt.Sprintf("%.1f%%", status.Database.CPUUtilization),
			Threshold:    "85%",
			Recommendation: "1. Check for slow queries in RDS Performance Insights\n" +
				"2. Look for missing indexes on frequently-queried columns\n" +
				"3. Consider upgrading RDS instance class\n" +
				"4. Enable query caching if not already enabled",
			DetectedAt: now,
		})
		status.Database.Status = models.HealthStatusCritical
	} else if status.Database.CPUUtilization >= 70 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-cpu-warning",
			Severity:     models.HealthStatusDegraded,
			Component:    "database",
			Title:        "Elevated Database CPU",
			Description:  "RDS database CPU is elevated. Monitor for query performance issues.",
			CurrentValue: fmt.Sprintf("%.1f%%", status.Database.CPUUtilization),
			Threshold:    "70%",
			Recommendation: "1. Review slow query log for optimization opportunities\n" +
				"2. Check if any batch processes are running\n" +
				"3. Monitor connection count for unusual spikes",
			DetectedAt: now,
		})
		status.Database.Status = models.HealthStatusDegraded
	}

	// Storage
	if status.Database.StorageUtilization >= 90 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-storage-critical",
			Severity:     models.HealthStatusCritical,
			Component:    "database",
			Title:        "Critical Database Storage",
			Description:  "Database storage is nearly full. Writes may fail soon.",
			CurrentValue: fmt.Sprintf("%.1f%% (%.1f GB free)", status.Database.StorageUtilization,
				status.Database.StorageTotalGB-status.Database.StorageUsedGB),
			Threshold:    "90%",
			Recommendation: "1. URGENT: Increase allocated storage in RDS console\n" +
				"2. Archive or delete old data (logs, old sessions)\n" +
				"3. Run VACUUM to reclaim space\n" +
				"4. Enable storage autoscaling in RDS",
			DetectedAt: now,
		})
		status.Database.Status = models.HealthStatusCritical
	} else if status.Database.StorageUtilization >= 75 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-storage-warning",
			Severity:     models.HealthStatusDegraded,
			Component:    "database",
			Title:        "Database Storage Warning",
			Description:  "Database storage is filling up. Plan for expansion.",
			CurrentValue: fmt.Sprintf("%.1f%% (%.1f GB free)", status.Database.StorageUtilization,
				status.Database.StorageTotalGB-status.Database.StorageUsedGB),
			Threshold:    "75%",
			Recommendation: "1. Plan storage increase within next 2 weeks\n" +
				"2. Review data retention policies\n" +
				"3. Consider enabling storage autoscaling",
			DetectedAt: now,
		})
		if status.Database.Status != models.HealthStatusCritical {
			status.Database.Status = models.HealthStatusDegraded
		}
	}

	// Connections
	if status.Database.ConnectionUtilization >= 80 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-connections-warning",
			Severity:     models.HealthStatusDegraded,
			Component:    "database",
			Title:        "High Database Connection Usage",
			Description:  "Database connection pool is running low.",
			CurrentValue: fmt.Sprintf("%d/%d connections (%.1f%%)",
				status.Database.ConnectionsActive, status.Database.ConnectionsMax, status.Database.ConnectionUtilization),
			Threshold:    "80%",
			Recommendation: "1. Check for connection leaks in application code\n" +
				"2. Reduce connection pool size per instance\n" +
				"3. Consider using PgBouncer for connection pooling\n" +
				"4. Upgrade to larger RDS instance for more connections",
			DetectedAt: now,
		})
		if status.Database.Status != models.HealthStatusCritical {
			status.Database.Status = models.HealthStatusDegraded
		}
	}

	// High latency
	if status.Database.ReadLatencyMs > 100 || status.Database.WriteLatencyMs > 100 {
		severity := models.HealthStatusDegraded
		if status.Database.ReadLatencyMs > 500 || status.Database.WriteLatencyMs > 500 {
			severity = models.HealthStatusCritical
		}
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "database-latency-high",
			Severity:     severity,
			Component:    "database",
			Title:        "High Database Latency",
			Description:  "Database read/write operations are slow.",
			CurrentValue: fmt.Sprintf("Read: %.1fms, Write: %.1fms",
				status.Database.ReadLatencyMs, status.Database.WriteLatencyMs),
			Threshold:    "100ms",
			Recommendation: "1. Check for long-running queries locking tables\n" +
				"2. Review index usage on frequent queries\n" +
				"3. Check IOPS utilization - may need provisioned IOPS\n" +
				"4. Consider read replicas for read-heavy workloads",
			DetectedAt: now,
		})
	}

	// --- APPLICATION ALERTS ---

	// High error count
	if errorCount >= 50 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "application-errors-critical",
			Severity:     models.HealthStatusCritical,
			Component:    "application",
			Title:        "Critical Error Rate",
			Description:  "High number of unacknowledged application errors.",
			CurrentValue: fmt.Sprintf("%d errors", errorCount),
			Threshold:    "50 errors",
			Recommendation: "1. Review Error Logs page for patterns\n" +
				"2. Check recent deployments for bugs\n" +
				"3. Verify database connectivity\n" +
				"4. Check external service dependencies\n" +
				"5. Consider rollback if errors started after deployment",
			DetectedAt: now,
		})
		status.Application.Status = models.HealthStatusCritical
	} else if errorCount >= 10 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "application-errors-warning",
			Severity:     models.HealthStatusDegraded,
			Component:    "application",
			Title:        "Elevated Error Rate",
			Description:  "Increased application errors detected.",
			CurrentValue: fmt.Sprintf("%d errors", errorCount),
			Threshold:    "10 errors",
			Recommendation: "1. Review Error Logs page for common patterns\n" +
				"2. Check if errors are from specific endpoints\n" +
				"3. Acknowledge handled errors to clear count",
			DetectedAt: now,
		})
		status.Application.Status = models.HealthStatusDegraded
	}

	// High error rate (from ALB)
	if status.Application.ErrorRate >= 5 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "application-error-rate-high",
			Severity:     models.HealthStatusCritical,
			Component:    "application",
			Title:        "High HTTP Error Rate",
			Description:  "Significant percentage of requests are returning 5xx errors.",
			CurrentValue: fmt.Sprintf("%.2f%% errors", status.Application.ErrorRate),
			Threshold:    "5%",
			Recommendation: "1. Check application logs for stack traces\n" +
				"2. Verify all dependent services are healthy\n" +
				"3. Check database connection availability\n" +
				"4. Review memory usage for OOM issues",
			DetectedAt: now,
		})
		status.Application.Status = models.HealthStatusCritical
	}

	// Slow response time
	if status.Application.AverageResponseTimeMs > 2000 {
		status.Alerts = append(status.Alerts, models.InfrastructureAlert{
			ID:           "application-response-slow",
			Severity:     models.HealthStatusDegraded,
			Component:    "application",
			Title:        "Slow Response Times",
			Description:  "Average response time is degraded.",
			CurrentValue: fmt.Sprintf("%.0fms avg", status.Application.AverageResponseTimeMs),
			Threshold:    "2000ms",
			Recommendation: "1. Check database query performance\n" +
				"2. Review CPU and memory utilization\n" +
				"3. Check for network latency issues\n" +
				"4. Verify Redis cache is being utilized",
			DetectedAt: now,
		})
		if status.Application.Status != models.HealthStatusCritical {
			status.Application.Status = models.HealthStatusDegraded
		}
	}

	// --- CACHE ALERTS ---

	if status.Cache.Available {
		// Low hit rate
		if status.Cache.HitRate < 70 && status.Cache.HitRate > 0 {
			status.Alerts = append(status.Alerts, models.InfrastructureAlert{
				ID:           "cache-hitrate-low",
				Severity:     models.HealthStatusDegraded,
				Component:    "cache",
				Title:        "Low Cache Hit Rate",
				Description:  "Cache is not being effectively utilized.",
				CurrentValue: fmt.Sprintf("%.1f%% hit rate", status.Cache.HitRate),
				Threshold:    "70%",
				Recommendation: "1. Review cache key strategies\n" +
					"2. Increase cache TTLs where appropriate\n" +
					"3. Check for cache invalidation issues\n" +
					"4. Verify application is using cache correctly",
				DetectedAt: now,
			})
			status.Cache.Status = models.HealthStatusDegraded
		}

		// High evictions
		if status.Cache.EvictedKeys > 1000 {
			status.Alerts = append(status.Alerts, models.InfrastructureAlert{
				ID:           "cache-evictions-high",
				Severity:     models.HealthStatusDegraded,
				Component:    "cache",
				Title:        "High Cache Evictions",
				Description:  "Cache is evicting keys due to memory pressure.",
				CurrentValue: fmt.Sprintf("%d evictions", status.Cache.EvictedKeys),
				Threshold:    "1000",
				Recommendation: "1. Increase ElastiCache node size\n" +
					"2. Review and reduce TTLs on less critical data\n" +
					"3. Audit what's being cached for optimization",
				DetectedAt: now,
			})
		}
	} else {
		// Cache not monitored - informational only
		status.Cache.StatusMessage = "Redis cache metrics not available - install CloudWatch agent or check ElastiCache ID"
	}

	// --- ASG ALERTS ---
	if status.ASG != nil {
		// At max capacity - no headroom
		if status.ASG.CapacityStatus == "at_max" {
			status.Alerts = append(status.Alerts, models.InfrastructureAlert{
				ID:        "asg-at-max",
				Severity:  models.HealthStatusCritical,
				Component: "compute",
				Title:     "ASG at Maximum Capacity",
				Description: fmt.Sprintf("Auto Scaling Group is at maximum capacity (%d/%d instances). "+
					"Cannot scale out further if load increases.", status.ASG.CurrentCapacity, status.ASG.MaxSize),
				CurrentValue: fmt.Sprintf("%d instances (max: %d)", status.ASG.CurrentCapacity, status.ASG.MaxSize),
				Threshold:    "0% headroom",
				Recommendation: "1. Consider increasing ASG max capacity if needed\n" +
					"2. Optimize application to handle more load per instance\n" +
					"3. Upgrade instance type for more capacity per instance\n" +
					"4. Review if traffic spike is legitimate or an attack",
				DetectedAt: now,
			})
		} else if status.ASG.ScalingHeadroom < 50 && status.ASG.ScalingHeadroom > 0 {
			// Low headroom warning
			status.Alerts = append(status.Alerts, models.InfrastructureAlert{
				ID:        "asg-low-headroom",
				Severity:  models.HealthStatusDegraded,
				Component: "compute",
				Title:     "Low ASG Scaling Headroom",
				Description: fmt.Sprintf("Auto Scaling Group has limited scaling capacity remaining (%.0f%% headroom).",
					status.ASG.ScalingHeadroom),
				CurrentValue: fmt.Sprintf("%d/%d instances (%.0f%% headroom)",
					status.ASG.CurrentCapacity, status.ASG.MaxSize, status.ASG.ScalingHeadroom),
				Threshold:    "50% headroom",
				Recommendation: "1. Monitor closely - may hit max capacity soon\n" +
					"2. Consider proactively increasing max capacity\n" +
					"3. Review scaling policies to ensure they're optimal",
				DetectedAt: now,
			})
		}

		// Unhealthy instances
		unhealthyCount := status.ASG.CurrentCapacity - status.ASG.InServiceCount
		if unhealthyCount > 0 {
			status.Alerts = append(status.Alerts, models.InfrastructureAlert{
				ID:        "asg-unhealthy-instances",
				Severity:  models.HealthStatusDegraded,
				Component: "compute",
				Title:     "Unhealthy ASG Instances",
				Description: fmt.Sprintf("%d instance(s) are not in 'InService' state. "+
					"Pending: %d, Terminating: %d",
					unhealthyCount, status.ASG.PendingCount, status.ASG.TerminatingCount),
				CurrentValue: fmt.Sprintf("%d healthy / %d total", status.ASG.InServiceCount, status.ASG.CurrentCapacity),
				Threshold:    "All instances healthy",
				Recommendation: "1. Check if scaling activity is in progress (normal)\n" +
					"2. Review health check failures in load balancer\n" +
					"3. Check instance logs for application errors\n" +
					"4. Verify launch template and user data are correct",
				DetectedAt: now,
			})
		}

		// Scaling policies approaching target
		for _, policy := range status.ASG.ScalingPolicies {
			if policy.MetricType == "ASGAverageCPUUtilization" && policy.TargetValue > 0 {
				percentOfTarget := (policy.CurrentValue / policy.TargetValue) * 100
				if percentOfTarget >= 90 {
					status.Alerts = append(status.Alerts, models.InfrastructureAlert{
						ID:        "asg-policy-near-target",
						Severity:  models.HealthStatusDegraded,
						Component: "compute",
						Title:     "Scaling Policy Near Target",
						Description: fmt.Sprintf("CPU is at %.0f%% of the %.0f%% scaling target. "+
							"ASG may scale out soon.",
							percentOfTarget, policy.TargetValue),
						CurrentValue: fmt.Sprintf("%.1f%% CPU (target: %.0f%%)",
							policy.CurrentValue, policy.TargetValue),
						Threshold:    "90% of target",
						Recommendation: "1. Monitor - scaling may occur automatically\n" +
							"2. If at max capacity, consider increasing max size\n" +
							"3. Review application performance for bottlenecks",
						DetectedAt: now,
					})
				}
			}
		}
	}
}

// RefreshInfrastructureStatus forces a refresh of infrastructure metrics
func (h *Handler) RefreshInfrastructureStatus(w http.ResponseWriter, r *http.Request) {
	h.GetInfrastructureStatus(w, r)
}

// Helper functions

func determineHealthStatus(value, warningThreshold, criticalThreshold float64) models.HealthStatus {
	if value >= criticalThreshold {
		return models.HealthStatusCritical
	}
	if value >= warningThreshold {
		return models.HealthStatusDegraded
	}
	return models.HealthStatusHealthy
}

func calculateOverallHealth(status *models.InfrastructureStatus) (models.HealthStatus, string, int, int) {
	alertCount := 0
	warningCount := 0

	for _, alert := range status.Alerts {
		switch alert.Severity {
		case models.HealthStatusCritical:
			alertCount++
		case models.HealthStatusDegraded:
			warningCount++
		}
	}

	var overall models.HealthStatus
	var summary string

	if alertCount > 0 {
		overall = models.HealthStatusCritical
		if alertCount == 1 {
			summary = "1 critical issue requires attention"
		} else {
			summary = fmt.Sprintf("%d critical issues require attention", alertCount)
		}
	} else if warningCount > 0 {
		overall = models.HealthStatusDegraded
		if warningCount == 1 {
			summary = "1 warning - monitor closely"
		} else {
			summary = fmt.Sprintf("%d warnings - monitor closely", warningCount)
		}
	} else {
		overall = models.HealthStatusHealthy
		summary = "All systems operational"
	}

	return overall, summary, alertCount, warningCount
}
