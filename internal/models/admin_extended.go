package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Error Log Extended Views
// ============================================================================

// ErrorSource indicates where the error originated from
type ErrorSource string

const (
	ErrorSourceUser           ErrorSource = "user"           // Logged-in user session
	ErrorSourceInfrastructure ErrorSource = "infrastructure" // Backend/server issues
	ErrorSourceScanner        ErrorSource = "scanner"        // Vulnerability scanners (auto-delete 7 days)
	ErrorSourceAnonymous      ErrorSource = "anonymous"      // Anonymous users (auto-delete 30 days)
	ErrorSourceUnknown        ErrorSource = "unknown"        // Unclassified (auto-delete 30 days)
)

// ErrorLogView extends ErrorLog with acknowledgement tracking for admin UI
type ErrorLogView struct {
	ID                uuid.UUID  `json:"id"`
	ErrorType         string     `json:"error_type"`
	StatusCode        int        `json:"status_code"`
	Method            string     `json:"method"`
	Path              string     `json:"path"`
	Message           string     `json:"message"`
	StackTrace        NullString `json:"stack_trace,omitempty"`
	UserID            NullUUID   `json:"user_id,omitempty"`
	RequestID         NullString `json:"request_id,omitempty"`
	UserAgent         NullString `json:"user_agent,omitempty"`
	IPAddress         NullString `json:"ip_address,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`

	// Classification fields
	ErrorSource       ErrorSource `json:"error_source"`
	IsNoise           bool        `json:"is_noise"`
	AutoDeleteAt      NullTime    `json:"auto_delete_at,omitempty"`

	// Acknowledgement fields
	AcknowledgedAt    NullTime   `json:"acknowledged_at,omitempty"`
	AcknowledgedBy    NullUUID   `json:"acknowledged_by,omitempty"`
	AcknowledgedNotes NullString `json:"acknowledged_notes,omitempty"`
	IsDeleted         bool       `json:"is_deleted"`
	DeletedAt         NullTime   `json:"deleted_at,omitempty"`
	DeletedBy         NullUUID   `json:"deleted_by,omitempty"`

	// Populated from JOINs
	AcknowledgedByEmail string `json:"acknowledged_by_email,omitempty"`
	AcknowledgedByName  string `json:"acknowledged_by_name,omitempty"`
	UserEmail           string `json:"user_email,omitempty"`
}

// ErrorLogFilter represents filter options for error logs
type ErrorLogFilter struct {
	ErrorType    string        `json:"error_type,omitempty"`
	ErrorSource  []ErrorSource `json:"error_source,omitempty"` // Filter by source(s)
	Acknowledged *bool         `json:"acknowledged,omitempty"`
	StartDate    *time.Time    `json:"start_date,omitempty"`
	EndDate      *time.Time    `json:"end_date,omitempty"`
	StatusCode   *int          `json:"status_code,omitempty"`
	IncludeNoise bool          `json:"include_noise,omitempty"` // Include scanner/noise errors
}

// ============================================================================
// Infrastructure Status
// ============================================================================

// InfrastructureStatus represents the current state of all infrastructure components
type InfrastructureStatus struct {
	LastUpdated time.Time `json:"last_updated"`

	// Compute (EC2/Container)
	Compute ComputeMetrics `json:"compute"`

	// Database (RDS)
	Database DatabaseMetrics `json:"database"`

	// Application
	Application ApplicationMetrics `json:"application"`

	// Cache (Redis/ElastiCache)
	Cache CacheMetrics `json:"cache"`

	// Auto Scaling Group
	ASG *ASGStatus `json:"asg,omitempty"`

	// Overall health
	OverallHealth   HealthStatus `json:"overall_health"`
	HealthSummary   string       `json:"health_summary"`
	AlertCount      int          `json:"alert_count"`
	WarningCount    int          `json:"warning_count"`

	// Detailed alerts with actionable information
	Alerts []InfrastructureAlert `json:"alerts"`
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
	InstanceID       string    `json:"instance_id"`
	HealthStatus     string    `json:"health_status"`
	LifecycleState   string    `json:"lifecycle_state"`
	AvailabilityZone string    `json:"availability_zone"`
	LaunchTime       time.Time `json:"launch_time,omitempty"`
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
	ActivityID  string    `json:"activity_id"`
	Description string    `json:"description"`
	Cause       string    `json:"cause"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time,omitempty"`
	StatusCode  string    `json:"status_code"`
	Progress    int       `json:"progress"`
}

