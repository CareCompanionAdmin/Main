package models

import (
	"time"

	"github.com/google/uuid"
)

// Report represents a generated report
type Report struct {
	ID           uuid.UUID   `json:"id"`
	ChildID      uuid.UUID   `json:"child_id"`
	FamilyID     uuid.UUID   `json:"family_id"`
	CreatedBy    uuid.UUID   `json:"created_by"`
	Title        string      `json:"title"`
	ReportType   string      `json:"report_type"`
	PeriodType   string      `json:"period_type"`
	StartDate    time.Time   `json:"start_date"`
	EndDate      time.Time   `json:"end_date"`
	DataFilters  StringArray `json:"data_filters"`
	FilePath     NullString  `json:"file_path,omitempty"`
	FileSize     *int64      `json:"file_size,omitempty"`
	Status       string      `json:"status"`
	ErrorMessage NullString  `json:"error_message,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	CompletedAt  NullTime    `json:"completed_at,omitempty"`
}

// ScheduledReport represents a recurring report configuration
type ScheduledReport struct {
	ID          uuid.UUID   `json:"id"`
	ChildID     uuid.UUID   `json:"child_id"`
	FamilyID    uuid.UUID   `json:"family_id"`
	CreatedBy   uuid.UUID   `json:"created_by"`
	Frequency   string      `json:"frequency"`
	DataFilters StringArray `json:"data_filters"`
	Recipients  UUIDArray   `json:"recipients"`
	IsActive    bool        `json:"is_active"`
	LastRunAt   NullTime    `json:"last_run_at,omitempty"`
	NextRunAt   time.Time   `json:"next_run_at"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// GenerateReportRequest is the request to generate an on-demand report
type GenerateReportRequest struct {
	PeriodType  string   `json:"period_type"`
	StartDate   string   `json:"start_date,omitempty"`
	EndDate     string   `json:"end_date,omitempty"`
	DataFilters []string `json:"data_filters"`
}

// CreateScheduledReportRequest is the request to create a scheduled report
type CreateScheduledReportRequest struct {
	Frequency   string      `json:"frequency"`
	DataFilters []string    `json:"data_filters"`
	Recipients  []uuid.UUID `json:"recipients"`
}

// ShareReportRequest is the request to share a report via chat
type ShareReportRequest struct {
	RecipientID uuid.UUID `json:"recipient_id"`
}

// ChartDataPoint represents a single data point for charting
type ChartDataPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Label string  `json:"label,omitempty"`
}

// ReportChartData holds chart series for the frontend
type ReportChartData struct {
	ReportID  uuid.UUID                    `json:"report_id"`
	ChildName string                       `json:"child_name"`
	StartDate string                       `json:"start_date"`
	EndDate   string                       `json:"end_date"`
	Charts    map[string][]ChartDataPoint  `json:"charts"`
	Logs      *DailyLogPage                `json:"logs"`
}
