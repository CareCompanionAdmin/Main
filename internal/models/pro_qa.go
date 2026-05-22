package models

import (
	"time"

	"github.com/google/uuid"
)

type ProQAInfo struct {
	BodyMD         string
	UpdatedAt      time.Time
	UpdatedByEmail string
}

type ProQARequestedCheck struct {
	ID             uuid.UUID
	Title          string
	BodyMD         string
	Status         string
	SortOrder      int
	CreatedAt      time.Time
	CreatedByEmail string
	UpdatedAt      time.Time
}

type ProQAIssue struct {
	ID              uuid.UUID
	IssueNumber     int
	ParentIssueID   *uuid.UUID
	Title           string
	DescriptionMD   string
	Environment     string
	Platform        string
	Status          string
	Severity        string
	CreatedAt       time.Time
	CreatedByEmail  string
	UpdatedAt       time.Time
	ClosedAt        *time.Time
	CommentCount    int
	AttachmentCount int
}

type ProQAIssueComment struct {
	ID             uuid.UUID
	IssueID        uuid.UUID
	BodyMD         string
	AuthorEmail    string
	AuthorName     string
	CreatedAt      time.Time
	IsStatusChange bool
	StatusFrom     string
	StatusTo       string
}

type ProQAAttachment struct {
	ID              uuid.UUID
	IssueID         uuid.UUID
	CommentID       *uuid.UUID
	Filename        string
	ContentType     string
	SizeBytes       int64
	StorageDriver   string
	StoragePath     string
	UploadedByEmail string
	UploadedAt      time.Time
}

var (
	ProQACheckStatuses = []string{"open", "in_review", "done"}
	ProQAIssueStatuses = []string{"open", "needs_info", "in_progress", "resolved", "closed", "wont_fix"}
	ProQAIssueSeverity = []string{"low", "medium", "high", "critical"}
	ProQAEnvironments  = []string{"dev", "prod"}
	ProQAPlatforms     = []string{"ios", "android", "web", "admin"}
)