// TargetHealth represents health status from load balancer perspective
type TargetHealth struct {
	InstanceID  string `json:"instance_id"`
	Port        int    `json:"port"`
	HealthState string `json:"health_state"`
	Reason      string `json:"reason,omitempty"`
	Description string `json:"description,omitempty"`
}

// InfrastructureAlert represents a specific issue with actionable information
type InfrastructureAlert struct {
	ID           string       `json:"id"`
	Severity     HealthStatus `json:"severity"` // critical, degraded (warning), healthy (info)
	Component    string       `json:"component"` // compute, database, application, cache
	Title        string       `json:"title"`
	Description  string       `json:"description"`
	CurrentValue string       `json:"current_value"`
	Threshold    string       `json:"threshold"`
	Recommendation string     `json:"recommendation"`
	DocumentationURL string   `json:"documentation_url,omitempty"`
	DetectedAt   time.Time    `json:"detected_at"`
}

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusCritical  HealthStatus = "critical"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// ComputeMetrics represents EC2/container compute metrics
type ComputeMetrics struct {
	CPUUtilization    float64      `json:"cpu_utilization"`     // percentage
	MemoryUtilization float64      `json:"memory_utilization"`  // percentage
	MemoryUsedGB      float64      `json:"memory_used_gb"`
	MemoryTotalGB     float64      `json:"memory_total_gb"`
	InstanceCount     int          `json:"instance_count"`
	HealthyInstances  int          `json:"healthy_instances"`
	UnhealthyInstances int         `json:"unhealthy_instances"`
	Status            HealthStatus `json:"status"`
	StatusMessage     string       `json:"status_message,omitempty"`
}

// DatabaseMetrics represents RDS database metrics
type DatabaseMetrics struct {
	CPUUtilization       float64      `json:"cpu_utilization"`       // percentage
	StorageUsedGB        float64      `json:"storage_used_gb"`
	StorageTotalGB       float64      `json:"storage_total_gb"`
	StorageUtilization   float64      `json:"storage_utilization"`   // percentage
	ConnectionsActive    int          `json:"connections_active"`
	ConnectionsMax       int          `json:"connections_max"`
	ConnectionUtilization float64     `json:"connection_utilization"` // percentage
	ReadIOPS             float64      `json:"read_iops"`
	WriteIOPS            float64      `json:"write_iops"`
	ReadLatencyMs        float64      `json:"read_latency_ms"`
	WriteLatencyMs       float64      `json:"write_latency_ms"`
	ReplicationLagMs     float64      `json:"replication_lag_ms"`
	Status               HealthStatus `json:"status"`
	StatusMessage        string       `json:"status_message,omitempty"`
}

// ApplicationMetrics represents application-level metrics
type ApplicationMetrics struct {
	RequestsPerMinute    float64      `json:"requests_per_minute"`
	AverageResponseTimeMs float64     `json:"average_response_time_ms"`
	P95ResponseTimeMs    float64      `json:"p95_response_time_ms"`
	P99ResponseTimeMs    float64      `json:"p99_response_time_ms"`
	ErrorRate            float64      `json:"error_rate"`           // percentage
	ErrorCount5m         int          `json:"error_count_5m"`       // errors in last 5 minutes
	SuccessRate          float64      `json:"success_rate"`         // percentage
	ActiveSessions       int          `json:"active_sessions"`
	Status               HealthStatus `json:"status"`
	StatusMessage        string       `json:"status_message,omitempty"`
}

