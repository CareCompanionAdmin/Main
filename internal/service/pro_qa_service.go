package service

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"

	"carecompanion/internal/models"
	"carecompanion/internal/repository"
)

// ProQAService orchestrates the admin-only QA workspace. It wraps the
// repository and owns the markdown renderer + attachment BlobStorage so
// handlers stay thin.
type ProQAService struct {
	repo    repository.ProQARepository
	storage BlobStorage
	md      goldmark.Markdown
}

func NewProQAService(repo repository.ProQARepository, storage BlobStorage) *ProQAService {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(gmhtml.WithHardWraps()),
	)
	return &ProQAService{repo: repo, storage: storage, md: md}
}

// RenderMarkdown safely converts markdown to HTML. goldmark escapes raw
// HTML in source by default (no rawHTML extension), so no extra sanitization
// is needed for trusted admin authors.
func (s *ProQAService) RenderMarkdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := s.md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

// ---- Info ----

func (s *ProQAService) GetInfo(ctx context.Context) (*models.ProQAInfo, error) {
	return s.repo.GetInfo(ctx)
}

func (s *ProQAService) UpdateInfo(ctx context.Context, bodyMD, email string) error {
	return s.repo.UpdateInfo(ctx, bodyMD, email)
}

// ---- Checks ----

func (s *ProQAService) ListChecks(ctx context.Context) ([]models.ProQARequestedCheck, error) {
	return s.repo.ListChecks(ctx)
}

func (s *ProQAService) GetCheck(ctx context.Context, id uuid.UUID) (*models.ProQARequestedCheck, error) {
	return s.repo.GetCheck(ctx, id)
}

func (s *ProQAService) CreateCheck(ctx context.Context, title, bodyMD, email string) (*models.ProQARequestedCheck, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("title required")
	}
	c := &models.ProQARequestedCheck{
		Title:          title,
		BodyMD:         bodyMD,
		Status:         "open",
		CreatedByEmail: email,
	}
	if err := s.repo.CreateCheck(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// UpdateCheck saves title/body/sort_order. Status changes route through
// ChangeCheckStatus so they emit an auto-comment in the thread.
func (s *ProQAService) UpdateCheck(ctx context.Context, c *models.ProQARequestedCheck, authorEmail, authorName string) error {
	prev, err := s.repo.GetCheck(ctx, c.ID)
	if err != nil {
		return err
	}
	// Preserve the existing status on the row update; status path is separate.
	c.Status = prev.Status
	if err := s.repo.UpdateCheck(ctx, c); err != nil {
		return err
	}
	return nil
}

// ChangeCheckStatus updates status and writes an auto-comment recording
// the transition so the thread shows a complete history. No-op if the
// requested status matches current.
func (s *ProQAService) ChangeCheckStatus(ctx context.Context, checkID uuid.UUID, newStatus, authorEmail, authorName string) error {
	prev, err := s.repo.GetCheck(ctx, checkID)
	if err != nil {
		return err
	}
	if prev.Status == newStatus {
		return nil
	}
	if err := s.repo.ChangeCheckStatus(ctx, checkID, newStatus); err != nil {
		return err
	}
	autoBody := fmt.Sprintf("_status changed: **%s** → **%s**_", prev.Status, newStatus)
	return s.repo.CreateCheckComment(ctx, &models.ProQACheckComment{
		CheckID:        checkID,
		BodyMD:         autoBody,
		AuthorEmail:    authorEmail,
		AuthorName:     authorName,
		IsStatusChange: true,
		StatusFrom:     prev.Status,
		StatusTo:       newStatus,
	})
}

func (s *ProQAService) DeleteCheck(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCheck(ctx, id)
}

// ---- Check comments ----

func (s *ProQAService) ListCheckComments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckComment, error) {
	return s.repo.ListCheckComments(ctx, checkID)
}