// CacheMetrics represents Redis/ElastiCache metrics
type CacheMetrics struct {
	MemoryUsedMB       float64      `json:"memory_used_mb"`
	MemoryTotalMB      float64      `json:"memory_total_mb"`
	MemoryUtilization  float64      `json:"memory_utilization"`    // percentage
	ConnectionsActive  int          `json:"connections_active"`
	ConnectionsMax     int          `json:"connections_max"`
	HitRate            float64      `json:"hit_rate"`              // percentage
	MissRate           float64      `json:"miss_rate"`             // percentage
	KeysTotal          int64        `json:"keys_total"`
	KeysExpiring       int64        `json:"keys_expiring"`
	EvictedKeys        int64        `json:"evicted_keys"`
	Status             HealthStatus `json:"status"`
	StatusMessage      string       `json:"status_message,omitempty"`
	Available          bool         `json:"available"`             // false if not configured
}

// ============================================================================
// Admin Dashboard Stats
// ============================================================================

// AdminDashboardStats represents overview statistics for the admin dashboard
type AdminDashboardStats struct {
	// Users
	TotalUsers       int `json:"total_users"`
	ActiveUsers24h   int `json:"active_users_24h"`
	NewUsersToday    int `json:"new_users_today"`
	NewUsersThisWeek int `json:"new_users_this_week"`

	// Families
	TotalFamilies    int `json:"total_families"`
	ActiveFamilies   int `json:"active_families"`

	// Support
	OpenTickets      int `json:"open_tickets"`
	TicketsToday     int `json:"tickets_today"`
	AvgResolutionHrs float64 `json:"avg_resolution_hrs"`

	// Errors
	UnacknowledgedErrors int `json:"unacknowledged_errors"`
	ErrorsToday          int `json:"errors_today"`
	ErrorsTrend          string `json:"errors_trend"` // "up", "down", "stable"

	// Financial (for super_admin)
	Revenue24hCents      int64 `json:"revenue_24h_cents,omitempty"`
	RevenueMTDCents      int64 `json:"revenue_mtd_cents,omitempty"`
	ActiveSubscriptions  int   `json:"active_subscriptions,omitempty"`
	ChurnRateMTD         float64 `json:"churn_rate_mtd,omitempty"`
}

// ============================================================================
// Bulk Operation Requests
// ============================================================================

// BulkAcknowledgeRequest represents a request to acknowledge multiple error logs
type BulkAcknowledgeRequest struct {
	IDs   []uuid.UUID `json:"ids"`
	Notes string      `json:"notes,omitempty"`
}

// BulkDeleteRequest represents a request to delete multiple error logs
type BulkDeleteRequest struct {
	IDs []uuid.UUID `json:"ids"`
}

// ============================================================================
// Report Types
// ============================================================================

type ReportFormat string

const (
	ReportFormatCSV  ReportFormat = "csv"
	ReportFormatPDF  ReportFormat = "pdf"
	ReportFormatJSON ReportFormat = "json"
)

type ReportType string

const (
	ReportTypeRevenue       ReportType = "revenue"
	ReportTypeSubscriptions ReportType = "subscriptions"
	ReportTypePayments      ReportType = "payments"
	ReportTypePromoUsage    ReportType = "promo_usage"
)

// FinancialReportRequest represents a request to generate a financial report
type FinancialReportRequest struct {
	ReportType ReportType   `json:"report_type"`
	Format     ReportFormat `json:"format"`
	StartDate  time.Time    `json:"start_date"`
	EndDate    time.Time    `json:"end_date"`
	IncludePromoData bool   `json:"include_promo_data,omitempty"`
}

// ============================================================================
// Create Ticket From Error
// ============================================================================

// CreateTicketFromErrorRequest represents a request to create a support ticket from an error
type CreateTicketFromErrorRequest struct {
	ErrorID     uuid.UUID `json:"error_id"`
	Priority    string    `json:"priority,omitempty"`    // defaults to "medium"
	AssignToID  NullUUID  `json:"assign_to_id,omitempty"`
	Notes       string    `json:"notes,omitempty"`
}