func (s *ProQAService) AddCheckComment(ctx context.Context, checkID uuid.UUID, body, email, name string) (*models.ProQACheckComment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("comment body required")
	}
	c := &models.ProQACheckComment{
		CheckID:     checkID,
		BodyMD:      body,
		AuthorEmail: email,
		AuthorName:  name,
	}
	if err := s.repo.CreateCheckComment(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---- Check attachments ----

func (s *ProQAService) ListCheckAttachments(ctx context.Context, checkID uuid.UUID) ([]models.ProQACheckAttachment, error) {
	return s.repo.ListCheckAttachments(ctx, checkID)
}

func (s *ProQAService) UploadCheckAttachment(ctx context.Context, checkID uuid.UUID, commentID *uuid.UUID, filename, contentType, email string, body io.Reader) (*models.ProQACheckAttachment, error) {
	path, size, err := s.storage.Save(ctx, "pro_qa", filename, contentType, body)
	if err != nil {
		return nil, err
	}
	a := &models.ProQACheckAttachment{
		CheckID:         checkID,
		CommentID:       commentID,
		Filename:        filename,
		ContentType:     contentType,
		SizeBytes:       size,
		StorageDriver:   s.storage.Driver(),
		StoragePath:     path,
		UploadedByEmail: email,
	}
	if err := s.repo.CreateCheckAttachment(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ProQAService) FetchCheckAttachment(ctx context.Context, id uuid.UUID) (*models.ProQACheckAttachment, io.ReadCloser, error) {
	a, err := s.repo.GetCheckAttachment(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.storage.Open(ctx, a.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return a, rc, nil
}

// ---- Issues ----

func (s *ProQAService) ListIssues(ctx context.Context, status string) ([]models.ProQAIssue, error) {
	return s.repo.ListIssues(ctx, status)
}

func (s *ProQAService) GetIssue(ctx context.Context, id uuid.UUID) (*models.ProQAIssue, error) {
	return s.repo.GetIssue(ctx, id)
}

func (s *ProQAService) CreateIssue(ctx context.Context, i *models.ProQAIssue) error {
	if strings.TrimSpace(i.Title) == "" {
		return fmt.Errorf("title required")
	}
	if i.Status == "" {
		i.Status = "open"
	}
	if i.Severity == "" {
		i.Severity = "medium"
	}
	return s.repo.CreateIssue(ctx, i)
}

func (s *ProQAService) UpdateIssue(ctx context.Context, i *models.ProQAIssue) error {
	return s.repo.UpdateIssue(ctx, i)
}

// ChangeStatus updates the issue and writes an auto-comment recording the
// transition so the thread shows a complete history.
func (s *ProQAService) ChangeStatus(ctx context.Context, issueID uuid.UUID, newStatus, authorEmail, authorName string) error {
	prev, err := s.repo.GetIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if prev.Status == newStatus {
		return nil
	}
	if err := s.repo.ChangeIssueStatus(ctx, issueID, newStatus); err != nil {
		return err
	}
	autoBody := fmt.Sprintf("_status changed: **%s** → **%s**_", prev.Status, newStatus)
	return s.repo.CreateComment(ctx, &models.ProQAIssueComment{
		IssueID:        issueID,
		BodyMD:         autoBody,
		AuthorEmail:    authorEmail,
		AuthorName:     authorName,
		IsStatusChange: true,
		StatusFrom:     prev.Status,
		StatusTo:       newStatus,
	})
}

// ---- Comments ----

func (s *ProQAService) ListComments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAIssueComment, error) {
	return s.repo.ListComments(ctx, issueID)
}

func (s *ProQAService) AddComment(ctx context.Context, issueID uuid.UUID, body, email, name string) (*models.ProQAIssueComment, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("comment body required")
	}
	c := &models.ProQAIssueComment{
		IssueID:     issueID,
		BodyMD:      body,
		AuthorEmail: email,
		AuthorName:  name,
	}
	if err := s.repo.CreateComment(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// ---- Attachments ----

func (s *ProQAService) ListAttachments(ctx context.Context, issueID uuid.UUID) ([]models.ProQAAttachment, error) {
	return s.repo.ListAttachments(ctx, issueID)
}

func (s *ProQAService) UploadAttachment(ctx context.Context, issueID uuid.UUID, commentID *uuid.UUID, filename, contentType, email string, body io.Reader) (*models.ProQAAttachment, error) {
	path, size, err := s.storage.Save(ctx, "pro_qa", filename, contentType, body)
	if err != nil {
		return nil, err
	}
	a := &models.ProQAAttachment{
		IssueID:         issueID,
		CommentID:       commentID,
		Filename:        filename,
		ContentType:     contentType,
		SizeBytes:       size,
		StorageDriver:   s.storage.Driver(),
		StoragePath:     path,
		UploadedByEmail: email,
	}
	if err := s.repo.CreateAttachment(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *ProQAService) FetchAttachment(ctx context.Context, id uuid.UUID) (*models.ProQAAttachment, io.ReadCloser, error) {
	a, err := s.repo.GetAttachment(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	rc, err := s.storage.Open(ctx, a.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	return a, rc, nil
}
